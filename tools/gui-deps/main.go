// Command gui-deps prepares checksum-pinned GUI sources and applies the local patches.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type source struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Patch  string `json:"patch"`
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	if err := prepare(*root); err != nil {
		fmt.Fprintln(os.Stderr, "GUI dependencies:", err)
		os.Exit(1)
	}
}

func prepare(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	gui := filepath.Join(root, "tools", "neoviolet-gui")
	deps := filepath.Join(gui, "deps")
	// #nosec G304 -- root is the chosen checkout; the manifest has a fixed repository path.
	manifest, err := os.ReadFile(filepath.Join(deps, "sources.json"))
	if err != nil {
		return err
	}
	var sources []source
	if err := json.Unmarshal(manifest, &sources); err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	cache = filepath.Join(cache, "neoviolet", "gui-deps")
	if err := os.MkdirAll(cache, 0700); err != nil {
		return err
	}
	generated := filepath.Join(root, ".build", "gui-deps")
	if err := os.MkdirAll(generated, 0700); err != nil {
		return err
	}
	for _, source := range sources {
		if err := prepareSource(root, generated, deps, cache, source); err != nil {
			return fmt.Errorf("%s: %w", source.Name, err)
		}
	}
	return nil
}

func prepareSource(root, generated, deps, cache string, source source) error {
	if !filepath.IsLocal(source.Name) || strings.ContainsAny(source.Name, `/\`) ||
		!filepath.IsLocal(source.Patch) || len(source.SHA256) != 64 {
		return errors.New("invalid source manifest")
	}
	// #nosec G304 -- Patch is validated as local and comes from the tracked source manifest.
	patch, err := os.ReadFile(filepath.Join(deps, source.Patch))
	if err != nil {
		return err
	}
	identity := sha256.Sum256(append([]byte(source.SHA256), patch...))
	stamp := hex.EncodeToString(identity[:])
	destination := filepath.Join(generated, source.Name)
	stampPath := filepath.Join(destination, ".neoviolet-source")
	// #nosec G304 -- stampPath stays beneath the validated generated dependency directory.
	if contents, err := os.ReadFile(stampPath); err == nil && string(contents) == stamp {
		return nil
	}
	lock := destination + ".lock"
	deadline := time.Now().Add(2 * time.Minute)
	for {
		err := os.Mkdir(lock, 0700)
		if err == nil {
			break
		}
		if !os.IsExist(err) || time.Now().After(deadline) {
			return fmt.Errorf("acquire source preparation lock: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer func() { _ = os.Remove(lock) }()
	// #nosec G304 -- the stamp is a fixed file under the generated dependency directory.
	if contents, err := os.ReadFile(stampPath); err == nil && string(contents) == stamp {
		return nil
	}
	archive := filepath.Join(cache, source.SHA256+".tar.gz")
	if !verified(archive, source.SHA256) {
		fmt.Printf("Fetching GUI dependency %s\n", source.Name)
		if err := download(source.URL, archive, source.SHA256); err != nil {
			return err
		}
	}
	stage, err := os.MkdirTemp(generated, ".prepare-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if err := extract(archive, stage); err != nil {
		return err
	}
	relative, err := filepath.Rel(root, stage)
	if err != nil {
		return err
	}
	// #nosec G204 -- fixed executable; paths select a temporary directory and a tracked local patch.
	command := exec.Command("git", "apply", "--whitespace=nowarn", "--directory="+filepath.ToSlash(relative), filepath.Join(deps, source.Patch))
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("apply pinned patch: %w: %s", err, output)
	}
	// #nosec G703 -- stage is created by MkdirTemp in the checkout's fixed dependency cache.
	if err := os.WriteFile(filepath.Join(stage, ".neoviolet-source"), []byte(stamp), 0600); err != nil {
		return err
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return err
	}
	fmt.Printf("Prepared GUI dependency %s\n", source.Name)
	return nil
}

func verified(path, checksum string) bool {
	// #nosec G304 -- callers select cache/temp files; their content must match the pinned digest.
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	return hex.EncodeToString(hash.Sum(nil)) == checksum
}

func download(url, destination, checksum string) error {
	client := http.Client{Timeout: 90 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", response.Status)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".download-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	_, copyErr := io.Copy(temporary, io.LimitReader(response.Body, 100<<20))
	closeErr := temporary.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !verified(temporary.Name(), checksum) {
		return errors.New("archive SHA-256 mismatch")
	}
	return os.Rename(temporary.Name(), destination)
}

func extract(archive, destination string) error {
	// #nosec G304 -- archive is a checksum-verified package in the user cache.
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = compressed.Close() }()
	reader := tar.NewReader(compressed)
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		parts := strings.SplitN(strings.TrimSuffix(header.Name, "/"), "/", 2)
		if len(parts) != 2 {
			continue
		}
		name := parts[1]
		if !filepath.IsLocal(name) || strings.Contains(name, `\`) {
			return errors.New("archive contains an unsafe path")
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return errors.New("archive contains a non-regular file")
		}
		// Keep package inputs; upstream demos and screenshots are not build dependencies.
		first := strings.SplitN(name, "/", 2)[0]
		if first != "src" && first != "examples" && first != "tests" &&
			first != "Cargo.toml" && first != "build.rs" && first != "LICENSE" && first != "rustfmt.toml" {
			continue
		}
		total += header.Size
		if header.Size < 0 || total > 100<<20 {
			return errors.New("archive exceeds source size limit")
		}
		path := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		// #nosec G304 -- name is local, links are rejected, and O_EXCL prevents replacing a path.
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(output, reader, header.Size)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
