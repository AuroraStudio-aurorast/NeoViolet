//go:build !darwin && !linux && !windows

package synth

import (
	"errors"
)

// libopenmpt is loaded at run time and this binding only knows how to do that
// on darwin, linux and windows. Everywhere else the loader reports the library
// as unavailable rather than failing to build, which keeps the package — and
// with it the gotracker fallback — compiling on any GOOS.

var errOpenmptUnsupported = errors.New("libopenmpt runtime loading is not supported on this system")

// openmptLibraryNames reports no library name, because nothing is opened here.
func openmptLibraryNames() []string { return nil }

// openmptSearchDirs reports no directory, for the same reason.
func openmptSearchDirs(_ string) []string { return nil }

// openmptOpen reports that run time loading is unsupported on this system.
func openmptOpen(_ string) (uintptr, error) { return 0, errOpenmptUnsupported }

// openmptSymbol is unreachable, because no handle is ever produced here.
func openmptSymbol(_ uintptr, _ string) (uintptr, error) { return 0, errOpenmptUnsupported }

// openmptRegister is unreachable for the same reason.
func openmptRegister(_ any, _ uintptr, _ string) error { return errOpenmptUnsupported }

// openmptClose is unreachable too, and reports success so that a close can
// never replace the error that made the load fail.
func openmptClose(_ uintptr) error { return nil }
