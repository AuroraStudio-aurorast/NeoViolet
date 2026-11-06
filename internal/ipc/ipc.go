package ipc

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// Server owns one authenticated GUI session at a time and accepts replacements
// after disconnection. Close stops all network work and closes Incoming.
type Server struct {
	listener    net.Listener
	conn        net.Conn
	peer        *session
	mu          sync.Mutex
	Incoming    chan Event
	secret      string
	done        chan struct{}
	closeOnce   sync.Once
	workers     sync.WaitGroup
	nextSession uint64
	closing     bool
}

// NewServer publishes a PID-scoped endpoint and starts accepting GUI sessions.
func NewServer() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ipc listen: %w", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("ipc generate token: %w", err)
	}
	secret := hex.EncodeToString(tokenBytes)
	// O_EXCL rejects existing files and symlinks; only this process owns removal.
	f, err := os.OpenFile(portFilePath(), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("ipc create port file: %w", err)
	}
	_, writeErr := fmt.Fprintf(f, "%s\n%s", listener.Addr(), secret)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(portFilePath())
		_ = listener.Close()
		return nil, fmt.Errorf("ipc write port file: %w", errors.Join(writeErr, closeErr))
	}
	s := &Server{listener: listener, Incoming: make(chan Event, 8), secret: secret, done: make(chan struct{})}
	s.workers.Add(1)
	go s.acceptLoop()
	return s, nil
}

// Session returns the authenticated connection generation, or zero when unavailable.
func (s *Server) Session() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peer == nil || s.closing {
		return 0
	}
	select {
	case <-s.peer.done:
		return 0
	default:
		return s.peer.id
	}
}

// Close flushes queued control frames for a bounded interval, then interrupts all
// socket operations. It is safe to call repeatedly, including on a zero Server.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		peer, conn := s.peer, s.conn
		s.mu.Unlock()
		if peer != nil {
			peer.flush()
			peer.stop()
		}
		if s.done != nil {
			close(s.done)
		}
		if s.listener != nil {
			_ = s.listener.Close()
		}
		if conn != nil {
			_ = conn.Close()
		}
		s.workers.Wait()
		if s.Incoming != nil {
			close(s.Incoming)
		}
		if s.listener != nil {
			_ = os.Remove(portFilePath())
		}
	})
}

func portFilePath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("neoviolet-ipc-%d", os.Getpid()))
}
