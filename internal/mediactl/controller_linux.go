//go:build linux

package mediactl

import (
	"fmt"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	mprisBusName     = "org.mpris.MediaPlayer2.neoviolet"
	mprisObjectPath  = dbus.ObjectPath("/org/mpris/MediaPlayer2")
	mprisRootIface   = "org.mpris.MediaPlayer2"
	mprisPlayerIface = "org.mpris.MediaPlayer2.Player"
	propsIface       = "org.freedesktop.DBus.Properties"
)

type linuxController struct {
	mu       sync.Mutex
	conn     *dbus.Conn
	cmdChan  chan Command
	state    PlayState
	trackID  string
	trackSeq int
	done     chan struct{}
	closed   bool
}

func newController() (Controller, error) {
	return &linuxController{
		trackID: "/neoviolet/track/0",
		done:    make(chan struct{}),
	}, nil
}

func (c *linuxController) Start() (<-chan Command, error) {
	if err := c.connect(); err != nil {
		return nil, err
	}

	ch := make(chan Command, 8)
	c.cmdChan = ch

	c.startReconnector()

	return ch, nil
}

func (c *linuxController) connect() error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("mediactl: connect session bus: %w", err)
	}

	reply, err := conn.RequestName(mprisBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return fmt.Errorf("mediactl: request name %s: %w", mprisBusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return fmt.Errorf("mediactl: name %s already taken", mprisBusName)
	}

	rootObj := &mprisRoot{}
	playerObj := &mprisPlayerObj{ctrl: c}

	conn.Export(rootObj, mprisObjectPath, mprisRootIface)
	conn.Export(playerObj, mprisObjectPath, mprisPlayerIface)
	conn.Export(playerObj, mprisObjectPath, propsIface)

	c.conn = conn
	return nil
}

func (c *linuxController) startReconnector() {
	go func() {
		sigCh := make(chan *dbus.Signal, 8)
		c.conn.Signal(sigCh)

		for {
			select {
			case <-c.done:
				c.conn.RemoveSignal(sigCh)
				return
			case sig := <-sigCh:
				if sig.Name == "org.freedesktop.DBus.Local.Disconnected" {
					c.mu.Lock()
					c.conn = nil
					c.mu.Unlock()
					c.reconnectLoop()
					return
				}
			}
		}
	}()
}

