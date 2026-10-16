package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetPrefersTheWorkingDirectoryCopy(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll("assets", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("assets", "font.otf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Asset("assets/font.otf"); got != "assets/font.otf" {
		t.Fatalf("Asset rewrote a path that already resolves: %q", got)
	}
}

func TestAssetFallsBackToTheExecutableDirectory(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Dir(executable)
	relative := filepath.Join("appdir-test-assets", "font.otf")
	if err := os.MkdirAll(filepath.Join(base, "appdir-test-assets"), 0o755); err != nil {
		t.Skipf("test binary directory is not writable: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(filepath.Join(base, "appdir-test-assets")) })
	if err := os.WriteFile(filepath.Join(base, relative), []byte("x"), 0o644); err != nil {
		t.Skipf("test binary directory is not writable: %v", err)
	}
	t.Chdir(t.TempDir())
	if got, want := Asset(relative), filepath.Join(base, relative); got != want {
		t.Fatalf("Asset = %q, want %q", got, want)
	}
}

func TestAssetKeepsTheRequestedNameWhenNothingResolves(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := Asset("nowhere/missing.json"); got != "nowhere/missing.json" {
		t.Fatalf("Asset = %q, want the requested name back", got)
	}
}

func TestAssetLeavesAbsolutePathsAlone(t *testing.T) {
	if got := Asset("/etc/hosts"); got != "/etc/hosts" {
		t.Fatalf("Asset rewrote an absolute path: %q", got)
	}
}

func TestUserDataPutsTheFileUnderTheProjectDirectory(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	got := UserData("slot.json", "workplace/save/slot.json")
	want := filepath.Join(config, "sundog-remake", "slot.json")
	if got != want {
		t.Fatalf("UserData = %q, want %q", got, want)
	}
}

func TestUserDataCreatesTheDirectoryItNames(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	UserData("slot.json", "fallback.json")
	if _, err := os.Stat(filepath.Join(config, "sundog-remake")); err != nil {
		t.Fatalf("UserData named a directory it did not create: %v", err)
	}
}

func TestUserDataFallsBackWhenTheDirectoryCannotBeUsed(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	// 一個檔案擋在目錄該在的位置上，MkdirAll 就過不了。
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", blocked)
	if got := UserData("slot.json", "workplace/save/slot.json"); got != "workplace/save/slot.json" {
		t.Fatalf("UserData = %q, want the fallback", got)
	}
}

func TestUserDataDirNamesTheProject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, ok := UserDataDir()
	if !ok {
		t.Skip("此平台沒有 user config 目錄")
	}
	if filepath.Base(dir) != "sundog-remake" {
		t.Fatalf("UserDataDir = %q", dir)
	}
}
