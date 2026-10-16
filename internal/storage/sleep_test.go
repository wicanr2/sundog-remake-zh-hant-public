package storage

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestSleepSaveResumesAtSameNextCallback(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := game.NewState()
	s.Screen, s.StatusReturn = game.ScreenCharacterStatus, game.ScreenShipboard
	s.Rest, s.TimeMinutes = 10, 59
	s, err = r.Step(s, game.ActionRest)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 47; i++ {
		s, err = r.Step(s, game.ActionSleepTick)
		if err != nil {
			t.Fatal(err)
		}
	}
	s.SleepTheftProtected = true
	dir := t.TempDir()
	file, library := filepath.Join(dir, "save.json"), filepath.Join(dir, "library.json")
	if err = Save(file, s); err != nil {
		t.Fatal(err)
	}
	if err = MakeLibrary(library); err != nil {
		t.Fatal(err)
	}
	menu, err := r.Step(s, game.ActionOpenLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if err = StoreGame(library, "slot-1", menu); err != nil {
		t.Fatal(err)
	}
	one, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	two, err := LoadGame(library, "slot-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, loaded := range []game.State{one, two} {
		if loaded.Sleep != s.Sleep || !loaded.SleepTheftProtected || loaded.Screen != s.Screen {
			t.Fatal("save reset sleep state")
		}
		original := s
		for i := 0; i < 13; i++ {
			original, err = r.Step(original, game.ActionSleepTick)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err = r.Step(loaded, game.ActionSleepTick)
			if err != nil {
				t.Fatal(err)
			}
		}
		if loaded.Sleep != original.Sleep || loaded.TimeMinutes != 60 || loaded.Rest != 11 || loaded.Vigor != original.Vigor || loaded.Nourishment != original.Nourishment || loaded.Attributes != original.Attributes || loaded.Fuel != original.Fuel || loaded.ShipPhase != original.ShipPhase {
			t.Fatal("saved next callback diverged")
		}
	}
}

func TestAutomaticSleepSavePreservesInterruptedView(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := game.NewState()
	s.Screen = game.ScreenTactical
	s.TimeMinutes, s.Rest = 59, 1
	s, err = r.Step(s, game.ActionTacticalMove)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Sleep.Forced || s.StatusReturn != game.ScreenTactical {
		t.Fatal("normal action did not interrupt")
	}
	dir := t.TempDir()
	file, library := filepath.Join(dir, "single.json"), filepath.Join(dir, "library.json")
	if err = Save(file, s); err != nil {
		t.Fatal(err)
	}
	if err = MakeLibrary(library); err != nil {
		t.Fatal(err)
	}
	menu, err := r.Step(s, game.ActionOpenLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if err = StoreGame(library, "slot-1", menu); err != nil {
		t.Fatal(err)
	}
	one, err := Load(file)
	if err != nil {
		t.Fatal(err)
	}
	two, err := LoadGame(library, "slot-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, loaded := range []game.State{one, two} {
		if loaded.Sleep != s.Sleep || loaded.StatusReturn != game.ScreenTactical {
			t.Fatal("save lost automatic interrupt")
		}
		for i := 0; i < 3060; i++ {
			loaded, err = r.Step(loaded, game.ActionWake)
			if err != nil {
				t.Fatal(err)
			}
			loaded, err = r.Step(loaded, game.ActionSleepTick)
			if err != nil {
				t.Fatal(err)
			}
		}
		loaded, err = r.Step(loaded, game.ActionBack)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Sleep.Active || loaded.Screen != game.ScreenTactical || loaded.Rest != 51 {
			t.Fatal("save resumed wrong view")
		}
	}
}

func TestCurrentSaveRejectsIncompleteOrInvalidSleep(t *testing.T) {
	data, err := json.Marshal(game.NewState())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sleep", "sleepTheftProtected"} {
		for _, value := range []string{"", "null"} {
			var fields map[string]json.RawMessage
			json.Unmarshal(data, &fields)
			if value == "" {
				delete(fields, name)
			} else {
				fields[name] = json.RawMessage(value)
			}
			raw, _ := json.Marshal(fields)
			if _, err = decodeCurrentState(raw); err == nil {
				t.Fatal("missing sleep field accepted")
			}
		}
	}
	for _, name := range []string{"active", "forced", "wakeRequested", "minuteCycles", "shipCycles", "elapsedMinutes", "shipActive"} {
		for _, value := range []string{"", "null"} {
			var fields map[string]json.RawMessage
			json.Unmarshal(data, &fields)
			var sleep map[string]json.RawMessage
			json.Unmarshal(fields["sleep"], &sleep)
			if value == "" {
				delete(sleep, name)
			} else {
				sleep[name] = json.RawMessage(value)
			}
			fields["sleep"], _ = json.Marshal(sleep)
			raw, _ := json.Marshal(fields)
			if _, err = decodeCurrentState(raw); err == nil {
				t.Fatalf("missing sleep.%s accepted", name)
			}
		}
	}
	for _, bad := range []game.SleepState{{MinuteCycles: -1}, {MinuteCycles: 60}, {ShipCycles: 32}, {ElapsedMinutes: -1}, {Forced: true}, {WakeRequested: true}, {Active: true}} {
		s := game.NewState()
		s.Sleep = bad
		raw, _ := json.Marshal(s)
		if _, err = decodeCurrentState(raw); err == nil {
			t.Fatalf("invalid sleep accepted: %+v", bad)
		}
		if err = Save(filepath.Join(t.TempDir(), "s.json"), s); err == nil {
			t.Fatal("writer accepted invalid sleep")
		}
		if _, err = game.Migrate(s); err == nil {
			t.Fatal("Migrate accepted invalid sleep")
		}
	}
	s := game.NewState()
	s.Schema = 42
	raw, _ := json.Marshal(s)
	if _, err = decodeCurrentState(raw); err == nil {
		t.Fatal("schema42 accepted")
	}
	if !reflect.DeepEqual(game.NewState().Sleep, game.SleepState{}) {
		t.Fatal("new game sleep not empty")
	}
}
