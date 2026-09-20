package format

import "testing"

func TestIsSupportedExt(t *testing.T) {
	for _, ext := range []string{".mp3", ".wav", ".flac", ".ogg", ".oga"} {
		if !IsSupportedExt(ext) {
			t.Errorf("IsSupportedExt(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{".txt", ".m4a.bak", "", "mp3"} {
		if IsSupportedExt(ext) {
			t.Errorf("IsSupportedExt(%q) = true, want false", ext)
		}
	}
}

// The read-only query must agree with the registry entry by entry: nothing
// registered may be missing and nothing missing may be reported.
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
