package game

import (
	"github.com/wicanr2/sundog-remake/internal/world"
	"testing"
)

func TestBanvilleMarketConsumesOneLotWithoutRandomDraw(t *testing.T) {
	for _, sold := range []bool{false, true} {
		for count := 0; count <= 7; count++ {
			market := CityMarket{Supply: make([]int, goodsCodes), Demand: make([]int, goodsCodes)}
			market.Supply[8] = count
			market.Demand[8] = count
			state := State{CityID: world.MissionDestinationID, TradeRolls: 41, Markets: map[string]CityMarket{world.MissionDestinationID: market}}
			state.Colony.Remaining[8] = count
			(Reducer{}).updateMarket(&state, 8, sold)
			got := state.Markets[state.CityID]
			want := count - 1
			if want < 0 {
				want = 0
			}
			updated, unchanged := got.Supply[8], got.Demand[8]
			if sold {
				updated, unchanged = got.Demand[8], got.Supply[8]
			}
			if updated != want || unchanged != count || state.TradeRolls != 41 {
				t.Fatalf("sold=%v count=%d updated=%d other=%d rolls=%d", sold, count, updated, unchanged, state.TradeRolls)
			}
		}
	}
}

func TestOrdinaryMarketStillConsumesRandomDraw(t *testing.T) {
	market := CityMarket{Supply: make([]int, goodsCodes), Demand: make([]int, goodsCodes)}
	market.Demand[8] = 7
	state := State{CityID: "drahew", TradeRolls: 41, Markets: map[string]CityMarket{"drahew": market}}
	(Reducer{}).updateMarket(&state, 8, true)
	if state.TradeRolls != 42 || state.Markets["drahew"].Demand[8] < 5 || state.Markets["drahew"].Demand[8] > 7 {
		t.Fatal("一般城市扣減規則被更動")
	}
}

func TestBanvilleInitialClassesAndReentryNeverAge(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.CityID = world.MissionDestinationID
	s.TimeMinutes = 42 * 24 * 60
	s.TradeRolls = 41
	if _, err := s.Colony.AdvanceAtExchange(); err != nil {
		t.Fatal(err)
	}
	classes, ok := world.TradeClassesFor(s.CityID, s.TradeClassShift)
	if !ok {
		t.Fatal("missing classes")
	}
	m := r.cityMarket(&s, s.CityID)
	if m.LastSeen != 0 || m.Supply[31] != 0 || s.TradeRolls != 41 {
		t.Fatal("original first-visit marker or no-RND changed")
	}
	for goods := 1; goods <= 30; goods++ {
		if m.Supply[goods] != classes.Supply[goods-1] || m.Demand[goods] != s.Colony.Remaining[goods] {
			t.Fatal("initial market source changed")
		}
	}
	m.Supply[8] = 0
	s.Markets[s.CityID] = m
	s.TimeMinutes += 90 * 24 * 60
	got := r.cityMarket(&s, s.CityID)
	if got.Supply[8] != 0 || got.LastSeen != 0 || s.TradeRolls != 41 {
		t.Fatal("Banville reentry aged or reinitialized market")
	}
}
