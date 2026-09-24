package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkTree builds a small directory to complete inside:
//
//	root/
//	  Album/            (directory)
//	  b.mp3             (supported)
//	  a.flac            (supported)
//	  c.txt             (not supported)
//	  d.mid             (synthetic: MIDI/tracker, no registered decoder)
//	  .hidden.mp3       (supported but hidden)
func mkTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Album"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.mp3", "a.flac", "c.txt", "d.mid", ".hidden.mp3"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestPathCandidatesFiltersAndOrders(t *testing.T) {
	root := mkTree(t)
	got := value(t, candidatesFor(completionContextAt("open "+root+"/", len("open "+root+"/"))))

	// d.mid is the regression guard for the synthetic half of playableExt:
	// format.IsSupportedExt(".mid") is false, so it is listed only when
	// audio.IsSyntheticFormat is consulted as well.
	want := []string{root + "/Album/", root + "/a.flac", root + "/b.mp3", root + "/d.mid"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want %v (dirs first, .txt filtered, dotfiles hidden, synthetic kept)", got, want)
	}
}

func TestPathCandidatesPrefixInsideDirectory(t *testing.T) {
	root := mkTree(t)
	prefix := root + "/A"
	got := value(t, candidatesFor(completionContextAt("open "+prefix, len("open "+prefix))))
	if len(got) != 1 || got[0] != root+"/Album/" {
		t.Errorf("candidates = %v, want [%s/Album/]", got, root)
	}
}

func TestPathCandidatesDotfilesShownWhenTyped(t *testing.T) {
	root := mkTree(t)
	prefix := root + "/."
	got := value(t, candidatesFor(completionContextAt("open "+prefix, len("open "+prefix))))
	if len(got) != 1 || got[0] != root+"/.hidden.mp3" {
		t.Errorf("candidates = %v, want [%s/.hidden.mp3]", got, root)
	}
}

func TestPathCandidatesEachIsAPath(t *testing.T) {
	root := mkTree(t)
	cands := candidatesFor(completionContextAt("open "+root+"/", len("open "+root+"/")))
	if len(cands) == 0 {
		t.Fatal("no candidates: the assertions below would hold vacuously")
	}
	for _, c := range cands {
		if !c.Path {
			t.Errorf("candidate %q is not marked as a path", c.Value)
		}
		if c.Desc != "" {
			t.Errorf("path candidate %q has a description %q, want none", c.Value, c.Desc)
		}
	}
}

func TestPathCandidatesReadDirFailureIsSilent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope") + "/"
	got := candidatesFor(completionContextAt("open "+missing, len("open "+missing)))
	if len(got) != 0 {
		t.Errorf("candidates = %v, want none", got)
	}
}

// A prefix that already names a directory descends into it without the user
// typing the separator: the values carry the one the listing needs.
func TestPathCandidatesDescendIntoNamedDirectory(t *testing.T) {
	root := mkTree(t)
	got := value(t, candidatesFor(completionContextAt("open "+root, len("open "+root))))
	want := []string{root + "/Album/", root + "/a.flac", root + "/b.mp3", root + "/d.mid"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

// On Windows a trailing backslash is a directory boundary, so the prefix names a
// directory to list with nothing typed inside it yet -- not a fragment whose
// directory has to be guessed back out of it.
func TestScanForWindowsTrailingBackslash(t *testing.T) {
	windowsSeparators(t)
	prefix := `C:\Music\Albums\`
	got := scanFor(prefix)
	if got.dir != prefix || got.typedDir != prefix || got.fragment != "" {
		t.Errorf("scanFor(%q) = %+v, want dir and typedDir %q with no fragment", prefix, got, prefix)
	}
}

// Directory candidates are marked with the separator style of the typed prefix.
// The listing is real -- a temporary directory stands in for the directory being
// completed -- while the shape echoed back is the one a Windows user typed.
func TestCandidatesInKeepsTheTypedSeparator(t *testing.T) {
	windowsSeparators(t)
	got := value(t, candidatesIn(pathScan{dir: mkTree(t), typedDir: `C:\Music\`}))
	want := `C:\Music\Album\,C:\Music\a.flac,C:\Music\b.mp3,C:\Music\d.mid`
	if strings.Join(got, ",") != want {
		t.Errorf("candidates = %v, want %s", got, want)
	}
}

func TestPathCandidatesKeepTildeShape(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, "song.mp3"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := value(t, candidatesFor(completionContextAt("open ~/", len("open ~/"))))
	if len(got) != 1 || got[0] != "~/song.mp3" {
		t.Errorf("candidates = %v, want [~/song.mp3]", got)
	}
}

func TestPathCandidatesTildeDotListsHidden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{".hidden.mp3", "plain.mp3"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := value(t, candidatesFor(completionContextAt("open ~/.", len("open ~/."))))
	if strings.Join(got, ",") != "~/.hidden.mp3" {
		t.Errorf("candidates = %v, want [~/.hidden.mp3]", got)
	}
}

func TestScanForMissingHomeDoesNotPanic(t *testing.T) {
	// "~" whose expansion is longer than the literal prefix used to index
	// past the start of the prefix.
	t.Setenv("HOME", filepath.Join(t.TempDir(), "missing"))
	if got := value(t, candidatesFor(completionContextAt("open ~", len("open ~")))); len(got) != 0 {
		t.Errorf("candidates = %v, want none", got)
	}
}

func TestPathCandidatesEmptyPrefix(t *testing.T) {
	root := mkTree(t)
	t.Chdir(root)
	got := value(t, candidatesFor(completionContextAt("open ", len("open "))))
	// d.mid belongs here too: it is playable as a synthetic format.
	if strings.Join(got, ",") != "Album/,a.flac,b.mp3,d.mid" {
		t.Errorf("candidates = %v", got)
	}
}
