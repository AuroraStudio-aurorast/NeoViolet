package synth

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
)

const (
	// minimalMODHeaderSize is the fixed ProTracker header: a 20 byte title, 31
	// sample headers of 30 bytes, the song length, the restart position, the
	// 128 entry order list and the "M.K." magic.
	minimalMODHeaderSize = 1084
	// minimalMODSampleBytes is 32 words of 8 bit sample data.
	minimalMODSampleBytes = 64
	// minimalMODPatternSize is one pattern: 64 rows, 4 channels, 4 bytes an entry.
	minimalMODPatternSize = 64 * 4 * 4
	// minimalMODSampleOffset is where the pattern ends and the sample begins.
	minimalMODSampleOffset = minimalMODHeaderSize + minimalMODPatternSize
	// minimalMODFixture is the generated module kept on disk so other packages
	// can play the same bytes without rebuilding them.
	minimalMODFixture = "testdata/minimal.mod"
)

// minimalMOD builds a minimal but valid ProTracker module: one 64 byte sample,
// one pattern holding a single note on its first row and a 64 row song. The
// bytes are generated rather than checked in as an opaque binary so a reader
// can see what "a playable module" means here.
func minimalMOD() []byte {
	mod := make([]byte, minimalMODSampleOffset+minimalMODSampleBytes)
	copy(mod, "minimal test module")

	// Sample 1, 30 byte header at offset 20: a 22 byte name, the length in
	// words, finetune, volume and the loop bounds, also in words. Zero loop
	// bounds mean the sample is not looped.
	const sampleHeader = 20
	copy(mod[sampleHeader:], "one shot")
	binary.BigEndian.PutUint16(mod[sampleHeader+22:], minimalMODSampleBytes/2)
	mod[sampleHeader+24] = 0  // finetune
	mod[sampleHeader+25] = 64 // volume
	binary.BigEndian.PutUint16(mod[sampleHeader+26:], 0)

	// Song: a single order entry pointing at pattern 0.
	mod[950] = 1 // song length
	mod[951] = 0 // restart position
	copy(mod[1080:], "M.K.")

	// Pattern 0, row 0, channel 0: play sample 1 at period 428 (C-3). The high
	// nibble of the sample number shares the first byte with the period.
	const (
		period          = 428
		sampleNumber    = 1
		patternOffset   = minimalMODHeaderSize
		patternEntryLen = 4
	)
	entry := mod[patternOffset : patternOffset+patternEntryLen]
	entry[0] = byte(sampleNumber>>4)<<4 | byte(period>>8)
	entry[1] = byte(period & 0xff)
	entry[2] = byte(sampleNumber&0x0f) << 4
	entry[3] = 0

	// 8 bit signed sample data: a square wave, so the module renders something
	// a listener could hear rather than silence.
	sample := mod[minimalMODSampleOffset:]
	for i := range sample {
		if i < len(sample)/2 {
			sample[i] = 100
			continue
		}
		sample[i] = 156 // -100 as a signed byte
	}
	return mod
}

// writeMinimalMOD writes the generated module into dir and returns its path.
func writeMinimalMOD(t *testing.T, dir string) string {
	t.Helper()
	return writeModuleBytes(t, filepath.Join(dir, "minimal.mod"), minimalMOD())
}

