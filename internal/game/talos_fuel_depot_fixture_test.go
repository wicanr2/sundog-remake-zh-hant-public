package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// 入口：docs/atari-st/ship-rules.md 的完整燃料整合契約與 Issue #8。
// 原版輸出僅作測試輸入身分，答案由驗證器在匯出後比較，不進 runtime。
func TestExportTalosFuelDepot(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定購油正式比較輸出")
	}
	oldSeed := NewGameSeed
	NewGameSeed = func() int64 { return 0 }
	t.Cleanup(func() { NewGameSeed = oldSeed })
	type fuelInput struct {
		System, Fuel, ShipCity, Bank, Cash, Menu int
	}
	type auxInput struct {
		System, Bank, Cash, Menu int
		Kinds                    [2]int
		Bit13                    bool
	}
	var native struct {
		FuelNative []fuelInput
		AuxNative  []auxInput
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-fuel-depot-chain.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.FuelNative) != 216 || len(native.AuxNative) != 576 {
		t.Fatal("原版 caller 案數不同")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	newInput := func(system, bank, cash int) State {
		s := NewState()
		s.Screen = ScreenGround
		s.SystemID = r.World.Systems()[system-1].ID
		found := false
		for _, city := range r.World.Destinations() {
			if city.SystemID != s.SystemID {
				continue
			}
			s.CityID, s.PlanetID, s.ShipCityID = city.ID, city.PlanetID, city.ID
			pos, ok := r.World.FacilityPosition(city.ID, world.FacilityDepot)
			if !ok {
				t.Fatalf("系統 %d 缺購油正式入口", system)
			}
			s.Position = Position{pos.X, pos.Y}
			found = true
			break
		}
		if !found {
			t.Fatalf("系統 %d 無城市", system)
		}
		s.Bank, s.Cash = bank, cash
		s.BankBalances = map[string]int{s.SystemID: bank}
		return s
	}
	type fuelRow struct {
		Index, Fuel, Bank, Cash, UnitPrice int
		Executed                           bool
		Unsupported                        []string
	}
	type holdRow struct{ Kind, Quality, Price, SourceCode int }
	type auxRow struct {
		Index, Bank, Cash, UnitPrice int
		Holds                        [2]holdRow
		Executed                     bool
		Unsupported                  []string
	}
	report := struct {
		Scope string
		Fuel  []fuelRow
		Aux   []auxRow
	}{Scope: "正式 reducer 的語意輸入投影；城市身分等價映射、入口使用來源 FacilityDepot；備用槽來源碼7對應原版 caller setup 的 system|7<<4|7<<9，正常世界producer另驗一基星球索引。非正常 GUI／原版全機同狀態。"}
	for i, in := range native.FuelNative {
		s := newInput(in.System, in.Bank, in.Cash)
		s.Fuel = in.Fuel
		if in.ShipCity != 7 {
			s.ShipCityID = "depot-parity-other-city"
		}
		row := fuelRow{Index: i}
		s, err = r.Step(s, ActionRefuel)
		if err != nil || s.Screen != ScreenFuelPurchase {
			t.Fatal("正式主購油入口", err, s.Screen)
		}
		if in.Menu == 1 || in.Menu == 2 {
			action := ActionFuelBuyFull
			if in.Menu == 2 {
				action = ActionFuelBuyOne
			}
			s, err = r.Step(s, action)
			if err != nil {
				t.Fatal(err)
			}
		} else if in.Menu != 3 {
			t.Fatal("原始選單不同", in.Menu)
		}
		s, err = r.Step(s, ActionFuelCancel)
		if err != nil {
			t.Fatal(err)
		}
		row.Executed = true
		row.Fuel, row.Bank, row.Cash = s.Fuel, s.Bank, s.Cash
		row.UnitPrice, _ = r.World.FuelPrice(s.CityID)
		report.Fuel = append(report.Fuel, row)
	}
	for i, in := range native.AuxNative {
		s := newInput(in.System, in.Bank, in.Cash)
		for slot, kind := range in.Kinds {
			h := Hold{Quality: 1, SourceCode: 0xab, Price: []int{0x1357, 0x2468}[slot]}
			switch kind {
			case 0:
				h.Kind = HoldEmpty
			case 1:
				h.Kind = HoldAuxFuel
			default:
				h.Kind, h.Goods = HoldGoods, kind
			}
			s.Holds[slot] = h
		}
		s.GroundMode = GroundInPod
		if in.Bit13 {
			s.GroundMode = GroundOnFoot
		}
		s, err = r.Step(s, ActionLoadAuxFuel)
		if err != nil || s.Screen != ScreenFuelPurchase {
			t.Fatal("正式備用購油入口", err, s.Screen)
		}
		row := auxRow{Index: i, UnitPrice: s.FuelPurchase.UnitPrice}
		// TestSunDogFuelDepotChain.setup independently supplies planet code 7
		// in system|7<<4|7<<9. Keep the same input on both sides; the
		// normal world producer is verified separately.
		s.FuelPurchase.SourceCode = 7
		if in.Menu == 1 || in.Menu == 2 {
			action := ActionFuelBuyFull
			if in.Menu == 2 {
				action = ActionFuelBuyOne
			}
			s, err = r.Step(s, action)
			if err != nil {
				t.Fatal(err)
			}
		} else if in.Menu != 3 {
			t.Fatal("原始選單不同", in.Menu)
		}
		s, err = r.Step(s, ActionFuelCancel)
		if err != nil {
			t.Fatal(err)
		}
		row.Executed = true
		row.Bank, row.Cash = s.Bank, s.Cash
		for slot, hold := range s.Holds {
			kind := hold.Goods
			if hold.Kind == HoldEmpty {
				kind = 0
			} else if hold.Kind == HoldAuxFuel {
				kind = 1
			}
			row.Holds[slot] = holdRow{kind, hold.Quality, hold.Price, int(hold.SourceCode)}
		}
		report.Aux = append(report.Aux, row)
	}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-fuel-depot.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式主燃料 %d 案／備用槽 %d 案匯出，未支援狀態由外部差異閘門拒絕", len(report.Fuel), len(report.Aux))
}
