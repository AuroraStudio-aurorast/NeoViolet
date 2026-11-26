//go:build darwin

package main

import (
	"os"
	"runtime"
	"testing"

	"github.com/ebitengine/purego"
)

var startupOnMainThread = true

// TestMain runs on the goroutine that will call main in the shipped executable.
// A normal test runs in a new goroutine and cannot verify entrypoint affinity.
func TestMain(m *testing.M) {
	handle, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		panic(err)
	}
	var isMainThread func() int32
	purego.RegisterLibFunc(&isMainThread, handle, "pthread_main_np")
	for range 1024 {
		// Startup does I/O and can yield before reaching NSApplication.run.
		runtime.Gosched()
		if isMainThread() != 1 {
			startupOnMainThread = false
			break
		}
	}
	if err := purego.Dlclose(handle); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestStartupRemainsOnMacOSMainThread(t *testing.T) {
	if !startupOnMainThread {
		t.Fatal("macOS entrypoint migrated off the process main thread before AppKit startup")
	}
}
