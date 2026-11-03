//go:build !windows

package buildtest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	root string
	bin  string
}

func newFixture(t *testing.T, scripts ...string) fixture {
	t.Helper()
	temp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{root: filepath.Join(temp, "project with spaces")}
	f.bin = filepath.Join(f.root, "test-bin")
	for _, name := range append([]string{"Makefile", "deps/libopenmpt.lock"}, scripts...) {
		data, err := os.ReadFile(filepath.Join("..", "..", name)) // #nosec G304 -- test-owned repository fixture paths.
		if err != nil {
			t.Fatal(err)
		}
		f.write(t, name, string(data))
	}
	f.tool(t, "go", "case \"$*\" in\n 'env GOOS') echo darwin ;;\n 'env GOARCH') echo arm64 ;;\n *) echo unexpected-go-call >&2; exit 90 ;;\nesac\n")
	f.tool(t, "cargo", "echo unexpected-cargo-call >&2; exit 90\n")
	return f
}

func (f fixture) write(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join(f.root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { // #nosec G703 -- test-owned temporary fixture paths.
		t.Fatal(err)
	}
}

func (f fixture) tool(t *testing.T, name, body string) {
	t.Helper()
	f.executable(t, filepath.Join("test-bin", name), "#!/bin/sh\nset -eu\n"+body)
}

func (f fixture) executable(t *testing.T, name, body string) {
	t.Helper()
	f.write(t, name, body)
	if err := os.Chmod(filepath.Join(f.root, name), 0o700); err != nil { // #nosec G302 -- owner-executable test fixtures.
		t.Fatal(err)
	}
}

func (f fixture) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "make", append([]string{"--no-print-directory"}, args...)...) // #nosec G204 -- test-owned commands and arguments.
	cmd.Dir = f.root
	// Do not inherit a parent make's jobserver, CLI overrides or target environment.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "MAKEFLAGS", "MFLAGS", "MAKELEVEL", "MAKEOVERRIDES", "GO", "GOOS", "GOARCH", "CARGO_BUILD_TARGET", "VERSION", "BUILD", "MACOS_MIN", "REQUIRE_RUST", "OS", "PATH":
			continue
		case "PACKAGE_INPUT_DIR", "PACKAGE_OUTPUT_DIR", "WINDOWS_DLL_DIR", "LIBOPENMPT_DLL", "OBJDUMP", "SIGN_IDENTITY", "LLVM_RC", "LLVM_CVTRES", "WINDRES", "RUST_TARGET", "HAS_CARGO":
			continue
		case "LIBOPENMPT_DYLIB":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "PATH="+f.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (f fixture) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, name)) // #nosec G304 -- test-owned temporary fixture paths.
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f fixture) absent(t *testing.T, name string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(f.root, name)); !os.IsNotExist(err) {
		t.Fatalf("%s should not exist, stat error: %v", name, err)
	}
}
