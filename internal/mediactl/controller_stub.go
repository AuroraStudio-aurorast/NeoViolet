//go:build !linux && !darwin

package mediactl

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
