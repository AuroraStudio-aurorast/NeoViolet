package lyrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSidecarParsersWithTestdata(t *testing.T) {
	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata"))
	audioPath := filepath.Join(base, "test_mp3_with_lrc.mp3")

	tests := []struct {
		name   string
		format string
	}{
		{"lrc", "lrc"},
		{"ttml", "ttml"},
		{"qrc", "qrc"},
		{"yrc", "yrc"},
		{"eslrc", "eslrc"},
		{"lys", "lys"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			data, err := FindAndParse(audioPath, []string{tt.format})
			if err != nil {
				t.Fatalf("FindAndParse error: %v", err)
			}
			if data == nil {
				t.Fatalf("%s: no lyrics found", tt.format)
			}
			if len(data.Lines) == 0 {
				t.Fatal("no lyric lines")
			}
			t.Logf("%s: %d lines, first=%q", tt.format, len(data.Lines), data.Lines[0].Text)
		})
	}
}

func TestPriorityChain_EmbeddedFirst(t *testing.T) {
	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata"))
	audioPath := filepath.Join(base, "test_mp3_with_emb_lrc.mp3")

	if _, err := os.Stat(audioPath); err != nil {
		t.Skipf("embedded test file not found: %v", err)
	}

	data, err := FindAndParse(audioPath, []string{"embedded", "lrc"})
	if err != nil {
		t.Fatalf("FindAndParse error: %v", err)
	}
	if data == nil {
		t.Fatal("no lyrics found (expected embedded lyrics)")
	}
	if len(data.Lines) == 0 {
		t.Fatal("no lyric lines")
	}
	// First line has offset+300, so timestamp should be 300ms.
	if data.Lines[0].Time != 300*time.Millisecond {
		t.Errorf("expected first line at 300ms (offset+300), got %v", data.Lines[0].Time)
	}
	if !strings.Contains(data.Lines[0].Text, "first line") {
		t.Errorf("unexpected first line text: %q", data.Lines[0].Text)
	}
	t.Logf("embedded priority OK: %d lines", len(data.Lines))
}

func TestPriorityChain_LrcFallback(t *testing.T) {
	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata"))
	audioPath := filepath.Join(base, "test_mp3_with_lrc.mp3")
	lrcPath := filepath.Join(base, "test_mp3_with_lrc.lrc")

	if _, err := os.Stat(audioPath); err != nil {
		t.Skipf("test file not found: %v", err)
	}
	if _, err := os.Stat(lrcPath); err != nil {
		t.Skipf("sidecar file not found: %v", err)
	}

	// Source MP3 has no embedded lyrics → should fall back to .lrc sidecar.
	data, err := FindAndParse(audioPath, []string{"embedded", "lrc"})
	if err != nil {
		t.Fatalf("FindAndParse error: %v", err)
	}
	if data == nil {
		t.Fatal("no lyrics found (expected lrc sidecar)")
	}
	if len(data.Lines) == 0 {
		t.Fatal("no lyric lines")
	}
	t.Logf("lrc fallback OK: %d lines, first=%q @ %v", len(data.Lines), data.Lines[0].Text, data.Lines[0].Time)
}

func TestEmbedded_PlainTextFallback(t *testing.T) {
	const input = "Line one\nLine two\nLine three"
	data := parsePlainText(input, "")
	if data == nil {
		t.Fatal("parsePlainText returned nil")
	}
	if len(data.Lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(data.Lines))
	}
	if data.Lines[0].Time != 0 {
		t.Errorf("line 0 time = %v, want 0s", data.Lines[0].Time)
	}
	if data.Lines[1].Time != 5*time.Second {
		t.Errorf("line 1 time = %v, want 5s", data.Lines[1].Time)
	}
	if data.Lines[2].Time != 10*time.Second {
		t.Errorf("line 2 time = %v, want 10s", data.Lines[2].Time)
	}
	if data.Lines[0].Text != "Line one" {
		t.Errorf("line 0 text = %q", data.Lines[0].Text)
	}
}

func TestEmbedded_SkipsNonAudioFiles(t *testing.T) {
	data, err := FindAndParse("/nonexistent/file.mp3", []string{"embedded"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data != nil {
		t.Fatal("expected nil for non-existent file")
	}
}