package audio

import (
	"os"
	"path/filepath"
	"testing"
)

// The export numbers its files so they sort in the original's table order; the
// number is not part of the name the game asks for.
func TestLoadStripsTheNumberFromTheName(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "04-menu_click.ogg"))
	write(t, filepath.Join(dir, "28-warp1_speed2.ogg"))

	library, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"menu_click", "warp1_speed2"} {
		if _, ok := library.Path(name); !ok {
			t.Errorf("%s is missing from %v", name, library.Names())
		}
	}
}

// A directory holding both containers for one sound keeps the compressed one.
func TestOggWinsOverWav(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "09-cannon.wav"))
	write(t, filepath.Join(dir, "09-cannon.ogg"))

	library, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	path, ok := library.Path("cannon")
	if !ok {
		t.Fatal("cannon is missing")
	}
	if filepath.Ext(path) != ".ogg" {
		t.Errorf("kept %s, want the .ogg", filepath.Base(path))
	}
}

// Nothing exported is the normal state of a fresh install, not a failure.
func TestMissingDirectoryIsEmptyNotAnError(t *testing.T) {
	library, err := Load(filepath.Join(t.TempDir(), "nothing-here"))
	if err != nil {
		t.Fatal(err)
	}
	if !library.Empty() {
		t.Error("a missing directory should hold no sounds")
	}
}

// Files the game cannot decode are left alone.
func TestOtherFilesAreIgnored(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "notes.txt"))
	write(t, filepath.Join(dir, "03-jackpot.ogg"))

	library, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if names := library.Names(); len(names) != 1 || names[0] != "jackpot" {
		t.Errorf("got %v, want just jackpot", names)
	}
}

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
