package modbus

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/model"
	"gorm.io/gorm"
)

// ConnStatus 连接运行状态（给前端展示）。
type ConnStatus struct {
	ID       uint      `json:"id"`
	Healthy  bool      `json:"healthy"`
	LastOK   time.Time `json:"lastOk"`
	LastError string   `json:"lastError"`
	LastErrorAt time.Time `json:"lastErrorAt"`
}

// Manager 管理全部连接的轮询循环与最新读数缓存。
type Manager struct {
	db *gorm.DB

	mu      sync.Mutex
	routines map[uint]*connRoutine // connectionID -> loop
	status  map[uint]*ConnStatus

	latestMu sync.RWMutex
	latest   map[uint]model.Reading // pointID -> 最新读数

	subsMu sync.RWMutex
	subs   map[int]func(model.Reading)
	subSeq int

	// ops：连接轮询循环持有的客户端的复用通道。串口（尤其 Windows）被轮询
	// 独占后，测试/立即读/手动写必须复用同一客户端串行执行，而不是二次拨号。
	opsMu sync.Mutex
	ops   map[uint]chan opRequest
}

// opRequest 一次在轮询客户端上串行执行的操作。
type opRequest struct {
	fn   func(Client) error
	resp chan error
}

type connRoutine struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewManager 创建轮询管理器。
func NewManager(db *gorm.DB) *Manager {
	return &Manager{
		db:      db,
		routines: make(map[uint]*connRoutine),
		status:  make(map[uint]*ConnStatus),
		latest:  make(map[uint]model.Reading),
		subs:    make(map[int]func(model.Reading)),
		ops:     make(map[uint]chan opRequest),
	}
}

// RunOp 在连接轮询循环持有的客户端上串行执行 fn。
// 连接未在轮询时返回 ran=false（调用方自行临时拨号）；
// 轮询退避重连中可能超时，返回明确的 ran=true + 错误。
func (m *Manager) RunOp(connectionID uint, fn func(Client) error, timeout time.Duration) (ran bool, err error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	m.opsMu.Lock()
	ch, ok := m.ops[connectionID]
	m.opsMu.Unlock()
	if !ok {
		return false, nil
	}
	req := opRequest{fn: fn, resp: make(chan error, 1)}
	select {
	case ch <- req:
	case <-time.After(timeout):
		return true, fmt.Errorf("连接正在退避重连，暂不能执行手动操作，请稍后再试")
	}
	select {
	case e := <-req.resp:
		return true, e
	case <-time.After(timeout):
		return true, fmt.Errorf("等待操作结果超时（连接可能正在重连）")
	}
}

// Subscribe 注册读数回调（转发层用它接收数据）。返回取消函数。
func (m *Manager) Subscribe(fn func(model.Reading)) func() {
	m.subsMu.Lock()
	m.subSeq++
	id := m.subSeq
	m.subs[id] = fn
	m.subsMu.Unlock()
	return func() {
		m.subsMu.Lock()
		delete(m.subs, id)
		m.subsMu.Unlock()
	}
}

func (m *Manager) notify(r model.Reading) {
	m.subsMu.RLock()
	subs := make([]func(model.Reading), 0, len(m.subs))
	for _, fn := range m.subs {
		subs = append(subs, fn)
	}
	m.subsMu.RUnlock()
	for _, fn := range subs {
		fn(r)
	}
}

// Reload 对照数据库重建轮询循环：新增的启动，删除/停用的停止，未变的保持。
func (m *Manager) Reload() {
	var conns []model.Connection
	if err := m.db.Find(&conns).Error; err != nil {
		log.Printf("[modbus] 读取连接配置失败: %v", err)
		return
	}
	want := make(map[uint]model.Connection)
	for _, c := range conns {
		if c.Enabled {
			want[c.ID] = c
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// 停掉不再需要的
	for id, rt := range m.routines {
		if _, ok := want[id]; !ok {
			rt.cancel()
			delete(m.routines, id)
		}
	}
	// 启动新增的
	for id, cfg := range want {
		if _, ok := m.routines[id]; !ok {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			m.routines[id] = &connRoutine{cancel: cancel, done: done}
			go m.pollLoop(ctx, done, cfg)
		}
	}
}

// Stop 停止全部轮询循环。
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, rt := range m.routines {
		rt.cancel()
		<-rt.done
		delete(m.routines, id)
	}
}

// Statuses 返回全部连接状态快照。
func (m *Manager) Statuses() map[uint]ConnStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[uint]ConnStatus, len(m.status))
	for id, s := range m.status {
		out[id] = *s
	}
	return out
}

