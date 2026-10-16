package game

import "testing"

// Each test names the rule it checks, so a failure points at the evidence
// rather than at a number someone once chose.

func tacticalState() State {
	// A new game has no fuel (worklist D6); the tractor gates below are about
	// range and room, and one test sets the tank back to zero on purpose.
	state := newGameWithFuel()
	state.Screen = ScreenTactical
	state.CombatActive = true
	state.CombatRange = 10
	state.Holds = [CargoLots]Hold{}
	return state
}

// tractor.gates: out of range, then no room, then out of fuel — in that order.
func TestTractorGatesRunInTheOriginalsOrder(t *testing.T) {
	state := tacticalState()
	state.CombatRange = tractorRange + 1
	state.Holds = fullOfTanks()
	state.Fuel = 0
	if got := runTractor(&state); got != TractorOutOfRange {
		t.Fatalf("range is checked first, got %q", got)
	}

	state = tacticalState()
	state.Holds = fullOfTanks()
	state.Fuel = 0
	if got := runTractor(&state); got != TractorNoRoom {
		t.Fatalf("room is checked before fuel, got %q", got)
	}

	state = tacticalState()
	state.Fuel = 0
	if got := runTractor(&state); got != TractorOutOfFuel {
		t.Fatalf("fuel is the last gate, got %q", got)
	}
}

// tractor.cargo-unknown: what comes aboard occupies a lot and has no identity.
func TestTractorFillsALotWithSomethingUnnamed(t *testing.T) {
	state := tacticalState()
	before := CargoLotsFree(state)
	if got := runTractor(&state); got != TractorLoaded {
		t.Fatalf("a clear tractor run should load, got %q", got)
	}
	if SalvageLots(state) != 1 {
		t.Fatalf("salvage lots = %d, want 1", SalvageLots(state))
	}
	// do_tractor takes the last free hold, counting back from hold 2.
	if state.Holds[CargoLots-1].Kind != HoldSalvage {
		t.Fatalf("the tractor did not fill the last hold: %+v", state.Holds)
	}
	if got := CargoLotsFree(state); got != before-1 {
		t.Fatalf("free lots = %d, want %d", got, before-1)
	}
	if CargoItemKey(state) != "" {
		t.Fatalf("salvage must not name itself as tradable goods: %q", CargoItemKey(state))
	}
}

// tractor.free-hold-scan: a full pod has no room, whatever fills it.
func TestTractorRefusesWhenSalvageAlreadyFillsThePod(t *testing.T) {
	state := tacticalState()
	for i := range state.Holds {
		state.Holds[i] = Hold{Kind: HoldSalvage}
	}
	if got := runTractor(&state); got != TractorNoRoom {
		t.Fatalf("a pod full of salvage still has no room, got %q", got)
	}
}

// jettison.selection: hold 1, hold 2, or both — and picking one leaves the
// other alone even when the two carry the same kind, which is the whole point
// of keeping the pod as two records.
func TestJettisonEmptiesOnlyTheChosenHold(t *testing.T) {
	base := tacticalState()
	base.Holds[0] = Hold{Kind: HoldGoods, Goods: 2, ItemKey: "atarist.cargo.food"}
	base.Holds[1] = Hold{Kind: HoldSalvage}

	state := base
	if freed := runJettison(&state, JettisonHold1); freed != 1 {
		t.Fatalf("freed %d holds, want 1", freed)
	}
	if state.Holds[0].Occupied() || state.Holds[1].Kind != HoldSalvage {
		t.Fatalf("blowing hold 1 touched hold 2: %+v", state.Holds)
	}

	state = base
	if freed := runJettison(&state, JettisonHold2); freed != 1 {
		t.Fatalf("freed %d holds, want 1", freed)
	}
	if state.Holds[1].Occupied() || state.Holds[0].Kind != HoldGoods {
		t.Fatalf("blowing hold 2 touched hold 1: %+v", state.Holds)
	}

	// Two lots of the same goods: the old model counted them and could not
	// tell them apart, so this is the case that needed the records.
	same := tacticalState()
	same.Holds[0] = Hold{Kind: HoldGoods, Goods: 2, Quality: 7}
	same.Holds[1] = Hold{Kind: HoldGoods, Goods: 2, Quality: 1}
	state = same
	if freed := runJettison(&state, JettisonHold2); freed != 1 {
		t.Fatalf("freed %d holds, want 1", freed)
	}
	if state.Holds[0].Quality != 7 || state.Holds[1].Occupied() {
		t.Fatalf("the wrong lot went out of the bolts: %+v", state.Holds)
	}

	state = base
	if freed := runJettison(&state, JettisonAll); freed != 2 {
		t.Fatalf("freed %d holds in all, want 2", freed)
	}
	if cargoLotsUsed(state) != 0 {
		t.Fatalf("the pod should be empty: %+v", state)
	}
}

// jettison.menu-and-pod: an empty pod has nothing to blow.
func TestJettisonOnAnEmptyPodFreesNothing(t *testing.T) {
	state := tacticalState()
	if freed := runJettison(&state, JettisonAll); freed != 0 {
		t.Fatalf("freed %d lots from an empty pod, want 0", freed)
	}
}

// comm.gates: something to hail first, then a channel to hail on.
func TestHailGatesOnAShipThenOnTheChannel(t *testing.T) {
	state := tacticalState()
	state.CombatActive = false
	if got := runHail(&state); got != HailNoShip {
		t.Fatalf("nothing to hail should come first, got %q", got)
	}

	state = tacticalState()
	state.SystemID = commJammedSystemID
	if got := runHail(&state); got != HailJammed {
		t.Fatalf("hailing from Enlie is jammed, got %q", got)
	}

	state = tacticalState()
	state.SystemID = "jondd"
	if got := runHail(&state); got != HailOpened {
		t.Fatalf("a clear hail should open, got %q", got)
	}
}

// system.enlie-flag: the jammed system is the source's system 3, not a name
// this remake picked. The world table is the check.
func TestTheJammedSystemIsTheThirdOneInTheSourceTable(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	systems := reducer.World.Systems()
	if len(systems) < 3 {
		t.Fatalf("world has %d systems", len(systems))
	}
	if systems[2].ID != commJammedSystemID {
		t.Fatalf("system 3 is %q, but comms are jammed in %q", systems[2].ID, commJammedSystemID)
	}
}

// comm.dialog-setup: hailing is not a subsystem, it is a dialog block.
func TestHailingHandsOffToTheDialogInterpreter(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := tacticalState()
	state.SystemID = "jondd"
	next, err := reducer.Step(state, ActionHail)
	if err != nil {
		t.Fatal(err)
	}
	opening, ok := dialogOpeningForFrame(hailDialogFrame)
	if !ok {
		t.Fatalf("frame 0x%02x has no opening in the corpus", hailDialogFrame)
	}
	if next.Screen != ScreenDialog || next.DialogBlock != opening.Block || next.DialogLine != opening.Line {
		t.Fatalf("hail did not open frame 0x%02x (block 0x%02x line %d): %+v",
			hailDialogFrame, opening.Block, opening.Line, next)
	}
	line, err := reducer.DialogLine(next)
	if err != nil {
		t.Fatalf("the hail opened a conversation the corpus cannot render: %v", err)
	}
	if line == "" {
		t.Fatal("the hail opened an empty line")
	}
}
