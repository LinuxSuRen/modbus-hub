package model

import "time"

// Connection 一条 Modbus 连接：TCP 或 USB 串口上的 RTU。
type Connection struct {
	ID       uint   `gorm:"primarykey" json:"id"`
	Name     string `gorm:"uniqueIndex;size:64" json:"name"`
	Mode     string `gorm:"size:8;not null" json:"mode"` // tcp | rtu
	Addr     string `gorm:"size:128;not null" json:"addr"` // tcp: host:port；rtu: /dev/ttyUSB0 或 COM3
	UnitID   uint8  `json:"unitId"`
	Baud     int    `json:"baud"`
	DataBits int    `json:"dataBits"`
	Parity   string `gorm:"size:1" json:"parity"` // N | E | O
	StopBits int    `json:"stopBits"`
	TimeoutMs int   `json:"timeoutMs"`
	Enabled  bool   `json:"enabled"`
}

// Point 一个采集点位：按功能码批量读取后按数据类型解码为工程值。
type Point struct {
	ID           uint   `gorm:"primarykey" json:"id"`
	ConnectionID uint   `gorm:"index;not null" json:"connectionId"`
	Name         string `gorm:"uniqueIndex;size:64" json:"name"`
	FunctionCode int    `gorm:"not null" json:"functionCode"` // 1 线圈 2 离散输入 3 保持寄存器 4 输入寄存器
	Address      uint16 `gorm:"not null" json:"address"`      // 0 起始
	Quantity     uint16 `gorm:"not null" json:"quantity"`     // 寄存器/线圈个数
	DataType     string `gorm:"size:16;not null" json:"dataType"` // uint16 int16 uint32 int32 float32 float32sw bool raw
	Scale        float64 `json:"scale"`
	Offset       float64 `json:"offset"`
	Unit         string `gorm:"size:16" json:"unit"`
	PollMs       int    `gorm:"not null" json:"pollMs"`
	Enabled      bool   `json:"enabled"`
}

// SinkMQTT MQTT 转发配置（单实例，ID 固定 1）。
type SinkMQTT struct {
	ID          uint   `gorm:"primarykey" json:"id"`
	Broker      string `gorm:"size:128" json:"broker"` // 如 tcp://127.0.0.1:1883
	ClientID    string `gorm:"size:64" json:"clientId"`
	Username    string `gorm:"size:64" json:"username"`
	Password    string `gorm:"size:128" json:"password"`
	TopicPrefix string `gorm:"size:64" json:"topicPrefix"` // 如 modbus
	Enabled     bool   `json:"enabled"`
}

// SinkTCP TCP 转发配置（单实例，ID 固定 1）：本机监听端口，JSON 行流推送。
type SinkTCP struct {
	ID      uint   `gorm:"primarykey" json:"id"`
	Listen  string `gorm:"size:64" json:"listen"` // 如 :9000
	Enabled bool   `json:"enabled"`
}

// Reading 一次点位读数事件。
type Reading struct {
	PointID       uint      `json:"pointId"`
	PointName     string    `json:"pointName"`
	Connection    string    `json:"connection"`
	Value         float64   `json:"value"`
	Unit          string    `json:"unit"`
	Timestamp     time.Time `json:"timestamp"`
	OK            bool      `json:"ok"`
	Error         string    `json:"error,omitempty"`
}
