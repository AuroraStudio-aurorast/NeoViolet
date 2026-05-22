package lyrics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedParserWithTestdata(t *testing.T) {
	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata"))
	files := []string{
		"test_mp3_with_emb_lrc.mp3",
		"test_flac_with_emb_lrc.flac",
	}
	for _, name := range files {
		path := filepath.Join(base, name)
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(path); err != nil {
				t.Skipf("test file not found: %v", err)
			}
			data, err := FindAndParse(path, []string{"embedded"})
			if err != nil {
				t.Fatalf("FindAndParse error: %v", err)
			}
			if data == nil {
				t.Fatal("FindAndParse returned nil (no embedded lyrics found)")
			}
			if len(data.Lines) == 0 {
				t.Fatal("no lyric lines parsed")
			}
			t.Logf("OK: %d lines", len(data.Lines))
		})
	}
}