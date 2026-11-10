package httpserver

import (
	"net/http"

	"github.com/linuxsuren/modbus-hub/internal/model"
)

type valuesResp struct {
	Connections []connBrief   `json:"connections"`
	Readings    []model.Reading `json:"readings"`
}

type connBrief struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Addr    string `json:"addr"`
	Enabled bool   `json:"enabled"`
	Healthy *bool  `json:"healthy"`
}

// values 总览数据：连接健康 + 全部最新读数。
func (s *Server) values(w http.ResponseWriter, r *http.Request) {
	var conns []model.Connection
	if err := s.DB.Order("id").Find(&conns).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	statuses := s.Mgr.Statuses()
	briefs := make([]connBrief, 0, len(conns))
	for _, c := range conns {
		var h *bool
		if v, ok := statuses[c.ID]; ok {
			hh := v.Healthy
			h = &hh
		}
		briefs = append(briefs, connBrief{ID: c.ID, Name: c.Name, Mode: c.Mode, Addr: c.Addr, Enabled: c.Enabled, Healthy: h})
	}
	writeJSON(w, valuesResp{Connections: briefs, Readings: s.Mgr.Latest()})
}

type writeReq struct {
	ConnectionID uint    `json:"connectionId"`
	FunctionCode int     `json:"functionCode"` // 5 写线圈，6 写单寄存器
	Address      uint16  `json:"address"`
	Value        float64 `json:"value"`
}

// write 手动写值。
func (s *Server) write(w http.ResponseWriter, r *http.Request) {
	var req writeReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.FunctionCode != 5 && req.FunctionCode != 6 {
		writeErr(w, http.StatusBadRequest, errStr("写入功能码仅支持 5（线圈）或 6（单寄存器）"))
		return
	}
	if err := s.Mgr.Write(req.ConnectionID, req.FunctionCode, req.Address, req.Value); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
