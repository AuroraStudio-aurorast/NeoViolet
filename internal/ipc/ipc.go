// Package ipc provides bidirectional TCP localhost communication
// between the NeoViolet TUI and the neoviolet-gui wrapper.
//
// The TUI listens on 127.0.0.1:0 (random port) and writes the assigned
// address and a random secret token to a temp file. The GUI reads this
// file, connects via TCP, and sends the token as the first line for
// authentication. All subsequent messages are newline-delimited plain
// text in the format "<type> [payload]".
//
// GUI → TUI message types:
//
//	{"type": "open", "path": "/path/to/file"}
//
// TUI → GUI message types:
//
//	{"type": "quit", "dialog": true}     — :q/:quit, show confirmation dialog
//	{"type": "quit", "dialog": false}    — :wq, quit immediately
package ipc

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// Message is a JSON message exchanged between TUI and GUI over IPC.
type Message struct {
	Type   string `json:"type"`
	Path   string `json:"path,omitempty"`
	Dialog *bool  `json:"dialog,omitempty"` // quit: true=show dialog, false=quit now
}

const secretLen = 32 // bytes for the random token

// Server listens on TCP localhost for a connection from the GUI wrapper.
// Messages from authenticated clients are delivered on the Incoming channel.
type Server struct {
	listener net.Listener
	conn     net.Conn
	mu       sync.Mutex

	// Incoming delivers messages received from the GUI (one per line).
	Incoming chan string

	// secret is a random hex token the client must send as its first
	// line after connecting. Stored for Close cleanup.
	secret string
}

// NewServer listens on 127.0.0.1:0 (random port), generates a random
// authentication token, and writes "<addr>\n<token>" to a port file
// that the GUI reads to discover the endpoint and authenticate.
func NewServer() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ipc listen: %w", err)
	}

	tokenBytes := make([]byte, secretLen)
	if _, err := rand.Read(tokenBytes); err != nil {
		listener.Close()
		return nil, fmt.Errorf("ipc generate token: %w", err)
	}
	secret := hex.EncodeToString(tokenBytes)

	addr := listener.Addr().String()
	// Write address + token to port file
	data := fmt.Sprintf("%s\n%s", addr, secret)
	if err := os.WriteFile(portFilePath(), []byte(data), 0600); err != nil {
		listener.Close()
		return nil, fmt.Errorf("ipc write port file: %w", err)
	}

	logger.Info("IPC server listening", "addr", addr)
	return &Server{
		listener: listener,
		Incoming: make(chan string, 8),
		secret:   secret,
	}, nil
}

// Accept blocks until a client connects. The first line from the client
// must match the authentication token; otherwise the connection is closed
// with an error. After successful authentication, a goroutine is started
// to read incoming messages.
func (s *Server) Accept() error {
	conn, err := s.listener.Accept()
	if err != nil {
		return fmt.Errorf("ipc accept: %w", err)
	}

	// Authenticate: first line must be the token
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		conn.Close()
		return fmt.Errorf("ipc auth: no token received from %s", conn.RemoteAddr())
	}
	line := strings.TrimSpace(scanner.Text())
	if line != s.secret {
		conn.Close()
		return fmt.Errorf("ipc auth: invalid token from %s", conn.RemoteAddr())
	}

	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()

	logger.Info("IPC client authenticated", "remote", conn.RemoteAddr())
	go s.readLoop(conn)
	return nil
}

// SendJSON marshals a Message to JSON and writes it to the authenticated
// client. Returns an error if no client is connected or the write fails.
func (s *Server) SendJSON(m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return fmt.Errorf("ipc: no client connected")
	}
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("ipc marshal: %w", err)
	}
	if _, err := fmt.Fprintf(s.conn, "%s\n", data); err != nil {
		logger.Warn("IPC send failed", "err", err)
		return err
	}
	return nil
}

// Send writes a raw string message (legacy, prefer SendJSON).
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

// portFilePath returns the temp file path where the TCP address and
// authentication token are stored.
func portFilePath() string {
	dir := os.TempDir()
	return filepath.Join(dir, fmt.Sprintf("neoviolet-ipc-%d", os.Getpid()))
}
