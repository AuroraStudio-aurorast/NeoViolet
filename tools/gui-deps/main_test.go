package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractRejectsEscapingPathsAndSymlinks(t *testing.T) {
	for _, test := range []struct {
		name string
		kind byte
	}{
		{"package/../escape.rs", tar.TypeReg},
		{"package/src/link", tar.TypeSymlink},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "source.tar.gz")
			// #nosec G304 -- the archive fixture lives exclusively under t.TempDir().
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			compressed := gzip.NewWriter(file)
			writer := tar.NewWriter(compressed)
			if err := writer.WriteHeader(&tar.Header{Name: test.name, Typeflag: test.kind, Linkname: "../../escape", Mode: 0644}); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := compressed.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := extract(archive, filepath.Join(dir, "output")); err == nil {
				t.Fatal("accepted unsafe archive")
			}
		})
	}
}

func TestDownloadOnlyPublishesVerifiedContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("source archive"))
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "source.tar.gz")
	if err := download(server.URL, destination, "incorrect"); err == nil {
		t.Fatal("accepted wrong checksum")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("published unverified content")
	}
	hash := sha256.Sum256([]byte("source archive"))
	if err := download(server.URL, destination, hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	if !verified(destination, hex.EncodeToString(hash[:])) {
		t.Fatal("verified content changed")
	}
}
