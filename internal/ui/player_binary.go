package ui

import (
	"fmt"
	"os"
)

func validatePlayerBinary(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !isPlayerBinary(path, info) {
		return fmt.Errorf("choose an executable player file")
	}
	return nil
}
