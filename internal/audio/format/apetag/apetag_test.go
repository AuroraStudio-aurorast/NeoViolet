package apetag

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
)

// buildFooter creates a valid 32-byte APEv2 footer.
func buildFooter(tagSize uint32, itemCount uint32, hasHeader bool) []byte {
	f := make([]byte, 32)
	copy(f[0:8], "APETAGEX")
	binary.LittleEndian.PutUint32(f[8:12], 2000)  // version
	binary.LittleEndian.PutUint32(f[12:16], tagSize)
	binary.LittleEndian.PutUint32(f[16:20], itemCount)
	var flags uint32
	if hasHeader {
		flags |= 0x80000000
	}
	binary.LittleEndian.PutUint32(f[20:24], flags)
	// bytes 24-32 are zero (reserved)
	return f
}

// buildItem creates a single APEv2 tag item.
func buildItem(key, value string) []byte {
	vdata := []byte(value)
	item := make([]byte, 8+len(key)+1+len(vdata))
	binary.LittleEndian.PutUint32(item[0:4], uint32(len(vdata))) // value size
	binary.LittleEndian.PutUint32(item[4:8], 0)                  // flags (string)
	copy(item[8:], key)
	item[8+len(key)] = 0 // null terminator
	copy(item[8+len(key)+1:], vdata)
	return item
}

// buildFileWithTags creates a file-like byte slice containing APE audio data
// (just the MAC magic), followed by the given tag items and footer.
func buildFileWithTags(tagItems [][]byte, hasHeader bool) []byte {
	// Header (optional)
	var header []byte
	if hasHeader {
		header = buildFooter(0, 0, true)
	}

	// Items
	var items []byte
	for _, it := range tagItems {
		items = append(items, it...)
	}

	// Footer: tag_size = len(footer) + len(items)
	footer := buildFooter(uint32(32+len(items)), uint32(len(tagItems)), hasHeader)

	// Assemble: [MAC audio garbage] [header?] [items] [footer]
	var buf bytes.Buffer
	buf.Write([]byte("MAC "))       // fake APE magic
	buf.Write(make([]byte, 200))    // fake audio data
	if hasHeader {
		buf.Write(header)
	}
	buf.Write(items)
	buf.Write(footer)
	return buf.Bytes()
}

func TestParseSimpleTags(t *testing.T) {
	data := buildFileWithTags([][]byte{
		buildItem("TITLE", "Test Song"),
		buildItem("ARTIST", "Test Artist"),
		buildItem("ALBUM", "Test Album"),
	}, false)

	r := bytes.NewReader(data)
	tags, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tags.Title != "Test Song" {
		t.Errorf("Title = %q, want %q", tags.Title, "Test Song")
	}
	if tags.Artist != "Test Artist" {
		t.Errorf("Artist = %q, want %q", tags.Artist, "Test Artist")
	}
	if tags.Album != "Test Album" {
		t.Errorf("Album = %q, want %q", tags.Album, "Test Album")
	}
}

func TestParseCaseInsensitiveKeys(t *testing.T) {
	data := buildFileWithTags([][]byte{
		buildItem("title", "Lowercase Title"),
		buildItem("Artist", "Mixed Case"),
	}, false)

	r := bytes.NewReader(data)
	tags, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tags.Title != "Lowercase Title" {
		t.Errorf("Title = %q", tags.Title)
	}
	if tags.Artist != "Mixed Case" {
		t.Errorf("Artist = %q", tags.Artist)
	}
}

func TestParseWithHeader(t *testing.T) {
	data := buildFileWithTags([][]byte{
		buildItem("TITLE", "Header Song"),
	}, true)

	r := bytes.NewReader(data)
	tags, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tags.Title != "Header Song" {
		t.Errorf("Title = %q", tags.Title)
	}
}

func TestParseCoverArt(t *testing.T) {
	// Cover art value = "cover.jpg\0" + fake JPEG bytes
	coverValue := append([]byte("cover.jpg\x00"), []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}...)

	data := buildFileWithTags([][]byte{
		buildItem("TITLE", "Song with Cover"),
		buildItem("COVER ART (FRONT)", string(coverValue)),
	}, false)

	r := bytes.NewReader(data)
	tags, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tags.Title != "Song with Cover" {
		t.Errorf("Title = %q", tags.Title)
	}
	if len(tags.CoverData) == 0 {
		t.Fatal("CoverData is empty")
	}
	expectedCover := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	if !bytes.Equal(tags.CoverData, expectedCover) {
		t.Errorf("CoverData = %x, want %x", tags.CoverData, expectedCover)
	}
}

func TestParseNoTags(t *testing.T) {
	r := bytes.NewReader([]byte("just some random data with no APEv2 tags"))
	_, err := Parse(r)
	if err != errNoTags {
		t.Errorf("expected errNoTags, got %v", err)
	}
}

func TestParseTooShort(t *testing.T) {
	r := bytes.NewReader([]byte("short"))
	_, err := Parse(r)
	if err != errNoTags {
		t.Errorf("expected errNoTags, got %v", err)
	}
}

func TestParseEmptyItems(t *testing.T) {
	data := buildFileWithTags(nil, false)
	r := bytes.NewReader(data)
	tags, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if tags.Title != "" || tags.Artist != "" {
		t.Error("expected empty tags for zero items")
	}
}

func TestParseFile(t *testing.T) {
	// Create a temp file with valid tags.
	data := buildFileWithTags([][]byte{
		buildItem("TITLE", "File Test"),
		buildItem("ARTIST", "File Artist"),
	}, false)

	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.ape")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}
	tmpFile.Seek(0, 0)

	tags, err := ParseFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if tags.Title != "File Test" {
		t.Errorf("Title = %q", tags.Title)
	}
	if tags.Artist != "File Artist" {
		t.Errorf("Artist = %q", tags.Artist)
	}
}

func TestParseFileNoTags(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "test-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmpFile.Close()
	tmpFile.Write([]byte("no tags here"))
	tmpFile.Seek(0, 0)

	_, err = ParseFile(tmpFile.Name())
	if err != errNoTags {
		t.Errorf("expected errNoTags, got %v", err)
	}
}