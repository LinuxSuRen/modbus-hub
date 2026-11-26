package modbus

import (
	"bytes"
	"encoding/binary"
	"testing"

	gx "github.com/grid-x/modbus"
)

func TestCrc16(t *testing.T) {
	// 经典校验样本："010300000001" 应得 CRC 840A（帧尾 84 0A）
	frame := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}
	crc := crc16(frame)
	if got := []byte{byte(crc), byte(crc >> 8)}; !bytes.Equal(got, []byte{0x84, 0x0A}) {
		t.Fatalf("crc16 = % X, want 84 0A", got)
	}
}

func TestBleHandlerEncodeDecode(t *testing.T) {
	h := &bleHandler{slaveID: 1}
	pdu := &gx.ProtocolDataUnit{FunctionCode: 3, Data: []byte{0x00, 0x00, 0x00, 0x01}}
	adu, err := h.Encode(pdu)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(adu) != 8 {
		t.Fatalf("adu len = %d, want 8", len(adu))
	}
	if _, err := h.Decode(adu); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	// 破坏 CRC 应报错
	adu[len(adu)-1] ^= 0xFF
	if _, err := h.Decode(adu); err == nil {
		t.Fatal("Decode 应校验出 CRC 错误")
	}
}

func TestExpectedResponseLength(t *testing.T) {
	cases := []struct {
		fc    byte
		count uint16
		want  int
	}{
		{3, 2, 3 + 4 + 2},   // 读保持寄存器 2 个：addr+fc+len+4数据+crc
		{1, 10, 3 + 2 + 2},  // 读线圈 10 个：2 字节
		{1, 1, 3 + 1 + 2},   // 读线圈 1 个：1 字节
		{4, 125, 3 + 250 + 2},
		{6, 0, 8},           // 写单寄存器回显
		{5, 0, 8},           // 写单线圈回显
	}
	for _, c := range cases {
		req := []byte{0x01, c.fc, 0x00, 0x00}
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], c.count)
		req = append(req, b[:]...)
		if got := expectedResponseLength(req); got != c.want {
			t.Errorf("fc=%d count=%d: got %d, want %d", c.fc, c.count, got, c.want)
		}
	}
}

func TestBleHandlerVerify(t *testing.T) {
	h := &bleHandler{slaveID: 1}
	req := []byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01, 0x84, 0x0A}
	if err := h.Verify(req, []byte{0x01, 0x03, 0x02, 0x00, 0x05, 0xB8, 0x49}); err != nil {
		t.Fatalf("Verify 合法应答: %v", err)
	}
	if err := h.Verify(req, []byte{0x02, 0x03, 0x02, 0x00, 0x05, 0x78, 0x49}); err == nil {
		t.Fatal("从站地址不符应报错")
	}
}
