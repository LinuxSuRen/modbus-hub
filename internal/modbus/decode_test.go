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
		name      string
		data      []byte
		typ       string
		scale     float64
		offset    float64
		precision int
		want      float64
	}{
		{"uint16", ub, "uint16", 1, 0, 0, 100},
		{"uint16 scaled", ub, "uint16", 0.1, 0, 0, 10},
		{"uint16 offset", ub, "uint16", 1, -50, 0, 50},
		// 3 × 0.1 的二进制浮点结果是 0.30000000000000004，precision=1 收敛为 0.3
		{"uint16 scaled rounded", []byte{0x00, 0x03}, "uint16", 0.1, 0, 1, 0.3},
		// precision=2 时 249 × 0.1 = 24.9（float64 下本身有噪声），收敛后干净
		{"uint16 scaled rounded 2", []byte{0x00, 0xF9}, "uint16", 0.1, 0, 2, 24.9},
		{"int16 negative", ib, "int16", 1, 0, 0, -100},
		{"raw", ub, "raw", 1, 0, 0, 100},
		{"bool true", []byte{0x01}, "bool", 1, 0, 0, 1},
		{"bool false", []byte{0x00}, "bool", 1, 0, 0, 0},
		{"uint32", u32, "uint32", 1, 0, 0, 10000},
		{"int32", u32, "int32", 1, 0, 0, 10000},
		{"float32", fb, "float32", 1, 0, 0, 1000.5},
		{"float32sw", fsw, "float32sw", 1, 0, 0, 1000.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := decode(c.data, c.typ, c.scale, c.offset, c.precision)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Fatalf("want %v, got %v", c.want, got)
			}
		})
	}
}

func TestTransform(t *testing.T) {
	// precision=0 原样保留运行时浮点结果（注意：用变量计算，避免 Go 常量折叠成精确 0.3）
	raw, scale := 3.0, 0.1
	if v := transform(raw, scale, 0, 0); v != raw*scale {
		t.Fatalf("precision=0 should keep raw float, got %v", v)
	}
	if v := transform(3, 0.1, 0, 1); v != 0.3 {
		t.Fatalf("want 0.3, got %v", v)
	}
	// 负数四舍五入按“远离零”处理：-2.25 保留 1 位 → -2.3
	if v := transform(-2.25, 1, 0, 1); v != -2.3 {
		t.Fatalf("want -2.3, got %v", v)
	}
	// NaN / Inf 原样返回
	if !math.IsNaN(transform(math.NaN(), 1, 0, 2)) {
		t.Fatal("NaN should pass through")
	}
	if !math.IsInf(transform(math.Inf(1), 1, 0, 2), 1) {
		t.Fatal("+Inf should pass through")
	}
	// 极大值放弃收敛，避免溢出为 Inf
	big := math.MaxFloat64 / 4
	if v := transform(big, 1, 0, 9); v != big {
		t.Fatalf("huge value should keep as-is, got %v", v)
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := decode([]byte{0x01}, "uint16", 1, 0, 0); err == nil {
		t.Fatal("short data should error")
	}
	if _, err := decode([]byte{0x01, 0x02}, "nope", 1, 0, 0); err == nil {
		t.Fatal("unknown type should error")
	}
}
