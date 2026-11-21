//go:build !windows

package buildtest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func resourceFixture(t *testing.T) (fixture, string) {
	t.Helper()
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, "tools/windows-resources/main.go", "tools/windows-resources/compile.sh")
	f.write(t, "go.mod", "module fixture\n\ngo 1.26.1\n")
	f.write(t, "assets/icons/neoviolet.rc", "1 ICON \"assets/icons/neoviolet.ico\"\n")
	f.write(t, "assets/icons/neoviolet.ico", "icon fixture")
	f.write(t, "tools/apecli/Cargo.toml", "[package]\nversion = \"0.2.1\"\n")
	f.tool(t, "llvm-rc", "while [ \"$#\" -gt 0 ]; do\n if [ \"$1\" = -FO ]; then echo resource > \"$2\"; break; fi\n shift\ndone\necho compiler-noise\n")
	f.tool(t, "llvm-cvtres", "for arg do\n case \"$arg\" in -out:*) printf '%s\\n' \"$*\" > \"${arg#-out:}\" ;; esac\ndone\n")
	f.tool(t, "windres", "while [ \"$#\" -gt 0 ]; do\n if [ \"$1\" = -o ]; then echo windres > \"$2\"; break; fi\n shift\ndone\n")
	return f, "GO=" + goTool
}

func resourceArgs(f fixture, goTool, target, arch string) []string {
	return []string{target, goTool, "GOOS=windows", "GOARCH=" + arch, "VERSION=v1.2.3", "LLVM_RC=" + filepath.Join(f.bin, "llvm-rc"),
		"LLVM_CVTRES=" + filepath.Join(f.bin, "llvm-cvtres"), "WINDRES=" + filepath.Join(f.bin, "windres")}
}

func TestWindowsResourcesCompileAndCache(t *testing.T) {
	t.Parallel()
	f, goTool := resourceFixture(t)
	args := resourceArgs(f, goTool, "windows-resource/tui-debug", "arm64")
	if output, err := f.run(t, args...); err != nil {
		t.Fatalf("resource build: %v\n%s", err, output)
	}
	path := strings.TrimSpace(f.read(t, ".build/windows/tui_arm64_debug.path"))
	if !filepath.IsAbs(path) {
		t.Fatalf("link path is not absolute: %q", path)
	}
	object := f.read(t, strings.TrimPrefix(path, f.root+string(filepath.Separator)))
	if !strings.Contains(object, "-machine:ARM64") || !strings.Contains(object, "-timestamp:0") {
		t.Fatalf("incorrect resource compilation: %s", object)
	}
	f.tool(t, "llvm-rc", "echo cache-missed >&2; exit 1\n")
	if output, err := f.run(t, args...); err != nil {
		t.Fatalf("cached resource should not need compiler: %v\n%s", err, output)
	}
}

func TestWindowsResourcesFallbackAndFailure(t *testing.T) {
	for _, name := range []string{"windres", "arm64-needs-llvm", "compiler-failure"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f, goTool := resourceFixture(t)
			arch := "amd64"
			if name == "arm64-needs-llvm" {
				arch = "arm64"
			}
			args := resourceArgs(f, goTool, "windows-resource/gui-release", arch)
			if name == "compiler-failure" {
				f.tool(t, "llvm-cvtres", "exit 1\n")
			} else {
				args = append(args, "LLVM_RC="+filepath.Join(f.bin, "absent-llvm-rc"))
			}
			output, err := f.run(t, args...)
			pathFile := ".build/windows/gui_" + arch + "_release.path"
			if name == "windres" {
				if err != nil {
					t.Fatalf("windres fallback: %v\n%s", err, output)
				}
				path := strings.TrimSpace(f.read(t, pathFile))
				if got := f.read(t, strings.TrimPrefix(path, f.root+string(filepath.Separator))); got != "windres\n" {
					t.Fatalf("incorrect fallback object: %q", got)
				}
			} else {
				if err == nil {
					t.Fatalf("expected resource compilation failure:\n%s", output)
				}
				f.absent(t, pathFile)
			}
		})
	}
}

func TestWindowsResourcesConvertsLinkPath(t *testing.T) {
	t.Parallel()
	f, goTool := resourceFixture(t)
	f.tool(t, "cygpath", "test \"$1\" = -m\ntest -f \"$2\"\nprintf '%s\\n' 'C:/Resource Objects/resource.o'\n")
	if output, err := f.run(t, resourceArgs(f, goTool, "windows-resource/tui-release", "amd64")...); err != nil {
		t.Fatalf("resource path conversion: %v\n%s", err, output)
	}
	if got := f.read(t, ".build/windows/tui_amd64_release.path"); got != "C:/Resource Objects/resource.o\n" {
		t.Fatalf("wrong linker path: %q", got)
	}
}

func TestWindowsResourcesFailureKeepsPreviousPath(t *testing.T) {
	t.Parallel()
	f, goTool := resourceFixture(t)
	f.write(t, ".build/windows/tui_amd64_release.path", "previous-resource.o\n")
	f.tool(t, "llvm-rc", "exit 1\n")
	output, err := f.run(t, resourceArgs(f, goTool, "windows-resource/tui-release", "amd64")...)
	if err == nil {
		t.Fatalf("resource failure should propagate:\n%s", output)
	}
	if got := f.read(t, ".build/windows/tui_amd64_release.path"); got != "previous-resource.o\n" {
		t.Fatalf("previous link path changed after failure: %q", got)
	}
}
