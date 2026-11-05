package modbus

import (
	"context"
	"fmt"
	"time"

	gx "github.com/grid-x/modbus"
	"github.com/linuxsuren/modbus-hub/internal/model"
)

// Client 抽象一层便于测试替身：上下文无关的读取/写入子集。
type Client interface {
	ReadCoils(address, quantity uint16) ([]byte, error)
	ReadDiscreteInputs(address, quantity uint16) ([]byte, error)
	ReadHoldingRegisters(address, quantity uint16) ([]byte, error)
	ReadInputRegisters(address, quantity uint16) ([]byte, error)
	WriteSingleCoil(address uint16, value bool) ([]byte, error)
	WriteSingleRegister(address, value uint16) ([]byte, error)
}

// closerClient 适配 grid-x 带 ctx 的 API，对外保持无 ctx 签名。
type closerClient struct {
	gx.Client
	ctx   context.Context
	close func() error
}

func (c *closerClient) ReadCoils(address, quantity uint16) ([]byte, error) {
	return c.Client.ReadCoils(c.ctx, address, quantity)
}
func (c *closerClient) ReadDiscreteInputs(address, quantity uint16) ([]byte, error) {
	return c.Client.ReadDiscreteInputs(c.ctx, address, quantity)
}
func (c *closerClient) ReadHoldingRegisters(address, quantity uint16) ([]byte, error) {
	return c.Client.ReadHoldingRegisters(c.ctx, address, quantity)
}
func (c *closerClient) ReadInputRegisters(address, quantity uint16) ([]byte, error) {
	return c.Client.ReadInputRegisters(c.ctx, address, quantity)
}
func (c *closerClient) WriteSingleCoil(address uint16, value bool) ([]byte, error) {
	v := uint16(0)
	if value {
		v = 0xFF00
	}
	return c.Client.WriteSingleCoil(c.ctx, address, v)
}
func (c *closerClient) WriteSingleRegister(address, value uint16) ([]byte, error) {
	return c.Client.WriteSingleRegister(c.ctx, address, value)
}
func (c *closerClient) Close() error { return c.close() }

// dial 按连接配置建立 Modbus 客户端（TCP 或串口 RTU）。
func dial(cfg model.Connection, timeout time.Duration) (*closerClient, error) {
	switch cfg.Mode {
	case "tcp":
		h := gx.NewTCPClientHandler(cfg.Addr)
		h.Timeout = timeout
		h.SlaveID = cfg.UnitID
		if err := h.Connect(context.Background()); err != nil {
			return nil, fmt.Errorf("modbus tcp 连接 %s: %w", cfg.Addr, err)
		}
		return &closerClient{Client: gx.NewClient(h), ctx: context.Background(), close: h.Close}, nil
	case "rtu":
		h := gx.NewRTUClientHandler(cfg.Addr)
		h.BaudRate = cfg.Baud
		h.DataBits = cfg.DataBits
		h.Parity = cfg.Parity
		h.StopBits = cfg.StopBits
		h.SlaveID = cfg.UnitID
		h.Timeout = timeout
		if err := h.Connect(context.Background()); err != nil {
			return nil, fmt.Errorf("modbus rtu 打开串口 %s: %w", cfg.Addr, err)
		}
		return &closerClient{Client: gx.NewClient(h), ctx: context.Background(), close: h.Close}, nil
	default:
		return nil, fmt.Errorf("未知连接模式 %q（仅支持 tcp/rtu）", cfg.Mode)
	}
}

// TestDial 验证连接配置的可达性（TCP 可连、串口可开），随即关闭。供管理台测试按钮。
func TestDial(cfg model.Connection, timeout time.Duration) error {
	c, err := dial(cfg, timeout)
	if err != nil {
		return err
	}
	return c.Close()
}
