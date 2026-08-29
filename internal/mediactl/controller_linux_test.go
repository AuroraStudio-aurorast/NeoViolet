//go:build linux

package mediactl

import "testing"

// TestStubController exercises the full D-Bus lifecycle: name acquisition,
// export, Update and Close. It requires a session bus (Linux CI).
func TestStubController(t *testing.T) {
	c, err := newController()
	if err != nil {
		t.Fatalf("newController() error: %v", err)
	}

	ch, err := c.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	c.Update(PlayState{Title: "test"})

	if err := c.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after Close()")
	}
}
