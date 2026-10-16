package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wicanr2/sundog-remake/internal/appdir"
)

// DefaultDir is where sundog-sound-export writes and the game looks for
// sounds. Nothing from the original ships with the remake, so this directory
// is empty until a player exports from a disk they own.
func DefaultDir() string {
	return appdir.UserData("audio", filepath.Join(".", "audio"))
}

// Library is the set of exported sound files found in one directory.
type Library struct {
	dir   string
	files map[string]string
}

// playable lists the containers the game can decode, best first.
var playable = []string{".ogg", ".wav"}

// Load reads a directory of exported sounds. A missing directory is not an
// error: the game runs silently until a player exports.
func Load(dir string) (*Library, error) {
	library := &Library{dir: dir, files: map[string]string{}}
	if dir == "" {
		return library, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return library, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		extension := strings.ToLower(filepath.Ext(name))
		rank := formatRank(extension)
		if rank < 0 {
			continue
		}
		key := soundKey(strings.TrimSuffix(name, filepath.Ext(name)))
		// A directory holding both containers for one sound keeps the better.
		if existing, ok := library.files[key]; ok {
			if formatRank(strings.ToLower(filepath.Ext(existing))) <= rank {
				continue
			}
		}
		library.files[key] = filepath.Join(dir, name)
	}
	return library, nil
}

func formatRank(extension string) int {
	for rank, candidate := range playable {
		if extension == candidate {
			return rank
		}
	}
	return -1
}

// soundKey turns a file name into the name the game asks for: the export
// numbers its files so they sort in table order, and the number is not part of
// the name.
func soundKey(base string) string {
	if index := strings.IndexByte(base, '-'); index > 0 {
		if _, err := fmt.Sscanf(base[:index], "%d", new(int)); err == nil {
			return base[index+1:]
		}
	}
	return base
}

// Dir is the directory the library was read from.
func (l *Library) Dir() string { return l.dir }

// Empty reports whether there is nothing to play.
func (l *Library) Empty() bool { return l == nil || len(l.files) == 0 }

// Path is the file holding one sound.
func (l *Library) Path(name string) (string, bool) {
	if l == nil {
		return "", false
	}
	path, ok := l.files[name]
	return path, ok
}

// Names lists what the library holds, in order.
func (l *Library) Names() []string {
	if l == nil {
		return nil
	}
	names := make([]string, 0, len(l.files))
	for name := range l.files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
