package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/modbus"
	"github.com/linuxsuren/modbus-hub/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	mgr := modbus.NewManager(db)
	s, err := NewServer(db, mgr)
	if err != nil && s == nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func doJSON(t *testing.T, method, url string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func doRaw(t *testing.T, method, url string, body interface{}) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	data := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		data = append(data, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return resp, data
}

func TestConnectionCRUD(t *testing.T) {
	_, ts := newTestServer(t)

	resp, out := doJSON(t, "POST", ts.URL+"/api/connections", map[string]interface{}{
		"name": "plc1", "mode": "tcp", "addr": "127.0.0.1:502", "unitId": 1, "enabled": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: %v %v", resp.StatusCode, out)
	}
	if out["id"].(float64) == 0 {
		t.Fatal("want nonzero id")
	}
	id := int(out["id"].(float64))

	resp, out = doJSON(t, "POST", ts.URL+"/api/connections", map[string]interface{}{
		"name": "bad", "mode": "abc", "addr": "x",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid mode should 400, got %v", resp.StatusCode)
	}

	resp, data := doRaw(t, "GET", ts.URL+"/api/connections", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %v", resp.StatusCode)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatalf("list decode: %v (%s)", err, data)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 connection, got %v", list)
	}

	resp, out = doJSON(t, "PUT", fmt.Sprintf("%s/api/connections/%d", ts.URL, id), map[string]interface{}{
		"name": "plc1", "mode": "tcp", "addr": "127.0.0.1:1502", "unitId": 2, "enabled": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %v %v", resp.StatusCode, out)
	}

	resp, out = doJSON(t, "DELETE", fmt.Sprintf("%s/api/connections/%d", ts.URL, id), nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %v", resp.StatusCode)
	}
}

func TestProbeBaudEndpoint(t *testing.T) {
	_, ts := newTestServer(t)

	// tcp 模式应被拒绝
	resp, out := doJSON(t, "POST", ts.URL+"/api/connections/probe-baud", map[string]interface{}{
		"name": "c1", "mode": "tcp", "addr": "127.0.0.1:502",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("tcp mode should 400, got %v %v", resp.StatusCode, out)
	}

	// 串口路径为空应被拒绝
	resp, out = doJSON(t, "POST", ts.URL+"/api/connections/probe-baud", map[string]interface{}{
		"name": "c1", "mode": "rtu", "addr": "",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty addr should 400, got %v %v", resp.StatusCode, out)
	}

	// 不存在的串口：打开失败应 400 并带错误说明
	resp, out = doJSON(t, "POST", ts.URL+"/api/connections/probe-baud", map[string]interface{}{
		"name": "c1", "mode": "rtu", "addr": "/dev/definitely-not-exist-tty",
		"unitId": 1, "timeoutMs": 300,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad port should 400, got %v %v", resp.StatusCode, out)
	}
	if out["error"] == "" {
		t.Fatalf("want error message, got %v", out)
	}
}

func TestSerialPortsEndpoint(t *testing.T) {
	_, ts := newTestServer(t)
	resp, data := doRaw(t, "GET", ts.URL+"/api/serial/ports", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("serial ports: %v", resp.StatusCode)
	}
	var ports []map[string]interface{}
	if err := json.Unmarshal(data, &ports); err != nil {
		t.Fatalf("decode: %v (%s)", err, data)
	}
	seen := map[string]bool{}
	for _, p := range ports {
		path := p["path"].(string)
		if path == "" || seen[path] {
			t.Fatalf("非法或重复串口: %v", ports)
		}
		seen[path] = true
	}
}

func TestSSEEventsStream(t *testing.T) {
	_, ts := newTestServer(t)

	// 建一个不可达 TCP 连接 + 点位：手动读会产生一条错误读数事件（无需真实设备）
	_, out := doJSON(t, "POST", ts.URL+"/api/connections", map[string]interface{}{
		"name": "sse-c", "mode": "tcp", "addr": "127.0.0.1:1", "enabled": false,
	})
	cid := int(out["id"].(float64))
	_, out = doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "sse-p", "functionCode": 3,
		"address": 0, "quantity": 1, "dataType": "uint16", "pollMs": 1000,
	})
	pid := int(out["id"].(float64))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type: %v", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal("缺少 X-Accel-Buffering: no")
	}

	// 稍后触发一次手动读 → SSE 应收到 reading 事件
	go func() {
		time.Sleep(200 * time.Millisecond)
		_, _ = doJSON(t, "POST", fmt.Sprintf("%s/api/points/%d/read", ts.URL, pid), nil)
	}()

	br := bufio.NewReader(resp.Body)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("读 SSE 流: %v", err)
		}
		if strings.HasPrefix(line, "event: reading") {
			dataLine, err := br.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(dataLine, `"pointId"`) || !strings.Contains(dataLine, `"pointName":"sse-p"`) {
				t.Fatalf("reading 事件数据异常: %v", dataLine)
			}
			return
		}
	}
}

func TestPointValidation(t *testing.T) {
	_, ts := newTestServer(t)
	_, out := doJSON(t, "POST", ts.URL+"/api/connections", map[string]interface{}{
		"name": "c1", "mode": "tcp", "addr": "127.0.0.1:502", "enabled": false,
	})
	cid := int(out["id"].(float64))

	resp, out := doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "p1", "functionCode": 3,
		"address": 0, "quantity": 2, "dataType": "float32", "pollMs": 500, "enabled": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create point: %v %v", resp.StatusCode, out)
	}

	resp, _ = doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "p2", "functionCode": 9,
		"address": 0, "quantity": 1, "dataType": "uint16", "pollMs": 500,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("fc=9 should 400, got %v", resp.StatusCode)
	}

	// float32 需要 2 个寄存器，quantity=1 应被拒
	resp, _ = doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "p3", "functionCode": 3,
		"address": 0, "quantity": 1, "dataType": "float32", "pollMs": 500,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("float32 with quantity=1 should 400, got %v", resp.StatusCode)
	}

	// API 建点漏传 scale 时应兜底为 1，否则读数恒为 0
	resp, out = doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "p4", "functionCode": 3,
		"address": 0, "quantity": 1, "dataType": "uint16", "pollMs": 500,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create without scale: %v", resp.StatusCode)
	}
	if out["scale"].(float64) != 1 {
		t.Fatalf("scale should default to 1, got %v", out["scale"])
	}

	// 小数位：缺省 0（不处理），合法范围 0..9，越界应被拒
	if out["precision"].(float64) != 0 {
		t.Fatalf("precision should default to 0, got %v", out["precision"])
	}
	resp, out = doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "p5", "functionCode": 3,
		"address": 0, "quantity": 1, "dataType": "uint16", "pollMs": 500, "precision": 2,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create with precision=2: %v %v", resp.StatusCode, out)
	}
	if out["precision"].(float64) != 2 {
		t.Fatalf("precision should persist 2, got %v", out["precision"])
	}
	pid := int(out["id"].(float64))
	resp, out = doJSON(t, "PUT", ts.URL+fmt.Sprintf("/api/points/%d", pid), map[string]interface{}{
		"connectionId": cid, "name": "p5", "functionCode": 3,
		"address": 0, "quantity": 1, "dataType": "uint16", "pollMs": 500, "precision": -1,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("precision=-1 should 400, got %v", resp.StatusCode)
	}
	resp, _ = doJSON(t, "POST", ts.URL+"/api/points", map[string]interface{}{
		"connectionId": cid, "name": "p6", "functionCode": 3,
		"address": 0, "quantity": 1, "dataType": "uint16", "pollMs": 500, "precision": 10,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("precision=10 should 400, got %v", resp.StatusCode)
	}
}

func TestSinkTCPConfig(t *testing.T) {
	_, ts := newTestServer(t)
	resp, out := doJSON(t, "GET", ts.URL+"/api/sinks/tcp", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: %v %v", resp.StatusCode, out)
	}
	if out["listen"] != ":9000" {
		t.Fatalf("default listen should be :9000, got %v", out["listen"])
	}
	// 禁用态保存
	resp, out = doJSON(t, "PUT", ts.URL+"/api/sinks/tcp", map[string]interface{}{
		"listen": ":9101", "enabled": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put: %v %v", resp.StatusCode, out)
	}
}

func TestValuesEndpoint(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, "POST", ts.URL+"/api/connections", map[string]interface{}{
		"name": "c1", "mode": "tcp", "addr": "127.0.0.1:502", "enabled": false,
	})
	resp, out := doJSON(t, "GET", ts.URL+"/api/values", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("values: %v", resp.StatusCode)
	}
	conns := out["connections"].([]interface{})
	if len(conns) != 1 {
		t.Fatalf("want 1 connection in values, got %v", conns)
	}
}

func TestStoreOpenCreatesDB(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, e := db.DB(); e == nil {
		_ = sqlDB.Close()
	}
	if _, err := os.Stat(filepath.Join(dir, "modbus-hub.db")); err != nil {
		t.Fatalf("db file should exist: %v", err)
	}
}
