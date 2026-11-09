//nolint:gosec // Synthetic file sizes and paths are controlled by these tests.
package format

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func probeTestFile(t *testing.T, name string, data []byte) ProbeInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder().Probe(context.Background(), path)
	if err != nil {
		t.Fatalf("Probe(%s): %v", name, err)
	}
	return got
}
func probeLE32(n uint32) []byte { return binary.LittleEndian.AppendUint32(nil, n) }
func probeBE32(n uint32) []byte { return binary.BigEndian.AppendUint32(nil, n) }
func probeComment(key, value string) []byte {
	s := []byte(key + "=" + value)
	return append(probeLE32(uint32(len(s))), s...)
}
func probeTestComments() []byte {
	data := append(probeLE32(0), probeLE32(4)...)
	for _, p := range [][2]string{{"TITLE", "Title"}, {"METADATA_BLOCK_PICTURE", "ignored"}, {"ARTIST", "Artist"}, {"ALBUM", "Album"}} {
		data = append(data, probeComment(p[0], p[1])...)
	}
	return data
}
func TestProbeFLACHeadersAndTagsWithoutAudio(t *testing.T) {
	data := append([]byte("fLaC\x00\x00\x00\x22"), make([]byte, 34)...)
	// STREAMINFO: sample rate 48000, total samples 96000.
	binary.BigEndian.PutUint64(data[18:26], uint64(48000)<<44|96000)
	comments := probeTestComments()
	data = append(data, 0x84, byte(len(comments)>>16), byte(len(comments)>>8), byte(len(comments)))
	data = append(data, comments...)
	got := probeTestFile(t, "renamed.audio", data)
	if got.Format != "FLAC" || got.Duration != 2*time.Second || got.Metadata != (Metadata{"Title", "Artist", "Album"}) {
		t.Fatalf("got %+v", got)
	}
}
func probeOggPage(serial, seq uint32, granule uint64, flags byte, packet []byte) []byte {
	page := make([]byte, 27)
	copy(page, "OggS")
	page[5] = flags
	binary.LittleEndian.PutUint64(page[6:], granule)
	binary.LittleEndian.PutUint32(page[14:], serial)
	binary.LittleEndian.PutUint32(page[18:], seq)
	for n := len(packet); n >= 255; n -= 255 {
		page = append(page, 255)
	}
	page = append(page, byte(len(packet)%255))
	page[26] = byte(len(page) - 27)
	return append(page, packet...)
}
func TestProbeOggHeadersAndFinalGranuleWithoutAudio(t *testing.T) {
	for _, opus := range []bool{true, false} {
		t.Run(map[bool]string{true: "opus", false: "vorbis"}[opus], func(t *testing.T) {
			var head, tags []byte
			var granule uint64 = 96000
			ext := ".ogg"
			if opus {
				head = make([]byte, 19)
				copy(head, "OpusHead")
				head[8] = 1
				head[9] = 2
				binary.LittleEndian.PutUint16(head[10:], 312)
				granule += 312
				tags = append([]byte("OpusTags"), probeTestComments()...)
				ext = ".opus"
			} else {
				head = make([]byte, 30)
				copy(head, "\x01vorbis")
				head[11] = 2
				binary.LittleEndian.PutUint32(head[12:], 48000)
				tags = append([]byte("\x03vorbis"), probeTestComments()...)
			}
			data := probeOggPage(7, 0, 0, 2, head)
			data = append(data, probeOggPage(7, 1, 0, 0, tags)...)
			data = append(data, probeOggPage(7, 2, granule, 4, []byte{0})...)
			got := probeTestFile(t, "track"+ext, data)
			if got.Duration != 2*time.Second || got.Metadata != (Metadata{"Title", "Artist", "Album"}) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
func probeAtom(kind string, body []byte) []byte {
	return append(append(probeBE32(uint32(len(body)+8)), kind...), body...)
}
func TestProbeM4AContainerDurationAndTextWithoutSamples(t *testing.T) {
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], 2000)
	var tags []byte
	for _, item := range [][2]string{{"\xa9nam", "Title"}, {"\xa9ART", "Artist"}, {"\xa9alb", "Album"}} {
		tags = append(tags, probeAtom(item[0], probeAtom("data", append(make([]byte, 8), item[1]...)))...)
	}
	meta := append(make([]byte, 4), probeAtom("ilst", tags)...)
	moov := append(probeAtom("mvhd", mvhd), probeAtom("udta", probeAtom("meta", meta))...)
	data := append(probeAtom("ftyp", []byte("M4A \x00\x00\x00\x00")), probeAtom("moov", moov)...)
	got := probeTestFile(t, "track.m4a", data)
	if got.Duration != 2*time.Second || got.Metadata != (Metadata{"Title", "Artist", "Album"}) {
		t.Fatalf("got %+v", got)
	}
}
func TestProbeMP3FrameCountAndUnknownWithoutIndex(t *testing.T) {
	for _, kind := range []string{"Xing", "Info", "VBRI", "none"} {
		t.Run(kind, func(t *testing.T) {
			data := make([]byte, 417)
			copy(data, []byte{0xff, 0xfb, 0x90, 0x00})
			switch kind {
			case "Xing", "Info":
				copy(data[36:], kind)
				binary.BigEndian.PutUint32(data[40:], 1)
				binary.BigEndian.PutUint32(data[44:], 100)
			case "VBRI":
				copy(data[36:], kind)
				binary.BigEndian.PutUint32(data[50:], 100)
			}
			got := probeTestFile(t, "track.mp3", data)
			want := time.Duration(0)
			if kind != "none" {
				want = time.Duration(100*1152) * time.Second / 44100
			}
			if got.Duration != want {
				t.Fatalf("duration=%v, want %v", got.Duration, want)
			}
		})
	}
}
func TestProbeSkipsLargeID3CoverAndPreservesText(t *testing.T) {
	// A cover frame is skipped; audio deliberately has no duration index.
	cover := make([]byte, 2<<20)
	body := append([]byte("APIC"), probeBE32(uint32(len(cover)))...)
	body = append(body, 0, 0)
	body = append(body, cover...)
	for _, item := range [][2]string{{"TIT2", "Title"}, {"TPE1", "Artist"}, {"TALB", "Album"}} {
		body = append(body, item[0]...)
		body = append(body, probeBE32(uint32(len(item[1])+1))...)
		body = append(body, 0, 0, 3)
		body = append(body, item[1]...)
	}
	n := len(body)
	data := []byte{'I', 'D', '3', 3, 0, 0, byte(n>>21) & 127, byte(n>>14) & 127, byte(n>>7) & 127, byte(n) & 127}
	data = append(data, body...)
	data = append(data, 0xff, 0xfb, 0x90, 0)
	got := probeTestFile(t, "track.mp3", data)
	if got.Metadata != (Metadata{"Title", "Artist", "Album"}) || got.Duration != 0 {
		t.Fatalf("got %+v", got)
	}
}
func TestProbeUnknownAndSyntheticFormatsRemainUnknown(t *testing.T) {
	for _, name := range []string{"track.mp2", "track.ape", "track.mid", "track.xm"} {
		got := probeTestFile(t, name, []byte("unknown"))
		if got.Duration != 0 {
			t.Fatalf("%s: %+v", name, got)
		}
	}
}

func TestProbeSkipsSparseCoverWithoutSizeDependentAllocation(t *testing.T) {
	const coverSize = 32 << 20
	path := filepath.Join(t.TempDir(), "cover.mp3")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Leave the cover sparse, followed by an ordinary text frame.
	frame := append([]byte("TIT2"), probeBE32(6)...)
	frame = append(frame, 0, 0, 3)
	frame = append(frame, "Title"...)
	bodySize := 10 + coverSize + len(frame)
	header := []byte{'I', 'D', '3', 3, 0, 0, byte(bodySize>>21) & 127, byte(bodySize>>14) & 127, byte(bodySize>>7) & 127, byte(bodySize) & 127}
	header = append(header, []byte("APIC")...)
	header = append(header, probeBE32(coverSize)...)
	header = append(header, 0, 0)
	if _, err = f.Write(header); err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt(append(frame, 0xff, 0xfb, 0x90, 0), 10+10+coverSize); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got, err := NewDecoder().Probe(context.Background(), path)
	runtime.ReadMemStats(&after)
	if err != nil || got.Title != "Title" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if bytes := after.TotalAlloc - before.TotalAlloc; bytes > 1<<20 {
		t.Fatalf("cover scan allocated %d bytes", bytes)
	}
}

func TestProbeBoundsRepeatedTextAllocation(t *testing.T) {
	data := append([]byte("fLaC\x00\x00\x00\x22"), make([]byte, 34)...)
	comments := append(probeLE32(0), probeLE32(128)...)
	value := strings.Repeat("x", probeTextLimit-6)
	for range 128 {
		comments = append(comments, probeComment("TITLE", value)...)
	}
	// More than one comment block makes it possible to declare a very large tag
	// stream while keeping each FLAC block's 24-bit length valid.
	for len(comments) > 0 {
		n := min(len(comments), 0xffffff)
		flag := byte(4)
		if n == len(comments) {
			flag |= 128
		}
		data = append(data, flag, byte(n>>16), byte(n>>8), byte(n))
		data = append(data, comments[:n]...)
		comments = comments[n:]
	}
	path := filepath.Join(t.TempDir(), "repeated.flac")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := NewDecoder().Probe(context.Background(), path)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if bytes := after.TotalAlloc - before.TotalAlloc; bytes > 2<<20 {
		t.Fatalf("repeated tags allocated %d bytes", bytes)
	}
}

func TestProbeSkipsFLACAndOggPictures(t *testing.T) {
	// Put a large irrelevant Vorbis comment before artist/album, including a
	// continuation over Ogg pages. Retaining the packet would allocate its cover.
	comments := append(probeLE32(0), probeLE32(4)...)
	comments = append(comments, probeComment("TITLE", "Title")...)
	comments = append(comments, probeComment("METADATA_BLOCK_PICTURE", strings.Repeat("x", 2<<20))...)
	comments = append(comments, probeComment("ARTIST", "Artist")...)
	comments = append(comments, probeComment("ALBUM", "Album")...)
	flac := []byte("fLaC")
	flac = append(flac, 0x84, byte(len(comments)>>16), byte(len(comments)>>8), byte(len(comments)))
	flac = append(flac, comments...)
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1
	head[9] = 2
	ogg := probeOggPage(7, 0, 0, 2, head)
	packet := append([]byte("OpusTags"), comments...)
	for seq := uint32(1); len(packet) > 0; seq++ {
		n := min(len(packet), 255*254)
		flags := byte(0)
		if seq > 1 {
			flags = 1
		}
		page := probeOggPage(7, seq, 0, flags, packet[:n])
		if n < len(packet) {
			page = append(page[:27+254], page[27+255:]...)
			page[26] = 254
		}
		ogg = append(ogg, page...)
		packet = packet[n:]
	}
	ogg = append(ogg, probeOggPage(7, 100, 48000, 4, []byte{0})...)
	for _, item := range []struct {
		name string
		data []byte
	}{{"cover.flac", flac}, {"cover.opus", ogg}} {
		path := filepath.Join(t.TempDir(), item.name)
		if err := os.WriteFile(path, item.data, 0600); err != nil {
			t.Fatal(err)
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		got, err := NewDecoder().Probe(context.Background(), path)
		runtime.ReadMemStats(&after)
		if err != nil || got.Metadata != (Metadata{"Title", "Artist", "Album"}) {
			t.Fatalf("%s: %+v, %v", item.name, got, err)
		}
		if bytes := after.TotalAlloc - before.TotalAlloc; bytes > 1<<20 {
			t.Fatalf("%s allocated %d bytes", item.name, bytes)
		}
	}
}

func TestProbeMalformedHeadersReturnErrors(t *testing.T) {
	for _, item := range []struct {
		name string
		data []byte
	}{
		{"broken.wav", []byte("broken")},
		{"bad.flac", []byte("fLaC\x84\xff\xff\xff")},
		{"bad.mp3", []byte("ID3\x04\x00\x00\x7f\x7f\x7f\x7f")},
		{"bad.m4a", append(probeAtom("ftyp", []byte("M4A \x00\x00\x00\x00")), []byte("\x00\x00\x00\x01moov\xff\xff\xff\xff\xff\xff\xff\xff")...)},
		{"bad.opus", []byte("OggS")},
	} {
		path := filepath.Join(t.TempDir(), item.name)
		if err := os.WriteFile(path, item.data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewDecoder().Probe(context.Background(), path); err == nil {
			t.Fatalf("%s: expected invalid header error", item.name)
		}
	}
}

func TestProbeChainedOggDoesNotReportPartialDuration(t *testing.T) {
	head := make([]byte, 19)
	copy(head, "OpusHead")
	head[8] = 1
	head[9] = 2
	var data []byte
	for _, serial := range []uint32{7, 8} {
		data = append(data, probeOggPage(serial, 0, 0, 2, head)...)
		data = append(data, probeOggPage(serial, 1, 48000, 4, []byte{0})...)
	}
	got := probeTestFile(t, "chained.opus", data)
	if got.Duration != 0 {
		t.Fatalf("partial chained duration reported as complete: %v", got.Duration)
	}
}

type probeCancelContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *probeCancelContext) Err() error {
	c.checks++
	if c.checks == 8 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestProbeCancelsBetweenHeaderReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tags.flac")
	comments := probeTestComments()
	data := append([]byte("fLaC\x84"), byte(len(comments)>>16), byte(len(comments)>>8), byte(len(comments)))
	data = append(data, comments...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := NewDecoder().Probe(&probeCancelContext{Context: ctx, cancel: cancel}, path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestProbeAPEv2SkipsBinaryCover(t *testing.T) {
	data := []byte("MAC ")
	var items []byte
	for _, item := range []struct {
		key, value string
		flags      uint32
	}{{"Cover Art (Front)", strings.Repeat("x", 2<<20), 2}, {"Title", "Title", 0}, {"Artist", "Artist", 0}, {"Album", "Album", 0}} {
		items = append(items, probeLE32(uint32(len(item.value)))...)
		items = append(items, probeLE32(item.flags)...)
		items = append(items, item.key...)
		items = append(items, 0)
		items = append(items, item.value...)
	}
	footer := append([]byte("APETAGEX"), probeLE32(2000)...)
	footer = append(footer, probeLE32(uint32(len(items)+32))...)
	footer = append(footer, probeLE32(4)...)
	footer = append(footer, make([]byte, 12)...)
	data = append(data, items...)
	data = append(data, footer...)
	got := probeTestFile(t, "metadata.ape", data)
	if got.Metadata != (Metadata{"Title", "Artist", "Album"}) || got.Duration != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestProbeID3TagUnsynchronizationPreservesTextAroundCover(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(fmt.Sprint(before), func(t *testing.T) {
			frame := func(key string, value []byte) []byte {
				body := append([]byte(key), probeBE32(uint32(len(value)))...)
				body = append(body, 0, 0)
				return append(body, value...)
			}
			title := frame("TIT2", []byte{0, 'T', 0xff, 0xe0})
			cover := frame("APIC", bytes.Repeat([]byte{0xff, 0xe0, 0xff, 0}, 128))
			body := append(append([]byte(nil), title...), cover...)
			if before {
				body = append(append([]byte(nil), cover...), title...)
			}
			body = append(body, frame("TPE1", []byte("\x03Artist"))...)
			body = append(body, frame("TALB", []byte("\x03Album"))...)
			var encoded []byte
			for i, b := range body {
				encoded = append(encoded, b)
				if b == 255 && (i+1 == len(body) || body[i+1] == 0 || body[i+1] >= 224) {
					encoded = append(encoded, 0)
				}
			}
			n := len(encoded)
			header := []byte{'I', 'D', '3', 3, 0, 128, byte(n>>21) & 127, byte(n>>14) & 127, byte(n>>7) & 127, byte(n) & 127}
			data := append(append([]byte(nil), header...), encoded...)
			data = append(data, 0xff, 0xfb, 0x90, 0)
			got := probeTestFile(t, "unsynchronized.mp3", data)
			if got.Metadata != (Metadata{"Tÿà", "Artist", "Album"}) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}
func TestProbeMP4LowercaseArtist(t *testing.T) {
	mvhd := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhd[12:], 1000)
	binary.BigEndian.PutUint32(mvhd[16:], 1000)
	artist := probeAtom("\xa9art", probeAtom("data", append(make([]byte, 8), "Artist"...)))
	meta := append(make([]byte, 4), probeAtom("ilst", artist)...)
	moov := append(probeAtom("mvhd", mvhd), probeAtom("udta", probeAtom("meta", meta))...)
	data := append(probeAtom("ftyp", []byte("M4A \x00\x00\x00\x00")), probeAtom("moov", moov)...)
	if got := probeTestFile(t, "artist.m4a", data); got.Artist != "Artist" {
		t.Fatalf("got %+v", got)
	}
}
func TestProbeID3v24FrameUnsynchronization(t *testing.T) {
	// UTF-16LE BOM FF FE is stuffed to FF 00 FE in the physical frame.
	body := []byte{'T', 'I', 'T', '2', 0, 0, 0, 6, 0, 2, 1, 255, 0, 254, 'T', 0}
	header := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, byte(len(body))}
	data := append(append([]byte(nil), header...), body...)
	data = append(data, 0xff, 0xfb, 0x90, 0)
	if got := probeTestFile(t, "frame.mp3", data); got.Title != "T" {
		t.Fatalf("got %+v", got)
	}
}

func TestProbeUnsynchronizedHugeCoverKeepsEarlierTextAndAudioHeader(t *testing.T) {
	title := append([]byte("TIT2"), probeBE32(6)...)
	title = append(title, 0, 0, 3)
	title = append(title, "Title"...)
	coverSize := 8 << 20
	cover := append([]byte("APIC"), probeBE32(uint32(coverSize))...)
	cover = append(cover, 0, 0)
	n := len(title) + len(cover) + coverSize
	header := []byte{'I', 'D', '3', 3, 0, 128, byte(n>>21) & 127, byte(n>>14) & 127, byte(n>>7) & 127, byte(n) & 127}
	path := filepath.Join(t.TempDir(), "huge.mp3")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(append(append(header, title...), cover...)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt([]byte{255, 251, 144, 0}, int64(n+10)); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := NewDecoder().Probe(context.Background(), path)
	if err != nil || got.Title != "Title" {
		t.Fatalf("got %+v, %v", got, err)
	}
}
