//go:build !darwin

package cmd

// runWithOSMedia is a no-op on non-macOS platforms.
func runWithOSMedia(_ bool, fn func() error) error {
	return fn()
}
