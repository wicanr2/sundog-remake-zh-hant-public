package game

import (
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
	"testing"
)

func TestOrdinaryCitySourceExchangeOpens(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, city := range data.Cities() {
		if city.Hidden {
			continue
		}
		m, ok := world.CityMap(city.ID)
		if !ok {
			t.Fatal("missing city map")
		}
		for _, building := range m.Buildings() {
			if building.Facility != atarist.FacilityExchange {
				continue
			}
			s := NewState()
			s.Screen, s.CityID = ScreenGround, city.ID
			d, ok := r.World.Destination(city.ID)
			if !ok {
				t.Fatal("missing source destination")
			}
			s.SystemID, s.PlanetID = d.SystemID, d.PlanetID
			s.Position = Position{building.Row, building.Column}
			if !r.GroundHasFacility(s, world.FacilityExchange) || r.GroundHasFacility(s, world.FacilityColony) {
				t.Fatalf("source exchange missing or colony leaked in %s", city.ID)
			}
			next, err := r.Step(s, ActionOpenTrade)
			if err != nil || next.Screen != ScreenTrade || next.TradePage != TradePageEntrance || !next.TradeSession.Active {
				t.Fatalf("source exchange refused in %s: %v", city.ID, err)
			}
			count++
		}
	}
	if count != 57 {
		t.Fatalf("checked %d ordinary exchanges, want57", count)
	}
}

func fitGroundScannerForTest(t *testing.T, state *State) {
	t.Helper()
	source, ok := sourceShipSystem(ShipSystemPilotage)
	if !ok || source.Alternate == nil {
		t.Fatal("缺少掃描器來源資料")
	}
	state.ShipSystems = append([]ShipSystem(nil), state.ShipSystems...)
	for i := range state.ShipSystems {
		if state.ShipSystems[i].ID == ShipSystemPilotage {
			state.ShipSystems[i].Rows = intactRows(ShipSystemPilotage)
			state.ShipSystems[i].Rows[0][source.Alternate.Column] = source.Alternate.ItemIndex
			state.ShipSystems[i].Status = shipSystemStatus(state.ShipSystems[i])
		}
	}
	if !GroundScannerFitted(*state) {
		t.Fatal("掃描器沒有裝妥")
	}
}

func TestGroundScannerBlocksSecondaryLandingWithoutSpending(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	state.Screen = ScreenNavigation
	state.NavigationTargetID = world.MissionDestinationID
	next, err := r.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if next.MessageKey != "ui.message.ground_scanner_required" || next.Fuel != state.Fuel || next.TimeMinutes != state.TimeMinutes || next.CombatRolls != state.CombatRolls {
		t.Fatalf("掃描器門檻改動航程：%s", next.MessageKey)
	}
	fitGroundScannerForTest(t, &state)
	next, err = r.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if next.ShipCityID != world.MissionDestinationID || next.ShipPosition != (Position{9, 6}) {
		t.Fatalf("Banville 停船位置：%s %+v", next.ShipCityID, next.ShipPosition)
	}
}

func TestOriginalSecondaryLandingPerimeter(t *testing.T) {
	state := NewState()
	city := ""
	for _, d := range geographyRegistry.Destinations() {
		if !PrimaryCity(d.CityID) && d.CityID != world.MissionDestinationID {
			city = d.CityID
			break
		}
	}
	if city == "" {
		t.Fatal("沒有非首城")
	}
	sides := map[int]bool{}
	for i := 0; i < 200; i++ {
		p := groundLandingPosition(&state, city)
		switch {
		case p.Y == -1 && p.X >= 0 && p.X <= 20:
			sides[0] = true
		case p.X == 21 && p.Y >= 0 && p.Y <= 11:
			sides[1] = true
		case p.Y == 12 && p.X >= 0 && p.X <= 20:
			sides[2] = true
		case p.X == -1 && p.Y >= 0 && p.Y <= 11:
			sides[3] = true
		default:
			t.Fatalf("落點不在原版四邊：%+v", p)
		}
	}
	if len(sides) != 4 {
		t.Fatalf("缺少四邊抽樣：%v", sides)
	}
}

func TestCityExitEntryKeepsShipAndOriginalAnchors(t *testing.T) {
	state := NewState()
	state.Screen = ScreenGround
	shipCity, shipPosition := state.ShipCityID, state.ShipPosition
	if GroundLeavesCity(state, Position{-8, -5}) {
		t.Fatal("合法鏡頭原點被判出界")
	}
	if !LeaveGroundCity(&state, Position{-9, 0}) {
		t.Fatal("沒有出城")
	}
	if !state.GroundWorld || state.GroundView.Anchor != (Position{163, 116}) {
		t.Fatalf("出城人物錨點：%+v", state.GroundView)
	}
	if !EnterGroundCity(&state, Position{3, 16}) {
		t.Fatal("無法從西側進城")
	}
	if state.Position.X != -1 || state.GroundView.Anchor != (Position{160, 108}) {
		t.Fatalf("進城位置：%+v %+v", state.Position, state.GroundView)
	}
	if state.ShipCityID != shipCity || state.ShipPosition != shipPosition {
		t.Fatal("步行把船搬走")
	}
}

func TestGroundBoardingUsesOriginalTolerance(t *testing.T) {
	for _, tc := range []struct {
		offset Position
		want   bool
	}{{Position{13, 16}, true}, {Position{21, 24}, true}, {Position{12, 20}, false}, {Position{17, 25}, false}} {
		state := NewState()
		state.Screen = ScreenGround
		if got := BoardGroundShip(&state, tc.offset, false); got != tc.want {
			t.Fatalf("回船 %+v=%v", tc.offset, got)
		}
	}
	state := NewState()
	state.Screen = ScreenGround
	state.GroundWorld = true
	if BoardGroundShip(&state, Position{17, 20}, false) {
		t.Fatal("地表借用了城市船位")
	}
}
