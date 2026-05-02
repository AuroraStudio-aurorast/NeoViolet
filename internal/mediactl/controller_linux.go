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
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("mediactl: connect session bus: %w", err)
	}
	c.conn = conn

	reply, err := conn.RequestName(mprisBusName, dbus.NameFlagDoNotQueue)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("mediactl: request name %s: %w", mprisBusName, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		conn.Close()
		return nil, fmt.Errorf("mediactl: name %s already taken", mprisBusName)
	}

	ch := make(chan Command, 8)
	c.cmdChan = ch

	rootObj := &mprisRoot{}
	playerObj := &mprisPlayerObj{ctrl: c}

	conn.Export(rootObj, mprisObjectPath, mprisRootIface)
	conn.Export(playerObj, mprisObjectPath, mprisPlayerIface)
	conn.Export(playerObj, mprisObjectPath, propsIface)

	return ch, nil
}

func (c *linuxController) Update(state PlayState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed || c.conn == nil {
		return
	}

	changed := make(map[string]dbus.Variant)
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

	if state.Position != c.state.Position {
		changed["Position"] = dbus.MakeVariant(int64(state.Position / time.Microsecond))
	}

	c.state = state

	if len(changed) > 0 {
		c.conn.Emit(mprisObjectPath, "org.freedesktop.DBus.Properties.PropertiesChanged",
			mprisPlayerIface, changed, []string{})
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
func (r *mprisRoot) DesktopEntry() string          { return "" }
func (r *mprisRoot) SupportedUriSchemes() []string { return []string{"file"} }
func (r *mprisRoot) SupportedMimeTypes() []string  { return nil }
func (r *mprisRoot) Quit() *dbus.Error             { return nil }
func (r *mprisRoot) Raise() *dbus.Error            { return nil }

type mprisPlayerObj struct {
	ctrl *linuxController
}

func (o *mprisPlayerObj) Next() *dbus.Error      { o.ctrl.cmdChan <- CmdNext; return nil }
func (o *mprisPlayerObj) Previous() *dbus.Error  { o.ctrl.cmdChan <- CmdPrev; return nil }
func (o *mprisPlayerObj) Pause() *dbus.Error     { o.ctrl.cmdChan <- CmdPause; return nil }
func (o *mprisPlayerObj) PlayPause() *dbus.Error { o.ctrl.cmdChan <- CmdPlayPause; return nil }
func (o *mprisPlayerObj) Stop() *dbus.Error      { o.ctrl.cmdChan <- CmdStop; return nil }
func (o *mprisPlayerObj) Play() *dbus.Error      { o.ctrl.cmdChan <- CmdPlay; return nil }
func (o *mprisPlayerObj) Seek(offset int64) *dbus.Error {
	o.ctrl.cmdChan <- CmdSeek
	return nil
}
func (o *mprisPlayerObj) SetPosition(trackID dbus.ObjectPath, pos int64) *dbus.Error {
	o.ctrl.cmdChan <- CmdSeek
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
