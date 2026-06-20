// Package ipc provides bidirectional TCP localhost communication
// between the NeoViolet TUI and the neoviolet-gui wrapper.
//
// The TUI listens on 127.0.0.1:0 (random port) and writes the assigned
// port to a temp file (/tmp/neoviolet-ipc-<pid> or %TEMP%\neoviolet-ipc-<pid>).
// The GUI reads this file, connects via TCP, and exchanges newline-delimited
// plain-text messages in the format "<type> [payload]".
//
// GUI → TUI message types:
//
//	open <path>          — request loading an audio file at <path>
//
// TUI → GUI message types (reserved for future use):
//
//	title <text>         — now-playing track title
//	state play|pause     — playback state change
package ipc

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// Server listens on TCP localhost for a connection from the GUI wrapper.
// Messages from the GUI are delivered on the Incoming channel.
type Server struct {
	listener net.Listener
	conn     net.Conn
	mu       sync.Mutex

	// Incoming delivers messages received from the GUI (one per line).
	Incoming chan string

	// Addr is the TCP address the server is listening on, written to the
	// port file so the GUI can discover it.
	Addr string
}

// NewServer listens on 127.0.0.1:0 (random port) and writes the assigned
// address to a port file that the GUI reads to discover the endpoint.
func NewServer() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ipc listen: %w", err)
	}

	addr := listener.Addr().String()
	if err := os.WriteFile(portFilePath(), []byte(addr), 0644); err != nil {
		listener.Close()
		return nil, fmt.Errorf("ipc write port file: %w", err)
	}

	logger.Info("IPC server listening", "addr", addr)
	return &Server{
		listener: listener,
		Incoming: make(chan string, 8),
		Addr:     addr,
	}, nil
}

// Accept blocks until the GUI connects. After accepting, a goroutine is
// started to read incoming messages and deliver them on s.Incoming.
func (s *Server) Accept() error {
	conn, err := s.listener.Accept()
	if err != nil {
		return fmt.Errorf("ipc accept: %w", err)
	}

	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	logger.Info("IPC client connected", "remote", conn.RemoteAddr())
	go s.readLoop(conn)
	return nil
}

// Send writes a message to the connected GUI. Returns an error if no
// client is connected or the write fails.
func (s *Server) Send(msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return fmt.Errorf("ipc: no client connected")
	}
	_, err := fmt.Fprintf(s.conn, "%s\n", msg)
	if err != nil {
		logger.Warn("IPC send failed", "err", err)
	}
	return err
}

// Close shuts down the listener and connection, and removes the port file.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	if s.listener != nil {
		s.listener.Close()
	}
	_ = os.Remove(portFilePath())
}

func (s *Server) readLoop(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		logger.Debug("IPC received", "msg", line)
		select {
		case s.Incoming <- line:
		default:
			logger.Warn("IPC incoming channel full, dropping message", "msg", line)
		}
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("IPC read error", "err", err)
	}
	logger.Info("IPC client disconnected")
}

// portFilePath returns the temp file path where the TCP address is stored.
func portFilePath() string {
	dir := os.TempDir()
	return filepath.Join(dir, fmt.Sprintf("neoviolet-ipc-%d", os.Getpid()))
}
