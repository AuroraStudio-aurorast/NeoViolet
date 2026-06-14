//go:build !linux && !darwin

package mediactl

import "errors"

type stubController struct {
	cmdChan chan Command
}

func newController() (Controller, error) {
	return &stubController{}, nil
}

func (s *stubController) Start() (<-chan Command, error) {
	s.cmdChan = make(chan Command)
	// No-op platform: no commands will ever be sent.
	return s.cmdChan, nil
}

func (s *stubController) Update(PlayState) {}

func (s *stubController) Close() error {
	if s.cmdChan != nil {
		close(s.cmdChan)
	}
	return nil
}

var _ Controller = (*stubController)(nil)

// EnsureNotImplemented panics — used for build verification on platforms
// without native media controls (neither MPRIS/D-Bus nor MediaPlayer).
func EnsureNotImplemented() {
	panic(errors.New("mediactl: this platform has no native media control backend"))
}
