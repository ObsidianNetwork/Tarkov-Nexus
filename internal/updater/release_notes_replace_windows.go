//go:build windows

package updater

import "golang.org/x/sys/windows"

func replaceNotesFile(tempPath, targetPath string) error {
	temp, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(temp, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
