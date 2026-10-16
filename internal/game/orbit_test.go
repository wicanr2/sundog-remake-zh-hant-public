package game

import "testing"

// planet.orbit-rate: angle += rate × elapsed days, wrapped into a byte, and
// nothing happens at all until the day count changes.
func TestPlanetsOnlyMoveWhenTheDayChanges(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	planets := reducer.World.Planets()
	state := NewState()
	before := append([]int(nil), state.PlanetAngles...)

	// Most of a day is not a day.
	state.TimeMinutes = minutesPerDay - 1
	advancePlanetOrbits(&state, planets)
	for index, angle := range state.PlanetAngles {
		if angle != before[index] {
			t.Fatalf("planet %d moved before the day rolled over: %d → %d", index, before[index], angle)
		}
	}

	state.TimeMinutes = minutesPerDay
	advancePlanetOrbits(&state, planets)
	moved := 0
	for _, planet := range planets {
		got := state.PlanetAngles[planet.Index]
		want := (before[planet.Index] + planet.OrbitRate) % OrbitTurnSteps()
		if got != want {
			t.Fatalf("planet %q at rate %d: angle %d → %d, want %d",
				planet.Name, planet.OrbitRate, before[planet.Index], got, want)
		}
		if planet.OrbitRate > 0 {
			moved++
		}
	}
	if moved == 0 {
		t.Fatal("no planet has a non-zero orbit rate, so this proves nothing")
	}
}

// The rates are the source's: eighteen planets, rates 0..8, and six that never
// move. A decoder change that flattened them would make the sky static without
// any test noticing.
func TestOrbitRatesComeFromTheSourceTable(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	planets := reducer.World.Planets()
	if len(planets) != 18 {
		t.Fatalf("world has %d planets, want 18", len(planets))
	}
	still := 0
	for _, planet := range planets {
		if planet.OrbitRate < 0 || planet.OrbitRate > 8 {
			t.Fatalf("planet %q has orbit rate %d, outside the decoded range 0..8", planet.Name, planet.OrbitRate)
		}
		if planet.OrbitRate == 0 {
			still++
		}
	}
	if still != 6 {
		t.Fatalf("%d planets have rate 0, want the six the source table holds", still)
	}
}

// An angle wraps rather than running off the end of the byte.
func TestOrbitAngleWrapsIntoOneTurn(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	planets := reducer.World.Planets()
	state := NewState()
	for index := range state.PlanetAngles {
		state.PlanetAngles[index] = OrbitTurnSteps() - 1
	}
	state.TimeMinutes = minutesPerDay * 40
	advancePlanetOrbits(&state, planets)
	for index, angle := range state.PlanetAngles {
		if angle < 0 || angle >= OrbitTurnSteps() {
			t.Fatalf("planet %d ended at angle %d, outside one turn", index, angle)
		}
	}
}

// planet.orbit-angle-is-live-state: a new game re-rolls every angle, and the
// original keeps re-rolling while a planet lands within fifteen steps of the
// one before it.
func TestNewGameRerollsTheSkyAndSpreadsIt(t *testing.T) {
	state := NewState()
	factory := append([]int(nil), state.PlanetAngles...)
	rolled := rollPlanetAngles(&state, len(factory))
	if len(rolled) != len(factory) {
		t.Fatalf("rolled %d angles, want %d", len(rolled), len(factory))
	}
	same := 0
	for index, angle := range rolled {
		if angle < 0 || angle >= OrbitTurnSteps() {
			t.Fatalf("rolled angle %d is outside one turn", angle)
		}
		if angle == factory[index] {
			same++
		}
	}
	if same == len(factory) {
		t.Fatal("the re-roll returned the factory sky unchanged")
	}
	for index := 1; index < len(rolled); index++ {
		if abs(rolled[index]-rolled[index-1]) <= orbitSpreadFloor {
			t.Fatalf("planets %d and %d are %d steps apart, want more than %d",
				index-1, index, abs(rolled[index]-rolled[index-1]), orbitSpreadFloor)
		}
	}
}

// landing.skyline: six buildings, fixed spacing, placement that moves.
func TestSkylineIsSixBuildingsAtTheOriginalsSpacing(t *testing.T) {
	state := NewState()
	buildings := rollSkyline(&state)
	if len(buildings) != skylineBuildings {
		t.Fatalf("skyline has %d buildings, want %d", len(buildings), skylineBuildings)
	}
	for index, building := range buildings {
		low := index * skylineSpacing
		if building.X < low || building.X >= low+skylineJitter {
			t.Fatalf("building %d sits at x=%d, outside its %d..%d slot",
				index, building.X, low, low+skylineJitter-1)
		}
		if building.Style < 0 || building.Style >= skylineStyles {
			t.Fatalf("building %d has style %d", index, building.Style)
		}
		if building.Size < skylineSizeBase || building.Size >= skylineStyles+skylineSizeBase {
			t.Fatalf("building %d has size %d", index, building.Size)
		}
	}
}

// Two arrivals do not produce the same street.
func TestSkylineChangesBetweenArrivals(t *testing.T) {
	state := NewState()
	first := rollSkyline(&state)
	second := rollSkyline(&state)
	same := true
	for index := range first {
		if first[index] != second[index] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("two arrivals generated the same skyline")
	}
}

// Landing and taking off both carry one.
func TestLandingAndTakeOffBothCarryASkyline(t *testing.T) {
	state := NewState()
	if got := beginLanding(&state, "drahew"); len(got.Skyline) != skylineBuildings {
		t.Fatalf("a landing carries %d buildings", len(got.Skyline))
	}
	if got := beginTakeOff(&state, "drahew"); len(got.Skyline) != skylineBuildings {
		t.Fatalf("a take-off carries %d buildings", len(got.Skyline))
	}
}
