package format

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

type interruptingDecodeReader struct {
	*bytes.Reader
	cancel          context.CancelFunc
	readAfterCancel bool
}

func (r *interruptingDecodeReader) Read(p []byte) (int, error) {
	if r.cancel == nil {
		r.readAfterCancel = true
		return r.Reader.Read(p)
	}
	n, err := r.Reader.Read(p)
	r.cancel()
	r.cancel = nil
	return n, err
}

func (*interruptingDecodeReader) Close() error { return nil }

func TestCachedMP2DecodeStopsReadingOnCancellation(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/test_mp2.mp2")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &interruptingDecodeReader{Reader: bytes.NewReader(bytes.Repeat(data, 8)), cancel: cancel}
	stream, _, err := NewDecoder().DecodeCachedAudio(ctx, r, ".mp2")
	if stream != nil {
		_ = stream.Close()
	}
	if !errors.Is(err, context.Canceled) || r.readAfterCancel {
		t.Fatalf("decode returned %v, read after cancellation: %v", err, r.readAfterCancel)
	}
}
