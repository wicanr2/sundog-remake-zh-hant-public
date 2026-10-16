package game

// A star system remembers how much trouble the player caused in it. The
// original keeps the count in nibble 7 of the system record's word +9, and the
// three ends of the mechanic are each pinned in docs/atari-st/ground-rules.json:
//
//   - system.heat-raised-by-firing: every shot fired in space adds one to the
//     current system's count, clamped to 15 (XDOFIGHT:0x06 fire_it, 0x02f4).
//   - system.heat-raises-encounters: a movement tick only passes without an
//     encounter while heat <= random() mod 10, so a hotter system throws more
//     pirates at you (SHIPLIB:0x1f update_movement, 0x0b4d).
//   - system.heat-cleared-by-jettison: dumping cargo writes it straight back to
//     zero (XDOFIGHT:0x17 do_jettison, 0x0e0a).
//
// The records live in the saved-game area, so the count survives a save. This
// remake keeps it per system ID rather than in a record, which is the same
// thing with names instead of offsets.

const (
	// maxSystemHeat is the ceiling MAINLIB:0x0a condition clamps the count to.
	maxSystemHeat = 15
	// heatEncounterSpan is the die the movement tick rolls the count against.
	heatEncounterSpan = 10
)

// SystemHeat is how hot the named star system currently is.
func SystemHeat(state State, systemID string) int {
	if state.SystemHeat == nil {
		return 0
	}
	return clamp(state.SystemHeat[systemID], 0, maxSystemHeat)
}

// raiseSystemHeat is what firing a shot does to the system it was fired in.
func raiseSystemHeat(state *State) {
	if state.SystemID == "" {
		return
	}
	if state.SystemHeat == nil {
		state.SystemHeat = map[string]int{}
	}
	state.SystemHeat[state.SystemID] = clamp(state.SystemHeat[state.SystemID]+1, 0, maxSystemHeat)
}

// clearSystemHeat is what jettisoning does to it.
func clearSystemHeat(state *State) {
	if state.SystemHeat == nil || state.SystemID == "" {
		return
	}
	delete(state.SystemHeat, state.SystemID)
}

// heatDrawsAnEncounter reports whether the current system's heat is enough to
// pull an encounter out of this roll. The original's test is the other way
// round — it passes *without* an encounter while heat <= random() mod 10 — so
// this is that condition negated, which keeps the direction the same: more heat,
// more encounters, and a cold system still throws the occasional one.
func heatDrawsAnEncounter(state *State) bool {
	heat := SystemHeat(*state, state.SystemID)
	if heat <= 0 {
		return false
	}
	return heat > combatRoll(state, heatEncounterSpan)
}

// cloneHeat keeps one step's map from writing into the caller's state.
func cloneHeat(source map[string]int) map[string]int {
	if source == nil {
		return nil
	}
	out := make(map[string]int, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
