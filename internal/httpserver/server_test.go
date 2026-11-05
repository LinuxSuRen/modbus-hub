package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
