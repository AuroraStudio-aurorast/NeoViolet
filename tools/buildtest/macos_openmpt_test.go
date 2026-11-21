//go:build !windows

package buildtest

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

const openmptAsset = "libopenmpt-1.2.3-macos13.0-universal.dylib"

func openmptFetchFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t, "tools/osx-appbundle-builder/fetch-openmpt.sh")
	prepareOpenmptDownload(t, f)
	return f
}

func prepareOpenmptDownload(t *testing.T, f fixture) {
	t.Helper()
	f.write(t, "deps/libopenmpt.lock", "LIBOPENMPT_VERSION=1.2.3\nMACOS_MIN=13.0\n")
	f.tool(t, "uname", "echo Darwin\n")
	f.tool(t, "gh", `test "$1 $2 $3" = 'release download deps'
printf '%s\n' "$*" >> fetch-calls
while [ "$#" -gt 0 ]; do
 if [ "$1" = --dir ]; then destination=$2; break; fi
 shift
done
cp download/* "$destination/"
`)
	contents := "prebuilt universal library"
	f.write(t, "download/"+openmptAsset, contents)
	f.write(t, "download/"+openmptAsset+".sha256", fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(contents)), openmptAsset))
}

func TestMacOpenmptFetchAndCache(t *testing.T) {
	t.Parallel()
	f := openmptFetchFixture(t)
	output, err := f.run(t, "deps/openmpt/macos", "MACOS_MIN=14.0")
	if err != nil {
		t.Fatalf("fetch: %v\n%s", err, output)
	}
	cache := ".build/macos-deps/" + openmptAsset
	if got := f.read(t, cache); got != "prebuilt universal library" {
		t.Fatalf("incorrect downloaded library: %q", got)
	}
	if calls := f.read(t, "fetch-calls"); !strings.Contains(calls, "--pattern "+openmptAsset) {
		t.Fatalf("fetch must use locked dependency floor, independent of app override: %s", calls)
	}
	f.tool(t, "gh", "echo unexpected-network-access >&2; exit 1\n")
	if output, err := f.run(t, "deps/openmpt/macos"); err != nil {
		t.Fatalf("verified cache should work offline: %v\n%s", err, output)
	}
}

func TestMacOpenmptRejectsBadChecksum(t *testing.T) {
	for _, name := range []string{"download", "cached-library"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := openmptFetchFixture(t)
			if name == "cached-library" {
				if output, err := f.run(t, "deps/openmpt/macos"); err != nil {
					t.Fatalf("initial fetch: %v\n%s", err, output)
				}
				f.write(t, ".build/macos-deps/"+openmptAsset, "corrupt cache")
				f.tool(t, "gh", "exit 1\n")
			} else {
				f.write(t, "download/"+openmptAsset, "corrupt download")
			}
			output, err := f.run(t, "deps/openmpt/macos")
			if err == nil {
				t.Fatalf("must reject corrupt %s:\n%s", name, output)
			}
			if name == "download" {
				f.absent(t, ".build/macos-deps/"+openmptAsset)
			}
		})
	}
}
