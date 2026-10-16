package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFileIsFirstRun(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if p.Theme != "" || p.Tileset != "" || p.AtariDisk != "" {
		t.Fatalf("first run carries wishes: %+v", p)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	want := Preferences{Locale: "ja", Theme: "atari-st-original", Tileset: "atari-st-tiles", Layout: "remake", AtariDisk: "/disks/sundog.st"}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want.Schema = CurrentSchema
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file left behind")
	}
}

func TestNewerSchemaIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"schema": 99}`), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatal("a newer schema was read as if understood")
	}
}
