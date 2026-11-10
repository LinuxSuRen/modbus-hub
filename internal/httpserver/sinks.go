package httpserver

import (
	"net/http"

	"github.com/linuxsuren/modbus-hub/internal/model"
)

func (s *Server) getSinkMQTT(w http.ResponseWriter, r *http.Request) {
	var mq model.SinkMQTT
	if err := s.DB.First(&mq, 1).Error; err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, mq)
}

func (s *Server) putSinkMQTT(w http.ResponseWriter, r *http.Request) {
	var in model.SinkMQTT
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	in.ID = 1
	if in.Enabled && in.Broker == "" {
		writeErr(w, http.StatusBadRequest, errStr("启用 MQTT 转发时 broker 地址不能为空"))
		return
	}
	if err := s.DB.Save(&in).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.restartSinks(); err != nil {
		// 保存成功但启动失败：返回保存结果与错误详情
		writeJSON(w, map[string]interface{}{"saved": true, "started": false, "error": err.Error()})
		return
	}
	writeJSON(w, in)
}

func (s *Server) getSinkTCP(w http.ResponseWriter, r *http.Request) {
	var tp model.SinkTCP
	if err := s.DB.First(&tp, 1).Error; err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, tp)
}

func (s *Server) putSinkTCP(w http.ResponseWriter, r *http.Request) {
	var in model.SinkTCP
	if err := decodeBody(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	in.ID = 1
	if in.Enabled && in.Listen == "" {
		writeErr(w, http.StatusBadRequest, errStr("启用 TCP 转发时监听地址不能为空"))
		return
	}
	if err := s.DB.Save(&in).Error; err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.restartSinks(); err != nil {
		writeJSON(w, map[string]interface{}{"saved": true, "started": false, "error": err.Error()})
		return
	}
	writeJSON(w, in)
}

// sinkStatus 两个转发器的运行状态。
func (s *Server) sinkStatus(w http.ResponseWriter, r *http.Request) {
	mqttOn := s.mqtt != nil && s.mqtt.Connected()
	tcpOn := s.tcp != nil
	clients := 0
	if tcpOn {
		clients = s.tcp.ClientCount()
	}
	writeJSON(w, map[string]interface{}{
		"mqtt": map[string]interface{}{"running": mqttOn},
		"tcp":  map[string]interface{}{"running": tcpOn, "clients": clients},
	})
}
