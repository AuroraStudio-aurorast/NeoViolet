//go:build !windows

package buildtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func windowsFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, "tools/windows-package/package.sh")
	for _, name := range []string{"neoviolet.exe", "neoviolet-gui.exe", "apecli.exe", "libopenmpt.dll"} {
		f.write(t, "input/"+name, name+" fixture\n")
	}
	f.write(t, "runtime/.keep", "")
	f.tool(t, "objdump", "test \"$1\" = -p\ncat \"$2\"\n")
	return f
}

func TestWindowsPackageDependencyWalk(t *testing.T) {
	t.Parallel()
	f := windowsFixture(t)
	f.write(t, "input/neoviolet.exe", "DLL Name: FIRST.DLL\nDLL Name: first.dll\nDLL Name: LIBOPENMPT.DLL\n")
	f.write(t, "input/neoviolet-gui.exe", "DLL Name: First.dll\n")
	f.write(t, "input/First.DlL", "DLL Name: SECOND.dll\n")
	f.write(t, "runtime with spaces/sEcOnD.DLL", "DLL Name: first.DLL\nDLL Name: third.dll\n")
	f.write(t, "runtime with spaces/THIRD.dll", "third fixture\n")
	f.write(t, "modules/versioned openmpt.dll", "DLL Name: OpenMptRuntime.dll\n")
	f.write(t, "runtime with spaces/OPENMPTRUNTIME.DLL", "DLL Name: KERNEL32.dll\n")
	output, err := packageWindows(t, f, "WINDOWS_DLL_DIR="+filepath.Join(f.root, "runtime with spaces"),
		"LIBOPENMPT_DLL="+filepath.Join(f.root, "modules/versioned openmpt.dll"))
	if err != nil {
		t.Fatalf("package: %v\n%s", err, output)
	}
	for _, pair := range [][2]string{
		{"first.dll", "input/First.DlL"},
		{"second.dll", "runtime with spaces/sEcOnD.DLL"},
		{"third.dll", "runtime with spaces/THIRD.dll"},
		{"libopenmpt.dll", "modules/versioned openmpt.dll"},
		{"openmptruntime.dll", "runtime with spaces/OPENMPTRUNTIME.DLL"},
	} {
		if got := f.read(t, "packages/windows/"+pair[0]); got != f.read(t, pair[1]) {
			t.Errorf("incorrect dependency %s: %q", pair[0], got)
		}
	}
	files, err := os.ReadDir(filepath.Join(f.root, "packages/windows"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 8 {
		t.Fatalf("expected three executables and five DLLs, got %d entries", len(files))
	}
}

func TestWindowsPackageSystemDLLs(t *testing.T) {
	t.Parallel()
	f := windowsFixture(t)
	var imports strings.Builder
	for _, name := range strings.Fields(`kernel32 ntdll user32 gdi32 advapi32 shell32 ole32 msvcrt
		vcruntime api-ms-win-core-synch-l1-2-0 ext-ms-win-ntuser-window-l1-1-0 ucrtbase ws2_32
		iphlpapi setupapi crypt32 bcrypt bcryptprimitives psapi winmm version combase rpcrt4 shlwapi
		oleaut32 imm32 msctf comctl32 uxtheme opengl32 d3d11 dcomp dwmapi dwrite dxgi icu icuuc icuin kernel.appcore
		windows.storage`) {
		imports.WriteString("\tDLL Name: " + strings.ToUpper(name) + ".DLL\r\n")
	}
	f.write(t, "input/neoviolet-gui.exe", imports.String())
	output, err := packageWindows(t, f)
	if err != nil {
		t.Fatalf("system DLLs must not need local copies: %v\n%s", err, output)
	}
}

func TestWindowsPackageImportLabelCase(t *testing.T) {
	for _, label := range []string{"DLL Name:", "DLL name:", "dll name:", "DLL NAME:"} {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			f := windowsFixture(t)
			f.write(t, "input/neoviolet.exe", "\t"+label+" LIBOPENMPT.dll\r\n\t"+label+" Runtime.DLL\r\n")
			f.write(t, "runtime/runtime.dll", "runtime fixture\n")
			output, err := packageWindows(t, f)
			if err != nil {
				t.Fatalf("import label %q: %v\n%s", label, err, output)
			}
			if got := f.read(t, "packages/windows/runtime.dll"); got != "runtime fixture\n" {
				t.Fatalf("incorrect packaged dependency: %q", got)
			}
		})
	}
}

