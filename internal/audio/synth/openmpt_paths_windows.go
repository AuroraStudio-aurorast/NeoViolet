//go:build windows

package synth

import (
	"syscall"

	"github.com/ebitengine/purego"
)

// openmptLibraryNames lists the DLL names libopenmpt is distributed under.
func openmptLibraryNames() []string {
	return []string{"libopenmpt-0.dll", "libopenmpt.dll"}
}

// openmptSearchDirs lists where the DLL is looked for: a bare file name for
// LoadLibrary's own search, which starts in the executable's directory, then
// that directory explicitly.
func openmptSearchDirs(exeDir string) []string {
	return []string{".", exeDir}
}

// openmptOpen loads one candidate. LoadLibrary is Windows' dlopen; purego's
// Dlopen does not exist here, so the handle comes straight from the system.
func openmptOpen(path string) (uintptr, error) {
	handle, err := syscall.LoadLibrary(path)
	if err != nil {
		return 0, err
	}
	return uintptr(handle), nil
}

// openmptSymbol resolves one symbol of a loaded DLL.
func openmptSymbol(handle uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(handle), name)
}

// openmptRegister binds one entry point of a loaded DLL. It is only called for
// symbols openmptSymbol has already resolved, so it cannot fail here.
func openmptRegister(target any, handle uintptr, name string) error {
	purego.RegisterLibFunc(target, handle, name)
	return nil
}

// openmptClose releases a handle openmptOpen returned.
func openmptClose(handle uintptr) error {
	return syscall.FreeLibrary(syscall.Handle(handle))
}
