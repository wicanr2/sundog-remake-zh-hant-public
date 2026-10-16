package storage

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestPendingWarehouseSwapSurvivesSaveAndLibrary(t *testing.T) {
	for _, library := range []bool{false, true} {
		for _, cancel := range []bool{false, true} {
			r, err := game.NewReducer()
			if err != nil {
				t.Fatal(err)
			}
			s := game.NewState()
			for _, action := range []game.Action{game.ActionPlay, game.ActionOpenGround, game.ActionOpenTrade} {
				s, err = r.Step(s, action)
				if err != nil {
					t.Fatal(err)
				}
			}
			s.Screen, s.TradePage = game.ScreenTrade, game.TradePageWarehouse
			kind, _ := r.World.CargoKindByCode(9)
			s.Holds = [2]game.Hold{{Kind: game.HoldGoods, Goods: 9, Quality: 4, Price: 31, SourceCode: 34, ItemKey: kind.Key}, {}}
			s.Warehouses = map[string][]game.Lot{s.CityID: {{Goods: 31, Quality: 7, SourceCode: 4}, {}, {}, {}}}
			beforeHold, beforeWarehouse := s.Holds[0].Lot(), s.Warehouses[s.CityID][0]
			step := func(a game.Action) {
				t.Helper()
				var e error
				s, e = r.Step(s, a)
				if e != nil {
					t.Fatal(e)
				}
			}
			step(game.ActionTradeSwapPage)
			step(game.ActionTradePick1)
			for _, a := range []game.Action{game.ActionBuyGems, game.ActionSellGems, game.ActionTradePass, game.ActionTradeExchange, game.ActionTradeWarehousePage, game.ActionTradeStorePage, game.ActionTradeFetchPage, game.ActionTradeSwapPage} {
				again, e := r.Step(s, a)
				if e != nil || !reflect.DeepEqual(again, s) {
					t.Fatalf("pending swap changed by %s: %v", a, e)
				}
			}
			path := filepath.Join(t.TempDir(), "swap.json")
			if library {
				if e := MakeLibrary(path); e != nil {
					t.Fatal(e)
				}
			}
			roundTrip := func() {
				t.Helper()
				if library {
					if e := StoreGame(path, "slot-1", s); e != nil {
						t.Fatal(e)
					}
					s, err = LoadGame(path, "slot-1")
				} else {
					err = Save(path, s)
					if err == nil {
						s, err = Load(path)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			roundTrip()
			if s.Schema != 44 || s.TradePage != game.TradePageSwapWarehouse || s.TradeSaleSlot != 5 || s.TradeSalePrice != 0 || s.Holds[0].Lot() != beforeHold || s.Warehouses[s.CityID][0] != beforeWarehouse {
				t.Fatal("pending source lost on reopen")
			}
			for _, mutate := range []func(*game.State){
				func(v *game.State) { v.TradeSaleSlot = 4 },
				func(v *game.State) { v.TradeSaleSlot = 0 },
				func(v *game.State) { v.TradeSalePrice = 1 },
				func(v *game.State) { v.Holds[0] = game.Hold{} },
			} {
				bad := s
				mutate(&bad)
				if e := Save(filepath.Join(t.TempDir(), "bad.json"), bad); e == nil {
					t.Fatal("invalid pending swap accepted")
				}
			}
			if cancel {
				step(game.ActionBack)
			} else {
				step(game.ActionTradePick1)
			}
			roundTrip()
			if s.TradePage != game.TradePageWarehouse || s.TradeSaleSlot != 0 {
				t.Fatal("completed/canceled swap not saved")
			}
			if cancel {
				if s.Holds[0].Lot() != beforeHold || s.Warehouses[s.CityID][0] != beforeWarehouse {
					t.Fatal("canceled swap moved cargo")
				}
			} else if s.Holds[0].Lot() != beforeWarehouse || s.Warehouses[s.CityID][0] != beforeHold {
				t.Fatal("swapped metadata lost on reopen")
			}
		}
	}
}
