// Package appdir resolves where a packaged build finds its files.
//
// A build run from the repository has its assets under the working directory,
// which is what the flag defaults name. A packaged build is started from an
// arbitrary working directory — a desktop launcher, an AppImage, a .app bundle
// — so the same relative path has to be retried next to the executable before
// the program can be said to be missing its assets.
package appdir

import (
	"os"
	"path/filepath"
)

// Asset resolves a read-only file. The candidates are tried in order and the
// first one that exists wins; if none do, the original path is returned so the
// caller reports the name the user asked for rather than a rewritten one.
//
// The macOS candidate is the bundle layout: the executable lives in
// Contents/MacOS and its resources in Contents/Resources.
func Asset(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	if exists(path) {
		return path
	}
	base, err := executableDir()
	if err != nil {
		return path
	}
	for _, candidate := range []string{
		filepath.Join(base, path),
		filepath.Join(base, "..", "Resources", path),
	} {
		if exists(candidate) {
			return candidate
		}
	}
	return path
}

// UserData is where the game writes when the player did not name a path.
//
// It is deliberately not the working directory. A packaged build is started
// from wherever the launcher happened to be — the desktop, a downloads folder —
// and growing a workplace/ tree there is both surprising and, inside an AppImage
// mount or an installed .app bundle, impossible. Callers pass the file's name
// and get one stable place per file.
//
// fallback is returned when the platform has no user config directory, so the
// caller still has somewhere to write rather than an empty path.
func UserData(name, fallback string) string {
	config, err := os.UserConfigDir()
	if err != nil {
		return fallback
	}
	directory := filepath.Join(config, "sundog-remake")
	if !writableDir(directory) {
		return fallback
	}
	return filepath.Join(directory, name)
}

// UserDataDir is the directory UserData writes into. It is reported in the
// packaged README so a player can find or back up a save.
func UserDataDir() (string, bool) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(config, "sundog-remake"), true
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// writableDir reports whether a directory exists and accepts a new file, or can
// be created. Checking the permission bits is not enough — read-only mounts and
// container users both pass that check and then fail on the actual write.
func writableDir(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe, err := os.CreateTemp(dir, ".sundog-write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	probe.Close()
	os.Remove(name)
	return true
}

// ExecutableDir is the directory the running binary lives in, symlinks
// resolved. It is where a player who unpacked the release will have put files
// next to the game.
func ExecutableDir() (string, error) { return executableDir() }

func executableDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		resolved = executable
	}
	return filepath.Dir(resolved), nil
}
