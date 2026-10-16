package game

// The tractor beam, the jettison bolts and the comm channel are the three
// tactical commands beside the guns. All three are XDOFIGHT's, and all three
// are decoded: see the "拖曳、拋艙、通訊" section of docs/atari-st/ship-rules.md
// and the tractor.*, jettison.* and comm.* rules in ship-rules.json.
//
// The original keeps two independent cargo holds, each a record whose low five
// bits name what is in it. This shell keeps the same two lots but tracks them
// as counts by kind, because a lot of goods here is always the same goods —
// see CargoLots. Salvage is the third kind, and it is what the tractor brings
// in.

const (
	// tractorRange is how close the target has to be before the beam will
	// reach it. The original tests a flag its combat loop maintains
	// (XDOFIGHT_01_L3), and what sets that flag is not decoded, so the
	// threshold itself is this remake's.
	tractorRange = 40
	// salvageItemKey is what a tractored lot is called. The original writes the
	// literal "Unknown" into the hold's name slot (tractor.cargo-unknown), so a
	// salvaged lot is worth nothing until it is identified somewhere else.
	salvageItemKey = "atarist.cargo.unknown"
)

// TractorOutcome is why a tractor attempt did what it did, so both shells can
// report it without repeating the gate order.
type TractorOutcome string

const (
	TractorOutOfRange TractorOutcome = "out-of-range"
	TractorNoRoom     TractorOutcome = "no-room"
	TractorOutOfFuel  TractorOutcome = "out-of-fuel"
	TractorLoaded     TractorOutcome = "loaded"
)

// runTractor is do_tractor's gate order: range, then room, then fuel. Only
// after all three does the beam run, and what it brings aboard is unnamed.
func runTractor(state *State) TractorOutcome {
	if !state.CombatActive || state.CombatRange > tractorRange {
		return TractorOutOfRange
	}
	if CargoLotsFree(*state) <= 0 {
		return TractorNoRoom
	}
	if state.Fuel <= 0 && !drainAuxFuelTank(state) {
		return TractorOutOfFuel
	}
	// do_tractor looks for the first empty hold counting back from hold 2.
	for index := len(state.Holds) - 1; index >= 0; index-- {
		if !state.Holds[index].Occupied() {
			state.Holds[index] = Hold{Kind: HoldSalvage}
			break
		}
	}
	return TractorLoaded
}

// JettisonTarget is which hold the bolts blow: do_jettison's menu is 1, 2 or
// both (jettison.selection), and value 3 is handled as 2 and then run again
// with the value less two. The pod is two records here too (hold.go), so the
// choice is the original's.
type JettisonTarget string

const (
	JettisonHold1 JettisonTarget = "hold-1"
	JettisonHold2 JettisonTarget = "hold-2"
	JettisonAll   JettisonTarget = "all"
)

// jettisonHolds is which slots a target blows.
func jettisonHolds(target JettisonTarget) []int {
	switch target {
	case JettisonHold1:
		return []int{0}
	case JettisonHold2:
		return []int{1}
	case JettisonAll:
		// do_jettison runs hold 2 first and then hold 1, which is the order
		// the menu value walks down in.
		return []int{1, 0}
	}
	return nil
}

// runJettison empties the chosen holds and reports how many it freed. The
// original also clears the star system's contraband heat
// (jettison.heat-cleared), which heat.go does at the call site.
func runJettison(state *State, target JettisonTarget) int {
	freed := 0
	for _, index := range jettisonHolds(target) {
		if emptyHold(state, index).Occupied() {
			freed++
		}
	}
	return freed
}

// HailOutcome is why a hail did what it did.
type HailOutcome string

const (
	HailNoShip HailOutcome = "no-ship"
	HailJammed HailOutcome = "jammed"
	HailOpened HailOutcome = "opened"
)

// commJammedSystemID is the one star system the original jams comms in. The
// global do_comm tests is defined in a single line — G17 := current system = 3
// (system.enlie-flag) — and system 3 of the source table is Enlie.
// EnlieSystemID is the far system: comms are jammed there and the streets
// park fewer vehicles.
const EnlieSystemID = "enlie"

const commJammedSystemID = EnlieSystemID

// hailDialogFrame is the conversation hailing a ship opens. do_comm is not a
// subsystem of its own: it fills in three bytes of the dialog parameter block
// and sets the window selector to 9, then lets the interpreter run
// (comm.dialog-setup). That 9 is the interaction window, the same value
// bring_up_window writes before a street encounter talks -- it is not a block
// number, and passing it in as one opened a conversation the corpus has no
// entry for. What the other ship actually says is frame 0x40, the only text in
// the corpus that is one ship demanding of another: "Give us your cargo, or
// we'll blast you!"
const hailDialogFrame = 0x40

// runHail is do_comm's gate order: something to hail, then a channel to hail on.
func runHail(state *State) HailOutcome {
	if !state.CombatActive {
		return HailNoShip
	}
	if state.SystemID == commJammedSystemID {
		return HailJammed
	}
	return HailOpened
}

// salvageName is what the cargo screens call a tractored lot.
func (r Reducer) salvageName() string {
	return r.localize("ui.cargo.unknown", "Unknown")
}
