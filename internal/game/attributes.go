package game

import "fmt"

// The five character attributes are the original's, and so is their order:
// XDOUSERM:0x0d show_stats labels them from the constant string "STIQDXCHLK",
// two letters each — ST, IQ, DX, CH, LK. MAINLIB_G35[0..4] holds the current
// values and G35[5..9] the maxima. Luck is special twice over: the status
// window prints its current value as "??", and a dialogue check that spends it
// halves it (XDOINTER:0x08 at 0x0227).
//
// What each attribute does is decoded where it is used: ground combat takes
// health × ST / 90 for hit points and DX / 5 + 3 for speed (arena.player-stats),
// the slot machine biases its roll by LK (slots.reel-roll), and dialogue rolls
// against CH and LK (docs/atari-st/dialog-vm.json).
//
// Character creation starts all five at 30 and exposes 440 points. The Atari
// ST live-run evidence and its boundaries are recorded in
// docs/ui/atari-st-character-allocation.md.

// Attribute indexes into State.Attributes, in the original's order.
const (
	AttrStrength     = 0
	AttrIntelligence = 1
	AttrDexterity    = 2
	AttrCharisma     = 3
	AttrLuck         = 4
	AttributeCount   = 5
)

// AttributeLabels are the original's own two-letter labels, in order.
var AttributeLabels = [AttributeCount]string{"ST", "IQ", "DX", "CH", "LK"}

// defaultAttribute is the value a character starts every attribute at. The
// original's roll is not decoded, so a flat mid-range value keeps a replay
// reproducible instead of inventing a distribution.
const defaultAttribute = 50

const (
	characterCreationAttribute = 30
	CharacterCreationPoints    = 440
)

// newAttributes returns a fresh set of attributes at their starting values.
func newAttributes() [AttributeCount]int {
	var attributes [AttributeCount]int
	for index := range attributes {
		attributes[index] = defaultAttribute
	}
	return attributes
}

func newCharacterCreationAttributes() [AttributeCount]int {
	var attributes [AttributeCount]int
	for index := range attributes {
		attributes[index] = characterCreationAttribute
	}
	return attributes
}

// Attribute reads the original byte value, including zero. Current saves are
// validated before loading; a missing historical field is not a default.
func (s State) Attribute(index int) int {
	if index < 0 || index >= AttributeCount {
		return 0
	}
	return clamp(s.Attributes[index], 0, 255)
}

// XSTARTUP:30 1bd1..1bf1 initializes the five rates; MAINLIB:35 applies them
// once per hour. See banville-restoration-spec.md, 屬性整點恢復 READY.
func advanceAttributes(state *State) {
	for index, rate := range [AttributeCount]int{5, 5, 5, 5, 1} {
		difference := state.AttributeMax[index] - state.Attributes[index]
		if difference > 0 {
			state.Attributes[index] += min(difference, rate)
		} else if difference < 0 {
			state.Attributes[index] -= min(-difference, rate)
		}
	}
}

// Zero and current values above their restoration targets are valid bytes.
func ValidateAttributeState(state State) error {
	for index := 0; index < AttributeCount; index++ {
		if state.Attributes[index] < 0 || state.Attributes[index] > 255 ||
			state.AttributeMax[index] < 0 || state.AttributeMax[index] > 255 {
			return fmt.Errorf("invalid attribute byte at %d", index)
		}
	}
	return nil
}

// SpendLuck halves luck, which is what a dialogue check that uses it does.
func SpendLuck(state *State) {
	state.Attributes[AttrLuck] = state.Attribute(AttrLuck) / 2
}
