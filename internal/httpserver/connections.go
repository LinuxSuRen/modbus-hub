package httpserver

import (
	"net/http"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/modbus"
	"github.com/linuxsuren/modbus-hub/internal/model"
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

// testConnection 只验证连通性：TCP 能建立连接、串口能打开即算通过。
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
	if err := modbus.TestDial(c, timeout); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
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
