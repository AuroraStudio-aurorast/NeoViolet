//go:build darwin

package cmd

import "github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"

// runWithOSMedia wraps fn with NSApplication setup so that MPRemoteCommandCenter
// and MPNowPlayingInfoCenter work on macOS. On other platforms it's a no-op.
func runWithOSMedia(fn func() error) error {
	var err error
	mediactl.MacOSRun(func() {
		err = fn()
	})
	return err
}
