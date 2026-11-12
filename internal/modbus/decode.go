package modbus

import (
	"encoding/binary"
	"fmt"
	"math"
)

// decode 将一次批量读取的原始字节按数据类型解为工程值，再套 scale/offset，
// precision>0 时四舍五入保留 n 位小数。
// data 布局遵循 Modbus 大端字序：每寄存器 2 字节。
func decode(data []byte, dataType string, scale, offset float64, precision int) (float64, error) {
	need := map[string]int{
		"uint16": 2, "int16": 2, "bool": 1,
		"uint32": 4, "int32": 4, "float32": 4, "float32sw": 4,
		"raw": 2,
	}[dataType]
	if need == 0 {
		return 0, fmt.Errorf("未知数据类型 %q", dataType)
	}
	if len(data) < need {
		return 0, fmt.Errorf("数据不足：需要 %d 字节，实际 %d（检查 Quantity）", need, len(data))
	}
	var raw float64
	switch dataType {
	case "uint16", "raw":
		raw = float64(binary.BigEndian.Uint16(data))
	case "int16":
		raw = float64(int16(binary.BigEndian.Uint16(data)))
	case "uint32":
		raw = float64(binary.BigEndian.Uint32(data))
	case "int32":
		raw = float64(int32(binary.BigEndian.Uint32(data)))
	case "float32":
		raw = float64(math.Float32frombits(binary.BigEndian.Uint32(data)))
	case "float32sw":
		// 字交换（low word first）后的 float32：常见于部分 PLC/变频器
		sw := []byte{data[2], data[3], data[0], data[1]}
		raw = float64(math.Float32frombits(binary.BigEndian.Uint32(sw)))
	case "bool":
		if data[0]&1 == 1 {
			raw = 1
		}
	}
	return transform(raw, scale, offset, precision), nil
}

// transform 换算工程值并按小数位收敛：倍率换算在二进制浮点下可能产生
// 0.30000000000000004 这类噪声（如 3×0.1），四舍五入后经 JSON 最短表示
// 输出即为 0.3，转发与页面显示都是干净数值。
func transform(raw, scale, offset float64, precision int) float64 {
	v := raw*scale + offset
	if precision <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	p := math.Pow(10, float64(precision))
	// 防溢出：值大到乘 p 会溢出 double 时放弃收敛，原样返回
	if math.Abs(v) > math.MaxFloat64/p {
		return v
	}
	return math.Round(v*p) / p
}
