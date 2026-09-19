package lyrics

import (
	"os"
	"path/filepath"
	"strings"
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

func TestParseSYLT_MultiLineTextBecomesParts(t *testing.T) {
	d := parseSYLT(syltBody(
		syltEntry{"第一行\n第二行", 1000},
		syltEntry{"third", 2000},
	))
	if d == nil {
		t.Fatal("parseSYLT returned nil")
	}
	if len(d.Lines) != 2 {
		t.Fatalf("len(Lines) = %d, want 2", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Text != "第一行 | 第二行" {
		t.Errorf("Text = %q, want %q", line.Text, "第一行 | 第二行")
	}
	if len(line.Parts) != 2 || line.Parts[0] != "第一行" || line.Parts[1] != "第二行" {
		t.Errorf("Parts = %v, want [第一行 第二行]", line.Parts)
	}
	if d.Lines[1].Parts != nil || d.Lines[1].Text != "third" {
		t.Errorf("Lines[1] = %q parts=%v, want plain %q", d.Lines[1].Text, d.Lines[1].Parts, "third")
	}
}

func TestParseSYLT_BlankOnlyTextIsDropped(t *testing.T) {
	d := parseSYLT(syltBody(
		syltEntry{"\n", 1000},
		syltEntry{"kept", 2000},
	))
	if d == nil {
		t.Fatal("parseSYLT returned nil")
	}
	if len(d.Lines) != 1 || d.Lines[0].Text != "kept" {
		t.Fatalf("Lines = %+v, want a single %q line", d.Lines, "kept")
	}
}

func TestParseSYLT_PartsCarryNoNewline(t *testing.T) {
	// The local form of C2.
	d := parseSYLT(syltBody(syltEntry{"a\nb\nc", 1000}))
	if d == nil {
		t.Fatal("parseSYLT returned nil")
	}
	line := d.Lines[0]
	if strings.ContainsAny(line.Text, "\n\r") {
		t.Errorf("Text contains a newline: %q", line.Text)
	}
	for i, p := range line.Parts {
		if strings.ContainsAny(p, "\n\r") {
			t.Errorf("Parts[%d] contains a newline: %q", i, p)
		}
	}
}

func TestParseSYLT_CRLFTextBecomesParts(t *testing.T) {
	// A Windows-written tag yields "\r\n"; the split is on "\n", and a segment's
	// trailing "\r" is removed by TrimSpace.
	d := parseSYLT(syltBody(syltEntry{"a\r\nb", 1000}))
	if d == nil {
		t.Fatal("parseSYLT returned nil")
	}
	if len(d.Lines) != 1 {
		t.Fatalf("len(Lines) = %d, want 1", len(d.Lines))
	}
	line := d.Lines[0]
	if line.Text != "a | b" {
		t.Errorf("Text = %q, want %q", line.Text, "a | b")
	}
	if len(line.Parts) != 2 || line.Parts[0] != "a" || line.Parts[1] != "b" {
		t.Fatalf("Parts = %v, want [a b]", line.Parts)
	}
	if strings.ContainsRune(line.Text, '\r') {
		t.Errorf("Text contains a CR: %q", line.Text)
	}
	for i, p := range line.Parts {
		if strings.ContainsRune(p, '\r') {
			t.Errorf("Parts[%d] contains a CR: %q", i, p)
		}
	}
}

func TestParseSYLT_EmptySegmentsAreDropped(t *testing.T) {
	// Neither empty nor whitespace-only segments reach Parts, which would otherwise
	// produce a blank display row.
	d := parseSYLT(syltBody(syltEntry{"a\n\n  \nb", 1000}))
	if d == nil {
		t.Fatal("parseSYLT returned nil")
	}
	line := d.Lines[0]
	if line.Text != "a | b" {
		t.Errorf("Text = %q, want %q", line.Text, "a | b")
	}
	if len(line.Parts) != 2 || line.Parts[0] != "a" || line.Parts[1] != "b" {
		t.Errorf("Parts = %v, want [a b]", line.Parts)
	}
}

func TestParseSYLT_AllBlankTextsYieldNil(t *testing.T) {
	// When every entry reduces to whitespace no empty Data is left behind: nil lets
	// embeddedParser move on to the next tag.
	if d := parseSYLT(syltBody(syltEntry{"\n", 1000}, syltEntry{"  \r\n", 2000})); d != nil {
		t.Fatalf("parseSYLT(blank entries) = %+v, want nil", d)
	}
}
