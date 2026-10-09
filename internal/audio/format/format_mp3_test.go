package format

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/gopxl/beep/v2/mp3"
)

const (
	// MPEG1 Layer III, 44100 Hz, 128 kbps, no padding.
	mp3TestFrameBytes      = 417
	mp3TestSamplesPerFrame = 1152
)

// mp3TestFrames returns n MPEG1 Layer III frames with valid headers. The
// payload is not decodable audio: the decoder's frame scan needs headers only.
func mp3TestFrames(n int) []byte {
	frame := make([]byte, mp3TestFrameBytes)
	copy(frame, []byte{0xFF, 0xFB, 0x90, 0x00})
	buf := make([]byte, 0, n*mp3TestFrameBytes)
	for range n {
		buf = append(buf, frame...)
	}
	return buf
}

// mp3TestFreeFormatRun repeats a byte run that parses as a valid MPEG header
// with bitrate index 0 — the free-format case go-mp3 rejects outright. Cover
// art carries such runs, which is what makes a trailing tag fatal.
func mp3TestFreeFormatRun(size int) []byte {
	return bytes.Repeat([]byte{0xFF, 0xFB, 0x05, 0x4D}, size/4)
}

// mp3TestApeTag builds an APEv2 tag holding a single binary item.
func mp3TestApeTag(key string, value []byte, withHeader bool) []byte {
	var items bytes.Buffer
	itemHead := make([]byte, 8)
	// #nosec G115 -- sizes are bounded by the small test fixtures.
	binary.LittleEndian.PutUint32(itemHead[0:4], uint32(len(value)))
	binary.LittleEndian.PutUint32(itemHead[4:8], 2) // binary item
	items.Write(itemHead)
	items.WriteString(key)
	items.WriteByte(0)
	items.Write(value)

	footer := func(flags uint32) []byte {
		f := make([]byte, 32)
		copy(f[0:8], "APETAGEX")
		binary.LittleEndian.PutUint32(f[8:12], 2000)
		// #nosec G115 -- sizes are bounded by the small test fixtures.
		binary.LittleEndian.PutUint32(f[12:16], uint32(32+items.Len()))
		binary.LittleEndian.PutUint32(f[16:20], 1)
		binary.LittleEndian.PutUint32(f[20:24], flags)
		return f
	}

	var tag bytes.Buffer
	if withHeader {
		tag.Write(footer(0xA0000000))
	}
	tag.Write(items.Bytes())
	footerFlags := uint32(0)
	if withHeader {
		footerFlags = 0x80000000 // bit 31: a header precedes the items
	}
	tag.Write(footer(footerFlags))
	return tag.Bytes()
}

type nopCloseReadSeeker struct{ io.ReadSeeker }

func (nopCloseReadSeeker) Close() error   { return nil }
func joinMP3Parts(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func TestMP3TrailingTagStart(t *testing.T) {
	frames := mp3TestFrames(20)
	id3v2 := append([]byte("ID3\x03\x00\x00\x00\x00\x00\x0A"), make([]byte, 10)...)
	id3v1 := append([]byte("TAG"), make([]byte, id3v1TagSize-3)...)
	ape := mp3TestApeTag("Cover Art (Front)", mp3TestFreeFormatRun(1024), true)
	apeNoHeader := mp3TestApeTag("TITLE", []byte("song"), false)

	tests := []struct {
		name string
		data []byte
		// wantStart is 0 when the stream has no trailing metadata.
		wantStart int64
	}{
		{"audio only", frames, 0},
		{"id3v1 only", joinMP3Parts(frames, id3v1), int64(len(frames))},
		{"apev2 with header", joinMP3Parts(frames, ape), int64(len(frames))},
		{"apev2 without header", joinMP3Parts(frames, apeNoHeader), int64(len(frames))},
		{"apev2 then id3v1", joinMP3Parts(frames, ape, id3v1), int64(len(frames))},
		{"leading id3v2", joinMP3Parts(id3v2, frames, ape, id3v1), int64(len(id3v2) + len(frames))},
		{"truncated id3v1 magic", joinMP3Parts(frames, []byte("TA")), 0},
		{"empty stream", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, trailing := mp3TrailingTagStart(bytes.NewReader(tt.data))
			if start != tt.wantStart || trailing != (tt.wantStart > 0) {
				t.Errorf("mp3TrailingTagStart = (%d, %v), want (%d, %v)",
					start, trailing, tt.wantStart, tt.wantStart > 0)
			}
		})
	}
}

