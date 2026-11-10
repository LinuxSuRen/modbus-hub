package modbus

import (
	"context"
	"errors"
	"fmt"
	"time"

	gx "github.com/grid-x/modbus"
	"github.com/linuxsuren/modbus-hub/internal/model"
)

// probeBauds 常见串口波特率档位，按工程现场出现频率从高到低排序，命中即停。
var probeBauds = []int{9600, 19200, 38400, 115200, 4800, 2400, 57600, 1200, 230400}

// probePause 换档重开串口之间的间歇，给 USB 转 485 芯片留出复位时间。
const probePause = 60 * time.Millisecond

// BaudAttempt 单档波特率的探测结果。
type BaudAttempt struct {
	Baud int    `json:"baud"`
	OK   bool   `json:"ok"`
	Note string `json:"note,omitempty"`
}

// ProbeBaudResult 波特率探测整体结果。
type ProbeBaudResult struct {
	OK       bool          `json:"ok"`
	Baud     int           `json:"baud"` // 命中的波特率；未命中为 0
	Attempts []BaudAttempt `json:"attempts"`
}

// ProbeBaud 对串口逐档尝试常见波特率：每档真实发送一次读保持寄存器（FC3）请求，
// 收到 CRC 校验通过的有效应答即判定该档可用，命中即停。Modbus 异常应答同样算命中——
// 异常码说明设备完整收到了请求帧并回出了 CRC 合法的应答帧，串口链路本身是通的。
// 串口打不开时直接报错终止：那是路径/占用/权限问题，与波特率无关。
func ProbeBaud(cfg model.Connection, timeout time.Duration) (*ProbeBaudResult, error) {
	if cfg.Mode != "rtu" {
		return nil, fmt.Errorf("波特率探测仅支持串口 RTU 连接")
	}
	if timeout <= 0 {
		timeout = 600 * time.Millisecond
	}
	if timeout > 800*time.Millisecond {
		timeout = 800 * time.Millisecond // 单档等待上限，控制整轮扫描时长
	}
	if cfg.DataBits <= 0 {
		cfg.DataBits = 8
	}
	if cfg.StopBits <= 0 {
		cfg.StopBits = 1
	}
	if cfg.Parity == "" {
		cfg.Parity = "N"
	}

	res := &ProbeBaudResult{Attempts: make([]BaudAttempt, 0, len(probeBauds))}
	for i, baud := range probeBauds {
		if i > 0 {
			time.Sleep(probePause)
		}
		hit, note, err := probeOnce(cfg, baud, timeout)
		if err != nil {
			return nil, err
		}
		res.Attempts = append(res.Attempts, BaudAttempt{Baud: baud, OK: hit, Note: note})
		if hit {
			res.OK, res.Baud = true, baud
			return res, nil
		}
	}
	return res, nil
}

// probeOnce 用指定波特率打开串口并发一帧 FC3 读请求（从站号取连接配置）。
// 返回（是否命中，说明，串口打开错误）。
func probeOnce(cfg model.Connection, baud int, timeout time.Duration) (bool, string, error) {
	h := gx.NewRTUClientHandler(cfg.Addr)
	h.BaudRate = baud
	h.DataBits = cfg.DataBits
	h.Parity = cfg.Parity
	h.StopBits = cfg.StopBits
	h.SlaveID = cfg.UnitID
	h.Timeout = timeout
	if err := h.Connect(context.Background()); err != nil {
		return false, "", fmt.Errorf("打开串口 %s（%d bps）: %w", cfg.Addr, baud, err)
	}
	defer func() { _ = h.Close() }()

	_, err := gx.NewClient(h).ReadHoldingRegisters(context.Background(), 0, 1)
	hit, note := ClassifyProbeErr(err)
	return hit, note, nil
}

// ClassifyProbeErr 把读请求的返回错误归类为「命中/未命中」。
func ClassifyProbeErr(err error) (hit bool, note string) {
	if err == nil {
		return true, "读保持寄存器成功"
	}
	var gxErr *gx.Error
	if errors.As(err, &gxErr) {
		return true, fmt.Sprintf("设备应答异常码（%s），串口链路已通", gxErr.Error())
	}
	return false, "无有效应答"
}
