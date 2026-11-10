package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/model"
)

// events SSE 实时读数流：GET /api/events，事件名 reading，data 为一条 Reading JSON。
//
// 设计要点：
//   - 三件套响应头，X-Accel-Buffering: no 防 nginx 代理缓冲吞事件
//   - 连接即推当前快照（Latest），页面打开/刷新立即有数，防首帧空白
//   - 订阅回调只做带 default 的 channel 投递：SSE 消费者再慢也绝不反压轮询循环
//   - 15s 心跳注释帧防代理空闲断连；客户端断开经 ctx.Done() 退出并退订
func (s *Server) events(w http.ResponseWriter, req *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errStr("当前响应不支持流式写入"))
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fl.Flush()

	ch := make(chan model.Reading, 128)
	cancel := s.Mgr.Subscribe(func(r model.Reading) {
		select {
		case ch <- r:
		default: // 慢消费者：丢新帧，保轮询
		}
	})
	defer cancel()

	// 快照先入队（在订阅之后读，避免快照与增量之间丢帧或乱序之外的重复）
	for _, r := range s.Mgr.Latest() {
		select {
		case ch <- r:
		default:
		}
	}

	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-req.Context().Done():
			return
		case <-tick.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		case r := <-ch:
			data, err := json.Marshal(r)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: reading\ndata: %s\n\n", data); err != nil {
				return
			}
			fl.Flush()
		}
	}
}
