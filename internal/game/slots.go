package game

import "github.com/wicanr2/sundog-remake/internal/world"

// The slot machine is XSLOTS, thirteen procedures in the original, reached from
// inside a building (XMOVEINB:0x1a do_slots). Every number below is the
// original's; each rule carries a byte signature in
// docs/atari-st/city-subsystems.json.
//
// The one thing worth knowing before reading the table: the machine reads the
// character's luck. roll_one takes random() mod 2000 and then subtracts
// random() mod luck, so luck only ever pushes the roll down — and the low end
// of that number line is where the rare, high-paying faces live. Luck is the
// attribute the status window hides behind "??", and this is one of the two
// places it does visible work.
//
// What stays remake-owned: the spin is resolved in one step instead of animated
// reel by reel, and the rolls come from a save-carried counter so a replay
// reproduces. The odds, the bet and the payouts are not remake-owned.

// slotFaceCount is how many symbols the reels carry.
const slotFaceCount = 8

// slotRollSpan is the width of the number line the odds table divides up. The
// eight bands add up to exactly this.
const slotRollSpan = 2000

// slotLuckMinimum and slotLuckMaximum are the clamp init_slots puts on luck
// before using it as the roll's bias.
const (
	slotLuckMinimum = 5
	slotLuckMaximum = 100
)

// slotBetUnit and slotBetDivisor build the stake from the star system.
const (
	slotBetUnit    = 50
	slotBetOffset  = 17
	slotBetDivisor = 6
)

// slotOdds is the original's table, written by init_slots through set_odds.
// Low and High bound the face's slice of the 1..2000 line; Pair and Triple are
// the multipliers get_pay applies to the stake.
type slotOddsEntry struct {
	Low, High    int
	Pair, Triple int
}

var slotOdds = [slotFaceCount]slotOddsEntry{
	0: {Low: 1600, High: 2000, Pair: 1, Triple: 5},
	1: {Low: 1200, High: 1600, Pair: 1, Triple: 5},
	2: {Low: 800, High: 1200, Pair: 1, Triple: 5},
	3: {Low: 600, High: 800, Pair: 2, Triple: 50},
	4: {Low: 400, High: 600, Pair: 2, Triple: 50},
	5: {Low: 200, High: 400, Pair: 2, Triple: 50},
	6: {Low: 100, High: 200, Pair: 10, Triple: 100},
	7: {Low: 0, High: 100, Pair: 10, Triple: 100},
}

// SlotFaceNames are the remake's own names for the eight symbols. The original
// draws them, and the drawings are not decoded, so these are labels rather than
// claims. The order is the original's: face 7 is the rarest.
var SlotFaceNames = [slotFaceCount]string{
	"ui.slots.face.ore",
	"ui.slots.face.grain",
	"ui.slots.face.crystal",
	"ui.slots.face.cell",
	"ui.slots.face.circuit",
	"ui.slots.face.medicine",
	"ui.slots.face.artifact",
	"ui.slots.face.sundog",
}

// SlotMachine is the state of the machine the player is standing at.
type SlotMachine struct {
	// Bet is the stake for one spin, fixed by the star system.
	Bet int `json:"bet"`
	// Reels holds the three faces after a spin; Spun says whether they mean
	// anything yet.
	Reels [3]int `json:"reels"`
	Spun  bool   `json:"spun"`
	// Matched is 3, 2 or 0; MatchedFace is which symbol paid.
	Matched     int `json:"matched"`
	MatchedFace int `json:"matchedFace"`
	// Payout is what the last spin returned, before the stake is counted.
	Payout int `json:"payout"`
	// Rolls counts the random draws so a replay reproduces the same sequence.
	Rolls int `json:"rolls"`
}

// SlotBet is the stake for one spin: 50 × ((price level + 17 - law) / 6 + 1).
// Both inputs are 4-bit fields of the star system record, the same two the
// ground rules call priceLevel and encounterRating. Expensive, lawless systems
// play for higher stakes.
func SlotBet(priceLevel, encounterRating int) int {
	span := (priceLevel + slotBetOffset - encounterRating) / slotBetDivisor
	if span < 0 {
		span = 0
	}
	return slotBetUnit * (span + 1)
}

// slotBetForState looks the stake up from the system the player is in.
func slotBetForState(state State, registry world.Registry) int {
	system, ok := registry.System(state.SystemID)
	if !ok {
		return slotBetUnit
	}
	return SlotBet(system.PriceLevel, system.EncounterRating)
}

// slotRoll is the remake's reproducible stand-in for the original's RNG. The
// original draws from MAINLIB:0x0b; substituting a save-carried counter is the
// same trade the rest of the remake makes, so a replay lands on one answer.
func slotRoll(machine *SlotMachine, span int) int {
	if span <= 0 {
		return 0
	}
	machine.Rolls++
	mixed := uint32(machine.Rolls)*1103515245 + 12345
	mixed ^= mixed >> 16
	return int(mixed % uint32(span))
}

// spinReel is roll_one: draw across the whole line, let luck pull the draw
// down, clamp, then find the band the draw lands in.
func spinReel(machine *SlotMachine, luck int) int {
	roll := slotRoll(machine, slotRollSpan) - slotRoll(machine, luck)
	if roll < 1 {
		roll = 1
	}
	if roll > slotRollSpan {
		roll = slotRollSpan
	}
	for face := 0; face < slotFaceCount; face++ {
		if slotOdds[face].Low <= roll && roll <= slotOdds[face].High {
			return face
		}
	}
	return slotFaceCount - 1
}

// clampLuck is the 5..100 clamp init_slots puts on the attribute.
func clampLuck(luck int) int {
	if luck < slotLuckMinimum {
		return slotLuckMinimum
	}
	if luck > slotLuckMaximum {
		return slotLuckMaximum
	}
	return luck
}

// matchReels is roll_slots: three alike, else any adjacent pair, else nothing.
// The pairs are compared in the original's order — 0-1, 1-2, 2-0 — which is
// what decides the paying face when two different pairs could be read.
func matchReels(reels [3]int) (matched, face int) {
	if reels[0] == reels[1] && reels[1] == reels[2] {
		return 3, reels[0]
	}
	if reels[0] == reels[1] {
		return 2, reels[0]
	}
	if reels[1] == reels[2] {
		return 2, reels[1]
	}
	if reels[2] == reels[0] {
		return 2, reels[2]
	}
	return 0, 0
}

// SlotPayout is get_pay: the stake times the multiplier of the face that paid.
func SlotPayout(bet, matched, face int) int {
	if matched < 2 || face < 0 || face >= slotFaceCount {
		return 0
	}
	if matched >= 3 {
		return bet * slotOdds[face].Triple
	}
	return bet * slotOdds[face].Pair
}

// spinSlots plays one spin. It returns the net change to the purse: the payout
// less the stake, which is what the original's sub-then-add pair works out to.
func spinSlots(state *State, luck int) int {
	machine := &state.Slots
	machine.Spun = true
	for reel := range machine.Reels {
		machine.Reels[reel] = spinReel(machine, clampLuck(luck))
	}
	machine.Matched, machine.MatchedFace = matchReels(machine.Reels)
	machine.Payout = SlotPayout(machine.Bet, machine.Matched, machine.MatchedFace)
	return machine.Payout - machine.Bet
}
