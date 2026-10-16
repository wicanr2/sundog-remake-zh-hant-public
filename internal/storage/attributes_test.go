package storage

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestAttributeBytesSurviveBothSavePathsAndNextHour(t *testing.T) {
	s := game.NewState()
	s.Screen, s.TimeMinutes = game.ScreenTactical, 59
	s.Attributes = [5]int{0, 255, 3, 99, 0}
	s.AttributeMax = [5]int{50, 0, 7, 99, 3}
	dir := t.TempDir()
	path, library := filepath.Join(dir, "save.json"), filepath.Join(dir, "library.json")
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	if err := MakeLibrary(library); err != nil {
		t.Fatal(err)
	}
	if err := StoreGame(library, "slot-1", s); err != nil {
		t.Fatal(err)
	}
	one, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	two, err := LoadGame(library, "slot-1")
	if err != nil {
		t.Fatal(err)
	}
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, loaded := range []game.State{one, two} {
		if loaded.Attributes != s.Attributes || loaded.AttributeMax != s.AttributeMax || loaded.Attribute(game.AttrStrength) != 0 {
			t.Fatal("loading changed original attribute bytes")
		}
		next, err := r.Step(loaded, game.ActionTacticalMove)
		if err != nil {
			t.Fatal(err)
		}
		if next.TimeMinutes != 60 || next.Attributes != [5]int{5, 250, 7, 99, 1} {
			t.Fatalf("next hour after load: %v at %d", next.Attributes, next.TimeMinutes)
		}
	}
}

func TestCurrentStateRejectsInvalidAttributeArrays(t *testing.T) {
	data, err := json.Marshal(game.NewState())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"attributes", "attributeMax"} {
		for _, value := range []string{"", "null", "[]", "[1,2,3,4]", "[1,2,3,4,5,6]", "[1,2,null,4,5]", "[-1,2,3,4,5]", "[256,2,3,4,5]"} {
			t.Run(name+value, func(t *testing.T) {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(data, &fields); err != nil {
					t.Fatal(err)
				}
				if value == "" {
					delete(fields, name)
				} else {
					fields[name] = json.RawMessage(value)
				}
				broken, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := decodeCurrentState(broken); err == nil {
					t.Fatal("invalid attribute array accepted")
				}
			})
		}
	}
	for _, value := range []int{-1, 256} {
		s := game.NewState()
		s.Attributes[0] = value
		if err := Save(filepath.Join(t.TempDir(), "bad.json"), s); err == nil {
			t.Fatal("invalid attribute saved")
		}
		s.Attributes[0], s.AttributeMax[0] = 0, value
		if _, err := game.Migrate(s); err == nil {
			t.Fatal("invalid target migrated")
		}
	}
}
