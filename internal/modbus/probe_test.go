package modbus

import (
	"context"
	"fmt"
	"testing"
	"time"

	gx "github.com/grid-x/modbus"
	"github.com/linuxsuren/modbus-hub/internal/model"
)

func TestClassifyProbeErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		hit  bool
	}{
		{"读通", nil, true},
		{"modbus异常应答", &gx.Error{FunctionCode: 3, ExceptionCode: gx.ExceptionCodeIllegalDataAddress}, true},
		{"包装后的modbus异常应答", fmt.Errorf("read: %w", &gx.Error{FunctionCode: 3, ExceptionCode: gx.ExceptionCodeIllegalFunction}), true},
		{"超时", context.DeadlineExceeded, false},
		{"CRC错误", fmt.Errorf("modbus: response crc '1' does not match expected '2'"), false},
	}
	for _, c := range cases {
		hit, _ := ClassifyProbeErr(c.err)
		if hit != c.hit {
			t.Fatalf("%s: want hit=%v, got %v (err=%v)", c.name, c.hit, hit, c.err)
		}
	}
}

func TestProbeBaudsList(t *testing.T) {
	seen := map[int]bool{}
	for _, b := range probeBauds {
		if seen[b] {
			t.Fatalf("重复档位 %d", b)
		}
		seen[b] = true
	}
	if !seen[9600] || !seen[115200] {
		t.Fatal("常见档位 9600/115200 缺失")
	}
}

func TestProbeBaudRejectsTCP(t *testing.T) {
	if _, err := ProbeBaud(model.Connection{Mode: "tcp"}, time.Second); err == nil {
		t.Fatal("tcp 模式应被拒绝")
	}
}

func TestProbeBaudPortOpenFailure(t *testing.T) {
	// 不存在的串口路径：打开即失败，应返回错误而非「未命中」
	_, err := ProbeBaud(model.Connection{
		Mode: "rtu", Addr: "/dev/definitely-not-exist-tty", UnitID: 1,
		Baud: 9600, DataBits: 8, Parity: "N", StopBits: 1,
	}, 300*time.Millisecond)
	if err == nil {
		t.Fatal("串口打不开应返回错误")
	}
}