func TestWindowsPackageReportsAllMissingDLLs(t *testing.T) {
	t.Parallel()
	f := windowsFixture(t)
	f.write(t, "packages/windows/old-marker", "previous distribution")
	f.write(t, "input/neoviolet.exe", "DLL Name: missing-first.dll\nDLL Name: present.dll\n")
	f.write(t, "input/neoviolet-gui.exe", "DLL Name: missing-second.dll\n")
	f.write(t, "input/libopenmpt.dll", "DLL Name: missing-runtime.dll\n")
	f.write(t, "runtime/present.dll", "DLL Name: missing-transitive.dll\n")
	output, err := packageWindows(t, f)
	if err == nil {
		t.Fatalf("missing dependencies must fail packaging:\n%s", output)
	}
	for _, name := range []string{"missing-first.dll", "missing-second.dll", "missing-runtime.dll", "missing-transitive.dll"} {
		if !strings.Contains(output, "missing non-system DLL: "+name+" (imported by ") {
			t.Errorf("missing dependency %s was not reported:\n%s", name, output)
		}
	}
	if got := f.read(t, "packages/windows/old-marker"); got != "previous distribution" {
		t.Fatalf("incomplete package replaced existing distribution: %q", got)
	}
}

func TestWindowsPackageFailurePreservesDistribution(t *testing.T) {
	for _, name := range []string{"missing-exe", "missing-openmpt", "missing-dll", "openmpt-import",
		"objdump", "copy", "publish"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := windowsFixture(t)
			f.write(t, "packages/windows/old-marker", "previous distribution")
			switch name {
			case "missing-exe":
				if err := os.Remove(filepath.Join(f.root, "input/apecli.exe")); err != nil {
					t.Fatal(err)
				}
			case "missing-openmpt":
				if err := os.Remove(filepath.Join(f.root, "input/libopenmpt.dll")); err != nil {
					t.Fatal(err)
				}
			case "missing-dll":
				f.write(t, "input/neoviolet-gui.exe", "DLL Name: unavailable.dll\n")
			case "openmpt-import":
				f.write(t, "input/libopenmpt.dll", "DLL Name: unavailable.dll\n")
			case "objdump":
				f.tool(t, "objdump", "echo bad-object >&2; exit 1\n")
			case "copy":
				f.tool(t, "cp", "exit 1\n")
			case "publish":
				f.tool(t, "mv", "case \"$1\" in */new) exit 1 ;; esac\nexec /bin/mv \"$@\"\n")
			}
			output, err := packageWindows(t, f)
			if err == nil {
				t.Fatalf("expected failure, got success:\n%s", output)
			}
			if got := f.read(t, "packages/windows/old-marker"); got != "previous distribution" {
				t.Fatalf("old distribution changed: %q", got)
			}
			leftovers, err := filepath.Glob(filepath.Join(f.root, "packages/.neoviolet-windows.*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("unexpected staging directories: %v, %v", leftovers, err)
			}
		})
	}
}

func TestWindowsPackageRejectsMalformedImports(t *testing.T) {
	for _, name := range []string{"../escape.dll", `..\escape.dll`, "/absolute.dll", "C:escape.dll",
		"bad name.dll", "-option.dll", "", "library.exe"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := windowsFixture(t)
			f.write(t, "input/neoviolet.exe", "DLL Name: "+name+"\n")
			output, err := packageWindows(t, f)
			if err == nil || !strings.Contains(output, "invalid DLL") {
				t.Fatalf("expected invalid DLL rejection, got %v:\n%s", err, output)
			}
			f.absent(t, "packages/windows")
		})
	}
}

func TestWindowsPackageRejectsUnsafeOutput(t *testing.T) {
	for _, name := range []string{"input", "ancestor", "dll-source", "alias", "symlink-parent"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := windowsFixture(t)
			out := filepath.Join(f.root, "input")
			switch name {
			case "ancestor":
				out = f.root
			case "dll-source":
				out = filepath.Join(f.root, "runtime")
			case "alias":
				out = filepath.Join(f.root, "alias")
				if err := os.Symlink(filepath.Join(f.root, "input"), out); err != nil {
					t.Fatal(err)
				}
			case "symlink-parent":
				if err := os.Symlink(f.root, filepath.Join(f.root, "alias")); err != nil {
					t.Fatal(err)
				}
				out = filepath.Join(f.root, "alias/input")
			}
			output, err := packageWindows(t, f, "PACKAGE_OUTPUT_DIR="+out)
			if err == nil {
				t.Fatalf("expected unsafe output rejection, got success:\n%s", output)
			}
			for _, source := range []string{"neoviolet.exe", "neoviolet-gui.exe", "apecli.exe", "libopenmpt.dll"} {
				if got := f.read(t, "input/"+source); got != source+" fixture\n" {
					t.Fatalf("source %s changed: %q", source, got)
				}
			}
		})
	}
}

