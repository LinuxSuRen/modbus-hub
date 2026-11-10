package modbus

import (
	"math"
	"testing"
)

func TestDecode(t *testing.T) {
	// 0x0064 = 100
	ub := []byte{0x00, 0x64}
	// -100 int16 = 0xFF9C
	ib := []byte{0xFF, 0x9C}
	// 1000.5 float32 = 0x447A2000
	fb := []byte{0x44, 0x7A, 0x20, 0x00}
	fsw := []byte{0x20, 0x00, 0x44, 0x7A}
	u32 := []byte{0x00, 0x00, 0x27, 0x10} // 10000
	cases := []struct {
		name   string
		data   []byte
		typ    string
		scale  float64
		offset float64
		want   float64
	}{
		{"uint16", ub, "uint16", 1, 0, 100},
		{"uint16 scaled", ub, "uint16", 0.1, 0, 10},
		{"uint16 offset", ub, "uint16", 1, -50, 50},
		{"int16 negative", ib, "int16", 1, 0, -100},
		{"raw", ub, "raw", 1, 0, 100},
		{"bool true", []byte{0x01}, "bool", 1, 0, 1},
		{"bool false", []byte{0x00}, "bool", 1, 0, 0},
		{"uint32", u32, "uint32", 1, 0, 10000},
		{"int32", u32, "int32", 1, 0, 10000},
		{"float32", fb, "float32", 1, 0, 1000.5},
		{"float32sw", fsw, "float32sw", 1, 0, 1000.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decode(c.data, c.typ, c.scale, c.offset)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got-c.want) > 1e-6 {
				t.Fatalf("want %v, got %v", c.want, got)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := decode([]byte{0x01}, "uint16", 1, 0); err == nil {
		t.Fatal("short data should error")
	}
	if _, err := decode([]byte{0x01, 0x02}, "nope", 1, 0); err == nil {
		t.Fatal("unknown type should error")
	}
}
