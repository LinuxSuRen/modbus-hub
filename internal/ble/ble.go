// Package ble 提供低功耗蓝牙（BLE）串口透传链路：扫描、连接、收发。
// 面向 E104-BT12NSP 一类的「BLE 转 UART 透传」模块：主机作为 BLE Central
// 连接模块，向写特征下发数据、从通知特征接收数据，即等价于一条串口链路。
package ble

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"tinygo.org/x/bluetooth"
)

// DeviceInfo 扫描到的 BLE 设备。
type DeviceInfo struct {
	Address string `json:"address"` // 如 "AA:BB:CC:DD:EE:FF"
	Name    string `json:"name"`
	RSSI    int16  `json:"rssi"`
}

// 默认透传服务/特征 UUID（E104-BT12 系列透传模块常用 FFE0/FFE1）。
// 连接配置里可覆盖；AT 设置页也会用到同一组默认值。
const (
	DefaultServiceUUID = "FFE0"
	DefaultWriteUUID   = "FFE1"
	DefaultNotifyUUID  = "FFE1"
)

// ErrAdapter 表示本机蓝牙适配器不可用等致命环境问题。
var ErrAdapter = errors.New("本机蓝牙适配器不可用")

var (
	scanMu sync.Mutex // 全机只有一个 BLE 适配器，扫描与连接串行化
	scanOn bool
)

func enableAdapter() (*bluetooth.Adapter, error) {
	a := bluetooth.DefaultAdapter
	if err := a.Enable(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAdapter, err)
	}
	return a, nil
}

// Scan 扫描 duration 时长，返回发现的设备（按信号强度降序）。
// filter 非空时按地址或名称包含过滤（大小写不敏感）。
func Scan(duration time.Duration, filter string) ([]DeviceInfo, error) {
	a, err := enableAdapter()
	if err != nil {
		return nil, err
	}
	scanMu.Lock()
	defer scanMu.Unlock()
	if scanOn {
		return nil, errors.New("已有扫描进行中，请稍后再试")
	}
	scanOn = true
	defer func() {
		scanOn = false
		_ = a.StopScan()
	}()

	var mu sync.Mutex
	seen := map[string]DeviceInfo{}
	done := make(chan struct{})
	err = a.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		mu.Lock()
		defer mu.Unlock()
		addr := r.Address.String()
		if addr == "" {
			return
		}
		name := r.LocalName()
		d := DeviceInfo{Address: addr, Name: name, RSSI: r.RSSI}
		if old, ok := seen[addr]; !ok || r.RSSI > old.RSSI || (name != "" && old.Name == "") {
			if ok {
				if d.Name == "" {
					d.Name = old.Name
				}
			}
			seen[addr] = d
		}
	})
	if err != nil {
		return nil, fmt.Errorf("启动 BLE 扫描失败: %w", err)
	}
	select {
	case <-done:
	case <-time.After(duration):
	}
	mu.Lock()
	list := make([]DeviceInfo, 0, len(seen))
	for _, d := range seen {
		if matchFilter(filter, d) {
			list = append(list, d)
		}
	}
	mu.Unlock()
	sort.Slice(list, func(i, j int) bool { return list[i].RSSI > list[j].RSSI })
	return list, nil
}

