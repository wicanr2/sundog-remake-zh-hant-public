//go:build !windows

package main

// Unix volumes are reachable by walking the root directory.
func mouseDriveRoots() ([]string, error) { return nil, nil }
