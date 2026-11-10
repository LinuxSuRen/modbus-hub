package httpserver

import (
	"net/http"

	"github.com/linuxsuren/modbus-hub/internal/model"
)

func (s *Server) listPoints(w http.ResponseWriter, r *http.Request) {
	q := s.DB.Order("id")
	if cid := r.URL.Query().Get("connection"); cid != "" {
		var points []model.Point
		if err := q.Where("connection_id = ?", cid).Find(&points).Error; err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, points)
		return
	}
	var points []model.Point
	if err := q.Find(&points).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, points)
}

func (s *Server) createPoint(w http.ResponseWriter, r *http.Request) {
	var p model.Point
	if err := decodeBody(r, &p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := validatePoint(&p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.DB.Create(&p).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Mgr.Reload()
	writeJSON(w, p)
}

func (s *Server) updatePoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var p model.Point
	if err := s.DB.First(&p, id).Error; err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	var in model.Point
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	in.ID = p.ID
	if err := validatePoint(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.DB.Model(&p).Updates(map[string]interface{}{
		"connection_id": in.ConnectionID, "name": in.Name, "function_code": in.FunctionCode,
		"address": in.Address, "quantity": in.Quantity, "data_type": in.DataType,
		"scale": in.Scale, "offset": in.Offset, "unit": in.Unit,
		"poll_ms": in.PollMs, "enabled": in.Enabled,
	}).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Mgr.Reload()
	writeJSON(w, in)
}

func (s *Server) deletePoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.DB.Delete(&model.Point{}, id).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Mgr.Reload()
	writeJSON(w, map[string]bool{"ok": true})
}

// readPoint 管理台手动触发一次立即读取。
func (s *Server) readPoint(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	reading, err := s.Mgr.ReadNow(id)
	writeJSON(w, reading)
}

func validatePoint(p *model.Point) error {
	if p.Name == "" {
		return errStr("点位名称不能为空")
	}
	if p.FunctionCode < 1 || p.FunctionCode > 4 {
		return errStr("功能码仅支持 1/2/3/4")
	}
	if p.Quantity < 1 || p.Quantity > 125 {
		return errStr("数量须在 1..125")
	}
	switch p.DataType {
	case "uint16", "int16", "uint32", "int32", "float32", "float32sw", "bool", "raw":
	default:
		return errStr("数据类型不支持")
	}
	if p.DataType == "bool" {
		// 线圈/离散输入按位打包，1 字节起
		if p.Quantity < 1 {
			return errStr("数量至少为 1")
		}
	} else if int(p.Quantity)*2 < typeBytes(p.DataType) {
		return errStr("数量不足以装下该数据类型")
	}
	if p.PollMs <= 0 {
		p.PollMs = 1000
	}
	// 倍率缺省为 0 会把一切读数变成 0（常见于 API 建点漏传 scale），兜底为 1
	if p.Scale == 0 {
		p.Scale = 1
	}
	return nil
}

func typeBytes(t string) int {
	switch t {
	case "uint32", "int32", "float32", "float32sw":
		return 4
	default:
		return 2
	}
}
