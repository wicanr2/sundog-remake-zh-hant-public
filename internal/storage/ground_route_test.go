package storage

import (
	"encoding/json"
	"github.com/wicanr2/sundog-remake/internal/game"
	"os"
	"path/filepath"
	"testing"
)

func TestColonySiteSingleAndLibraryContracts(t *testing.T) {
	for _, mode := range []string{"single", "library"} {
		for _, scenario := range []string{"pending-city", "selected-surface", "pending-other-planet"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				s := game.NewState()
				s.Screen = game.ScreenGround
				if scenario == "selected-surface" {
					s.GroundWorld = true
					if err := game.PrepareColonySite(&s, nil); err != nil {
						t.Fatal(err)
					}
				} else if scenario == "pending-other-planet" {
					r, err := game.NewReducer()
					if err != nil {
						t.Fatal(err)
					}
					for _, p := range r.World.Planets() {
						if p.ID != s.PlanetID {
							s.PlanetID = p.ID
							break
						}
					}
					s.GroundWorld = true
				}
				path := filepath.Join(t.TempDir(), "save.json")
				var got game.State
				var err error
				if mode == "single" {
					err = Save(path, s)
					if err == nil {
						got, err = Load(path)
					}
				} else {
					err = MakeLibrary(path)
					if err == nil {
						err = StoreGame(path, "slot-1", s)
					}
					if err == nil {
						got, err = LoadGame(path, "slot-1")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if got.ColonySite != s.ColonySite || got.CombatRolls != s.CombatRolls || got.Colony != s.Colony || got.Position != s.Position || got.GroundWorld != s.GroundWorld {
					t.Fatal("位置、亂數或世界狀態未保存")
				}
			})
		}
		for _, mutation := range []string{"missing-site", "null-site", "missing-pending", "null-pending", "missing-position", "null-position", "missing-x", "null-x", "missing-y", "null-y", "x-range", "y-range", "pending-wrong-site", "selected-illegal", "selected-city", "pending-home-world", "old-schema"} {
			t.Run(mode+"/reject/"+mutation, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "save.json")
				s := game.NewState()
				if mode == "single" {
					if err := Save(path, s); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := MakeLibrary(path); err != nil {
						t.Fatal(err)
					}
					if err := StoreGame(path, "slot-1", s); err != nil {
						t.Fatal(err)
					}
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var document map[string]any
				if err = json.Unmarshal(data, &document); err != nil {
					t.Fatal(err)
				}
				var state map[string]any
				if mode == "single" {
					state = document["state"].(map[string]any)
				} else {
					state = document["slots"].(map[string]any)["slot-1"].(map[string]any)
				}
				site := state["colonySite"].(map[string]any)
				position := site["position"].(map[string]any)
				switch mutation {
				case "missing-site":
					delete(state, "colonySite")
				case "null-site":
					state["colonySite"] = nil
				case "missing-pending":
					delete(site, "pending")
				case "null-pending":
					site["pending"] = nil
				case "missing-position":
					delete(site, "position")
				case "null-position":
					site["position"] = nil
				case "missing-x":
					delete(position, "x")
				case "null-x":
					position["x"] = nil
				case "missing-y":
					delete(position, "y")
				case "null-y":
					position["y"] = nil
				case "x-range":
					site["pending"] = false
					position["x"] = 40
				case "y-range":
					site["pending"] = false
					position["y"] = -1
				case "pending-wrong-site":
					position["x"] = 3
				case "selected-illegal":
					site["pending"] = false
					position["x"] = 0
					position["y"] = 0
				case "selected-city":
					site["pending"] = false
					position["x"] = 12
					position["y"] = 7
				case "pending-home-world":
					state["groundWorld"] = true
				case "old-schema":
					state["schema"] = 38
					if mode == "single" {
						document["format"] = formatV38
					}
				}
				data, err = json.Marshal(document)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				if mode == "single" {
					_, err = Load(path)
				} else {
					_, err = LoadGame(path, "slot-1")
				}
				if err == nil {
					t.Fatal("接受缺項、非法或舊城址保存")
				}
			})
		}
	}
}

func TestGroundRouteSurvivesSaveAndLibrary(t *testing.T) {
	for _, surface := range []bool{false, true} {
		state := game.NewState()
		state.Screen = game.ScreenGround
		state.GroundWorld = surface
		if err := game.PrepareColonySite(&state, nil); err != nil {
			t.Fatal(err)
		}
		state.Position = game.Position{X: 3, Y: 7}
		game.ResetGroundView(&state, game.Position{X: 23, Y: 20})
		state.ShipPosition = game.Position{X: 21, Y: 8}
		path := filepath.Join(t.TempDir(), "save.json")
		if err := Save(path, state); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.GroundWorld != surface || loaded.GroundView != state.GroundView || loaded.Position != state.Position || loaded.ShipPosition != state.ShipPosition {
			t.Fatal("保存失去地面座標")
		}
		state.Screen = game.ScreenCharacterLibrary
		state.LibraryReturn = game.ScreenGround
		library := filepath.Join(t.TempDir(), "library.json")
		if err := MakeLibrary(library); err != nil {
			t.Fatal(err)
		}
		if err := StoreGame(library, "1", state); err != nil {
			t.Fatal(err)
		}
		loaded, err = LoadGame(library, "1")
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Screen != game.ScreenGround || loaded.GroundView != state.GroundView || loaded.GroundWorld != surface {
			t.Fatal("圖書館將地面保存搬回船艙")
		}
	}
}

func TestSchema31IsRejectedWithoutInferringFuelState(t *testing.T) {
	state := game.NewState()
	state.Schema = 31
	state.ShipPosition = game.Position{X: 1, Y: 0}
	if _, err := game.Migrate(state); err == nil {
		t.Fatal("舊保存不應推測相位或接駁狀態")
	}
}
