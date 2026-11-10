//go:build !windows

package serialport

import (
	"path/filepath"
	"sort"
	"strings"
)

// devGlobs 常见串口设备路径模式；by-id 是稳定路径（重插不变），排在最前优先展示。
var devGlobs = []struct {
	glob string
	rank int
}{
	{"/dev/serial/by-id/*", 0},   // usb-1a86_USB_Serial-if00-port0
	{"/dev/serial/by-path/*", 1}, // pci-0000:00:14.0-usb-0:1:1.0-port0
	{"/dev/ttyUSB*", 2},
	{"/dev/ttyACM*", 2},
	{"/dev/tty.usb*", 2}, // darwin
	{"/dev/cu.usb*", 2},  // darwin
	{"/dev/ttyAMA*", 3},  // 树莓派板载
	{"/dev/ttyTHS*", 3},  // NVIDIA
	{"/dev/ttyS*", 4},
}

// List 按路径模式枚举 Unix 串口；by-id 的文件名本身就是设备描述。
func List() ([]Port, error) {
	var ports []Port
	seen := map[string]bool{}
	for _, g := range devGlobs {
		ms, err := filepath.Glob(g.glob)
		if err != nil {
			continue
		}
		for _, m := range ms {
			if seen[m] {
				continue
			}
			seen[m] = true
			name := ""
			if strings.Contains(g.glob, "by-id") || strings.Contains(g.glob, "by-path") {
				name = filepath.Base(m)
			}
			ports = append(ports, Port{Path: m, Name: name})
		}
	}
	sort.SliceStable(ports, func(i, j int) bool { return ports[i].Path < ports[j].Path })
	return ports, nil
}
