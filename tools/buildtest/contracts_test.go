//go:build !windows

package buildtest

import (
	"strings"
	"testing"
)

func buildFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, "tools/build/check-rust.sh")
	f.write(t, "tools/apecli/Cargo.toml", "apecli fixture")
	f.write(t, "tools/neoviolet-gui/Cargo.toml", "GUI fixture")
	f.tool(t, "go", "case \"$1\" in\n env) case \"$2\" in GOOS) echo darwin ;; GOARCH) echo arm64 ;; esac ;;\n build) echo \"$*\" >> go-build ;;\n run) if [ \"$2\" = ./tools/gui-deps ]; then echo prepared > gui-prepared; else echo \"$*\" >> go-build; exit 90; fi ;;\n *) exit 90 ;;\nesac\n")
	f.tool(t, "cargo", "echo compile >> ../../cargo-build\nexit 9\n")
	f.tool(t, "rustc", "case \"$1\" in\n -vV) echo 'host: aarch64-apple-darwin' ;;\n --print) printf '%s/lib/%s\\n' \"$(pwd)\" \"$4\" ;;\n *) exit 90 ;;\nesac\n")
	return f
}

func TestBuildMissingRustPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  string
		strict  string
		wantErr bool
	}{
		{name: "optional", target: "build/tui", strict: "0"},
		{name: "required", target: "build/tui", strict: "1", wantErr: true},
		{name: "GUI-launch", target: "run/gui", strict: "0", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := buildFixture(t)
			output, err := f.run(t, "-j4", tc.target, "HAS_CARGO=0", "GOOS=linux", "GOARCH=amd64", "REQUIRE_RUST="+tc.strict)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, want error %v:\n%s", err, tc.wantErr, output)
			}
			if tc.wantErr {
				f.absent(t, "go-build")
			} else {
				if flags := f.read(t, "go-build"); !strings.Contains(flags, "-ldflags=-s -w -X") {
					t.Fatalf("production flags lost: %s", flags)
				}
			}
			f.absent(t, "cargo-build")
		})
	}
}

func TestStrictBuildPreflight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		os     string
	}{
		{name: "wrong-os", target: "x86_64-apple-darwin", os: "linux"},
		{name: "wrong-architecture", target: "aarch64-unknown-linux-gnu", os: "linux"},
		{name: "missing-standard-library", target: "x86_64-unknown-linux-gnu", os: "linux"},
		{name: "before-windows-resources", target: "aarch64-pc-windows-gnullvm", os: "windows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := buildFixture(t)
			output, err := f.run(t, "-j4", "build", "HAS_CARGO=1", "REQUIRE_RUST=1", "GOOS="+tc.os, "GOARCH=amd64", "RUST_TARGET="+tc.target)
			if err == nil {
				t.Fatalf("preflight should fail:\n%s", output)
			}
			f.absent(t, "go-build")
			f.absent(t, "cargo-build")
			f.absent(t, "gui-prepared")
		})
	}
}

func TestStrictBuildCopiesRustArtifacts(t *testing.T) {
	t.Parallel()
	f := buildFixture(t)
	f.write(t, "lib/x86_64-unknown-linux-gnu/libstd-fixture.rlib", "stdlib")
	f.tool(t, "cargo", "name=${PWD##*/}\nmkdir -p \"target/$CARGO_BUILD_TARGET/release\"\nprintf '%s' \"$name\" > \"target/$CARGO_BUILD_TARGET/release/$name\"\n")
	output, err := f.run(t, "-j4", "build", "HAS_CARGO=1", "REQUIRE_RUST=1", "GOOS=linux", "GOARCH=amd64", "CARGO_BUILD_TARGET=x86_64-unknown-linux-gnu")
	if err != nil {
		t.Fatalf("strict build: %v\n%s", err, output)
	}
	for _, name := range []string{"apecli", "neoviolet-gui"} {
		if got := f.read(t, name); got != name {
			t.Errorf("wrong %s release artifact: %q", name, got)
		}
	}
	_ = f.read(t, "go-build")
	_ = f.read(t, "gui-prepared")
}

func TestRustPreflightNormalizesWindowsLibraryPath(t *testing.T) {
	t.Parallel()
	f := buildFixture(t)
	f.write(t, "lib/x86_64-pc-windows-gnullvm/libstd-fixture.rlib", "stdlib")
	f.tool(t, "rustc", "printf '%s\\n' 'C:\\Rust\\lib'\n")
	f.tool(t, "cygpath", "test \"$1\" = -u\ntest \"$2\" = 'C:\\Rust\\lib'\nprintf '%s/lib/x86_64-pc-windows-gnullvm\\n' \"$(pwd)\"\n")
	output, err := f.run(t, "check/rust", "HAS_CARGO=1", "GOOS=windows", "GOARCH=amd64", "CARGO_BUILD_TARGET=x86_64-pc-windows-gnullvm")
	if err != nil {
		t.Fatalf("native Windows library path must work in MSYS2: %v\n%s", err, output)
	}
	f.absent(t, "go-build")
	f.absent(t, "cargo-build")
}

func TestRustFailureDoesNotLaunchOldGUI(t *testing.T) {
	for _, name := range []string{"compiler", "copy"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := buildFixture(t)
			if name == "copy" {
				f.tool(t, "cargo", "exit 0\n")
			}
			f.executable(t, "neoviolet-gui", "#!/bin/sh\necho launched > launched\n")
			output, err := f.run(t, "-j4", "run/gui", "HAS_CARGO=1", "GOOS=linux", "GOARCH=amd64")
			if err == nil {
				t.Fatalf("Rust %s failure should stop GUI launch:\n%s", name, output)
			}
			f.absent(t, "launched")
		})
	}
}

func TestInfoDoesNotBuild(t *testing.T) {
	t.Parallel()
	f := buildFixture(t)
	output, err := f.run(t, "info", "VERSION=v7.8.9", "GOOS=linux", "GOARCH=amd64", "HAS_CARGO=0")
	if err != nil {
		t.Fatalf("info: %v\n%s", err, output)
	}
	for _, want := range []string{"v7.8.9", "linux", "amd64", "neoviolet"} {
		if !strings.Contains(output, want) {
			t.Errorf("info missing %q:\n%s", want, output)
		}
	}
	f.absent(t, "go-build")
	f.absent(t, "cargo-build")
	f.absent(t, ".build")
}

func TestFailedBuildDoesNotLaunch(t *testing.T) {
	t.Parallel()
	f := buildFixture(t)
	f.tool(t, "go", "exit 9\n")
	f.executable(t, "neoviolet", "#!/bin/sh\necho launched > launched\n")
	output, err := f.run(t, "run", "HAS_CARGO=0", "GOOS=linux", "GOARCH=amd64")
	if err == nil {
		t.Fatalf("failed compilation should stop launch:\n%s", output)
	}
	f.absent(t, "launched")
}

func TestUniversalBuildChecksHost(t *testing.T) {
	t.Parallel()
	f := buildFixture(t)
	f.tool(t, "uname", "echo Linux\n")
	f.write(t, "lib/x86_64-apple-darwin/libstd-fixture.rlib", "stdlib")
	f.write(t, "lib/aarch64-apple-darwin/libstd-fixture.rlib", "stdlib")
	output, err := f.run(t, "build/macos", "HAS_CARGO=1", "GOOS=darwin", "GOARCH=arm64")
	if err == nil {
		t.Fatalf("universal build needs macOS host:\n%s", output)
	}
	f.absent(t, "go-build")
	f.absent(t, "cargo-build")
}
