package modbus

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	gx "github.com/grid-x/modbus"
	"github.com/linuxsuren/modbus-hub/internal/ble"
	"github.com/linuxsuren/modbus-hub/internal/model"
)

// bleHandler 把 BLE 透传链路适配成 grid-x 的 Packager + Transporter：
// RTU 帧编解码与 CRC 校验照搬标准，传输层走 BLE 写特征/通知特征。
// BLE 会打乱字节间隔，因此应答按「预期长度」解析而不是字节间超时。
type bleHandler struct {
	slaveID byte
	link    *ble.Link
	timeout time.Duration
}

func (h *bleHandler) SetSlave(slaveID byte) { h.slaveID = slaveID }
func (h *bleHandler) Connect(_ context.Context) error {
	return nil // 链路在构造时已建立
}
func (h *bleHandler) Close() error {
	if h.link != nil {
		return h.link.Close()
	}
	return nil
}

// Encode 按 RTU 帧格式编码：从站地址 + 功能码 + 数据 + CRC16。
func (h *bleHandler) Encode(pdu *gx.ProtocolDataUnit) ([]byte, error) {
	length := len(pdu.Data) + 4
	if length > 256 {
		return nil, fmt.Errorf("modbus: RTU 帧过长 %d", length)
	}
	adu := make([]byte, length)
	adu[0] = h.slaveID
	adu[1] = pdu.FunctionCode
	copy(adu[2:], pdu.Data)
	crc := crc16(adu[:length-2])
	adu[length-1] = byte(crc >> 8)
	adu[length-2] = byte(crc)
	return adu, nil
}

// Verify 校验应答从站地址与请求一致。
func (h *bleHandler) Verify(req, resp []byte) error {
	if len(resp) < 4 {
		return fmt.Errorf("modbus: 应答过短 %d 字节", len(resp))
	}
	if resp[0] != req[0] {
		return fmt.Errorf("modbus: 应答从站地址 %d 与请求 %d 不符", resp[0], req[0])
	}
	return nil
}

// Decode 从 RTU 帧提取 PDU 并校验 CRC。
func (h *bleHandler) Decode(adu []byte) (*gx.ProtocolDataUnit, error) {
	length := len(adu)
	if length < 4 {
		return nil, fmt.Errorf("modbus: RTU 帧过短 %d 字节", length)
	}
	crc := uint16(adu[length-1])<<8 | uint16(adu[length-2])
	if crc != crc16(adu[:length-2]) {
		return nil, fmt.Errorf("modbus: 应答 CRC 校验失败")
	}
	return &gx.ProtocolDataUnit{FunctionCode: adu[1], Data: adu[2 : length-2]}, nil
}

// Send 发送请求帧并按预期长度接收应答帧。
func (h *bleHandler) Send(ctx context.Context, req []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h.link.Discard()
	if err := h.link.Write(req); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(h.timeout)

	// 先收从站地址 + 功能码
	head, err := h.link.ReadUntil(2, deadline)
	if err != nil {
		return nil, fmt.Errorf("等待 BLE 应答超时: %v", err)
	}
	fc := head[1]
	var total int
	switch {
	case fc == req[1]:
		total = expectedResponseLength(req)
	case fc == req[1]|0x80:
		total = 5 // 异常应答：地址 + 功能码 + 异常码 + CRC
	default:
		return nil, fmt.Errorf("modbus: 意外的应答功能码 0x%02X（期望 0x%02X）", fc, req[1])
	}
	adu, err := h.link.ReadUntil(total, deadline)
	if err != nil {
		if len(adu) >= total {
			err = nil
		} else {
			return nil, fmt.Errorf("BLE 应答不完整: %v", err)
		}
	}
	if err := h.Verify(req, adu); err != nil {
		return nil, err
	}
	if _, err := h.Decode(adu); err != nil {
		return nil, err
	}
	return adu, nil
}

// expectedResponseLength 按请求帧推算应答帧总长（覆盖 FC 1/2/3/4/5/6/15/16）。
func expectedResponseLength(req []byte) int {
	switch req[1] {
	case 1, 2:
		count := int(binary.BigEndian.Uint16(req[4:6]))
		return 3 + (count+7)/8 + 2
	case 3, 4:
		count := int(binary.BigEndian.Uint16(req[4:6]))
		return 3 + count*2 + 2
	case 5, 6, 15, 16:
		return 8 // 地址 + 功能码 + 4 字节数据 + CRC
	default:
		return 8
	}
}

// crc16 Modbus RTU 的 CRC-16/MODBUS 校验。
func crc16(data []byte) uint16 {
	crc := uint16(0xFFFF)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// dialBLE 建立 BLE 透传链路并返回 Modbus 客户端。
// Baud 等串口参数属于模块 UART 一侧，对主机侧 BLE 透明，因此不使用。
func dialBLE(cfg model.Connection, timeout time.Duration) (*closerClient, error) {
	h := &bleHandler{slaveID: byte(cfg.UnitID), timeout: timeout}
	if timeout <= 0 {
		h.timeout = 2 * time.Second
	}
	// BLE 建链+服务发现可能明显慢于串口打开，单独放宽
	link, err := ble.Dial(cfg.Addr, cfg.BleServiceUUID, cfg.BleWriteUUID, cfg.BleNotifyUUID, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("BLE 连接 %s: %w", cfg.Addr, err)
	}
	h.link = link
	return &closerClient{Client: gx.NewClient(h), ctx: context.Background(), close: h.Close}, nil
}
