//go:build !windows

package ui

import "os"

func isPlayerBinary(_ string, info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}