// writeModuleBytes writes data to path, creating the parent directory.
func writeModuleBytes(t *testing.T, path string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// writeMinimalMODFixture leaves one copy of the generated module in testdata
// for the tests in other packages that play the same bytes. An existing copy is
// compared with the generator, so a drifted fixture fails here instead of
// quietly being played by those tests.
func writeMinimalMODFixture(t *testing.T) {
	t.Helper()
	generated := minimalMOD()

	if _, err := os.Stat(minimalMODFixture); err != nil {
		writeModuleBytes(t, minimalMODFixture, generated)
		return
	}

	onDisk, err := os.ReadFile(minimalMODFixture)
	if err != nil {
		t.Fatalf("read %s: %v", minimalMODFixture, err)
	}
	if !bytes.Equal(onDisk, generated) {
		t.Errorf("%s drifted from minimalMOD(): %d bytes on disk, %d generated, first difference at offset %d",
			minimalMODFixture, len(onDisk), len(generated), firstDifference(onDisk, generated))
	}
}

// firstDifference reports where two byte slices stop matching: the first
// differing offset, or the length they share when one is a prefix of the other.
func firstDifference(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// requireOpenmpt skips a test that only means something with a real library.
func requireOpenmpt(t *testing.T) {
	t.Helper()
	resetOpenmptState()
	if !OpenmptAvailable() {
		t.Skip("libopenmpt not installed")
	}
}

// newMinimalMODPlayer opens the generated module from a temporary directory.
func newMinimalMODPlayer(t *testing.T) *OpenmptPlayer {
	t.Helper()
	p, err := NewOpenmptPlayer(writeMinimalMOD(t, t.TempDir()), beep.SampleRate(44100))
	if err != nil {
		t.Fatalf("NewOpenmptPlayer(minimal module): %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestOpenmptProbeShortInput(t *testing.T) {
	// The probe reads a header window out of the caller's slice, so anything
	// shorter than a header — including nothing at all — must answer false
	// instead of indexing past the end.
	cases := []struct {
		name string
		data []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"one byte", []byte{0}},
		{"magic only", []byte("M.K.")},
	}
	for _, tc := range cases {
		if OpenmptProbe(tc.data) {
			t.Errorf("OpenmptProbe(%s) = true, want false", tc.name)
		}
	}
}

func TestOpenmptSupportedFormatsNoLib(t *testing.T) {
	t.Setenv(openmptEnvOverride, "/nonexistent/libopenmpt.0.dylib")
	resetOpenmptState()
	defer resetOpenmptState()

	if formats := OpenmptSupportedFormats(); formats != nil {
		t.Errorf("OpenmptSupportedFormats() = %q, want nil when libopenmpt is unavailable", formats)
	}
}

func TestNewOpenmptPlayerEmptyFile(t *testing.T) {
	// An empty file has no bytes to hand to the library: this must be an error
	// rather than a pointer into an empty slice.
	path := writeModuleBytes(t, filepath.Join(t.TempDir(), "empty.mod"), nil)

	p, err := NewOpenmptPlayer(path, beep.SampleRate(44100))
	if err == nil {
		_ = p.Close()
		t.Error("NewOpenmptPlayer(empty file) = nil error, want an error")
	}
}

func TestNewOpenmptPlayerTruncated(t *testing.T) {
	// Below a complete header there is nothing for libopenmpt to read, so those
	// inputs must come back as an error rather than as a pointer into a short
	// slice. From a complete header on, the library zero-pads whatever the file
	// is missing and may well accept the module; what must never happen is a
	// panic, a hang or a player with no duration.
	cases := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"empty", 0, true},
		{"one byte", 1, true},
		{"no header", 200, true},
		{"header only", minimalMODHeaderSize, false},
		{"partial pattern", minimalMODHeaderSize + 16, false},
		{"pattern, no sample data", minimalMODSampleOffset, false},
		{"partial sample data", minimalMODSampleOffset + 1, false},
		{"one byte short", minimalMODSampleOffset + minimalMODSampleBytes - 1, false},
	}
	mod := minimalMOD()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.size > len(mod) {
				t.Fatalf("truncating to %d bytes is not a truncation of a %d byte module", tc.size, len(mod))
			}
			path := writeModuleBytes(t, filepath.Join(t.TempDir(), "truncated.mod"), mod[:tc.size])

			p, err := NewOpenmptPlayer(path, beep.SampleRate(44100))
			if err != nil {
				if !tc.wantErr {
					t.Logf("NewOpenmptPlayer(%d byte module) = %v", tc.size, err)
				}
				return
			}
			defer func() { _ = p.Close() }()

			if tc.wantErr {
				t.Errorf("NewOpenmptPlayer(%d byte module) = nil error, want an error", tc.size)
				return
			}
			if d := p.Duration(); d <= 0 {
				t.Errorf("NewOpenmptPlayer(%d byte module) opened with Duration() = %v", tc.size, d)
			}

			// Rendering an accepted fragment must not hang or read past the
			// module the library built from it.
			if err := p.Play(); err != nil {
				t.Fatalf("Play(): %v", err)
			}
			frames := make([][2]float64, 512)
			if n, ok := p.Stream(frames); n != len(frames) || !ok {
				t.Errorf("Stream() = %d, %v, want %d, true", n, ok, len(frames))
			}
		})
	}
}