func (c *linuxController) reconnectLoop() {
	backoff := 2 * time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-c.done:
			return
		case <-time.After(backoff):
			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				return
			}
			c.mu.Unlock()

			if err := c.connect(); err == nil {
				// Re-export succeeded; restart the signal watcher
				c.mu.Lock()
				c.startReconnector()
				c.mu.Unlock()
				return
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (c *linuxController) Update(state PlayState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.conn == nil {
		return
	}

	changed := make(map[string]dbus.Variant)
	seekedSignal := false
	sameTrack := c.state.Title == state.Title && c.state.Artist == state.Artist

	if !sameTrack {
		c.trackSeq++
		c.trackID = fmt.Sprintf("/neoviolet/track/%d", c.trackSeq)
	}

	if !sameTrack || c.state.Playing != state.Playing {
		changed["PlaybackStatus"] = dbus.MakeVariant(playbackStatus(state.Playing))
	}

	if !sameTrack {
		changed["Metadata"] = dbus.MakeVariant(buildMetadata(state, c.trackID))
	}

	// Emit Seeked signal when position changes by a meaningful amount
	// (more than 100ms) from an external source (not MPRIS-initiated seek).
	posChanged := state.Position != c.state.Position
	if posChanged {
		posDelta := state.Position - c.state.Position
		if posDelta < 0 {
			posDelta = -posDelta
		}
		if posDelta > 100*time.Millisecond {
			seekedSignal = true
		}
		changed["Position"] = dbus.MakeVariant(int64(state.Position / time.Microsecond))
	}

	c.state = state

	// Emit PropertiesChanged for standard property updates
	if len(changed) > 0 {
		c.conn.Emit(mprisObjectPath, "org.freedesktop.DBus.Properties.PropertiesChanged",
			mprisPlayerIface, changed, []string{})
	}

	// Emit Seeked signal per MPRIS spec — required when position changes
	// from non-MPRIS sources (keyboard seek, next-track, etc.)
	if seekedSignal {
		c.conn.Emit(mprisObjectPath, "org.mpris.MediaPlayer2.Player.Seeked",
			int64(state.Position/time.Microsecond))
	}
}

func (c *linuxController) Close() error {
	c.mu.Lock()
	wasClosed := c.closed
	c.closed = true
	c.mu.Unlock()

	if wasClosed {
		return nil
	}

	close(c.done)

	if c.conn != nil {
		c.conn.Close()
	}
	if c.cmdChan != nil {
		close(c.cmdChan)
	}
	return nil
}

type mprisRoot struct{}

func (r *mprisRoot) CanQuit() bool                 { return true }
func (r *mprisRoot) CanRaise() bool                { return false }
func (r *mprisRoot) HasTrackList() bool            { return false }
func (r *mprisRoot) Identity() string              { return "NeoViolet" }
func (r *mprisRoot) DesktopEntry() string          { return "neoviolet" }
func (r *mprisRoot) SupportedUriSchemes() []string { return []string{"file"} }
func (r *mprisRoot) SupportedMimeTypes() []string  { return nil }
func (r *mprisRoot) Quit() *dbus.Error             { return nil }
func (r *mprisRoot) Raise() *dbus.Error            { return nil }

type mprisPlayerObj struct {
	ctrl *linuxController
}

func (o *mprisPlayerObj) Next() *dbus.Error      { o.ctrl.cmdChan <- Command{Type: CmdNext}; return nil }
func (o *mprisPlayerObj) Previous() *dbus.Error  { o.ctrl.cmdChan <- Command{Type: CmdPrev}; return nil }
func (o *mprisPlayerObj) Pause() *dbus.Error     { o.ctrl.cmdChan <- Command{Type: CmdPause}; return nil }
func (o *mprisPlayerObj) PlayPause() *dbus.Error { o.ctrl.cmdChan <- Command{Type: CmdPlayPause}; return nil }
func (o *mprisPlayerObj) Stop() *dbus.Error      { o.ctrl.cmdChan <- Command{Type: CmdStop}; return nil }
func (o *mprisPlayerObj) Play() *dbus.Error      { o.ctrl.cmdChan <- Command{Type: CmdPlay}; return nil }
func (o *mprisPlayerObj) Seek(offset int64) *dbus.Error {
	o.ctrl.cmdChan <- Command{Type: CmdSeek, Value: offset}
	return nil
}
func (o *mprisPlayerObj) SetPosition(trackID dbus.ObjectPath, pos int64) *dbus.Error {
	o.ctrl.cmdChan <- Command{Type: CmdSetPosition, Value: pos}
	return nil
}
func (o *mprisPlayerObj) OpenUri(uri string) *dbus.Error { return nil }

func errDBus(msg string) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs", []any{msg})
}

func errReadOnly(prop string) *dbus.Error {
	return dbus.NewError("org.freedesktop.DBus.Error.PropertyReadOnly", []any{prop})
}

func (o *mprisPlayerObj) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	if iface != mprisPlayerIface {
		return dbus.Variant{}, errDBus("unknown interface")
	}
	o.ctrl.mu.Lock()
	s := o.ctrl.state
	tid := o.ctrl.trackID
	o.ctrl.mu.Unlock()

	p := playerProps(s, tid)
	if v, ok := p[prop]; ok {
		return v, nil
	}
	return dbus.Variant{}, errDBus("unknown property")
}

func (o *mprisPlayerObj) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	if iface != mprisPlayerIface {
		return nil, errDBus("unknown interface")
	}
	o.ctrl.mu.Lock()
	s := o.ctrl.state
	tid := o.ctrl.trackID
	o.ctrl.mu.Unlock()

	return playerProps(s, tid), nil
}

func (o *mprisPlayerObj) Set(iface, prop string, val dbus.Variant) *dbus.Error {
	return errReadOnly(prop)
}

var _ Controller = (*linuxController)(nil)
