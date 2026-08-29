package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerLifecycle(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer s.Close()

	// Accept blocks until a client connects; run it in the background and
	// drive the handshake from a loopback client.
	go func() { _ = s.Accept() }()

	conn, err := net.Dial("tcp", s.listener.Addr().String())
	if err != nil {
		t.Skipf("cannot dial ipc server (CI may restrict): %v", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}

	// Authentication handshake: first line must be the secret token.
	if _, err := fmt.Fprintf(conn, "%s\n", s.secret); err != nil {
		t.Fatalf("send token: %v", err)
	}

	// Wait for Accept to authenticate and register the connection.
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		connected := s.conn != nil
		s.mu.Unlock()
		if connected {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not authenticate client in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Server → client: SendJSON writes a newline-delimited JSON message.
	if err := s.SendJSON(Message{Type: "ping"}); err != nil {
		t.Fatalf("SendJSON: %v", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read from client: %v", err)
	}
	if n == 0 {
		t.Fatal("expected non-empty response from server")
	}
	if got := string(buf[:n]); !strings.Contains(got, `"type":"ping"`) {
		t.Fatalf("unexpected server response: %q", got)
	}
}

func TestPortFilePathShape(t *testing.T) {
	p := portFilePath()
	if p == "" {
		t.Fatal("portFilePath returned empty string")
	}
	wantName := fmt.Sprintf("neoviolet-ipc-%d", os.Getpid())
	if filepath.Base(p) != wantName {
		t.Errorf("portFilePath base = %q, want %q", filepath.Base(p), wantName)
	}
	if filepath.Clean(filepath.Dir(p)) != filepath.Clean(os.TempDir()) {
		t.Errorf("portFilePath dir = %q, want %q", filepath.Dir(p), filepath.Clean(os.TempDir()))
	}
}
