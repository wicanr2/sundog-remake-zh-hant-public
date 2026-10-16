package storage

// 驗收入口：docs/atari-st/ship-rules.md「完整燃料整合契約」，Issue #8。
// 保存格式是 remake 自有契約；這些測試不計入原版對拍案數。
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/world"
)

func assertOldSaveUnchanged(t *testing.T, path string, original []byte) {
	t.Helper()
	if _, err := Load(path); !errors.Is(err, ErrOldSave) {
		t.Fatalf("old save error = %v, want ErrOldSave", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("old file changed: %v", err)
	}
}

func TestEveryOldSaveFormatAndForgedCurrentEnvelopeAreRejected(t *testing.T) {
	for schema := 0; schema < game.CurrentSchema; schema++ {
		t.Run(fmt.Sprint(schema), func(t *testing.T) {
			state := game.NewState()
			state.Schema = schema
			for _, format := range []string{fmt.Sprintf("sundog-remake-prototype-save-v%d", schema), currentFormat} {
				path := filepath.Join(t.TempDir(), "old.json")
				raw, err := json.Marshal(Envelope{Format: format, State: state})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				assertOldSaveUnchanged(t, path, raw)
				if err := Save(path, state); err == nil {
					t.Fatal("saved old state")
				}
				after, _ := os.ReadFile(path)
				if !bytes.Equal(raw, after) {
					t.Fatal("failed save replaced old file")
				}
			}
		})
	}
}

func TestMissingFuelStateIsRejectedWithoutDefaulting(t *testing.T) {
	raw, err := json.Marshal(game.NewState())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"shipPhase", "shipControls", "cargoAttached", "cargoPosition", "groundMode", "shieldSetting", "fuelPurchase", "fuelRescueDue"} {
		for _, absent := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/missing=%t", key, absent), func(t *testing.T) {
				var state map[string]json.RawMessage
				if err := json.Unmarshal(raw, &state); err != nil {
					t.Fatal(err)
				}
				if absent {
					delete(state, key)
				} else {
					state[key] = json.RawMessage("null")
				}
				encoded, _ := json.Marshal(state)
				if _, err := decodeCurrentState(encoded); !errors.Is(err, ErrOldSave) {
					t.Fatalf("missing state accepted or wrong error: %v", err)
				}
			})
		}
	}
}

func TestAllLibraryOperationsRefuseOldOrContaminatedLibrary(t *testing.T) {
	current := game.NewState()
	old := current
	old.Schema = game.CurrentSchema - 1
	for _, fixture := range []LibraryEnvelope{
		{Format: libraryFormatV1, Slots: map[string]game.State{"old": old}},
		{Format: libraryFormatV2, Slots: map[string]game.State{"good": current, "old": old}},
	} {
		raw, err := json.Marshal(fixture)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "library.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		operations := map[string]func() error{
			"make":   func() error { return MakeLibrary(path) },
			"list":   func() error { _, err := ListLibrary(path); return err },
			"load":   func() error { _, err := LoadGame(path, "good"); return err },
			"store":  func() error { return StoreGame(path, "new", current) },
			"delete": func() error { return DeleteGame(path, "good") },
		}
		for name, operation := range operations {
			if err := operation(); !errors.Is(err, ErrOldSave) {
				t.Fatalf("%s accepted old library: %v", name, err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(raw, after) {
				t.Fatalf("%s changed old library", name)
			}
		}
	}
}

func TestFuelControlsAndSourceByteMustBeExplicitAndInRange(t *testing.T) {
	raw, err := json.Marshal(game.NewState())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ before, after string }{
		{`"secondary":[false,false,false,false,false,false]`, `"secondary":[false,null,false,false,false,false]`},
		{`"secondary":[false,false,false,false,false,false]`, `"secondary":[false,false,false,false,false]`},
		{`"sourceCode":0`, `"sourceCode":256`},
		{`"sourceCode":0`, `"sourceCode":null`},
	} {
		modified := bytes.Replace(raw, []byte(tc.before), []byte(tc.after), 1)
		if bytes.Equal(raw, modified) {
			t.Fatal("negative fixture was not changed")
		}
		if _, err := decodeCurrentState(modified); err == nil {
			t.Fatalf("accepted %s", tc.after)
		}
	}
}

func TestFuelPurchaseContinuesAfterSingleSaveAndLibraryLoad(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := game.NewState()
	s.Screen, s.Fuel, s.Bank, s.Cash = game.ScreenGround, 15, -9, 1000
	s.BankBalances[s.SystemID] = s.Bank
	depot, ok := r.World.FacilityPosition(s.CityID, world.FacilityDepot)
	if !ok {
		t.Fatal("source depot missing")
	}
	s.Position = game.Position{X: depot.X, Y: depot.Y}
	s.ShipPhase, s.CurrentSpeed, s.TargetSpeed = 2, 224, 255
	s.CargoAttached, s.GroundMode = false, game.GroundInPod
	s.CargoCityID, s.CargoPosition = s.CityID, s.Position
	s.Holds[0] = game.Hold{Kind: game.HoldAuxFuel, Quality: 3, Price: 123, SourceCode: 0xab}
	s.FuelRescuePending, s.FuelRescueDue = true, 2010
	s, err = r.Step(s, game.ActionRefuel)
	if err != nil || s.Screen != game.ScreenFuelPurchase {
		t.Fatal("open purchase", err)
	}
	s, err = r.Step(s, game.ActionFuelBuyOne)
	if err != nil {
		t.Fatal(err)
	}
	if s.Bank != -9 || s.Cash != 1000 {
		t.Fatal("paid before caller returned")
	}
	want, err := r.Step(s, game.ActionFuelCancel)
	if err != nil {
		t.Fatal(err)
	}
	if want.Fuel != 47 || want.Bank != -9 || want.Cash != 843 {
		t.Fatalf("caller result fuel/bank/cash %d/%d/%d", want.Fuel, want.Bank, want.Cash)
	}
	path := filepath.Join(t.TempDir(), "current.json")
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, loaded) {
		t.Fatal("single save changed purchase or phase")
	}
	got, err := r.Step(loaded, game.ActionFuelCancel)
	if err != nil || !reflect.DeepEqual(want, got) {
		t.Fatal("single save continued differently", err)
	}
	library := filepath.Join(t.TempDir(), "library.json")
	if err := MakeLibrary(library); err != nil {
		t.Fatal(err)
	}
	menu, err := r.Step(s, game.ActionOpenLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if err := StoreGame(library, "purchase", menu); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadGame(library, "purchase")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Screen != game.ScreenFuelPurchase || loaded.LibraryReturn != "" || loaded.FuelPurchase != s.FuelPurchase || loaded.ShipPhase != 2 || loaded.Holds[0].SourceCode != 0xab {
		t.Fatal("library changed persistent purchase")
	}
	got, err = r.Step(loaded, game.ActionFuelCancel)
	if err != nil || got.Fuel != want.Fuel || got.Bank != want.Bank || got.Cash != want.Cash || got.ShipPhase != want.ShipPhase || got.Holds != want.Holds {
		t.Fatal("library continued differently", err)
	}
}
