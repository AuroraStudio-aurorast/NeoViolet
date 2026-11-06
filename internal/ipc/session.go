package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

const frameLimit = 10 * 1024 * 1024
const writeTimeout = time.Second

// Event is delivered in connection order, including lifecycle transitions.
// Session identifies the connection so delayed commands cannot affect a new GUI.
type Event struct {
	Kind    string // connected, request, disconnected
	Session uint64
	Message Message
}

type outgoing struct {
	data    []byte
	flushed chan struct{}
}

type session struct {
	id       uint64
	conn     net.Conn
	done     chan struct{}
	stopOnce sync.Once
	control  chan outgoing
	state    chan outgoing
	lyrics   chan outgoing
}

func (p *session) stop() {
	p.stopOnce.Do(func() { close(p.done); _ = p.conn.Close() })
}

func (p *session) flush() {
	timer := time.NewTimer(writeTimeout)
	defer timer.Stop()
	flushed := make(chan struct{})
	select {
	case p.control <- outgoing{flushed: flushed}:
	case <-p.done:
		return
	case <-timer.C:
		return
	}
	select {
	case <-flushed:
	case <-p.done:
	case <-timer.C:
	}
}

func (s *Server) acceptLoop() {
	defer s.workers.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.conn = conn
		s.mu.Unlock()
		s.serve(conn)
		s.mu.Lock()
		s.conn, s.peer = nil, nil
		s.mu.Unlock()
	}
}

func (s *Server) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), frameLimit)
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != s.secret {
		return
	}
	hello, err := json.Marshal(Hello())
	if err != nil {
		return
	}
	if _, err := fmt.Fprintf(conn, "%s\n", hello); err != nil {
		return
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return
	}
	s.mu.Lock()
	s.nextSession++
	p := &session{id: s.nextSession, conn: conn, done: make(chan struct{}),
		control: make(chan outgoing, 32), state: make(chan outgoing, 1), lyrics: make(chan outgoing, 1)}
	s.peer = p
	s.mu.Unlock()
	s.workers.Add(1)
	go s.writeLoop(p)
	defer p.stop()
	if !s.deliver(Event{Kind: "connected", Session: p.id}) {
		return
	}
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var message Message
		if err := json.Unmarshal([]byte(raw), &message); err != nil {
			logger.Warn("IPC invalid request", "err", err)
			break
		}
		if !s.deliver(Event{Kind: "request", Session: p.id, Message: message}) {
			return
		}
	}
	p.stop()
	s.deliver(Event{Kind: "disconnected", Session: p.id})
}

func (s *Server) deliver(event Event) bool {
	select {
	case s.Incoming <- event:
		return true
	case <-s.done:
		return false
	}
}

// SendJSON queues a message without performing socket I/O on the caller.
// State and lyric frames retain the latest value; control overload disconnects
// the session instead of silently dropping a command result.
func (s *Server) SendJSON(message Message) error { return s.SendTo(0, message) }

// SendTo rejects responses belonging to a connection that has been replaced.
// A zero session ID targets the current authenticated client.
func (s *Server) SendTo(id uint64, message Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("ipc marshal: %w", err)
	}
	if len(data)+1 > frameLimit {
		return fmt.Errorf("ipc frame too large")
	}
	frame := outgoing{data: append(data, '\n')}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.peer
	if p == nil || s.closing || (id != 0 && p.id != id) {
		return fmt.Errorf("ipc: no matching client connected")
	}
	select {
	case <-p.done:
		return fmt.Errorf("ipc: disconnected")
	default:
	}
	var latest chan outgoing
	switch message.Type {
	case "state":
		latest = p.state
	case "lyrics":
		latest = p.lyrics
	}
	if latest != nil {
		select {
		case <-latest:
		default:
		}
		latest <- frame
		return nil
	}
	select {
	case p.control <- frame:
		return nil
	default:
		p.stop()
		return fmt.Errorf("ipc: control queue full; disconnected")
	}
}

func (s *Server) writeLoop(p *session) {
	defer s.workers.Done()
	defer p.stop()
	for {
		var frame outgoing
		// Prefer command results over replaceable display updates.
		select {
		case <-p.done:
			return
		case frame = <-p.control:
		default:
			select {
			case <-p.done:
				return
			case frame = <-p.control:
			case frame = <-p.state:
			case frame = <-p.lyrics:
			}
		}
		if frame.flushed != nil {
			close(frame.flushed)
			continue
		}
		if err := p.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
			return
		}
		if _, err := p.conn.Write(frame.data); err != nil {
			return
		}
	}
}
