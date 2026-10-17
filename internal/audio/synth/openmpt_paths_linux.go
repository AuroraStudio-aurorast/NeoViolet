//go:build linux

package synth

import (
	"github.com/ebitengine/purego"
)

// openmptLibraryNames lists the sonames libopenmpt is installed under.
func openmptLibraryNames() []string {
	return []string{"libopenmpt.so.0", "libopenmpt.so"}
}

// openmptSearchDirs lists where the library is looked for: a bare soname for
// the loader's own search (the ldconfig cache and LD_LIBRARY_PATH), then the
// usual library directories named explicitly. The executable's directory is not
// searched, because a Linux install is reached through the loader and this
// project ships no bundled library there.
func openmptSearchDirs(_ string) []string {
	return []string{".", "/usr/lib", "/usr/lib64", "/usr/local/lib"}
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
