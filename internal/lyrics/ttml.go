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

// Parse reads TTML through go-amll-ttml-parser and maps it onto Data. The library
// owns XML parsing, time resolution and text derivation; the size limit and the
// sentinel errors below belong to this package.
func (p *ttmlParser) Parse(r io.Reader, sourcePath string) (*Data, error) {
	// This is the 1 MB ceiling (ErrLyricTooLarge) and it runs first, so the
	// library's WithMaxBytes below is a backstop that cannot fire today.
	data, err := readAllWithLimit(r)
	if err != nil {
		return nil, err
	}

	doc, err := amllttml.ParseReader(bytes.NewReader(data),
		amllttml.WithMaxBytes(maxLyricSize),
		// A <p> without itunes:key is still a lyric line: most non-AMLL TTML has
		// no key, and dropping those lines would empty whole files.
		amllttml.WithMissingLineKey(amllttml.MissingKeyKeep),
	)
	if err != nil {
		return nil, ttmlParseError(err)
	}

	// Diagnostics never change the outcome: Parse fails only on XML-level
	// problems. Log them and keep reading the file.
	logTTMLDiagnostics(doc)

	// Zero displayable lines is "no lyrics", not an empty song: the registry
	// accepts any non-error result, so an empty song.ttml would otherwise shadow a
	// real song.lrc next to it. The count is taken after the adapter dropped every
	// <p> that maps to nothing.
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

// logTTMLDiagnostics records the library's findings without affecting the result.
// It reads the public Diagnostics() surface on purpose: parse-time findings also
// live in doc.Diags, but validation-layer ones (notably bad-time-syntax for an
// unsupported clock frame) are visible only there.
//
// An Error-severity diagnostic is not a parse failure, so it is logged at Warn
// and the file is still read; everything else is logged at Debug.
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
