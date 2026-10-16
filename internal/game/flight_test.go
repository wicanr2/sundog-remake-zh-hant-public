package game

import (
	"math"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// TestJumpRangeAndWarpDriveGateNavigation covers the three gates
// XSHOWMOV:0x06 start_jump puts in front of an interstellar jump.
func TestJumpRangeAndWarpDriveGateNavigation(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	// Find a destination whose system is out of range from the start, and one
	// that is inside it. The distances come from the star system coordinates,
	// so this is a statement about the source's own map.
	// The game measures on its own re-rolled chart, so the pair is picked on
	// the chart the state under test carries.
	state := NewState()
	var far, near string
	for _, destination := range reducer.World.Destinations() {
		distance, ok := reducer.World.JumpDistanceWith(state.SystemCoordinates, world.StartSystemID, destination.SystemID)
		if !ok || distance == 0 {
			continue
		}
		if distance > world.MaxJumpDistance() && far == "" {
			far = destination.ID
		}
		if distance <= world.MaxJumpDistance() && near == "" {
			near = destination.ID
		}
	}
	if far == "" || near == "" {
		t.Fatalf("the source map has no reachable/unreachable pair: far=%q near=%q", far, near)
	}

	for _, action := range []Action{ActionPlay, ActionOpenNavigation} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.Fuel = 100

	beyond := cloneStateForTest(state)
	beyond.NavigationTargetID = far
	beyond, err = reducer.Step(beyond, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if beyond.MessageKey != "ui.message.beyond_maximum_jump" || beyond.SystemID != world.StartSystemID {
		t.Fatalf("a jump past the ship's range was allowed: %+v", beyond)
	}

	// A reachable system is refused too when the warp drive is gone.
	blown := cloneStateForTest(state)
	blown.NavigationTargetID = near
	for row := range blown.ShipSystems[0].Rows {
		blown.ShipSystems[0].Rows[row][0] = 0x07
	}
	blown.ShipSystems[0].Status = shipSystemStatus(blown.ShipSystems[0])
	blown, err = reducer.Step(blown, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if blown.MessageKey != "ui.message.warp_engines_blown" || blown.SystemID != world.StartSystemID {
		t.Fatalf("a jump with a blown warp drive was allowed: %+v", blown)
	}

	// With fuel, range and a working drive, the jump goes through and costs the
	// source's own warp charge on top of the in-system leg.
	ok := cloneStateForTest(state)
	ok.NavigationTargetID = near
	before, found := reducer.RouteCost(ok)
	if !found || before.JumpMinutes <= 0 {
		t.Fatalf("route to %q has no jump: %+v", near, before)
	}
	ok, err = reducer.Step(ok, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	ok, err = reducer.Step(ok, ActionFlightEngage)
	if err != nil {
		t.Fatal(err)
	}
	if ok.MessageKey != "ui.message.navigated" || ok.SystemID == world.StartSystemID {
		t.Fatalf("a legal jump was refused: %+v", ok)
	}
	if ok.TimeMinutes < before.JumpMinutes {
		t.Fatalf("the jump took %d minutes, less than its warp charge %d", ok.TimeMinutes, before.JumpMinutes)
	}
}

// TestJumpDistanceMatchesTheSourceFormula pins the measurement itself: the
// integer square root of the summed squared coordinate differences, plus one
// when either end is the adjusted system.
func TestJumpDistanceMatchesTheSourceFormula(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	systems := reducer.World.Systems()
	if len(systems) != 12 {
		t.Fatalf("the world has %d systems", len(systems))
	}
	for _, system := range systems {
		distance, ok := reducer.World.JumpDistance(system.ID, system.ID)
		if !ok || distance != 0 {
			t.Fatalf("a system is %d from itself", distance)
		}
	}
	// Distance is symmetric, and no pair is further apart than the coordinate
	// space allows.
	limit := 0
	for axis := 0; axis < 3; axis++ {
		limit += 15 * 15
	}
	for _, from := range systems {
		for _, to := range systems {
			forward, ok := reducer.World.JumpDistance(from.ID, to.ID)
			if !ok {
				t.Fatalf("no distance between %q and %q", from.ID, to.ID)
			}
			backward, _ := reducer.World.JumpDistance(to.ID, from.ID)
			if forward != backward {
				t.Fatalf("%q→%q is %d but %q→%q is %d", from.ID, to.ID, forward, to.ID, from.ID, backward)
			}
			if forward > 27 {
				t.Fatalf("%q→%q is %d, beyond the coordinate space", from.ID, to.ID, forward)
			}
		}
	}
}

// cloneStateForTest gives a test its own ship system grid. Copying the struct
// alone shares the slices underneath, so editing one copy would edit the other.
func cloneStateForTest(state State) State {
	clone := state
	clone.ShipSystems = make([]ShipSystem, len(state.ShipSystems))
	for index, system := range state.ShipSystems {
		clone.ShipSystems[index] = system
		clone.ShipSystems[index].Rows = copyShipSystemRows(system.Rows)
	}
	return clone
}

// TestSteppingDoesNotMutateTheCallersShipSystems is the property the aliasing
// fix protects: a reducer step works on its own copy, so the state handed in is
// unchanged even when the step destroys a part.
func TestSteppingDoesNotMutateTheCallersShipSystems(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.ShieldLevel = 0
	state.ShieldSetting = 0
	state.ShieldsActive = false
	before := cloneStateForTest(state)
	if _, err = reducer.Step(state, ActionTacticalMove); err != nil {
		t.Fatal(err)
	}
	for index, system := range state.ShipSystems {
		for row := range system.Rows {
			for column, installed := range system.Rows[row] {
				if installed != before.ShipSystems[index].Rows[row][column] {
					t.Fatalf("stepping changed the caller's system %d slot (%d,%d)", index, row, column)
				}
			}
		}
		if system.Status != before.ShipSystems[index].Status {
			t.Fatalf("stepping changed the caller's system %d status", index)
		}
	}
}

// TestFiringNeedsWorkingGunsAndEmptiesTheCharge covers XDOFIGHT:0x06 fire_it's
// two conditions and its one effect.
func TestFiringNeedsWorkingGunsAndEmptiesTheCharge(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}

	fired, err := reducer.Step(cloneStateForTest(state), ActionFire)
	if err != nil {
		t.Fatal(err)
	}
	if fired.WeaponCharge != 0 {
		t.Fatalf("a shot left %d charge behind", fired.WeaponCharge)
	}
	// The next turn refills the weapon at the rate the guns supply.
	next, err := reducer.Step(fired, ActionTacticalMove)
	if err != nil {
		t.Fatal(err)
	}
	if next.WeaponCharge != MaxShipSystemStatus()*weaponRechargePerStatus {
		t.Fatalf("intact guns refilled %d", next.WeaponCharge)
	}

	// With the guns gone, firing is refused outright.
	blown := cloneStateForTest(state)
	gunsIndex := 2
	if blown.ShipSystems[gunsIndex].ID != ShipSystemShipGuns {
		t.Fatalf("system %d is %q", gunsIndex, blown.ShipSystems[gunsIndex].ID)
	}
	for row := range blown.ShipSystems[gunsIndex].Rows {
		blown.ShipSystems[gunsIndex].Rows[row][0] = 0x07
	}
	blown.ShipSystems[gunsIndex].Status = shipSystemStatus(blown.ShipSystems[gunsIndex])
	blown, err = reducer.Step(blown, ActionFire)
	if err != nil {
		t.Fatal(err)
	}
	if blown.MessageKey != "ui.message.guns_blown" || blown.EnemyHull != state.EnemyHull {
		t.Fatalf("a wrecked gun system still fired: %+v", blown)
	}
}

// TestSubcEngineGatesEveryTrip covers the other engine: XSHOWMOV:0x0a
// do_in_system refuses to move without it, and every trip has an in-system leg.
func TestSubcEngineGatesEveryTrip(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// 此案例測航程／任務，先滿足原版非首城降落所需的掃描器。
	fitGroundScannerForTest(t, &state)
	for _, action := range []Action{ActionPlay, ActionOpenNavigation} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.Fuel = 100
	blown := cloneStateForTest(state)
	subcIndex := 1
	if blown.ShipSystems[subcIndex].ID != ShipSystemSubcEngine {
		t.Fatalf("system %d is %q", subcIndex, blown.ShipSystems[subcIndex].ID)
	}
	for row := range blown.ShipSystems[subcIndex].Rows {
		blown.ShipSystems[subcIndex].Rows[row][0] = 0x07
	}
	blown.ShipSystems[subcIndex].Status = shipSystemStatus(blown.ShipSystems[subcIndex])
	blown, err = reducer.Step(blown, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if blown.MessageKey != "ui.message.subc_engines_blown" || blown.CityID != state.CityID {
		t.Fatalf("a trip without a sub-c engine went ahead: %+v", blown)
	}
}

// TestEveryStarSystemCarriesAnEncounterRating pins the field that was carried
// as unused until update_movement was read: it is what the original checks
// before generating an enemy.
func TestEveryStarSystemCarriesAnEncounterRating(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	ratings := make(map[int]int)
	for _, system := range reducer.World.Systems() {
		if system.EncounterRating < 0 || system.EncounterRating > 15 {
			t.Fatalf("system %q has rating %d, outside a 4-bit field", system.ID, system.EncounterRating)
		}
		ratings[system.EncounterRating]++
	}
	if len(ratings) < 5 {
		t.Fatalf("the ratings barely vary across the map: %v", ratings)
	}
}

// TestPolarToRectMatchesTheSourceGeometry checks the projection against facts
// the table cannot fake: a full turn has to come back to where it started, the
// four cardinal angles have to land on the axes, and every point has to sit on
// its own circle.
func TestPolarToRectMatchesTheSourceGeometry(t *testing.T) {
	turn := FlightTurnSteps()
	if turn <= 0 {
		t.Fatal("the sine table is not loaded")
	}
	const radius = 30
	for angle := 0; angle < turn; angle++ {
		x, y := PolarToRect(angle, radius)
		wrapped, wrappedY := PolarToRect(angle+turn, radius)
		if x != wrapped || y != wrappedY {
			t.Fatalf("angle %d and %d project differently", angle, angle+turn)
		}
		// Rounding to whole units costs a little accuracy, so the radius is
		// checked with the tolerance that rounding alone can produce.
		got := math.Hypot(float64(x), float64(y))
		if math.Abs(got-radius) > 1.5 {
			t.Fatalf("angle %d lands %0.2f from the star, want about %d", angle, got, radius)
		}
	}
	// Angle 0 points along +x and the turn runs clockwise on a screen whose y
	// grows downward: SHIPLIB:0x0c only negates y above 0x80, and the original's
	// scanner dots land where that rule says (hatari-live-run.md, round nine).
	// It is what decides which way a trip appears to go.
	for _, cardinal := range []struct {
		angle int
		wantX int
		wantY int
	}{
		{0, radius, 0},
		{turn / 4, 0, radius},
		{turn / 2, -radius, 0},
		{3 * turn / 4, 0, -radius},
	} {
		x, y := PolarToRect(cardinal.angle, radius)
		if x != cardinal.wantX || y != cardinal.wantY {
			t.Fatalf("angle %d projects to %d,%d, want %d,%d", cardinal.angle, x, y, cardinal.wantX, cardinal.wantY)
		}
	}
}

// TestFlightTakesTheShortWayRound is the one rule that would be invisible in a
// screenshot but wrong in play: a trip across the wrap point must not wind the
// long way round.
func TestFlightTakesTheShortWayRound(t *testing.T) {
	turn := FlightTurnSteps()
	state := NewState()
	state.FlightAngle = turn - 4
	state.FlightRadius = 20
	beginInSystemFlight(&state, world.Orbit{Angle: 4, Radius: 20})
	if state.FlightDeltaAngle != 8 {
		t.Fatalf("the short way from %d to 4 is 8 steps, got %d", turn-4, state.FlightDeltaAngle)
	}
	for !FlightArrived(state) {
		advanceInSystemFlight(&state)
		if state.FlightAngle < 0 || state.FlightAngle >= turn {
			t.Fatalf("the angle left one turn: %d", state.FlightAngle)
		}
	}
	if state.FlightAngle != 4 {
		t.Fatalf("the trip ended at %d, want 4", state.FlightAngle)
	}
}

// TestFlightArrivesExactlyOnTheTarget guards the interpolation itself: reading
// both coordinates off the tick count rather than accumulating them is what
// keeps a long trip from drifting past its destination.
func TestFlightArrivesExactlyOnTheTarget(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, planet := range reducer.World.Planets() {
		state := newGameWithFuel()
		state.FlightAngle = 0
		state.FlightRadius = 9
		beginInSystemFlight(&state, planet.Orbit)
		if state.FlightTicks <= 0 {
			t.Fatalf("planet %q gives a trip of %d ticks", planet.ID, state.FlightTicks)
		}
		for updates := 0; !FlightArrived(state) && updates < state.FlightTicks*3; updates++ {
			before := state.FlightTick
			advanceInSystemFlight(&state)
			if state.ShipPhase == 1 && state.FlightTick != before {
				t.Fatal("phase1 advanced the interpolation")
			}
		}
		if !FlightArrived(state) {
			t.Fatalf("planet %q did not arrive within three updates per tick", planet.ID)
		}
		if state.FlightRadius != planet.Orbit.Radius || state.FlightAngle != planet.Orbit.Angle {
			t.Fatalf("trip to %q ended at %d,%d, want %d,%d", planet.ID,
				state.FlightAngle, state.FlightRadius, planet.Orbit.Angle, planet.Orbit.Radius)
		}
	}
}

// TestNavigatingFliesTheTripRatherThanTeleporting pins the player-visible half:
// confirming a destination puts the ship on the flight screen, and only running
// the trip out lands it.
func TestNavigatingFliesTheTripRatherThanTeleporting(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	for _, action := range []Action{ActionPlay, ActionOpenNavigation} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// Two cities of one planet have no in-system leg between them, so the
	// target has to be on another planet for there to be a trip at all.
	target := ""
	for _, destination := range reducer.World.Destinations() {
		if destination.SystemID == state.SystemID && destination.PlanetID != state.PlanetID {
			target = destination.ID
			break
		}
	}
	if target == "" {
		t.Fatal("the start system has only one planet")
	}
	// A new game starts with no fuel (the original writes none in set_new_game),
	// and this test is about the trip, not the starting tank.
	state.Fuel = maxFuel
	state.NavigationTargetID = target
	state, err = reducer.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenFlight || state.FlightTicks <= 0 || FlightArrived(state) {
		t.Fatalf("navigation did not start a flight: %+v", state)
	}
	origin := state.CityID
	ticks := state.FlightTicks
	for updates := 0; state.Screen == ScreenFlight && updates < ticks*3; updates++ {
		beforeTick := state.FlightTick
		state, err = reducer.Step(state, ActionFlightTick)
		if err != nil {
			t.Fatal(err)
		}
		if state.ShipPhase == 1 && state.FlightTick != beforeTick {
			t.Fatal("phase1 advanced the flight")
		}
		if state.FlightTick < ticks && (state.Screen != ScreenFlight || state.CityID != origin) {
			t.Fatal("the ship arrived before the trip was over")
		}
	}
	if state.Screen != ScreenShipboard || state.CityID == origin {
		t.Fatalf("the last tick did not land the ship: %+v", state)
	}
}

func TestMode2DepartureDoesNotPrechargeTheArc(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	state.Screen = ScreenNavigation
	for _, destination := range reducer.World.Destinations() {
		if destination.SystemID == state.SystemID && destination.PlanetID != state.PlanetID && PrimaryCity(destination.CityID) {
			state.NavigationTargetID = destination.ID
			break
		}
	}
	destination, ok := reducer.World.Destination(state.NavigationTargetID)
	if !ok || destination.PlanetID == state.PlanetID {
		t.Fatal("source system lacks a primary city on another planet")
	}
	// Keep the existing world-leg charge8, then leave exactly one fuel for
	// the real phase1 movement. The old combined estimate refused this trip.
	state.Fuel = 9
	next, err := reducer.Step(state, ActionNavigate)
	if err != nil || next.Screen != ScreenFlight || next.Fuel != 1 || next.ShipPhase != state.ShipPhase {
		t.Fatalf("departure: screen=%s fuel=%d phase=%d err=%v", next.Screen, next.Fuel, next.ShipPhase, err)
	}
	next, err = reducer.Step(next, ActionFlightEngage)
	if err != nil || next.Screen != ScreenFlight || next.Fuel != 0 || next.ShipPhase != 1 || next.FlightTick != 0 || next.ShipControls.Subc || next.MessageKey != "ui.message.out_of_fuel" {
		t.Fatalf("stopped flight: screen=%s fuel=%d phase=%d tick=%d subc=%v message=%s err=%v", next.Screen, next.Fuel, next.ShipPhase, next.FlightTick, next.ShipControls.Subc, next.MessageKey, err)
	}
}

// The warp reading is the manual's PROBABILITY WARP SUCCESS. These pin the two
// numbers the original applies to the polar radius, and the one value that ties
// them to something else in the game: the system map's ring.
func TestWarpSuccessReachesOneHundredAtTheSystemRing(t *testing.T) {
	// 43 is the radius of the ring the system map draws (screen rule
	// map.system-orbit-ring). The original's (r + 7) * 2 lands exactly on 100
	// there, which is what says the reading is a percentage.
	state := NewState()
	state.FlightRadius = 43
	if got := WarpSuccessPercent(state); got != 100 {
		t.Fatalf("warp success at the ring = %d, want 100", got)
	}
}

func TestWarpSuccessGrowsWithDistanceFromTheStar(t *testing.T) {
	state := NewState()
	state.FlightRadius = 10
	near := WarpSuccessPercent(state)
	state.FlightRadius = 30
	far := WarpSuccessPercent(state)
	if near != 34 {
		t.Fatalf("warp success at radius 10 = %d, want 34", near)
	}
	if far <= near {
		t.Fatalf("further out is not safer: %d at 30 vs %d at 10", far, near)
	}
}

func TestWarpSuccessIsZeroWithoutAPolarRadius(t *testing.T) {
	state := NewState()
	state.FlightRadius = 0
	if got := WarpSuccessPercent(state); got != 0 {
		t.Fatalf("warp success without a radius = %d, want 0", got)
	}
}
