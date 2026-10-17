//go:build windows

package main

import "golang.org/x/sys/windows"

// GetLogicalDrives lists volumes without probing disconnected network paths.
func mouseDriveRoots() ([]string, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	var roots []string
	for i := 0; i < 26; i++ {
		if mask&(1<<i) != 0 {
			roots = append(roots, string(rune('A'+i))+":\\")
		}
	}
	return roots, nil
}
