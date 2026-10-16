package game

import "testing"

// system.heat-raised-by-firing: a shot in space heats the system, clamped to 15.
func TestFiringHeatsTheSystemUpToTheCeiling(t *testing.T) {
	state := NewState()
	state.SystemID = "jondd"
	for shot := 1; shot <= maxSystemHeat+5; shot++ {
		raiseSystemHeat(&state)
		want := shot
		if want > maxSystemHeat {
			want = maxSystemHeat
		}
		if got := SystemHeat(state, "jondd"); got != want {
			t.Fatalf("after %d shots heat = %d, want %d", shot, got, want)
		}
	}
}

// Heat belongs to the system it was earned in, not to the player.
func TestHeatIsPerSystem(t *testing.T) {
	state := NewState()
	state.SystemID = "jondd"
	raiseSystemHeat(&state)
	state.SystemID = "ferr"
	raiseSystemHeat(&state)
	raiseSystemHeat(&state)
	if got := SystemHeat(state, "jondd"); got != 1 {
		t.Fatalf("jondd heat = %d, want 1", got)
	}
	if got := SystemHeat(state, "ferr"); got != 2 {
		t.Fatalf("ferr heat = %d, want 2", got)
	}
}

// system.heat-cleared-by-jettison: dumping cargo wipes it, and only for the
// system the player is standing in.
func TestJettisonClearsOnlyTheCurrentSystemsHeat(t *testing.T) {
	state := NewState()
	state.SystemID = "jondd"
	raiseSystemHeat(&state)
	state.SystemID = "ferr"
	raiseSystemHeat(&state)
	clearSystemHeat(&state)
	if got := SystemHeat(state, "ferr"); got != 0 {
		t.Fatalf("ferr heat = %d after jettison, want 0", got)
	}
	if got := SystemHeat(state, "jondd"); got != 1 {
		t.Fatalf("jettison in ferr cleared jondd too: %d", got)
	}
}

// The whole point of the mechanic, end to end: fire in a system, and jettisoning
// buys the record back.
func TestFiringThenJettisoningLeavesNoTrace(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenTactical
	state.CombatActive = true
	state.Holds[0] = Hold{Kind: HoldGoods, Goods: 2, ItemKey: "atarist.cargo.food"}
	fired, err := reducer.Step(state, ActionFire)
	if err != nil {
		t.Fatal(err)
	}
	if SystemHeat(fired, fired.SystemID) == 0 {
		t.Fatalf("firing left the system cold: %+v", fired.SystemHeat)
	}
	dumped, err := reducer.Step(fired, ActionJettisonHold1)
	if err != nil {
		t.Fatal(err)
	}
	if got := SystemHeat(dumped, dumped.SystemID); got != 0 {
		t.Fatalf("heat after jettison = %d, want 0", got)
	}
}

// system.heat-raises-encounters: more heat, more encounters. A cold system never
// draws one from this rule at all.
func TestHeatMakesEncountersMoreLikely(t *testing.T) {
	count := func(heat int) int {
		drawn := 0
		for trial := 0; trial < 400; trial++ {
			state := NewState()
			state.SystemID = "jondd"
			state.CombatRolls = trial
			if heat > 0 {
				state.SystemHeat = map[string]int{"jondd": heat}
			}
			if heatDrawsAnEncounter(&state) {
				drawn++
			}
		}
		return drawn
	}
	if cold := count(0); cold != 0 {
		t.Fatalf("a cold system drew %d encounters from heat alone, want 0", cold)
	}
	low, high := count(3), count(maxSystemHeat)
	if !(low < high) {
		t.Fatalf("heat 3 drew %d and heat %d drew %d; more heat has to mean more encounters",
			low, maxSystemHeat, high)
	}
}

// A step must not write heat into the caller's state.
func TestSteppingDoesNotMutateTheCallersHeat(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenTactical
	state.CombatActive = true
	state.SystemHeat = map[string]int{"jondd": 2}
	if _, err := reducer.Step(state, ActionFire); err != nil {
		t.Fatal(err)
	}
	if got := SystemHeat(state, "jondd"); got != 2 {
		t.Fatalf("the caller's heat moved to %d", got)
	}
}

// XPILOTAG:0b only schedules the tender. The original crossing midnight
// changes the ETA record, without advancing the main clock or charging RND.
func TestDistressCallSchedulesAcrossMidnight(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenShipboard
	state.ShipCityID = "banville"
	state.TimeMinutes = 1439
	state.Fuel = 10
	next, err := reducer.Step(state, ActionDistressCall)
	if err != nil {
		t.Fatal(err)
	}
	if next.Fuel != 10 || next.TimeMinutes != 1439 || next.FuelRescueDue != 1454 || !next.FuelRescuePending || next.CombatRolls != state.CombatRolls {
		t.Fatal("city distress must preserve fuel/clock/RNG and schedule the next day")
	}
	if next.MessageKey != "ui.message.refuel_eta" {
		t.Fatalf("message = %q", next.MessageKey)
	}
}

func TestDistressCallUsesFirstCityGateWithAFullTank(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenShipboard
	state.Fuel = maxFuel
	next, err := reducer.Step(state, ActionDistressCall)
	if err != nil {
		t.Fatal(err)
	}
	if next.TimeMinutes != 0 || next.FuelRescuePending || next.MessageKey != "ui.message.distress_unavailable" {
		t.Fatal("first city must not offer distress")
	}
	state.ShipCityID = "banville"
	next, err = reducer.Step(state, ActionDistressCall)
	if err != nil {
		t.Fatal(err)
	}
	if !next.FuelRescuePending || next.Fuel != maxFuel || next.TimeMinutes != 0 || next.FuelRescueDue != 15 {
		t.Fatal("a full tank outside the first city must still offer distress")
	}
}
