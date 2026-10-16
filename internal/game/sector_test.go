package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// TestSectorRerollKeepsTheSourceShape pins the facts init_systems guarantees
// on every chart it rolls: Woremed never moves, Enlie sits at the far edge and
// exactly one jump past a new ship's range, every field stays inside its
// clamp, and no two systems share a 3x3 neighbourhood.
func TestSectorRerollKeepsTheSourceShape(t *testing.T) {
	systems := world.FactorySystems()
	if len(systems) != 12 || systems[sectorFixedSystem].ID != "woremed" || systems[sectorFarSystem].ID != "enlie" {
		t.Fatalf("the source table is not the twelve records this rule is about: %+v", systems)
	}
	for seed := int64(1); seed <= 64; seed++ {
		coords := JitterSystemCoordinates(seed, systems)
		if got := coords["woremed"]; got != sectorFixedCoordinates {
			t.Fatalf("seed %d moved Woremed to %v", seed, got)
		}
		enlie := coords["enlie"]
		if enlie[2] < sectorFarZBase || enlie[2] >= sectorFarZBase+sectorFarZSpan {
			t.Fatalf("seed %d put Enlie at z=%d, want %d..%d", seed, enlie[2], sectorFarZBase, sectorFarZBase+sectorFarZSpan-1)
		}
		table := make([][3]int, len(systems))
		for i, system := range systems {
			c := coords[system.ID]
			table[i] = c
			for f := 0; f < 3; f++ {
				lo, hi := sectorBounds[f][0], sectorBounds[f][1]
				if i == sectorFarSystem && f == 2 {
					continue
				}
				if c[f] < lo || c[f] > hi {
					t.Fatalf("seed %d: %s field %d = %d, outside %d..%d", seed, system.ID, f, c[f], lo, hi)
				}
			}
		}
		if got := sectorNearestJump(table, sectorFarSystem); got != sectorFarNearestJump {
			t.Fatalf("seed %d: Enlie's nearest neighbour is %d jumps away, want %d", seed, got, sectorFarNearestJump)
		}
		for i := range table {
			for j := range table {
				if i == j {
					continue
				}
				if abs(table[i][0]-table[j][0]) < sectorSpacing && abs(table[i][1]-table[j][1]) < sectorSpacing {
					t.Fatalf("seed %d: %s and %s share a neighbourhood: %v %v", seed, systems[i].ID, systems[j].ID, table[i], table[j])
				}
			}
		}
	}
}

// TestSectorRerollIsSeededAndDiffersBetweenGames: the same seed gives the same
// chart (replays stay reproducible), different seeds give different charts
// (two new games do not open on the same map).
func TestSectorRerollIsSeededAndDiffersBetweenGames(t *testing.T) {
	systems := world.FactorySystems()
	a, b := JitterSystemCoordinates(7, systems), JitterSystemCoordinates(7, systems)
	for id := range a {
		if a[id] != b[id] {
			t.Fatalf("seed 7 rolled %s twice as %v and %v", id, a[id], b[id])
		}
	}
	same := 0
	for seed := int64(1); seed <= 16; seed++ {
		c := JitterSystemCoordinates(seed, systems)
		equal := true
		for id := range a {
			if a[id] != c[id] {
				equal = false
			}
		}
		if equal {
			same++
		}
	}
	if same > 1 {
		t.Fatalf("%d of 16 seeds rolled the same chart as seed 7", same)
	}
}

// TestNewGameMeasuresJumpsOnItsOwnChart: the route cost the reducer reports
// comes from the state's chart, not the factory table.
func TestNewGameMeasuresJumpsOnItsOwnChart(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	if len(state.SystemCoordinates) != 12 {
		t.Fatalf("a new game carries %d system coordinates, want 12", len(state.SystemCoordinates))
	}
	state.NavigationTargetID = world.MissionDestinationID
	cost, ok := reducer.RouteCost(state)
	if !ok {
		t.Fatal("no route cost to the mission destination")
	}
	destination, _ := reducer.World.Destination(world.MissionDestinationID)
	want, _ := reducer.World.JumpDistanceWith(state.SystemCoordinates, state.SystemID, destination.SystemID)
	if cost.JumpDistance != want {
		t.Fatalf("route cost measured %d jumps, the state's chart says %d", cost.JumpDistance, want)
	}
}

// 舊保存不再遷移；不能用預設星圖或相位推測舊進度。
func TestOlderSaveWithoutFactoryChartIsRejected(t *testing.T) {
	state := NewState()
	state.Schema = 21
	state.SystemCoordinates = nil
	if _, err := Migrate(state); err == nil {
		t.Fatal("舊保存被自動補成目前格式")
	}
}
