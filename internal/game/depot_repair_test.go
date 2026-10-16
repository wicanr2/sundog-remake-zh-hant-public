package game

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// 原版與正式輸入投影見docs/atari-st/ftl-boost-spec.md。
func TestDepotRepairNativeMatrix(t *testing.T) {
	file := os.Getenv("SUNDOG_DEPOT_REPAIR_NATIVE")
	if file == "" {
		t.Skip("需要Talos重生的原始修船矩陣")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Cases, Differences int
		Native             []struct{ System, Hull, ShipCity, Bank, Cash, Menu, UnitPrice, ResultHull, ResultBank, ResultCash int }
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if native.Cases != 2160 || native.Differences != 0 || len(native.Native) != 2160 {
		t.Fatal("原始矩陣未通過")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for i, in := range native.Native {
		s := NewState()
		s.Screen = ScreenGround
		s.SystemID = r.World.Systems()[in.System-1].ID
		found := false
		for _, city := range r.World.Destinations() {
			if city.SystemID != s.SystemID {
				continue
			}
			pos, ok := r.World.FacilityPosition(city.ID, world.FacilityDepot)
			if !ok {
				continue
			}
			s.CityID, s.PlanetID, s.ShipCityID = city.ID, city.PlanetID, city.ID
			s.Position = Position{pos.X, pos.Y}
			found = true
			break
		}
		if !found {
			t.Fatal("原始系統無depot", in.System)
		}
		if in.ShipCity != 7 {
			s.ShipCityID = "other-city"
		}
		s.HullCondition, s.Bank, s.Cash = in.Hull, in.Bank, in.Cash
		s.BankBalances = map[string]int{s.SystemID: in.Bank}
		fuel := s.Fuel
		step := func(action Action) {
			if in.Hull == 0 {
				// Hull zero is already a terminal player state. Compare the
				// isolated original caller, without claiming a living UI path.
				switch action {
				case ActionOpenDepot:
					err = r.openDepot(&s)
				case ActionRepairHull:
					err = r.openFuelPurchase(&s, FuelPurchaseHull)
				case ActionFuelBuyFull:
					buyFuelQuantity(&s, true)
				case ActionFuelBuyOne:
					buyFuelQuantity(&s, false)
				case ActionFuelCancel:
					finishFuelPurchase(&s)
				case ActionBack:
					s, err = r.Step(s, action)
				}
			} else {
				s, err = r.Step(s, action)
			}
			if err != nil {
				t.Fatal(i, err)
			}
		}
		for _, action := range []Action{ActionOpenDepot, ActionRepairHull} {
			step(action)
		}
		if s.FuelPurchase.UnitPrice != in.UnitPrice {
			t.Fatalf("case%d price%d original%d", i, s.FuelPurchase.UnitPrice, in.UnitPrice)
		}
		if err = ValidateFuelState(s); err != nil {
			t.Fatal(i, err)
		}
		// JSON roundtrip must preserve the pending quote and its depot return.
		data, _ := json.Marshal(s)
		var saved State
		if err = json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.FuelPurchase != s.FuelPurchase || saved.GroundReturn != ScreenDepot {
			t.Fatal("修船保存不同")
		}
		s = saved
		if in.Menu != 3 {
			action := ActionFuelBuyFull
			if in.Menu == 2 {
				action = ActionFuelBuyOne
			}
			step(action)
		}
		step(ActionFuelCancel)
		if s.HullCondition != in.ResultHull || s.Bank != in.ResultBank || s.Cash != in.ResultCash || s.BankBalances[s.SystemID] != s.Bank || s.Fuel != fuel || s.Screen != ScreenDepot {
			t.Fatalf("case%d result hull%d bank%d cash%d original%+v", i, s.HullCondition, s.Bank, s.Cash, in)
		}
		s, err = r.Step(s, ActionBack)
		if err != nil || s.Screen != ScreenGround {
			t.Fatal("父頁未返回地面", err)
		}
	}
}
