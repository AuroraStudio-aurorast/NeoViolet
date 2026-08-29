package audio

import (
	"bytes"
	"fmt"
	"os"

	"github.com/gopxl/beep/v2/speaker"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/cover"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// readSeekCloser wraps *bytes.Reader to implement both io.ReadSeeker and io.ReadCloser.
// This is needed by decoders that require Seek for Len() computation (e.g. MP3)
// while also needing Close().
type readSeekCloser struct {
	*bytes.Reader
}

func (r *readSeekCloser) Close() error { return nil }

// OpenReader opens audio from an in-memory byte buffer (e.g. from stdin).
// name is a display label (e.g. "stdin") shown in the UI.
// For formats that require a file path (APE, MIDI, tracker), the data is
// transparently written to a temporary file and opened via the normal Open path.
func (p *Player) OpenReader(name string, data []byte) error {
	logger.Debug("Player.OpenReader", "name", name, "size", len(data))

	if len(data) == 0 {
		return fmt.Errorf("stdin is empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Detect format from magic bytes
	ext, detectErr := p.decoder.DetectFormatFromBytes(data)
	synthExt := ext
	if detectErr != nil {
		// Can't detect — attempt to pick a reasonable default or error out
		return fmt.Errorf("stdin: %w", detectErr)
	}

	// Synthetic formats (MIDI, tracker) and APE require a file path.
	// Write to a temp file and delegate to normal Open.
	if isSyntheticFormat(synthExt) || ext == ".ape" {
		tmpFile, err := os.CreateTemp("", "neoviolet-stdin-*"+ext)
		if err != nil {
			return fmt.Errorf("create temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		if _, err := tmpFile.Write(data); err != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("write temp file: %w", err)
		}
		if err := tmpFile.Close(); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("close temp file: %w", err)
		}
		// Track for cleanup
		p.tempFiles = append(p.tempFiles, tmpPath)
		p.mu.Unlock()
		err = p.Open(tmpPath)
		p.mu.Lock()
		// Override the path to "stdin" for display purposes
		p.path = name
		return err
	}

	// Standard audio formats: decode from memory
	if p.isPlaying {
		speaker.Clear()
		p.isPlaying = false
	}
	if p.streamer != nil && p.file != nil {
		_ = p.file.Close()
	}

	// Read metadata from the buffer FIRST, before decoding.
	// Use a separate reader so we never touch the decoder's internal reader.
	metaReader := bytes.NewReader(data)
	metadata := p.tagReader.ReadFromSeeker(metaReader)
	p.title = metadata.Title
	p.artist = metadata.Artist
	p.album = metadata.Album

	// Extract cover art from the buffer (using another reader).
	coverReader := bytes.NewReader(data)
	img, err := cover.ExtractFromReader(coverReader)
	if err == nil {
		p.coverImage = img
	}

	// Now decode — use its own fresh *bytes.Reader so the decoder has
	// exclusive ownership and its internal buffers are never corrupted.
	// The wrapper implements both io.ReadSeeker (for Len()) and io.ReadCloser.
	decReader := &readSeekCloser{Reader: bytes.NewReader(data)}
	streamer, format, err := p.decoder.DecodeFromReader(decReader, ext)
	if err != nil {
		return fmt.Errorf("decode stdin: %w", err)
	}

	if err := ensureSpeakerInit(format.SampleRate); err != nil {
		return fmt.Errorf("speaker init failed: %w", err)
	}

	ctrlStreamer := resampleIfNeeded(streamer, format)

	logger.Info("Audio loaded from stdin", "name", name, "format", format.SampleRate)

	p.setupStreamer(streamer, format, decReader, name, ctrlStreamer)

	// If no title detected from tags, use the display name
	if p.title == "" {
		p.title = name
	}

	return nil
}
