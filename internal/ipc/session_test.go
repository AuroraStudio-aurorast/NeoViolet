package ipc

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func sessionClient(t *testing.T, s *Server, initial string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", s.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(conn, "%s\n%s", s.secret, initial); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	hello, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("automatic authentication: %v", err)
	}
	if !strings.Contains(hello, `"type":"hello"`) {
		t.Fatalf("hello: %s", hello)
	}
	return conn, reader
}

func TestServerAcceptsAndReconnects(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, _ := sessionClient(t, s, "")
	<-s.Incoming
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	sessionClient(t, s, "")
}

func TestServerKeepsPipelinedAuthenticationRequests(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessionClient(t, s, "{\"type\":\"play_pause\"}\n{\"type\":\"quit\"}\n")
	<-s.Incoming // connected
	for range 2 {
		select {
		case <-s.Incoming:
		case <-time.After(time.Second):
			t.Fatal("pipelined request lost")
		}
	}
}

func TestServerDeliversBurstWithoutDroppingCommands(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, _ := sessionClient(t, s, "")
	if _, err := fmt.Fprint(conn, strings.Repeat("{\"type\":\"play_pause\"}\n", 40)); err != nil {
		t.Fatal(err)
	}
	<-s.Incoming // connected
	for range 40 {
		select {
		case <-s.Incoming:
		case <-time.After(time.Second):
			t.Fatal("command burst was dropped")
		}
	}
}

func TestSlowPeerCannotBlockProducersAndLatestLyricsSurvive(t *testing.T) {
	local, remote := net.Pipe()
	defer func() { _ = remote.Close() }()
	p := &session{id: 1, conn: local, done: make(chan struct{}), control: make(chan outgoing, 32), state: make(chan outgoing, 1), lyrics: make(chan outgoing, 1)}
	s := &Server{peer: p, done: make(chan struct{})}
	s.workers.Add(1)
	go s.writeLoop(p)
	defer s.Close()
	sent := make(chan error, 1)
	go func() {
		for range 100 {
			if err := s.SendJSON(Message{Type: "lyrics", Title: "old"}); err != nil {
				sent <- err
				return
			}
		}
		sent <- s.SendJSON(Message{Type: "lyrics", Title: "latest"})
	}()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("socket write blocked caller")
	}
	if err := remote.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(remote)
	for range 2 {
		raw, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(raw, `"title":"latest"`) {
			return
		}
	}
	t.Fatal("latest lyric snapshot not delivered")
}

func TestCloseInterruptsUnauthenticatedClientAndIsIdempotent(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", s.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	done := make(chan struct{})
	go func() { s.Close(); s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close waited for unauthenticated input")
	}
}

func TestInvalidAuthenticationDoesNotConsumeTheListener(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, err := net.Dial("tcp", s.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(conn, "wrong-token"); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	sessionClient(t, s, "")
}

func TestCloseFlushesQueuedControlMessages(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	_, reader := sessionClient(t, s, "")
	<-s.Incoming // authentication is published after the hello frame
	if err := s.SendJSON(Message{Type: "quit"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	raw, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"type":"quit"`) {
		t.Fatalf("quit was not flushed: %s", raw)
	}
}

func TestObsoleteSessionCannotWriteToNewClient(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	conn, _ := sessionClient(t, s, "")
	first := <-s.Incoming
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	_, reader := sessionClient(t, s, "")
	if err := s.SendTo(first.Session, Message{Type: "quit"}); err == nil {
		t.Fatal("stale response accepted")
	}
	// Session publication follows the hello write; wait for its lifecycle event.
	for event := range s.Incoming {
		if event.Kind == "connected" {
			break
		}
	}
	if err := s.SendJSON(Message{Type: "desktop_lyrics"}); err != nil {
		t.Fatal(err)
	}
	raw, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, `"type":"quit"`) {
		t.Fatal("old session closed replacement")
	}
}
