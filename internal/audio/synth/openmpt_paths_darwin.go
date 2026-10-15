//go:build darwin

package synth

import (
	"path/filepath"

	"github.com/ebitengine/purego"
)

// openmptLibraryNames lists the file names libopenmpt is published under on
// macOS: the versioned dylib and the unversioned link Homebrew and MacPorts
// install beside it.
func openmptLibraryNames() []string {
	return []string{"libopenmpt.0.dylib", "libopenmpt.dylib"}
}

// openmptSearchDirs lists where the dylib is looked for, in order: the
// executable's own directory, the Frameworks directory of the bundle it lives
// in, then the Homebrew (both prefixes) and MacPorts locations. /opt/homebrew/lib
// is not on dyld's default search path, which is why the directories are named
// rather than left to the loader.
func openmptSearchDirs(exeDir string) []string {
	return []string{
		exeDir,
		filepath.Join(exeDir, "..", "Frameworks"),
		"/opt/homebrew/lib",
		"/opt/homebrew/opt/libopenmpt/lib",
		"/usr/local/lib",
		"/usr/local/opt/libopenmpt/lib",
		"/opt/local/lib",
	}
}

// openmptOpen loads one candidate. RTLD_NOW resolves its symbols up front, so a
// broken library fails here instead of at the first call.
func openmptOpen(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

// openmptSymbol resolves one symbol of a loaded library.
func openmptSymbol(handle uintptr, name string) (uintptr, error) {
	return purego.Dlsym(handle, name)
}

// openmptRegister binds one entry point of a loaded library. It is only called
// for symbols openmptSymbol has already resolved, so it cannot fail here.
func openmptRegister(target any, handle uintptr, name string) error {
	purego.RegisterLibFunc(target, handle, name)
	return nil
}

// openmptClose releases a handle openmptOpen returned. The loader reference
// counts, so the library is only unmapped once every load of it is closed.
func openmptClose(handle uintptr) error {
	return purego.Dlclose(handle)
}