// Latest 返回全部最新读数快照。
func (m *Manager) Latest() []model.Reading {
	m.latestMu.RLock()
	defer m.latestMu.RUnlock()
	out := make([]model.Reading, 0, len(m.latest))
	for _, r := range m.latest {
		out = append(out, r)
	}
	return out
}

func (m *Manager) markStatus(id uint, mutate func(*ConnStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.status[id]
	if !ok {
		s = &ConnStatus{ID: id}
		m.status[id] = s
	}
	mutate(s)
}

// pollLoop 单连接轮询循环：串行执行事务（RTU 半双工的正确姿势，TCP 也够用）。
// 每个最小调度周期检查各点位是否到期；连接级错误按指数退避重连。
func (m *Manager) pollLoop(ctx context.Context, done chan struct{}, cfg model.Connection) {
	defer close(done)
	type due struct {
		point model.Point
		next  time.Time
	}
	var dues []due
	var client *closerClient
	// 注册手动操作通道：测试/立即读/手动写复用本客户端（串口独占时的唯一通路）
	opsCh := make(chan opRequest, 4)
	m.opsMu.Lock()
	m.ops[cfg.ID] = opsCh
	m.opsMu.Unlock()
	defer func() {
		m.opsMu.Lock()
		delete(m.ops, cfg.ID)
		m.opsMu.Unlock()
		if client != nil {
			_ = client.Close()
		}
	}()
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	backoff := time.Second
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()

	ensureClient := func() error {
		if client != nil {
			return nil
		}
		c, err := dial(cfg, timeout)
		if err != nil {
			return err
		}
		client = c
		return nil
	}
	loadPoints := func() {
		var pts []model.Point
		if err := m.db.Where("connection_id = ? AND enabled = ?", cfg.ID, true).Find(&pts).Error; err != nil {
			log.Printf("[modbus] 连接 %s 读取点位失败: %v", cfg.Name, err)
			return
		}
		now := time.Now()
		dues = dues[:0]
		for _, p := range pts {
			pm := p.PollMs
			if pm <= 0 {
				pm = 1000
			}
			dues = append(dues, due{point: p, next: now})
		}
	}
	lastLoad := time.Time{}

	readOne := func(p model.Point) (model.Reading, error) {
		r := model.Reading{
			PointID: p.ID, PointName: p.Name, Connection: cfg.Name,
			Unit: p.Unit, Timestamp: time.Now(),
		}
		if err := ensureClient(); err != nil {
			r.Error = err.Error()
			return r, err
		}
		var data []byte
		var err error
		switch p.FunctionCode {
		case 1:
			data, err = client.ReadCoils(p.Address, p.Quantity)
		case 2:
			data, err = client.ReadDiscreteInputs(p.Address, p.Quantity)
		case 3:
			data, err = client.ReadHoldingRegisters(p.Address, p.Quantity)
		case 4:
			data, err = client.ReadInputRegisters(p.Address, p.Quantity)
		default:
			err = errUnsupportedFC
		}
		if err != nil {
			_ = client.Close()
			client = nil
			r.Error = err.Error()
			return r, err
		}
		v, derr := decode(data, p.DataType, p.Scale, p.Offset)
		if derr != nil {
			r.Error = derr.Error()
			return r, derr
		}
		r.Value, r.OK = v, true
		return r, nil
	}

	for {
		select {
		case <-ctx.Done():
			return
		case op := <-opsCh:
			// 与轮询共用同一客户端串行执行；连接级错误同样触发客户端重建
			var opErr error
			if opErr = ensureClient(); opErr == nil {
				opErr = op.fn(client)
				if opErr != nil {
					_ = client.Close()
					client = nil
				}
			}
			op.resp <- opErr
		case now := <-tick.C:
			if now.Sub(lastLoad) > 3*time.Second {
				loadPoints()
				lastLoad = now
			}
			for i := range dues {
				d := &dues[i]
				if now.Before(d.next) {
					continue
				}
				pm := d.point.PollMs
				if pm <= 0 {
					pm = 1000
				}
				d.next = now.Add(time.Duration(pm) * time.Millisecond)
				r, err := readOne(d.point)
				if r.Timestamp.IsZero() {
					r.Timestamp = now
				}
				m.latestMu.Lock()
				m.latest[r.PointID] = r
				m.latestMu.Unlock()
				m.notify(r)
				if err != nil {
					m.markStatus(cfg.ID, func(s *ConnStatus) {
						s.Healthy = false
						s.LastError = err.Error()
						s.LastErrorAt = r.Timestamp
					})
					if isConnErr(err) {
						backoff = min(backoff*2, 30*time.Second)
						select {
						case <-ctx.Done():
							return
						case <-time.After(backoff):
						}
					}
					continue
				}
				backoff = time.Second
				m.markStatus(cfg.ID, func(s *ConnStatus) {
					s.Healthy = true
					s.LastOK = r.Timestamp
				})
			}
		}
	}
}

var errUnsupportedFC = &fcError{}

type fcError struct{}

func (*fcError) Error() string { return "不支持的功能码（仅支持 1/2/3/4）" }

// isConnErr 区分连接级故障（需要退避重连）与单点数据错误（跳过该点继续）。
func isConnErr(err error) bool {
	if _, ok := err.(*fcError); ok {
		return false
	}
	return true
}

// ReadNow 立即强制读一次指定点位（管理台手动触发）。
// 连接正在轮询时复用轮询客户端（串口独占下二次拨号必然失败），否则临时拨号。
func (m *Manager) ReadNow(pointID uint) (model.Reading, error) {
	var p model.Point
	if err := m.db.First(&p, pointID).Error; err != nil {
		return model.Reading{}, err
	}
	var cfg model.Connection
	if err := m.db.First(&cfg, p.ConnectionID).Error; err != nil {
		return model.Reading{}, err
	}
	timeout := opTimeout(cfg)
	var r model.Reading
	ran, err := m.RunOp(cfg.ID, func(c Client) error {
		var e error
		r, e = readOnClient(c, p, cfg)
		return e
	}, timeout)
	if !ran {
		r, err = m.readWithFreshClient(cfg, p)
	}
	m.latestMu.Lock()
	m.latest[r.PointID] = r
	m.latestMu.Unlock()
	m.notify(r)
	return r, err
}

// opTimeout 手动操作等待上限：连接超时加裕量，至少 3 秒。
func opTimeout(cfg model.Connection) time.Duration {
	t := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if t <= 0 {
		t = time.Second
	}
	if d := t + 2*time.Second; d > 3*time.Second {
		return d
	}
	return 3 * time.Second
}

// readOnClient 在给定客户端上读一个点位并解码。
func readOnClient(c Client, p model.Point, cfg model.Connection) (model.Reading, error) {
	r := model.Reading{
		PointID: p.ID, PointName: p.Name, Connection: cfg.Name,
		Unit: p.Unit, Timestamp: time.Now(),
	}
	var data []byte
	var err error
	switch p.FunctionCode {
	case 1:
		data, err = c.ReadCoils(p.Address, p.Quantity)
	case 2:
		data, err = c.ReadDiscreteInputs(p.Address, p.Quantity)
	case 3:
		data, err = c.ReadHoldingRegisters(p.Address, p.Quantity)
	case 4:
		data, err = c.ReadInputRegisters(p.Address, p.Quantity)
	default:
		err = errUnsupportedFC
	}
	if err != nil {
		r.Error = err.Error()
		return r, err
	}
	v, derr := decode(data, p.DataType, p.Scale, p.Offset)
	if derr != nil {
		r.Error = derr.Error()
		return r, derr
	}
	r.Value, r.OK = v, true
	return r, nil
}

// readWithFreshClient 建临时连接读一次（连接未在轮询时使用）。
func (m *Manager) readWithFreshClient(cfg model.Connection, p model.Point) (model.Reading, error) {
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	c, err := dial(cfg, timeout)
	if err != nil {
		r := model.Reading{
			PointID: p.ID, PointName: p.Name, Connection: cfg.Name,
			Unit: p.Unit, Timestamp: time.Now(), Error: err.Error(),
		}
		return r, err
	}
	defer c.Close()
	return readOnClient(c, p, cfg)
}

// Write 手动写值：functionCode 5 写线圈（bool），6 写单寄存器（uint16）。
// 连接正在轮询时复用轮询客户端串行执行。
func (m *Manager) Write(connectionID uint, functionCode int, address uint16, value float64) error {
	var cfg model.Connection
	if err := m.db.First(&cfg, connectionID).Error; err != nil {
		return err
	}
	writeFn := func(c Client) error {
		switch functionCode {
		case 5:
			_, err := c.WriteSingleCoil(address, value != 0)
			return err
		case 6:
			if value < 0 || value > 65535 {
				return &writeRangeError{}
			}
			_, err := c.WriteSingleRegister(address, uint16(value))
			return err
		default:
			return &fcError{}
		}
	}
	ran, err := m.RunOp(connectionID, writeFn, opTimeout(cfg))
	if ran {
		return err
	}
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	c, err := dial(cfg, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	return writeFn(c)
}

type writeRangeError struct{}

func (*writeRangeError) Error() string { return "寄存器写入值须在 0..65535" }

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
