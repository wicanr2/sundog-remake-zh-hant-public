package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestSchema45CombatStateSurvivesBothSavePaths(t *testing.T) {
	s := game.NewState()
	s.EnemyCourse = game.EnemyCourseState{Ticks: 17, FireReady: true, CanPrepareFire: false, Maneuver: [2]int{4, -1}}
	s.EnemyClosing = 3
	for _, library := range []bool{false, true} {
		p := filepath.Join(t.TempDir(), "combat.json")
		var got game.State
		var err error
		if library {
			if err = MakeLibrary(p); err == nil {
				err = StoreGame(p, "slot-1", s)
			}
			if err == nil {
				got, err = LoadGame(p, "slot-1")
			}
		} else {
			if err = Save(p, s); err == nil {
				got, err = Load(p)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		if got.Schema != 45 || got.EnemyCourse != s.EnemyCourse || got.EnemyClosing != s.EnemyClosing {
			t.Fatal("combat continuation state was not retained")
		}
	}
}

func TestSchema45CombatContinuesAfterSave(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for mode := 1; mode <= 7; mode++ {
		for _, ready := range []bool{false, true} {
			for _, library := range []bool{false, true} {
				path := filepath.Join(t.TempDir(), "combat.json")
				s := game.NewState()
				s.Screen, s.CombatActive = game.ScreenTactical, true
				s.EnemyCourseMode, s.EnemyHull, s.EnemyMaxHull = mode, 200, 200
				s.CombatRange, s.EnemyBearing, s.EnemyPitch = 70, 512, 512
				s.EnemyFireCountdown, s.EnemyClosing = 1, -1
				s.EnemyCourse = game.EnemyCourseState{Ticks: 1, FireReady: ready, CanPrepareFire: true, Maneuver: [2]int{2, 1}}
				if err := Save(path, s); err != nil {
					t.Fatal(err)
				}
				s, err = Load(path)
				if err != nil {
					t.Fatal(err)
				}
				want, got := s, s
				if library {
					path = filepath.Join(t.TempDir(), "library.json")
					if err := MakeLibrary(path); err != nil {
						t.Fatal(err)
					}
				}
				for update := 1; update <= 12; update++ {
					want, err = r.Step(want, game.ActionRechargeWeapon)
					if err != nil {
						t.Fatal(err)
					}
					got, err = r.Step(got, game.ActionRechargeWeapon)
					if err != nil {
						t.Fatal(err)
					}
					if update == 4 || update == 8 {
						if library {
							err = StoreGame(path, "slot-1", got)
							if err == nil {
								got, err = LoadGame(path, "slot-1")
							}
						} else {
							err = Save(path, got)
							if err == nil {
								got, err = Load(path)
							}
						}
						if err != nil {
							t.Fatal(err)
						}
					}
					// Library deliberately resets only these UI lifecycle fields.
					a, b := want, got
					a.MessageKey, b.MessageKey = "", ""
					a.Saved, b.Saved = false, false
					a.QuitRequested, b.QuitRequested = false, false
					if !reflect.DeepEqual(a, b) {
						t.Fatalf("continuation differs: mode=%d ready=%v library=%v update=%d", mode, ready, library, update)
					}
				}
			}
		}
	}
}

func TestSchema45RequiresCompleteCombatState(t *testing.T) {
	s := game.NewState()
	raw, err := json.Marshal(Envelope{Format: currentFormat, State: s})
	if err != nil {
		t.Fatal(err)
	}
	var valid map[string]any
	if err = json.Unmarshal(raw, &valid); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"missing-object", "null-object", "missing-ticks", "null-ticks", "missing-ready", "null-ready", "missing-allowed", "null-allowed", "missing-memory", "null-memory", "short-memory", "long-memory", "null-element", "negative-ticks", "large-ticks", "large-memory", "schema44"} {
		t.Run(mutation, func(t *testing.T) {
			var e map[string]any
			if err = json.Unmarshal(raw, &e); err != nil {
				t.Fatal(err)
			}
			st := e["state"].(map[string]any)
			c := st["enemyCourse"].(map[string]any)
			switch mutation {
			case "missing-object":
				delete(st, "enemyCourse")
			case "null-object":
				st["enemyCourse"] = nil
			case "missing-ticks":
				delete(c, "ticks")
			case "null-ticks":
				c["ticks"] = nil
			case "missing-ready":
				delete(c, "fireReady")
			case "null-ready":
				c["fireReady"] = nil
			case "missing-allowed":
				delete(c, "canPrepareFire")
			case "null-allowed":
				c["canPrepareFire"] = nil
			case "missing-memory":
				delete(c, "maneuver")
			case "null-memory":
				c["maneuver"] = nil
			case "short-memory":
				c["maneuver"] = []any{0}
			case "long-memory":
				c["maneuver"] = []any{0, 0, 1}
			case "null-element":
				c["maneuver"] = []any{nil, 0}
			case "negative-ticks":
				c["ticks"] = -1
			case "large-ticks":
				c["ticks"] = 32768
			case "large-memory":
				c["maneuver"] = []any{32768, 0}
			case "schema44":
				st["schema"] = 44
				e["format"] = formatV44
			}
			b, _ := json.Marshal(e)
			p := filepath.Join(t.TempDir(), "bad.json")
			if err = os.WriteFile(p, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = Load(p); err == nil {
				t.Fatal("incomplete/legacy combat state accepted")
			}
			after, _ := os.ReadFile(p)
			if !reflect.DeepEqual(b, after) {
				t.Fatal("rejected save was modified")
			}
		})
	}
}
