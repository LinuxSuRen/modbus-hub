package sink

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/linuxsuren/modbus-hub/internal/model"
)

func TestTCPServerBroadcast(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	s := NewTCPServer(fmt.Sprintf("127.0.0.1:%d", port))
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Stop()

	conn, err := net.Dial("tcp", s.listen)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for s.ClientCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s.ClientCount() != 1 {
		t.Fatalf("want 1 client, got %d", s.ClientCount())
	}

	s.Handle(model.Reading{PointID: 7, PointName: "temp", Value: 25.5, OK: true})

	rd := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := rd.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var got model.Reading
	if err := json.Unmarshal(line[:len(line)-1], &got); err != nil {
		t.Fatal(err)
	}
	if got.PointName != "temp" || got.Value != 25.5 || !got.OK {
		t.Fatalf("unexpected reading: %+v", got)
	}
}

func TestMQTTPublisherDisabled(t *testing.T) {
	p := NewMQTTPublisher(model.SinkMQTT{Enabled: false})
	if err := p.Start(); err != nil {
		t.Fatalf("disabled sink should start as no-op, got %v", err)
	}
	p.Handle(model.Reading{}) // 不应 panic
	p.Stop()
}
