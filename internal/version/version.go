// Package version holds build-time version info injected via ldflags.
package version

// Version is the application version, set via ldflags at build time.
// Defaults to "dev" when built without ldflags.
var Version = "dev"

// UserAgent returns the LRCLIB-compliant User-Agent string.
func UserAgent() string {
	return "NEOVIOLET v" + Version + " (https://github.com/AuroraStudio-aurorast/NeoViolet)"
}
