package storage

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestLibraryLifecyclePreservesCJKCharacterAndDeletesSlot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := MakeLibrary(path); err != nil {
		t.Fatal(err)
	}
	want := game.NewState()
	want.Screen = game.ScreenShipboard
	want.Character = "陽犬"
	want.CharacterDraft = ""
	want.Cash = 37
	if err := StoreGame(path, "slot-1", want); err != nil {
		t.Fatal(err)
	}
	if want.Saved {
		t.Fatal("store mutated the caller state")
	}
	summaries, err := ListLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(summaries, []SlotSummary{{ID: "slot-1", Character: "陽犬", Location: want.Location, TimeMinutes: want.TimeMinutes}}) {
		t.Fatalf("unexpected library summaries: %+v", summaries)
	}
	got, err := LoadGame(path, "slot-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("library round trip mismatch: got=%+v want=%+v", got, want)
	}
	if err := DeleteGame(path, "slot-1"); err != nil {
		t.Fatal(err)
	}
	summaries, err = ListLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("deleted slot remains: %+v", summaries)
	}
}

func TestLibraryRequiresMakeBeforeStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-library.json")
	err := StoreGame(path, "slot-1", game.NewState())
	if !errors.Is(err, ErrLibraryNotFound) {
		t.Fatalf("missing library error = %v, want ErrLibraryNotFound", err)
	}
}

func TestLibraryNormalizesMenuStateToPlayableScreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := MakeLibrary(path); err != nil {
		t.Fatal(err)
	}
	state := game.NewState()
	state.Screen = game.ScreenCharacterLibrary
	if err := StoreGame(path, "slot-menu", state); err != nil {
		t.Fatal(err)
	}
	got, err := LoadGame(path, "slot-menu")
	if err != nil {
		t.Fatal(err)
	}
	if got.Screen != game.ScreenShipboard || got.MessageKey != "ui.message.ready" {
		t.Fatalf("menu state was not normalized: %+v", got)
	}
}