func TestDecodeMP3IgnoresFreeFormatBytesInTrailingTag(t *testing.T) {
	const frames = 200
	audio := mp3TestFrames(frames)
	withTag := joinMP3Parts(audio, mp3TestApeTag("Cover Art (Front)", mp3TestFreeFormatRun(4096), true))

	// The raw decoder still fails on these bytes: that is the bug being guarded.
	_, _, err := mp3.Decode(nopCloseReadSeeker{bytes.NewReader(withTag)})
	if err == nil {
		t.Fatal("mp3.Decode accepted the trailing tag; fixture no longer reproduces the bug")
	}
	if !strings.Contains(err.Error(), "free bitrate") {
		t.Fatalf("mp3.Decode error = %v, want the free-format failure", err)
	}

	streamer, format, err := decodeMP3(nopCloseReadSeeker{bytes.NewReader(withTag)})
	if err != nil {
		t.Fatalf("decodeMP3: %v", err)
	}
	defer func() { _ = streamer.Close() }()

	if format.SampleRate != 44100 {
		t.Errorf("SampleRate = %v, want 44100", format.SampleRate)
	}
	want := frames * mp3TestSamplesPerFrame
	if streamer.Len() != want {
		t.Errorf("Len() = %d, want %d: trailing metadata counted as audio", streamer.Len(), want)
	}
	if err := streamer.Seek(streamer.Len() - 1); err != nil {
		t.Errorf("Seek(Len()-1) = %v, want nil", err)
	}
}

func TestDecodeMP3TrailingMetadataDoesNotInflateLength(t *testing.T) {
	const frames = 200
	audio := mp3TestFrames(frames)
	// Tag bytes that parse as ordinary frames inflate the frame table without
	// tripping the free-format error.
	tag := mp3TestApeTag("TITLE", bytes.Repeat([]byte{0xFF, 0xFB, 0x90, 0x00}, 1000), true)
	withTag := joinMP3Parts(audio, tag)

	want := frames * mp3TestSamplesPerFrame

	raw, _, err := mp3.Decode(nopCloseReadSeeker{bytes.NewReader(withTag)})
	if err != nil {
		t.Fatalf("mp3.Decode: %v", err)
	}
	defer func() { _ = raw.Close() }()
	if raw.Len() <= want {
		t.Fatalf("raw Len() = %d, want more than %d for the tag bytes", raw.Len(), want)
	}

	streamer, _, err := decodeMP3(nopCloseReadSeeker{bytes.NewReader(withTag)})
	if err != nil {
		t.Fatalf("decodeMP3: %v", err)
	}
	defer func() { _ = streamer.Close() }()
	if streamer.Len() != want {
		t.Errorf("Len() = %d, want %d: trailing metadata counted as audio", streamer.Len(), want)
	}
}

func TestDecodeMP3AcceptsNonSeekableReader(t *testing.T) {
	rc := io.NopCloser(bytes.NewReader(mp3TestFrames(2)))
	streamer, _, err := decodeMP3(rc)
	if err != nil {
		t.Fatalf("decodeMP3: %v", err)
	}
	defer func() { _ = streamer.Close() }()
}

func TestAudioBoundedReaderHidesTail(t *testing.T) {
	r := &audioBoundedReader{
		rs:     bytes.NewReader([]byte("0123456789")),
		closer: nopCloseReadSeeker{},
		limit:  4,
	}
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "0123" {
		t.Errorf("ReadAll = %q, want %q", got, "0123")
	}

	if _, err := r.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	buf := make([]byte, 2)
	if n, err := r.Read(buf); err != nil || n != 2 || string(buf) != "01" {
		t.Errorf("Read after Seek = (%d, %v) %q, want (2, nil) %q", n, err, buf, "01")
	}

	// Seeking into the hidden tail yields no data instead of tag bytes.
	if _, err := r.Seek(8, io.SeekStart); err != nil {
		t.Fatalf("Seek past limit: %v", err)
	}
	if n, err := r.Read(buf); n != 0 || err != io.EOF {
		t.Errorf("Read past limit = (%d, %v), want (0, EOF)", n, err)
	}
}