func TestWindowsPackageReplacesChildDistribution(t *testing.T) {
	t.Parallel()
	f := windowsFixture(t)
	f.write(t, "input/dist/old-marker", "previous distribution")
	output, err := packageWindows(t, f, "PACKAGE_OUTPUT_DIR="+filepath.Join(f.root, "input/dist"))
	if err != nil {
		t.Fatalf("child distribution: %v\n%s", err, output)
	}
	f.absent(t, "input/dist/old-marker")
	if got := f.read(t, "input/dist/neoviolet.exe"); got != "neoviolet.exe fixture\n" {
		t.Fatalf("incorrect executable: %q", got)
	}
}

func TestWindowsPackageRejectsSymlinkOutput(t *testing.T) {
	for _, suffix := range []string{"", "/", "/.", "/./"} {
		t.Run("suffix="+suffix, func(t *testing.T) {
			t.Parallel()
			f := windowsFixture(t)
			f.write(t, "existing/old-marker", "previous distribution")
			if err := os.Symlink(filepath.Join(f.root, "existing"), filepath.Join(f.root, "output")); err != nil {
				t.Fatal(err)
			}
			output, err := packageWindows(t, f, "PACKAGE_OUTPUT_DIR="+filepath.Join(f.root, "output")+suffix)
			if err == nil || !strings.Contains(output, "symlink") {
				t.Fatalf("expected symlink rejection, got %v:\n%s", err, output)
			}
			if got := f.read(t, "existing/old-marker"); got != "previous distribution" {
				t.Fatalf("symlink target changed: %q", got)
			}
		})
	}
}

func TestWindowsPackageRejectsSourcesInsideOutput(t *testing.T) {
	for _, name := range []string{"exe", "dll", "openmpt"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := windowsFixture(t)
			f.write(t, "packages/windows/old-marker", "previous distribution")
			f.write(t, "packages/windows/source", "source fixture")
			link := "input/neoviolet.exe"
			args := []string{}
			switch name {
			case "dll":
				f.write(t, "input/neoviolet.exe", "DLL Name: linked.dll\n")
				link = "runtime/linked.dll"
			case "openmpt":
				link = "modules/custom.dll"
				f.write(t, link, "")
				args = append(args, "LIBOPENMPT_DLL="+filepath.Join(f.root, link))
			}
			if err := os.Remove(filepath.Join(f.root, link)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.root, "packages/windows/source"), filepath.Join(f.root, link)); err != nil {
				t.Fatal(err)
			}
			output, err := packageWindows(t, f, args...)
			if err == nil || !strings.Contains(output, "contains a source") {
				t.Fatalf("expected source protection, got %v:\n%s", err, output)
			}
			if got := f.read(t, "packages/windows/old-marker"); got != "previous distribution" {
				t.Fatalf("old distribution changed: %q", got)
			}
			if got := f.read(t, link); got != "source fixture" {
				t.Fatalf("linked source changed: %q", got)
			}
		})
	}
}

func TestWindowsPackageFailedRollbackKeepsBackup(t *testing.T) {
	t.Parallel()
	f := windowsFixture(t)
	f.write(t, "packages/windows/old-marker", "previous distribution")
	f.tool(t, "mv", "case \"$1\" in */new|*/old) exit 1 ;; esac\nexec /bin/mv \"$@\"\n")
	output, err := packageWindows(t, f)
	if err == nil || !strings.Contains(output, "previous distribution is at") {
		t.Fatalf("expected recoverable backup diagnostic, got %v:\n%s", err, output)
	}
	backups, err := filepath.Glob(filepath.Join(f.root, "packages/.neoviolet-windows.*/old/old-marker"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one recoverable backup: %v, %v", backups, err)
	}
	data, err := os.ReadFile(backups[0])
	if err != nil || string(data) != "previous distribution" {
		t.Fatalf("backup damaged: %q, %v", data, err)
	}
}

func packageWindows(t *testing.T, f fixture, args ...string) (string, error) {
	t.Helper()
	return f.run(t, append([]string{"package/windows", "PACKAGE_INPUT_DIR=" + filepath.Join(f.root, "input"),
		"PACKAGE_OUTPUT_DIR=" + filepath.Join(f.root, "packages/windows"),
		"WINDOWS_DLL_DIR=" + filepath.Join(f.root, "runtime")}, args...)...)
}

func TestWindowsPackageExistingArtifacts(t *testing.T) {
	t.Parallel()
	f := windowsFixture(t)
	output, err := packageWindows(t, f)
	if err != nil {
		t.Fatalf("package: %v\n%s", err, output)
	}
	for _, name := range []string{"neoviolet.exe", "neoviolet-gui.exe", "apecli.exe", "libopenmpt.dll"} {
		if got := f.read(t, "packages/windows/"+name); got != name+" fixture\n" {
			t.Errorf("incorrect packaged %s: %q", name, got)
		}
	}
}
