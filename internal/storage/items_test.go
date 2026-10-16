package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestDraggedItemSurvivesSingleAndLibraryReopenThenCancel(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"stores", "pocket"} {
		t.Run(source, func(t *testing.T) {
			s := game.NewState()
			s.Screen = game.ScreenStores
			var held game.State
			if source == "stores" {
				s.Stores[19] = 255
				held = r.SelectStoreItem(s, 19)
			} else {
				s.Parts = []int{9, 0, 255, 11}
				held = r.PickPocketPart(s, 2)
			}
			if held.HandItem != 255 {
				t.Fatal("packed byte lost")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "save.json")
			if err := Save(path, held); err != nil {
				t.Fatal(err)
			}
			one, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			library := filepath.Join(dir, "library.json")
			if err := MakeLibrary(library); err != nil {
				t.Fatal(err)
			}
			if err := StoreGame(library, "slot-1", held); err != nil {
				t.Fatal(err)
			}
			two, err := LoadGame(library, "slot-1")
			if err != nil {
				t.Fatal(err)
			}
			for _, loaded := range []game.State{one, two} {
				if loaded.HandItem != 255 || loaded.HandOrigin != held.HandOrigin {
					t.Fatal("drag lost on reopen")
				}
				cancelled := r.CancelHand(loaded)
				if cancelled.HandItem != 0 || cancelled.HandOrigin != (game.ItemOrigin{}) || !reflect.DeepEqual(cancelled.Stores, s.Stores) || !reflect.DeepEqual(cancelled.Parts, s.Parts) {
					t.Fatal("cancel lost or moved items")
				}
			}
		})
	}
}

func TestCurrentSaveRejectsMissingOrContradictoryItemState(t *testing.T) {
	r, _ := game.NewReducer()
	s := game.NewState()
	s.Screen = game.ScreenStores
	s = r.SelectStoreItem(s, 0)
	if s.HandItem == 0 {
		t.Fatal("new game stores slot0 empty")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]json.RawMessage
	if err := json.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"handOrigin", "parts", "stores", "handItem", "occupied-source", "too-many-body", "invalid-byte", "null-item"} {
		t.Run(kind, func(t *testing.T) {
			fields := map[string]json.RawMessage{}
			for key, v := range base {
				fields[key] = v
			}
			switch kind {
			case "occupied-source":
				bad := append([]int(nil), s.Stores...)
				bad[s.HandOrigin.Index] = 11
				fields["stores"], _ = json.Marshal(bad)
			case "too-many-body":
				fields["parts"] = json.RawMessage(`[0,0,0,0,0]`)
			case "invalid-byte":
				fields["handItem"] = json.RawMessage(`256`)
			case "null-item":
				fields["parts"] = json.RawMessage(`[0,0,null,0]`)
			default:
				delete(fields, kind)
			}
			raw, _ := json.Marshal(fields)
			if _, err := decodeCurrentState(raw); err == nil {
				t.Fatal("accepted invalid current save")
			}
		})
	}
	old := Envelope{Format: formatV41, State: s}
	data, _ = json.Marshal(old)
	path := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted schema41 envelope")
	}
}
