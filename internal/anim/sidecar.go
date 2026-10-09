package anim

import (
	"os"
	"path/filepath"
)

// sidecarExt is the extension an animation sidecar carries.
const sidecarExt = ".nvaa"

// FindSidecar returns the path of the animation beside an audio file, or "" when
// the track has none.
//
// An animation belongs to one track and is found the way its lyrics are: same
// directory, same base name, different extension. So "Midnight Drive.flac" looks
// for "Midnight Drive.nvaa". Only the file's existence is established here; the
// contents are not read until something asks for them.
func FindSidecar(audioPath string) string {
	if audioPath == "" {
		return ""
	}

	ext := filepath.Ext(audioPath)
	path := audioPath[:len(audioPath)-len(ext)] + sidecarExt

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return ""
	}
	return path
}
