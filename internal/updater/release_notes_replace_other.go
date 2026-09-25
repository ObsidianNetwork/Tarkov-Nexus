//go:build !windows

package updater

import (
	"os"
	"path/filepath"
)

func replaceNotesFile(tempPath, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
