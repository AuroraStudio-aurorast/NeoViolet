// Package cmd contains the command-line entry points for neoviolet.
package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestVersionCommandOutput(t *testing.T) {
	// versionCmd's RunE writes via fmt.Println to os.Stdout (see version.go),
	// not through cobra's output writer, so capture the real stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	rootCmd.SetArgs([]string{"version"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command: %v", err)
	}

	// Restore stdout and close the write end so ReadAll gets EOF.
	os.Stdout = oldStdout
	_ = w.Close()
	out, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if !bytes.Contains(out, []byte("neoviolet")) {
		t.Errorf("version output = %q, want prefix 'neoviolet'", string(out))
	}
}
