// Package serialport 枚举当前系统可用的串口，供连接管理界面的下拉选择。
package serialport

// Port 一个可用串口：Path 是打开串口所用的路径（如 COM8、/dev/ttyUSB0），
// Name 是设备描述（尽力而为，可能为空，如 "USB-SERIAL CH340"）。
type Port struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}
