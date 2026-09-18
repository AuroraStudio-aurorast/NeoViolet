package lyrics

import (
	"bytes"
	"errors"
	"io"

	amllttml "github.com/WhatDamon/go-amll-ttml-parser"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

func init() {
	RegisterParser("ttml", &ttmlParser{})
}

type ttmlParser struct{}

func (p *ttmlParser) FindSidecar(audioPath string) string {
	return findSidecarWithExt(audioPath, ".ttml", ".xml")
}

// Parse reads TTML through go-amll-ttml-parser and maps the result onto Data.
// The hand-written XML structs, clock-time parsing and CJK spacing heuristics
// that used to live here are all gone: the library owns each of those concerns
// now (spec D1/D5), so this is a thin shell around ParseReader.
func (p *ttmlParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	// readAllWithLimit is the primary 1 MB ceiling (ErrLyricTooLarge). The
	// library's WithMaxBytes below is a second guard, never the first.
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, err
	}

	doc, err := amllttml.ParseReader(bytes.NewReader(data),
		amllttml.WithMaxBytes(maxLyricSize),
		// A <p> without itunes:key is still a lyric line (spec D3): most
		// non-AMLL TTML has no key, and dropping those lines would empty files.
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep),
	)
	if err != nil {
		return nil, ttmlParseError(err)
	}

	// Diagnostics never change success (spec D10): the library guarantees Parse
	// fails only on XML-level problems, and an Error-severity finding is still
	// just a finding. We log them for debugging and keep reading the file.
	logTTMLDiagnostics(doc)

	// A cleanly parsed document that maps to zero displayable lines is "no
	// lyrics", not an empty song: FindAndParsePreferred would otherwise let an
	// empty song.ttml shadow a real song.lrc sitting next to it (the registry
	// accepts any err == nil result). The count is taken AFTER the adapter
	// dropped every <p> that maps to nothing, not on the raw <p> count.
	d := ttmlToData(doc, sourcePath)
	if len(d.Lines) == 0 {
		return nil, ErrNoLyrics
	}

	return d, nil
}

// ttmlParseError maps the library's resource-limit errors back onto the sentinel
// errors the registry already understands. Everything else - malformed XML, a
// missing document element - is returned unchanged.
func ttmlParseError(err error) error {
	var perr *amllttml.Error
	if errors.As(err, &perr) && perr.Code == amllttml.CodeTooLarge {
		return ErrLyricTooLarge
	}
	return err
}

// logTTMLDiagnostics records the library's findings without affecting the
// result. It reads the public Diagnostics() surface on purpose: parse-time
// findings live in doc.Diags, but validation-layer findings (notably
// bad-time-syntax for an unsupported clock frame) are only visible there and
// never in Diags.
//
// Error-severity diagnostics are NOT a parse failure - the library keeps that
// guarantee - so they are logged at Warn and the file is still read; everything
// else is logged at Debug.
func logTTMLDiagnostics(doc *amllttml.Document) {
	for _, diag := range doc.Diagnostics() {
		keyvals := []any{
			"code", diag.Code.String(),
			"line", diag.Pos.Line,
			"col", diag.Pos.Col,
			"message", diag.Msg,
		}
		if diag.Severity == amllttml.SeverityError {
			logger.Warn("ttml diagnostic", keyvals...)
			continue
		}
		logger.Debug("ttml diagnostic", keyvals...)
	}
}
