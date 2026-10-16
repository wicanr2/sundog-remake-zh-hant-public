package game

import (
	"reflect"
	"testing"
)

func TestWarehouseSwapFullContainersAndCancellation(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	base := NewState()
	base.Screen, base.TradePage = ScreenTrade, TradePageWarehouse
	base.Holds = [2]Hold{{Kind: HoldGoods, Goods: 9, Quality: 4, Price: 31, SourceCode: 34}, {Kind: HoldGoods, Goods: 10, Quality: 5, Price: 97, SourceCode: 35}}
	base.Warehouses = map[string][]Lot{base.CityID: {{Goods: 31, Quality: 7, SourceCode: 4}, {Goods: 2, Quality: 3, Price: 13, SourceCode: 17}, {Goods: 3, Quality: 1, Price: 230, SourceCode: 29}, {Goods: 24, Quality: 6, Price: 1024, SourceCode: 45}}, "tuie": {{Goods: 31, Quality: 7, SourceCode: 17}}}
	for _, cancelStage := range []int{0, 1, 2} {
		s := base
		step := func(a Action) {
			t.Helper()
			var e error
			s, e = r.Step(s, a)
			if e != nil {
				t.Fatal(e)
			}
		}
		step(ActionTradeSwapPage)
		if cancelStage == 1 {
			step(ActionBack)
		} else {
			step(ActionTradePick2)
			if s.TradeSaleSlot != 6 || s.TradePage != TradePageSwapWarehouse {
				t.Fatal("pod selector not retained")
			}
			if cancelStage == 2 {
				step(ActionBack)
			} else {
				step(ActionTradePick4)
			}
		}
		if s.TradePage != TradePageWarehouse || s.TradeSaleSlot != 0 || s.TradeSalePrice != 0 {
			t.Fatal("unfinished swap")
		}
		if cancelStage != 0 {
			if s.Holds != base.Holds || !reflect.DeepEqual(s.Warehouses, base.Warehouses) {
				t.Fatal("cancel moved cargo")
			}
		} else {
			if s.Holds[1].Lot() != base.Warehouses[base.CityID][3] || s.Warehouses[s.CityID][3] != base.Holds[1].Lot() {
				t.Fatal("full-container swap lost metadata")
			}
			if s.Holds[0] != base.Holds[0] || !reflect.DeepEqual(s.Warehouses["tuie"], base.Warehouses["tuie"]) {
				t.Fatal("other slot/city changed")
			}
			for i := 0; i < 3; i++ {
				if s.Warehouses[s.CityID][i] != base.Warehouses[base.CityID][i] {
					t.Fatal("unselected warehouse slot changed")
				}
			}
		}
		if s.Bank != base.Bank || s.Cash != base.Cash || s.TradeRolls != base.TradeRolls || s.CombatRolls != base.CombatRolls || s.TimeMinutes != base.TimeMinutes {
			t.Fatal("swap spent funds, time or random input")
		}
	}
	if base.Warehouses[base.CityID][3].Goods != 24 {
		t.Fatal("reducer mutated input map")
	}
}

func TestWarehouseSwapRejectsEmptySelectors(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen, s.TradePage = ScreenTrade, TradePageWarehouse
	s.Holds = [2]Hold{{Kind: HoldGoods, Goods: 9, Quality: 4, Price: 31, SourceCode: 34}, {}}
	s.Warehouses = map[string][]Lot{s.CityID: make([]Lot, 4)}
	s, err = r.Step(s, ActionTradeSwapPage)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []Action{ActionTradePick2, ActionTradePick3, ActionTradePick4} {
		s, err = r.Step(s, a)
		if err != nil || s.TradePage != TradePageSwapPod || s.TradeSaleSlot != 0 {
			t.Fatalf("empty pod accepted: %s %v", a, err)
		}
	}
	s, err = r.Step(s, ActionTradePick1)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []Action{ActionTradePick1, ActionTradePick2, ActionTradePick3, ActionTradePick4} {
		s, err = r.Step(s, a)
		if err != nil || s.TradePage != TradePageSwapWarehouse || s.TradeSaleSlot != 5 {
			t.Fatalf("empty warehouse accepted: %s %v", a, err)
		}
	}
}
