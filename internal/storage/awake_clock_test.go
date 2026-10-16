package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestAwakeClockBothSavePathsContinueMidWait(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, sleep := range []bool{false, true} {
		s := game.NewState()
		s.Screen = game.ScreenStores
		for i := 0; i < 13; i++ {
			s, err = r.Step(s, game.ActionVBLTick)
			if err != nil {
				t.Fatal(err)
			}
		}
		s = r.SelectStoreItem(s, 0)
		if sleep {
			s = r.CancelHand(s)
			s.Screen, s.StatusReturn = game.ScreenCharacterStatus, game.ScreenShipboard
			s, err = r.Step(s, game.ActionRest)
			if err != nil {
				t.Fatal(err)
			}
		}
		dir := t.TempDir()
		single, library := filepath.Join(dir, "save.json"), filepath.Join(dir, "library.json")
		if err = Save(single, s); err != nil {
			t.Fatal(err)
		}
		if err = MakeLibrary(library); err != nil {
			t.Fatal(err)
		}
		if err = StoreGame(library, "slot-1", s); err != nil {
			t.Fatal(err)
		}
		a, err := Load(single)
		if err != nil {
			t.Fatal(err)
		}
		b, err := LoadGame(library, "slot-1")
		if err != nil {
			t.Fatal(err)
		}
		for _, loaded := range []game.State{a, b} {
			if !reflect.DeepEqual(s.AwakeClock, loaded.AwakeClock) {
				t.Fatal("clock changed on save/load")
			}
			left, right := s, loaded
			for i := 0; i < 480; i++ {
				if !sleep && i == 17 {
					left, right = r.CancelHand(left), r.CancelHand(right)
				}
				for _, action := range []game.Action{game.ActionVBLTick, game.ActionSleepTick} {
					left, err = r.Step(left, action)
					if err != nil {
						t.Fatal(err)
					}
					right, err = r.Step(right, action)
					if err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(left.AwakeClock, right.AwakeClock) || left.TimeMinutes != right.TimeMinutes || left.Sleep != right.Sleep || left.ShipPhase != right.ShipPhase || left.Fuel != right.Fuel {
					t.Fatalf("continuation diverged at input%d sleep%t", i, sleep)
				}
			}
		}
	}
}

func TestCurrentClockMalformedAndOldSavesRejected(t *testing.T) {
	s := game.NewState()
	raw, err := json.Marshal(Envelope{Format: currentFormat, State: s})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(s map[string]any) { delete(s, "awakeClock") },
		func(s map[string]any) { s["schema"] = 43 },
		func(s map[string]any) { s["awakeClock"].(map[string]any)["source"] = nil },
		func(s map[string]any) {
			delete(s["awakeClock"].(map[string]any)["source"].(map[string]any), "eventCountdown")
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["source"].(map[string]any)["eventCountdown"] = 0
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["background"].(map[string]any)["slots"] = []any{}
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["background"].(map[string]any)["slots"].([]any)[7] = nil
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["background"].(map[string]any)["waitEvents"] = 65537
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["background"].(map[string]any)["waitEvents"] = 0
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["background"].(map[string]any)["waitEvents"] = 65536
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["minutePause"] = -1
		},
		func(s map[string]any) {
			s["awakeClock"].(map[string]any)["background"].(map[string]any)["slots"].([]any)[0].(map[string]any)["period"] = 1
		},
		func(s map[string]any) { delete(s["awakeClock"].(map[string]any)["ground"].(map[string]any), "updates") },
	} {
		var e map[string]any
		if err = json.Unmarshal(raw, &e); err != nil {
			t.Fatal(err)
		}
		mutate(e["state"].(map[string]any))
		broken, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "save.json")
		if err = os.WriteFile(path, broken, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(path); err == nil {
			t.Fatal("malformed clock accepted")
		}
		library, err := json.Marshal(map[string]any{"format": libraryFormatV2, "slots": map[string]any{"slot-1": e["state"]}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, library, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = LoadGame(path, "slot-1"); err == nil {
			t.Fatal("malformed library clock accepted")
		}
	}
}
