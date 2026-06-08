package sink

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestUDPSinkWriteOneDatagramPerLine(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer pc.Close()

	cfg := DefaultConfig()
	cfg.ServiceName = "ctx"
	cfg.InstanceID = "ctx-0"
	cfg.Environment = "test"
	s, err := NewUDPSink(&UDPSinkConfig{Config: cfg, Address: pc.LocalAddr().String()})
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	defer s.Close()

	if err := s.Write(context.Background(), &LogEntry{Level: "info", Message: "hello"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 65535)
	_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("readfrom: %v", err)
	}
	var got LogEntry
	if err := json.Unmarshal(buf[:n], &got); err != nil {
		t.Fatalf("not valid json datagram: %v (%q)", err, buf[:n])
	}
	if got.Message != "hello" || got.Level != "info" {
		t.Fatalf("bad payload: %+v", got)
	}
	if got.ServiceName != "ctx" || got.InstanceID != "ctx-0" || got.Environment != "test" {
		t.Fatalf("entry not enriched: %+v", got)
	}
}

func TestUDPSinkWriteBatchSendsOneDatagramEach(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer pc.Close()

	s, err := NewUDPSink(&UDPSinkConfig{Config: DefaultConfig(), Address: pc.LocalAddr().String()})
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	defer s.Close()

	entries := []*LogEntry{{Message: "a"}, {Message: "b"}, {Message: "c"}}
	if err := s.WriteBatch(context.Background(), entries); err != nil {
		t.Fatalf("writebatch: %v", err)
	}

	got := map[string]bool{}
	buf := make([]byte, 65535)
	for i := 0; i < 3; i++ {
		_ = pc.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, _, err := pc.ReadFrom(buf)
		if err != nil {
			t.Fatalf("expected datagram %d: %v", i, err)
		}
		var e LogEntry
		if err := json.Unmarshal(buf[:n], &e); err != nil {
			t.Fatalf("datagram %d not single-line json: %v", i, err)
		}
		got[e.Message] = true
	}
	for _, m := range []string{"a", "b", "c"} {
		if !got[m] {
			t.Fatalf("missing datagram %q", m)
		}
	}
}

func TestNewUDPSinkRejectsEmptyAddress(t *testing.T) {
	if _, err := NewUDPSink(&UDPSinkConfig{Config: DefaultConfig(), Address: ""}); err == nil {
		t.Fatal("expected error for empty address")
	}
}
