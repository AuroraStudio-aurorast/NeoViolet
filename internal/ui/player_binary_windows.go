package ui

import (
	"os"
	"path/filepath"
	"strings"
)

func isPlayerBinary(path string, info os.FileInfo) bool {
	return info.Mode().IsRegular() && strings.EqualFold(filepath.Ext(path), ".exe")
}
