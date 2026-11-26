//go:build darwin

package main

import "runtime"

func init() {
	// AppKit must run on the process's original thread. Lock during initialization,
	// before startup I/O can move the main goroutine to another OS thread.
	runtime.LockOSThread()
}
