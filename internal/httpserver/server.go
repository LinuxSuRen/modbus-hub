// Package httpserver 提供 REST API 与内嵌前端静态资源。
package httpserver

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/linuxsuren/modbus-hub/internal/modbus"
	"github.com/linuxsuren/modbus-hub/internal/model"
	"github.com/linuxsuren/modbus-hub/internal/sink"
	"github.com/linuxsuren/modbus-hub/internal/version"
	"github.com/linuxsuren/modbus-hub/web"
	"gorm.io/gorm"
)

// Server 汇聚 API 路由、轮询管理器与两个转发器。
type Server struct {
	DB  *gorm.DB
	Mgr *modbus.Manager

	mqtt      *sink.MQTTPublisher
	tcp       *sink.TCPServer
	cancelMqtt func()
	cancelTCP  func()
}

// NewServer 创建服务并装配转发链路。
func NewServer(db *gorm.DB, mgr *modbus.Manager) (*Server, error) {
	s := &Server{DB: db, Mgr: mgr}
	if err := s.restartSinks(); err != nil {
		// 转发器起不来不阻塞服务（例如 broker 暂不可达），只记录状态
		log.Printf("[sink] 初始化转发器存在失败项: %v", err)
	}
	return s, nil
}

// restartSinks 依据配置重建两个转发器（配置变更后调用）。
func (s *Server) restartSinks() error {
	var errMQTT, errTCP error
	if s.cancelMqtt != nil {
		s.cancelMqtt()
	}
	if s.mqtt != nil {
		s.mqtt.Stop()
	}
	if s.cancelTCP != nil {
		s.cancelTCP()
	}
	if s.tcp != nil {
		s.tcp.Stop()
	}

	var mq model.SinkMQTT
	if e := s.DB.First(&mq, 1).Error; e == nil {
		pub := sink.NewMQTTPublisher(mq)
		if e := pub.Start(); e != nil {
			errMQTT = e
		} else {
			s.mqtt = pub
			s.cancelMqtt = s.Mgr.Subscribe(pub.Handle)
		}
	}
	var tp model.SinkTCP
	if e := s.DB.First(&tp, 1).Error; e == nil && tp.Enabled {
		srv := sink.NewTCPServer(tp.Listen)
		if e := srv.Start(); e != nil {
			errTCP = e
		} else {
			s.tcp = srv
			s.cancelTCP = s.Mgr.Subscribe(srv.Handle)
		}
	}
	if errMQTT != nil {
		return errMQTT
	}
	return errTCP
}

// Handler 返回完整 HTTP 路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": version.Version, "platform": version.Platform()})
	})

	// 连接
	mux.HandleFunc("GET /api/connections", s.listConnections)
	mux.HandleFunc("POST /api/connections", s.createConnection)
	mux.HandleFunc("PUT /api/connections/{id}", s.updateConnection)
	mux.HandleFunc("DELETE /api/connections/{id}", s.deleteConnection)
	mux.HandleFunc("POST /api/connections/{id}/test", s.testConnection)
	mux.HandleFunc("POST /api/connections/probe-baud", s.probeBaud)
	mux.HandleFunc("GET /api/serial/ports", s.serialPorts)

	// 点位
	mux.HandleFunc("GET /api/points", s.listPoints)
	mux.HandleFunc("POST /api/points", s.createPoint)
	mux.HandleFunc("PUT /api/points/{id}", s.updatePoint)
	mux.HandleFunc("DELETE /api/points/{id}", s.deletePoint)
	mux.HandleFunc("POST /api/points/{id}/read", s.readPoint)

	// 实时值 / 手动写
	mux.HandleFunc("GET /api/values", s.values)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/write", s.write)

	// 转发配置
	mux.HandleFunc("GET /api/sinks/mqtt", s.getSinkMQTT)
	mux.HandleFunc("PUT /api/sinks/mqtt", s.putSinkMQTT)
	mux.HandleFunc("GET /api/sinks/tcp", s.getSinkTCP)
	mux.HandleFunc("PUT /api/sinks/tcp", s.putSinkTCP)
	mux.HandleFunc("GET /api/sinks/status", s.sinkStatus)

	// 静态前端（SPA 兜底到 index.html）
	dist, err := fs.Sub(web.Dist(), "dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(dist))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path != "" {
				if f, err := dist.Open(path); err == nil {
					_ = f.Close()
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			// SPA 路由兜底
			index, err := dist.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer func() { _ = index.Close() }()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			buf := make([]byte, 0, 4096)
			tmp := make([]byte, 4096)
			for {
				n, err := index.Read(tmp)
				buf = append(buf, tmp[:n]...)
				if err != nil {
					break
				}
			}
			_, _ = w.Write(buf)
		})
	}
	return mux
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func pathID(r *http.Request) (uint, error) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		return 0, fmt.Errorf("无效的 id: %w", err)
	}
	return uint(id), nil
}

func decodeBody(r *http.Request, v interface{}) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}
