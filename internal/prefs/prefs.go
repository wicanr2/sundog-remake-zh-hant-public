// Package prefs is the player's settings file: which interface colours and
// tile art to show, and where their own Atari ST disk is. It is deliberately
// not the save file. A save is one game; these are the player's wishes across
// every game, so switching chrome on the title screen has to outlive a new
// game and a load.
package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wicanr2/sundog-remake/internal/appdir"
)

// CurrentSchema is bumped when a field changes meaning; a file from an older
// schema is still read, missing fields keep their zero value.
const CurrentSchema = 1

// FileName is the settings file's name under the user data directory.
const FileName = "settings.json"

// Preferences is what the file holds. Empty strings mean "no wish expressed":
// the game then picks its own default, which is how a player who has never
// opened the options gets the Atari ST look the moment a disk is found.
type Preferences struct {
	Schema int `json:"schema"`
	// Locale 是跨所有遊戲存檔共用的玩家偏好；舊設定缺少時由 shell 回退繁中。
	Locale string `json:"locale,omitempty"`
	// Presentation is the preset the player chose. Its absence is what makes a
	// run the first one: the game asks before the title instead of guessing,
	// and a settings file from before the presets therefore asks once.
	Presentation string `json:"presentation,omitempty"`
	// Theme, Tileset and Layout are the preset spread out. They stay in the
	// file so an older build still reads a look it understands, and so a file
	// written by hand still says something; Presentation is what is trusted.
	Theme   string `json:"theme,omitempty"`
	Tileset string `json:"tileset,omitempty"`
	Layout  string `json:"layout,omitempty"`
	// AtariDisk is the path the player typed or the detector found. It is a
	// path on their machine, never the disk's bytes.
	AtariDisk string `json:"atariDisk,omitempty"`
	// Nil means an older settings file: new remake music defaults to on.
	// This preference is deliberately outside the schema45 game save.
	MusicEnabled *bool `json:"musicEnabled,omitempty"`
}

// DefaultPath is where the file lives when the player did not name one: next
// to the save under the user's config directory, or under workplace/ when the
// platform has no such directory.
func DefaultPath() string {
	return appdir.UserData(FileName, filepath.Join("workplace", FileName))
}

// Load reads the file. A file that does not exist is not an error: it is the
// first run, and the zero Preferences say so.
func Load(path string) (Preferences, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Preferences{Schema: CurrentSchema}, nil
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("read settings: %w", err)
	}
	var p Preferences
	if err := json.Unmarshal(data, &p); err != nil {
		return Preferences{}, fmt.Errorf("parse settings %s: %w", path, err)
	}
	if p.Schema > CurrentSchema {
		return Preferences{}, fmt.Errorf("settings %s are schema %d, this build reads up to %d", path, p.Schema, CurrentSchema)
	}
	p.Schema = CurrentSchema
	return p, nil
}

// Save writes the file whole, through a temporary name, so a crash mid-write
// leaves the previous settings rather than half a file.
func (p Preferences) Save(path string) error {
	p.Schema = CurrentSchema
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("settings directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}

// DiskCandidates lists the .st files in the places a player is likely to have
// put their disk: the user data directory (next to the save and the exported
// sounds), the directory the executable is in, and the working directory.
// Callers verify each one; a file named .st is only a candidate.
func DiskCandidates() []string {
	dirs := []string{}
	if dir, ok := appdir.UserDataDir(); ok {
		dirs = append(dirs, dir)
	}
	if dir, err := appdir.ExecutableDir(); err == nil {
		dirs = append(dirs, dir)
	}
	if dir, err := os.Getwd(); err == nil {
		dirs = append(dirs, dir)
	}
	seen := map[string]bool{}
	var out []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var here []string
		for _, entry := range entries {
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".st") {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			here = append(here, path)
		}
		sort.Strings(here)
		out = append(out, here...)
	}
	return out
}
