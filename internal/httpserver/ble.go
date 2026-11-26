package httpserver

import (
	"net/http"
	"strconv"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/ble"
)

// bleScan 扫描周边 BLE 设备。查询参数：duration（秒，默认 3）、filter（地址/名称包含过滤）。
func (s *Server) bleScan(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	secs, _ := strconv.Atoi(q.Get("duration"))
	if secs <= 0 || secs > 30 {
		secs = 3
	}
	list, err := ble.Scan(time.Duration(secs)*time.Second, q.Get("filter"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, list)
}

// bleATRequest 发送 AT 指令的请求体。
type bleATRequest struct {
	Addr           string `json:"addr"`
	ServiceUUID    string `json:"serviceUUID"`
	WriteUUID      string `json:"writeUUID"`
	NotifyUUID     string `json:"notifyUUID"`
	Command        string `json:"command"`
	Terminator     string `json:"terminator"`
	TimeoutMs      int    `json:"timeoutMs"`
}

// bleAT 连接透传模块发送一条 AT 指令并返回应答文本（模块设置页）。
func (s *Server) bleAT(w http.ResponseWriter, r *http.Request) {
	var req bleATRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Addr == "" {
		writeErr(w, http.StatusBadRequest, errStr("设备地址不能为空"))
		return
	}
	resp, err := ble.SendAT(req.Addr, req.ServiceUUID, req.WriteUUID, req.NotifyUUID,
		req.Command, req.Terminator, time.Duration(req.TimeoutMs)*time.Millisecond)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]string{"response": resp})
}
