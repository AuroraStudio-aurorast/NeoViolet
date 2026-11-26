package format

import (
	"io"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/format/apetag"
)

// id3v1TagSize is the fixed size of a trailing ID3v1 tag.
const id3v1TagSize = 128

// decodeMP3 decodes MP3 audio with any trailing ID3v1/APEv2 metadata hidden
// from the decoder.
//
// go-mp3 (behind beep's mp3 package) skips a leading ID3v2 tag and a trailing
// ID3v1 tag but knows nothing about APEv2. Past the last audio frame it leaves
// the frame grid and resynchronises byte by byte, reading tag bytes — embedded
// cover art above all — as frame headers. A run that parses as a free-format
// header (bitrate index 0) aborts the decode with "free bitrate format is not
// supported"; the runs that do parse inflate the frame table beep reports as
// Len, i.e. the duration and the seek range. Bounding the stream to the audio
// prevents both.
func decodeMP3(rc io.ReadCloser) (beep.StreamSeekCloser, beep.Format, error) {
	rs, ok := rc.(io.ReadSeeker)
	if !ok {
		return mp3.Decode(rc)
	}
	if tagStart, trailing := mp3TrailingTagStart(rs); trailing {
		rc = &audioBoundedReader{rs: rs, closer: rc, limit: tagStart}
	}
	stream, format, err := mp3.Decode(rc)
	if err != nil {
		return nil, format, err
	}
	return &mp3Stream{StreamSeekCloser: stream, length: stream.Len()}, format, nil
}

// go-mp3 tries to decode a frame even when seeking to the end of the stream.
// Represent that position without asking the decoder to read past the audio.
type mp3Stream struct {
	beep.StreamSeekCloser
	length int
	atEnd  bool
}

func (s *mp3Stream) Seek(position int) error {
	if s.length > 0 && position == s.length {
		s.atEnd = true
		return nil
	}
	if err := s.StreamSeekCloser.Seek(position); err != nil {
		return err
	}
	s.atEnd = false
	return nil
}

func (s *mp3Stream) Position() int {
	if s.atEnd {
		return s.length
	}
	return s.StreamSeekCloser.Position()
}

func (s *mp3Stream) Stream(samples [][2]float64) (int, bool) {
	if s.atEnd {
		return 0, false
	}
	return s.StreamSeekCloser.Stream(samples)
}

// mp3TrailingTagStart returns the offset at which trailing ID3v1 or APEv2
// metadata starts in a seekable MP3 stream, and whether there is any.
func mp3TrailingTagStart(rs io.ReadSeeker) (int64, bool) {
	pos, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, false
	}
	defer func() { _, _ = rs.Seek(pos, io.SeekStart) }()

	size, err := rs.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, false
	}
	end := size
	if start, ok := apetag.TrailingTagStart(rs, size); ok {
		end = start // an APEv2 tag sits before the optional ID3v1 tag
	} else if hasID3v1Tag(rs, size) {
		end = size - id3v1TagSize
	}
	if end <= 0 || end >= size {
		return 0, false
	}
	return end, true
}

// hasID3v1Tag reports whether the stream ends with a 128-byte ID3v1 tag.
func hasID3v1Tag(rs io.ReadSeeker, size int64) bool {
	if size < id3v1TagSize {
		return false
	}
	var magic [3]byte
	if _, err := rs.Seek(size-id3v1TagSize, io.SeekStart); err != nil {
		return false
	}
	if _, err := io.ReadFull(rs, magic[:]); err != nil {
		return false
	}
	return string(magic[:]) == "TAG"
}

// audioBoundedReader presents only [0, limit) of the stream it wraps, so
// trailing metadata is never scanned. Seek is forwarded untouched: absolute
// audio offsets stay valid because only the tail is hidden.
type audioBoundedReader struct {
	rs     io.ReadSeeker
	closer io.Closer
	limit  int64
	pos    int64
}

func (r *audioBoundedReader) Read(p []byte) (int, error) {
	if r.pos >= r.limit {
		return 0, io.EOF
	}
	if int64(len(p)) > r.limit-r.pos {
		p = p[:r.limit-r.pos]
	}
	n, err := r.rs.Read(p)
	r.pos += int64(n)
	return n, err
}

func (r *audioBoundedReader) Seek(offset int64, whence int) (int64, error) {
	pos, err := r.rs.Seek(offset, whence)
	if err != nil {
		return pos, err
	}
	r.pos = pos
	return pos, nil
}

func (r *audioBoundedReader) Close() error { return r.closer.Close() }
