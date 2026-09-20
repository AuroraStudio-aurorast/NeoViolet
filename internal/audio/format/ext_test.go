package format

import "testing"

func TestIsSupportedExt(t *testing.T) {
	for _, ext := range []string{".mp3", ".wav", ".flac", ".ogg", ".oga"} {
		if !IsSupportedExt(ext) {
			t.Errorf("IsSupportedExt(%q) = false, want true", ext)
		}
	}
	// "song.mp3" pins "an extension, not a path"; ".mid" pins the scope: it is
	// playable but recognised by content, so it has no extension registry entry.
	for _, ext := range []string{".txt", ".m4a.bak", "", "mp3", "song.mp3", ".mid"} {
		if IsSupportedExt(ext) {
			t.Errorf("IsSupportedExt(%q) = true, want false", ext)
		}
	}
}

// A query that drifts from the registry makes the player's own formats
// disappear, so every registered extension must be reported.
func TestIsSupportedExtMatchesRegistry(t *testing.T) {
	if len(extLookup) == 0 {
		t.Fatal("extLookup is empty")
	}
	for ext := range extLookup {
		if !IsSupportedExt(ext) {
			t.Errorf("registered extension %q is not reported", ext)
		}
	}
}
