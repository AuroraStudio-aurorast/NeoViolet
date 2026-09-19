//go:build darwin

package cmd

import (
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
)

// runWithOSMedia wraps fn with NSApplication setup so that MPRemoteCommandCenter
// and MPNowPlayingInfoCenter work on macOS. On other platforms it's a no-op.
//
// OS media controls are an optional integration: when the ObjC runtime cannot be
// prepared the player still runs, only without Control Center / lock screen /
// media key support. The fallback calls fn directly, which is the same code path
// the non-darwin build takes and therefore needs no AppKit.
func runWithOSMedia(fn func() error) error {
	var err error
	if setupErr := mediactl.MacOSRun(func() { err = fn() }); setupErr != nil {
		logger.Warn("mediactl unavailable, continuing without OS media controls", "err", setupErr)
		return fn()
	}
	return err
}
