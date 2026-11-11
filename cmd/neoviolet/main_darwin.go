//go:build darwin

package main

import "runtime"

func init() {
	// AppKit must stay on the startup thread, including preference loading
	// before its event loop begins. Locking only inside MacOSRun is too late.
	runtime.LockOSThread()
}
