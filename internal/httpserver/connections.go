package httpserver

import (
	"fmt"
	"net/http"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/modbus"
	"github.com/linuxsuren/modbus-hub/internal/model"
	"github.com/linuxsuren/modbus-hub/internal/serialport"
)

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) {
	var conns []model.Connection
	if err := s.DB.Order("id").Find(&conns).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	statuses := s.Mgr.Statuses()
	type connWithStatus struct {
		model.Connection
		Status *modbus.ConnStatus `json:"status"`
	}
	out := make([]connWithStatus, 0, len(conns))
	for _, c := range conns {
		var st *modbus.ConnStatus
		if v, ok := statuses[c.ID]; ok {
			st = &v
		}
		out = append(out, connWithStatus{Connection: c, Status: st})
	}
	writeJSON(w, out)
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var c model.Connection
	if err := decodeBody(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validateConnection(&c); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.DB.Create(&c).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Mgr.Reload()
	writeJSON(w, c)
}

func (s *Server) updateConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var c model.Connection
	if err := s.DB.First(&c, id).Error; err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var in model.Connection
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	in.ID = c.ID
	if err := validateConnection(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.DB.Model(&c).Updates(map[string]interface{}{
		"name": in.Name, "mode": in.Mode, "addr": in.Addr, "unit_id": in.UnitID,
		"baud": in.Baud, "data_bits": in.DataBits, "parity": in.Parity,
		"stop_bits": in.StopBits, "timeout_ms": in.TimeoutMs, "enabled": in.Enabled,
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Mgr.Reload()
	writeJSON(w, in)
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.DB.Delete(&model.Connection{}, id).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	// 级联删除其点位
	_ = s.DB.Where("connection_id = ?", id).Delete(&model.Point{}).Error
	s.Mgr.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

// testConnection 验证连通性。连接正在轮询时（串口被轮询客户端独占），
// 复用轮询客户端真发一帧读请求验证，避免二次拨号必然 Access is denied。
func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var c model.Connection
	if err := s.DB.First(&c, id).Error; err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	timeout := time.Duration(c.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Second
	}
	ran, rerr := s.Mgr.RunOp(c.ID, func(cl modbus.Client) error {
		_, e := cl.ReadHoldingRegisters(0, 1)
		return e
	}, timeout+2*time.Second)
	if ran {
		hit, note := modbus.ClassifyProbeErr(rerr)
		if hit {
			writeJSON(w, map[string]interface{}{"ok": true, "note": "串口正被本连接轮询占用，已复用轮询连接验证：" + note})
			return
		}
		errMsg := "复用轮询连接测试失败"
		if rerr != nil {
			errMsg += "：" + rerr.Error()
		}
		writeJSON(w, map[string]interface{}{"ok": false, "error": errMsg})
		return
	}
	if err := modbus.TestDial(c, timeout); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}

// serialPorts 列出系统串口（含设备描述），供前端下拉选择。
func (s *Server) serialPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := serialport.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, ports)
}

// probeBaud 按请求体提交的连接配置（无需先保存）探测串口设备波特率。
func (s *Server) probeBaud(w http.ResponseWriter, r *http.Request) {
	var c model.Connection
	if err := decodeBody(r, &c); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if c.Mode != "rtu" {
		writeErr(w, http.StatusBadRequest, errStr("波特率探测仅支持串口 RTU 连接"))
		return
	}
	if c.Addr == "" {
		writeErr(w, http.StatusBadRequest, errStr("串口设备路径不能为空"))
		return
	}
	// 复用连接校验补全串口参数默认值（数据位/停止位/校验）
	_ = validateConnection(&c)
	res, err := modbus.ProbeBaud(c, time.Duration(c.TimeoutMs)*time.Millisecond)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if !res.OK {
		writeJSON(w, map[string]interface{}{
			"ok":        false,
			"attempts":  res.Attempts,
			"error":     fmt.Sprintf("已尝试 %d 档波特率均无有效应答，请检查：从站地址是否正确、校验位/停止位是否匹配、485 接线（A/B 是否接反）与设备供电", len(res.Attempts)),
		})
		return
	}
	writeJSON(w, res)
}

func validateConnection(c *model.Connection) error {
	if c.Name == "" {
		return errStr("名称不能为空")
	}
	if c.Mode != "tcp" && c.Mode != "rtu" {
		return errStr("模式仅支持 tcp 或 rtu")
	}
	if c.Addr == "" {
		return errStr("地址不能为空")
	}
	if c.Mode == "rtu" {
		if c.Baud <= 0 {
			c.Baud = 9600
		}
		if c.DataBits <= 0 {
			c.DataBits = 8
		}
		if c.StopBits <= 0 {
			c.StopBits = 1
		}
		if c.Parity == "" {
			c.Parity = "N"
		}
	}
	if c.TimeoutMs <= 0 {
		c.TimeoutMs = 1000
	}
	return nil
}

type errStr string

func (e errStr) Error() string { return string(e) }