func TestNewOpenmptPlayerMissingFile(t *testing.T) {
	p, err := NewOpenmptPlayer(filepath.Join(t.TempDir(), "absent.mod"), beep.SampleRate(44100))
	if err == nil {
		_ = p.Close()
		t.Error("NewOpenmptPlayer(missing file) = nil error, want an error")
	}
}

func TestOpenmptPlayerPlaysGeneratedModule(t *testing.T) {
	requireOpenmpt(t)
	writeMinimalMODFixture(t)

	mod := minimalMOD()
	if !OpenmptProbe(mod) {
		t.Fatal("OpenmptProbe(minimalMOD()) = false, want true")
	}

	formats := OpenmptSupportedFormats()
	for _, want := range []string{"mod", "xm", "s3m", "it"} {
		if !slices.Contains(formats, want) {
			t.Errorf("OpenmptSupportedFormats() = %q, want it to contain %q", formats, want)
		}
	}

	p := newMinimalMODPlayer(t)

	if d := p.Duration(); d <= 0 {
		t.Errorf("Duration() = %v, want a positive duration", d)
	}
	if p.Position() != 0 {
		t.Errorf("Position() = %v, want 0 before playback", p.Position())
	}
	if err := p.Play(); err != nil {
		t.Fatalf("Play(): %v", err)
	}

	frames := make([][2]float64, 512)
	n, ok := p.Stream(frames)
	if n == 0 || !ok {
		t.Errorf("Stream() = %d, %v, want rendered frames", n, ok)
	}
	if p.Position() <= 0 {
		t.Errorf("Position() = %v after Stream, want it to advance", p.Position())
	}

	// Close releases the module handle and is safe to repeat (as is the
	// deferred close in newMinimalMODPlayer).
	if err := p.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if err := p.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

func TestOpenmptPlayerSeekAfterClose(t *testing.T) {
	requireOpenmpt(t)

	p := newMinimalMODPlayer(t)
	frames := make([][2]float64, 512)
	if err := p.Play(); err != nil {
		t.Fatalf("Play(): %v", err)
	}
	if n, _ := p.Stream(frames); n == 0 {
		t.Fatal("Stream() returned no frames before Close")
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	// The module handle is gone: seeking must report that instead of handing
	// the freed pointer back to the library.
	if err := p.Seek(time.Second); err == nil {
		t.Error("Seek() after Close = nil, want an error")
	}

	n, ok := p.Stream(frames)
	if n != 0 || ok {
		t.Errorf("Stream() after Close = %d, %v, want 0, false", n, ok)
	}
}

func TestOpenmptPlayerSeekBeyondDuration(t *testing.T) {
	requireOpenmpt(t)

	p := newMinimalMODPlayer(t)
	duration := p.Duration()
	if duration <= 0 {
		t.Fatalf("Duration() = %v, want a positive duration", duration)
	}
	if err := p.Play(); err != nil {
		t.Fatalf("Play(): %v", err)
	}

	// Past the end is a caller mistake the library clamps rather than rejects,
	// so either answer is fine — a panic or a hang is not.
	if err := p.Seek(10 * duration); err != nil {
		t.Logf("Seek(10 * duration) = %v", err)
	}

	frames := make([][2]float64, 512)
	n, ok := p.Stream(frames)
	if n != len(frames) || !ok {
		t.Errorf("Stream() after seeking past the end = %d, %v, want %d, true", n, ok, len(frames))
	}
}

// The streaming races below render long calls (a second of audio each: dozens
// of library block renders) rather than short ones, so the control call each
// test makes provably lands inside a render that already passed the closed
// check instead of racing the goroutine's startup. The buffers they use are
// allocated before the goroutine signals, so nothing but the render stands
// between that signal and the control call.
const (
	// streamRaceRounds repeats the close race: one round already overlaps, the
	// repeats cover scheduling luck, and Close is final so each round needs a
	// fresh player.
	streamRaceRounds = 4
	// streamLongFrames is one long Stream call, and the two call counts keep the
	// player rendering across a control call without waiting on wall clock time.
	streamLongFrames = 44100
	streamLongCalls  = 16
	streamShortCalls = 200
)

// streamWhileControl starts a goroutine that renders into the player and runs
// control from the caller's goroutine while that render is under way. The
// streaming goroutine signals from inside its loop, so the overlap is sequenced
// by channels rather than by sleeping, and every wait is bounded so a deadlock
// fails the test instead of hanging it. The player stays open for the caller to
// assert on.
func streamWhileControl(t *testing.T, control func(*OpenmptPlayer)) *OpenmptPlayer {
	t.Helper()
	requireOpenmpt(t)

	p := newMinimalMODPlayer(t)
	if err := p.Play(); err != nil {
		t.Fatalf("Play(): %v", err)
	}

	long := make([][2]float64, streamLongFrames)
	short := long[:512]
	started := make(chan struct{})
	entered := make(chan struct{})
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		<-started
		// A short call first: once it returns, the goroutine is demonstrably
		// inside the render path, so the long calls below start before control.
		p.Stream(short)
		close(entered)
		for range streamLongCalls {
			p.Stream(long)
		}
		for range streamShortCalls {
			p.Stream(short)
		}
	}()

	close(started)
	waitForClosed(t, entered, "the streaming goroutine to enter its render loop")
	control(p)
	waitForClosed(t, streamDone, "the streaming goroutine to finish")

	return p
}

// TestOpenmptPlayerStreamCloseRace pins the concurrency contract of the module
// handle: Close frees it, so a render that is already under way must not hand
// the library that freed handle, and the render state the two paths share must
// not race. Run with -race.
func TestOpenmptPlayerStreamCloseRace(t *testing.T) {
	for round := range streamRaceRounds {
		p := streamWhileControl(t, func(p *OpenmptPlayer) {
			if err := p.Close(); err != nil {
				t.Errorf("round %d: Close() while streaming = %v, want nil", round, err)
			}
		})

		if n, ok := p.Stream(make([][2]float64, 512)); n != 0 || ok {
			t.Errorf("round %d: Stream() after Close = %d, %v, want 0, false", round, n, ok)
		}
	}
}

// TestOpenmptPlayerStreamStopAndSeekRace drives the other writers of the
// rendering state while a render is under way: Stop rewinds renderSamples,
// renderPos and the playback flags, and Seek resets the rendered buffer and the
// module position. Both take the same lock Stream holds for its whole call. Run
// with -race.
func TestOpenmptPlayerStreamStopAndSeekRace(t *testing.T) {
	cases := []struct {
		name    string
		control func(t *testing.T, p *OpenmptPlayer)
	}{
		{
			name: "stop",
			control: func(_ *testing.T, p *OpenmptPlayer) {
				p.Stop()
			},
		},
		{
			name: "seek",
			control: func(t *testing.T, p *OpenmptPlayer) {
				if err := p.Seek(time.Second); err != nil {
					t.Errorf("Seek() while streaming = %v, want nil", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := streamWhileControl(t, func(p *OpenmptPlayer) { tc.control(t, p) })

			// The player survives either rewind and renders again afterwards.
			if err := p.Play(); err != nil {
				t.Fatalf("Play() after %s: %v", tc.name, err)
			}
			frames := make([][2]float64, 512)
			if n, ok := p.Stream(frames); n != len(frames) || !ok {
				t.Errorf("Stream() after %s = %d, %v, want %d, true", tc.name, n, ok, len(frames))
			}
		})
	}
}

// waitForClosed waits until ch is closed and fails the test rather than hanging
// if that never happens.
func waitForClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
