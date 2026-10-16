package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

func groundAtDepot(t *testing.T) (Reducer, State) {
	t.Helper()
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenGround
	depot, ok := reducer.World.FacilityPosition(state.CityID, world.FacilityDepot)
	if !ok {
		t.Fatal("source city has no depot")
	}
	state.Position = Position{depot.X, depot.Y}
	state.GroundMode, state.CargoAttached = GroundInPod, false
	state.Bank = 0
	return reducer, state
}

func TestAFuelTankTakesOneOfTheCargoPodsLots(t *testing.T) {
	reducer, state := groundAtDepot(t)
	state.Cash = 5000
	loaded := state
	for _, action := range []Action{ActionLoadAuxFuel, ActionFuelBuyOne, ActionFuelCancel} {
		var err error
		loaded, err = reducer.Step(loaded, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	if AuxFuelTanks(loaded) != 1 {
		t.Fatalf("tanks = %d, want 1", AuxFuelTanks(loaded))
	}
	// Source system1 factor22 gives a 2200-price auxiliary tank.
	if loaded.Cash != 2800 {
		t.Fatalf("cash = %d", loaded.Cash)
	}
	if got, want := CargoLotsFree(loaded), CargoLots-1; got != want {
		t.Fatalf("free lots = %d, want %d", got, want)
	}
}

func TestTheCargoPodCannotHoldMoreLotsThanItHas(t *testing.T) {
	reducer, state := groundAtDepot(t)
	state.Cash = 5000
	state.Holds = fullOfTanks()
	next := state
	for _, action := range []Action{ActionLoadAuxFuel, ActionFuelBuyOne, ActionFuelBuyFull, ActionFuelCancel} {
		var err error
		next, err = reducer.Step(next, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	if AuxFuelTanks(next) != CargoLots {
		t.Fatalf("a full pod still took a tank: %d", AuxFuelTanks(next))
	}
	if next.Holds != state.Holds || next.Cash != state.Cash || next.Bank != state.Bank {
		t.Fatal("buying into a full pod changed cargo or funds")
	}
}

func TestAFullTankEmptiesItselfIntoADryMainTank(t *testing.T) {
	state := State{Fuel: 0, Holds: oneTank(), CargoAttached: true}
	if !drainAuxFuelTank(&state) {
		t.Fatal("a dry tank with a spare did not refill")
	}
	if state.Fuel != maxFuel {
		t.Fatalf("fuel = %d, want the tank to fill it", state.Fuel)
	}
	if AuxFuelTanks(state) != 0 {
		t.Fatalf("tanks = %d, want the spent one gone", AuxFuelTanks(state))
	}
}

func TestATankIsNotSpentWhileThereIsStillFuel(t *testing.T) {
	state := State{Fuel: 1, Holds: oneTank()}
	if drainAuxFuelTank(&state) {
		t.Fatal("a tank was spent with fuel still in the main tank")
	}
	if AuxFuelTanks(state) != 1 {
		t.Fatalf("tanks = %d", AuxFuelTanks(state))
	}
}

func TestDroppingATankNeedsOneAboard(t *testing.T) {
	reducer, state := groundAtDepot(t)
	next, err := reducer.Step(state, ActionUnloadAuxFuel)
	if err != nil {
		t.Fatal(err)
	}
	if next.MessageKey != "ui.message.no_aux_fuel" {
		t.Fatalf("message = %q", next.MessageKey)
	}
}

// SHIPLIB pays phase1 movement before its <=0 fuel consumer. The separate
// XMOVEONS <5 consumer belongs to pod reentry, not the flight loop.

func inSystemFlight(fuel, tanks int) State {
	state := NewState()
	state.Fuel = fuel
	for i := 0; i < tanks; i++ {
		loadHold(&state, Hold{Kind: HoldAuxFuel})
	}
	state.FlightAngle = 0
	state.FlightRadius = 20
	beginInSystemFlight(&state, world.Orbit{Angle: 8, Radius: 40})
	return state
}

func TestAFlightTickLeavesTheTankAloneBelowFive(t *testing.T) {
	state := inSystemFlight(4, 1)
	if advanceInSystemFlight(&state) {
		t.Fatal("flight incorrectly applied the pod-reentry threshold")
	}
	if state.Fuel != 3 {
		t.Fatalf("phase1 fuel = %d, want 3", state.Fuel)
	}
	if AuxFuelTanks(state) != 1 {
		t.Fatalf("tanks = %d, want it retained", AuxFuelTanks(state))
	}
}

func TestAFlightTickLeavesTheTankAloneAtFive(t *testing.T) {
	state := inSystemFlight(auxPodRefillThreshold, 1)
	if advanceInSystemFlight(&state) {
		t.Fatal("a tank was spent at the threshold rather than under it")
	}
	if AuxFuelTanks(state) != 1 {
		t.Fatalf("tanks = %d", AuxFuelTanks(state))
	}
	if state.Fuel != 4 {
		t.Fatalf("phase1 fuel = %d, want 4", state.Fuel)
	}
}

func TestAFlightTickWithNoTankStopsAtZero(t *testing.T) {
	state := inSystemFlight(1, 0)
	if advanceInSystemFlight(&state) {
		t.Fatal("a tick reported a refill with no tank aboard")
	}
	if state.Fuel != 0 || state.ShipControls.Subc {
		t.Fatalf("fuel=%d subc=%v, want 0 and stopped", state.Fuel, state.ShipControls.Subc)
	}
}

func TestAFlightTickAtZeroUsesOnlyAnAttachedTank(t *testing.T) {
	for _, attached := range []bool{false, true} {
		state := inSystemFlight(1, 1)
		state.CargoAttached = attached
		spent := advanceInSystemFlight(&state)
		if spent != attached {
			t.Fatalf("attached=%v spent=%v", attached, spent)
		}
		if attached && (state.Fuel != 255 || AuxFuelTanks(state) != 0 || !state.ShipControls.Subc) {
			t.Fatalf("attached tank failed: fuel=%d tanks=%d subc=%v", state.Fuel, AuxFuelTanks(state), state.ShipControls.Subc)
		}
		if !attached && (state.Fuel != 0 || AuxFuelTanks(state) != 1 || state.ShipControls.Subc) {
			t.Fatal("detached tank affected the ship")
		}
	}
}

func TestPodReentryAppliesTheBelowFiveThreshold(t *testing.T) {
	for _, tc := range []struct {
		fuel, tanks         int
		mode                GroundMode
		wantFuel, wantTanks int
	}{
		{4, 1, GroundInPod, 255, 0},
		{5, 1, GroundInPod, 5, 1},
		{1, 0, GroundInPod, 1, 0},
		{4, 1, GroundOnFoot, 4, 1},
	} {
		state := NewState()
		state.Screen, state.GroundMode = ScreenGround, tc.mode
		state.CargoAttached = false
		state.Position, state.Fuel = state.ShipPosition, tc.fuel
		for i := 0; i < tc.tanks; i++ {
			loadHold(&state, Hold{Kind: HoldAuxFuel})
		}
		offset := Position{23, 20}
		if tc.mode == GroundOnFoot {
			offset.X = 17
		}
		if !BoardGroundShip(&state, offset, true) {
			t.Fatal("normal ship anchor did not board")
		}
		if state.Fuel != tc.wantFuel || AuxFuelTanks(state) != tc.wantTanks || state.GroundMode != GroundOnFoot || state.Screen != ScreenShipboard {
			t.Fatalf("fuel=%d mode=%s: got fuel=%d tanks=%d", tc.fuel, tc.mode, state.Fuel, AuxFuelTanks(state))
		}
		if state.CargoAttached != (tc.mode == GroundInPod) {
			t.Fatal("reentry attached the wrong cargo mode")
		}
	}
}

// The dry-tank threshold is the other call site's, and it stays where it was:
// four units left is not dry, so SHIPLIB:0x24's check does not fire.
func TestTheDryTankThresholdIsStillZero(t *testing.T) {
	state := State{Fuel: 4, Holds: oneTank()}
	if drainAuxFuelTank(&state) {
		t.Fatal("the dry-tank check fired with fuel still aboard")
	}
	if AuxFuelTanks(state) != 1 {
		t.Fatalf("tanks = %d", AuxFuelTanks(state))
	}
}

// oneTank and fullOfTanks are pods carrying auxiliary fuel, for the tests that
// used to set a counter.
func oneTank() [CargoLots]Hold {
	var holds [CargoLots]Hold
	holds[0] = Hold{Kind: HoldAuxFuel}
	return holds
}

func fullOfTanks() [CargoLots]Hold {
	var holds [CargoLots]Hold
	for i := range holds {
		holds[i] = Hold{Kind: HoldAuxFuel}
	}
	return holds
}
