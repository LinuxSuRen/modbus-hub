package sink

import (
	"fmt"
	"log"
	"net/url"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/linuxsuren/modbus-hub/internal/model"
)

// MQTTPublisher 将读数发布到 MQTT。实现要点来自 mqtt-device-gateway-dev：
// AutoReconnect+ConnectRetry、CleanSession(true)、OrderMatters(false)、
// 唯一 client-id、连接错误带定位信息；采集流不 retain。
type MQTTPublisher struct {
	client paho.Client
	cfg    model.SinkMQTT
	// queue 缓冲读数，发布在工作协程里做，避免阻塞轮询
	queue chan model.Reading
	stop  chan struct{}
}

// NewMQTTPublisher 按 cfg 建立 MQTT 发布器（cfg.Enabled 为 false 时返回惰性实例）。
func NewMQTTPublisher(cfg model.SinkMQTT) *MQTTPublisher {
	return &MQTTPublisher{
		cfg:  cfg,
		stop: make(chan struct{}),
	}
}

// Start 连接 broker 并开始消费队列。
func (p *MQTTPublisher) Start() error {
	if !p.cfg.Enabled {
		return nil
	}
	broker := p.cfg.Broker
	if broker == "" {
		return fmt.Errorf("MQTT broker 地址为空")
	}
	if _, err := url.Parse(broker); err != nil {
		return fmt.Errorf("MQTT broker 地址 %q 无效: %w", broker, err)
	}
	clientID := p.cfg.ClientID
	if clientID == "" {
		clientID = "modbus-hub"
	}
	opts := paho.NewClientOptions().
		AddBroker(broker).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5*time.Second).
		SetConnectTimeout(10*time.Second).
		SetCleanSession(true).
		SetOrderMatters(false).
		SetKeepAlive(30*time.Second).
		SetConnectionLostHandler(func(c paho.Client, err error) {
			log.Printf("[mqtt] 连接断开（将自动重连）: %v", err)
		})
	if p.cfg.Username != "" {
		opts.SetUsername(p.cfg.Username)
		opts.SetPassword(p.cfg.Password)
	}
	p.client = paho.NewClient(opts)
	if tok := p.client.Connect(); tok.WaitTimeout(10*time.Second) && tok.Error() != nil {
		return fmt.Errorf("MQTT 连接 %s 失败（client-id=%s，检查地址可达/认证/证书）: %w",
			broker, clientID, tok.Error())
	}
	p.queue = make(chan model.Reading, 1024)
	go p.loop()
	log.Printf("[mqtt] 已连接 %s，topic 前缀 %q", broker, p.cfg.TopicPrefix)
	return nil
}

// Handle 实现 Manager 订阅回调。
func (p *MQTTPublisher) Handle(r model.Reading) {
	if p.queue == nil {
		return
	}
	select {
	case p.queue <- r:
	default: // 队列满丢弃最旧语义不可行，直接丢新样本并记数，不阻塞采集
		log.Printf("[mqtt] 发布队列已满，丢弃 %s", r.PointName)
	}
}

func (p *MQTTPublisher) loop() {
	for {
		select {
		case <-p.stop:
			return
		case r := <-p.queue:
			topic := fmt.Sprintf("%s/%s/%s", p.cfg.TopicPrefix, r.Connection, r.PointName)
			if p.cfg.TopicPrefix == "" {
				topic = fmt.Sprintf("%s/%s", r.Connection, r.PointName)
			}
			payload, _ := jsonMarshal(r)
			tok := p.client.Publish(topic, 1, false, payload)
			if !tok.WaitTimeout(3 * time.Second) {
				log.Printf("[mqtt] 发布 %s 超时", topic)
			} else if tok.Error() != nil {
				log.Printf("[mqtt] 发布 %s 失败: %v", topic, tok.Error())
			}
		}
	}
}

// Stop 断开连接。
func (p *MQTTPublisher) Stop() {
	close(p.stop)
	if p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(500)
	}
}

// Connected 供状态展示。
func (p *MQTTPublisher) Connected() bool {
	return p.client != nil && p.client.IsConnected()
}
