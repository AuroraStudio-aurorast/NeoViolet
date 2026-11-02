//go:build !windows

package buildtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bundle = "packages/NeoViolet GUI.app"

func macFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, "tools/osx-appbundle-builder/build.sh", "tools/build/check-rust.sh")
	for _, name := range []string{"neoviolet", "neoviolet-gui", "apecli"} {
		f.write(t, "input/"+name, name+" fixture")
	}
	for _, name := range []string{"NeoViolet.icns", "Assets.car"} {
		f.write(t, "tools/neoviolet-gui/assets/pre-compiled/"+name, name)
	}
	f.write(t, "tools/neoviolet-gui/LICENSE", "GUI license fixture")
	f.write(t, "tools/apecli/LICENSE", "MIT license fixture")
	f.write(t, "docs/ACKNOWLEDGEMENTS.md", "acknowledgements fixture")
	f.tool(t, "uname", "echo Darwin\n")
	f.tool(t, "lipo", "[ \"$1\" = -verify_arch ]; [ \"$2\" = arm64 ]; [ \"$3\" = x86_64 ]; test -f \"$4\"\n")
	f.tool(t, "xcrun", "exit 1\n")
	f.tool(t, "codesign", "exit 0\n")
	return f
}

func packageMac(t *testing.T, f fixture) (string, error) {
	t.Helper()
	return f.run(t, "package/osxappbundle", "PACKAGE_INPUT_DIR="+filepath.Join(f.root, "input"),
		"PACKAGE_OUTPUT_DIR="+filepath.Join(f.root, "packages"), "VERSION=v1.2.3", "BUILD=42", "MACOS_MIN=14.0")
}

func TestMacPackageExistingArtifacts(t *testing.T) {
	t.Parallel()
	f := macFixture(t)
	output, err := packageMac(t, f)
	if err != nil {
		t.Fatalf("package: %v\n%s", err, output)
	}
	plist := f.read(t, bundle+"/Contents/Info.plist")
	for _, want := range []string{"<string>v1.2.3</string>", "<string>42</string>", "<string>14.0</string>"} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist missing %q", want)
		}
	}
	for _, name := range []string{"neoviolet", "neoviolet-gui", "apecli"} {
		if got := f.read(t, bundle+"/Contents/MacOS/"+name); got != name+" fixture" {
			t.Errorf("incorrect packaged %s: %q", name, got)
		}
	}
	for _, name := range []string{"LICENSE-GPL", "LICENSE-MIT", "ACKNOWLEDGEMENTS.md", "NeoViolet.icns", "Assets.car"} {
		_ = f.read(t, bundle+"/Contents/Resources/"+name)
	}
}

func TestMacPackageFailurePreservesBundle(t *testing.T) {
	for _, name := range []string{"missing-input", "wrong-architecture", "copy", "sign", "publish"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := macFixture(t)
			f.write(t, bundle+"/old-marker", "previous bundle")
			switch name {
			case "missing-input":
				if err := os.Remove(filepath.Join(f.root, "input/apecli")); err != nil {
					t.Fatal(err)
				}
			case "wrong-architecture":
				f.tool(t, "lipo", "echo missing-x86_64 >&2; exit 1\n")
			case "copy":
				f.tool(t, "cp", "exit 1\n")
			case "sign":
				f.tool(t, "codesign", "exit 1\n")
			case "publish":
				f.tool(t, "mv", "case \"$1\" in */new) exit 1 ;; esac\nexec /bin/mv \"$@\"\n")
			}
			output, err := packageMac(t, f)
			if err == nil {
				t.Fatalf("expected failure, got success:\n%s", output)
			}
			if got := f.read(t, bundle+"/old-marker"); got != "previous bundle" {
				t.Fatalf("old bundle changed: %q", got)
			}
		})
	}
}

func TestMacBuildThenPackage(t *testing.T) {
	t.Parallel()
	f := macFixture(t)
	f.tool(t, "go", `case "$1" in
 env) case "$2" in GOOS) echo darwin ;; GOARCH) echo arm64 ;; esac ;;
 build)
   while [ "$#" -gt 0 ]; do
     if [ "$1" = -o ]; then echo slice > "$2"; exit; fi
     shift
   done
   exit 90 ;;
 *) exit 90 ;;
esac
`)
	f.tool(t, "cargo", `name=${PWD##*/}
while [ "$#" -gt 0 ]; do
 if [ "$1" = --target ]; then target=$2; break; fi
 shift
done
mkdir -p "target/$target/release"
echo slice > "target/$target/release/$name"
`)
	f.tool(t, "rustc", "case \"$1\" in -vV) echo 'host: aarch64-apple-darwin' ;; --print) printf '%s/lib/%s\\n' \"$(pwd)\" \"$4\" ;; *) exit 90 ;; esac\n")
	for _, target := range []string{"aarch64-apple-darwin", "x86_64-apple-darwin"} {
		f.write(t, "lib/"+target+"/libstd-fixture.rlib", "stdlib")
	}
	f.tool(t, "lipo", `case "$1" in
 -create)
   test -f "$2"; test -f "$3"; test "$4" = -output
   echo universal > "$5" ;;
 -verify_arch) grep -q '^universal$' "$4" ;;
 *) exit 90 ;;
esac
`)
	output, err := f.run(t, "-j4", "build/osxappbundle", "VERSION=v2.3.4", "BUILD=43",
		"PACKAGE_OUTPUT_DIR="+filepath.Join(f.root, "packages"), "PACKAGE_INPUT_DIR="+filepath.Join(f.root, "input"))
	if err != nil {
		t.Fatalf("build then package must use freshly built binaries: %v\n%s", err, output)
	}
	if got := f.read(t, bundle+"/Contents/MacOS/neoviolet"); got != "universal\n" {
		t.Fatalf("wrong packaged build: %q", got)
	}
	if plist := f.read(t, bundle+"/Contents/Info.plist"); !strings.Contains(plist, "<string>v2.3.4</string>") {
		t.Fatalf("bundle lost the build version: %s", plist)
	}
}