func matchFilter(filter string, d DeviceInfo) bool {
	if filter == "" {
		return true
	}
	f := lower(filter)
	return lower(d.Address) == f || contains(lower(d.Name), f)
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func contains(s, sub string) bool {
	n, m := len(s), len(sub)
	if m == 0 {
		return true
	}
	for i := 0; i+m <= n; i++ {
		if s[i:i+m] == sub {
			return true
		}
	}
	return false
}

// parseUUID 解析用户输入的 UUID，兼容 "FFE0" 16 位短格式与完整 128 位格式。
func parseUUID(s string) (bluetooth.UUID, error) {
	if s == "" {
		return bluetooth.UUID{}, errors.New("UUID 不能为空")
	}
	u, err := bluetooth.ParseUUID(s)
	if err == nil {
		return u, nil
	}
	// ParseUUID 不认裸 16 位短格式时退化处理
	var v uint16
	_, e := fmt.Sscanf(s, "%04hX", &v)
	if e != nil {
		return bluetooth.UUID{}, fmt.Errorf("无法解析 UUID %q: %v", s, err)
	}
	return bluetooth.New16BitUUID(v), nil
}

// normalizeUUIDs 补全空缺的 UUID 为默认透传值，并解析校验。
func normalizeUUIDs(service, write, notify string) (bluetooth.UUID, bluetooth.UUID, bluetooth.UUID, error) {
	if service == "" {
		service = DefaultServiceUUID
	}
	if write == "" {
		write = DefaultWriteUUID
	}
	if notify == "" {
		notify = DefaultNotifyUUID
	}
	su, err := parseUUID(service)
	if err != nil {
		return su, su, su, err
	}
	wu, err := parseUUID(write)
	if err != nil {
		return su, wu, wu, err
	}
	nu, err := parseUUID(notify)
	if err != nil {
		return su, wu, nu, err
	}
	return su, wu, nu, nil
}

// Link 一条已建立的 BLE 透传链路（等价于打开的串口）。
type Link struct {
	dev     bluetooth.Device
	writeCh bluetooth.DeviceCharacteristic
	mu      sync.Mutex // 串行化 Write（RTU 半双工）
	buf     []byte
	notify  chan []byte
	closed  chan struct{}
	once    sync.Once
}

// Dial 连接指定地址的透传模块并订阅通知特征。
// service/write/notify 传空则使用默认 FFE0/FFE1/FFE1。
func Dial(addr, service, write, notify string, connectTimeout time.Duration) (*Link, error) {
	a, err := enableAdapter()
	if err != nil {
		return nil, err
	}
	su, wu, nu, err := normalizeUUIDs(service, write, notify)
	if err != nil {
		return nil, err
	}

	scanMu.Lock()
	defer scanMu.Unlock()
	var dev bluetooth.Device
	connected := make(chan error, 1)
	go func() {
		// Address 结构随平台而异：Linux 内嵌 MACAddress，macOS 则为 UUID；
		// 两者的公共入口都是 Set(string)，此处按平台各自解析。
		var address bluetooth.Address
		address.Set(addr)
		d, e := a.Connect(address, bluetooth.ConnectionParams{})
		if e != nil {
			connected <- e
			return
		}
		dev = d
		connected <- nil
	}()
	select {
	case e := <-connected:
		if e != nil {
			return nil, fmt.Errorf("连接 BLE 设备 %s 失败: %w", addr, e)
		}
	case <-time.After(connectTimeout):
		return nil, fmt.Errorf("连接 BLE 设备 %s 超时（%v）", addr, connectTimeout)
	}

	l := &Link{dev: dev, notify: make(chan []byte, 64), closed: make(chan struct{})}
	// 通知回调不区分特征：透传模块通常只有一条下行通道
	if err := l.setupServices(su, wu, nu); err != nil {
		_ = dev.Disconnect()
		return nil, err
	}
	return l, nil
}

// setupServices 发现服务/特征并订阅通知。
func (l *Link) setupServices(serviceUUID, writeUUID, notifyUUID bluetooth.UUID) error {
	svcs, err := l.dev.DiscoverServices(nil)
	if err != nil {
		return fmt.Errorf("发现 BLE 服务失败: %w", err)
	}
	foundSvc := false
	var svc bluetooth.DeviceService
	for _, s := range svcs {
		if s.UUID() == serviceUUID {
			svc, foundSvc = s, true
			break
		}
	}
	if !foundSvc {
		ids := ""
		for _, s := range svcs {
			ids += " " + s.UUID().String()
		}
		return fmt.Errorf("未找到透传服务 %s，设备提供的服务:%s", serviceUUID.String(), ids)
	}
	chars, err := svc.DiscoverCharacteristics(nil)
	if err != nil {
		return fmt.Errorf("发现 BLE 特征失败: %w", err)
	}
	foundWrite, foundNotify := false, false
	for _, c := range chars {
		if c.UUID() == writeUUID {
			l.writeCh, foundWrite = c, true
		}
		if c.UUID() == notifyUUID {
			nc := c
			foundNotify = true
			if err := nc.EnableNotifications(func(buf []byte) {
				b := make([]byte, len(buf))
				copy(b, buf)
				select {
				case l.notify <- b:
				default: // 队列满时丢弃最旧数据，避免阻塞系统回调
					select {
					case <-l.notify:
					default:
					}
					select {
					case l.notify <- b:
					default:
					}
				}
			}); err != nil {
				return fmt.Errorf("订阅通知特征失败: %w", err)
			}
		}
	}
	if !foundWrite {
		ids := ""
		for _, c := range chars {
			ids += " " + c.UUID().String()
		}
		return fmt.Errorf("未找到写特征 %s，服务提供的特征:%s", writeUUID.String(), ids)
	}
	if !foundNotify {
		return fmt.Errorf("未找到通知特征 %s", notifyUUID.String())
	}
	return nil
}

// Write 向写特征下发数据（带响应写，保证可靠）。
func (l *Link) Write(p []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for len(p) > 0 {
		n := len(p)
		if n > 240 { // 单包不超过常见 MTU 上限
			n = 240
		}
		if _, err := l.writeCh.Write(p[:n]); err != nil {
			return fmt.Errorf("BLE 写入失败: %w", err)
		}
		p = p[n:]
	}
	return nil
}

// ReadUntil 从通知流累积读取，凑满 n 字节或在 deadline 超时。
// 返回已读数据与错误；超时时返回累计到的部分数据。
func (l *Link) ReadUntil(n int, deadline time.Time) ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.buf
	l.buf = nil
	for len(out) < n {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		timer := time.NewTimer(remaining)
		select {
		case b := <-l.notify:
			out = append(out, b...)
			timer.Stop()
		case <-timer.C:
			l.buf = append([]byte(nil), out[n:]...)
			if len(out) >= n {
				return out[:n], nil
			}
			return out, fmt.Errorf("BLE 接收超时（已收 %d/%d 字节）", len(out), n)
		case <-l.closed:
			timer.Stop()
			if len(out) >= n {
				return out[:n], nil
			}
			return out, errors.New("BLE 链路已关闭")
		}
	}
	if len(out) < n {
		return out, fmt.Errorf("BLE 接收超时（已收 %d/%d 字节）", len(out), n)
	}
	l.buf = append([]byte(nil), out[n:]...)
	return out[:n], nil
}

// Discard 清空未消费的通知缓存（半双工请求-响应模型下防串包）。
func (l *Link) Discard() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for {
		select {
		case <-l.notify:
		default:
			return
		}
	}
}

// Close 断开链路。
func (l *Link) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.dev.Disconnect()
}
