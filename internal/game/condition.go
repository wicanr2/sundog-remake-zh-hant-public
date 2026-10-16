package game

// The four condition readings the manual's page 6 lists, in its order. The
// original keeps them in MAINLIB_G3d as four bytes and a new game sets all four
// to 0xff (XSTARTUP:0x30). Ground rules condition.decay, condition.decay-tables
// and condition.vigor-collapse pin what follows.

// MaxCondition is the original's own ceiling for a reading: one byte.
const MaxCondition = 0xff

// conditionBand is the width of a decay table's band. MAINLIB:0x52 indexes its
// table with value/0x40, so a reading falls in one of four bands.
const conditionBand = 0x40

// The decay tables, read from the constant pool the original hands
// update_condition (rule condition.decay-tables). Each entry is how much the
// reading loses per tick while it sits in that band.
var (
	restDecay      = [4]int{4, 2, 3, 4}
	healthDecay    = [4]int{1, 0, 0, 0}
	nourishedDecay = [4]int{1, 1, 2, 3}
)

// decayReading applies one tick of a table to a reading: the original's
//
//	value -= table[value / 0x40] - '0'
//
// with the ASCII digits already turned into numbers.
func decayReading(value int, table [4]int) int {
	if value <= 0 {
		return 0
	}
	band := value / conditionBand
	if band > 3 {
		band = 3
	}
	return clamp(value-table[band], 0, MaxCondition)
}

// healthRecovery is the second half of index 2's rule. Above 0x80 the reading
// climbs instead of falling, and how fast is not a constant: the_time works out
//
//	(vigor - 0x80) / 0x20
//
// clamped at zero, and update_condition multiplies it by four. So a rested
// character heals quickly, a tired one heals slowly, and one under half vigour
// does not heal at all. It is why a lightly hurt character mends on their own
// and an exhausted one does not.
func healthRecovery(health, vigor int) int {
	if health <= 0x80 {
		return 0
	}
	steps := (vigor - 0x80) / 0x20
	if steps < 0 {
		steps = 0
	}
	return steps * 4
}

// advanceConditions runs one tick of the original's four readings. Rest is the
// exception the original writes into update_condition itself: while the sleep
// flag (G53 bit 0x3b) is set the reading does not decay at all, which is what
// makes sleeping worth the time it costs.
//
// The fourth reading, Vigor, is the only one whose floor has a consequence:
// the_time clears it, sets G3 and puts G32's field to 12 -- the collapse the
// manual's page 10 warns about. This reports that so the caller can act.
func advanceConditions(state *State, asleep bool) (collapsed bool) {
	if !asleep {
		state.Rest = decayReading(state.Rest, restDecay)
	}
	if gain := healthRecovery(state.Health, state.Vigor); gain > 0 {
		state.Health = clamp(state.Health+gain, 0, MaxCondition)
	} else {
		state.Health = decayReading(state.Health, healthDecay)
	}
	state.Nourishment = decayReading(state.Nourishment, nourishedDecay)
	// MAINLIB:52 adds each updated reading below 64 to the parent's L2.
	// MAINLIB:35 uses this penalty instead of the average-following branch.
	penalty := 0
	for _, reading := range []int{state.Rest, state.Health, state.Nourishment} {
		if reading < conditionBand {
			penalty += reading/16 - 4
		}
	}
	if penalty < 0 {
		state.Vigor = clamp(state.Vigor+penalty, 0, MaxCondition)
	} else {
		state.Vigor = advanceVigor(state.Vigor, state.Rest, state.Health, state.Nourishment)
	}
	return state.Vigor <= 0
}

// advanceVigor is the one reading the_time works out itself rather than handing
// to update_condition, and it does not use a decay table at all:
//
//	Vigor := Vigor + 2 * sign((Rest + Health + Nourishment) div 3 - Vigor)
//
// Vigor chases the average of the other three, moving two points a tick. That is
// why the_time keeps index 0 out of update_condition -- there is no table to
// look up. The three readings are the values *after* this tick's decay, because
// the original runs its three update_condition calls first.
//
// Read out of MAINLIB:0x35 at segment offset 0x0c7d, with the shape confirmed by
// five breakpoint samples on the original: Vigor 255 -> 253 -> 251 -> 249 -> 247
// against sums of 758, 751 and 744. See docs/atari-st/ground-rules.md.
func advanceVigor(vigor, rest, health, nourishment int) int {
	target := (rest + health + nourishment) / 3
	switch {
	case target > vigor:
		vigor += 2
	case target < vigor:
		vigor -= 2
	}
	return clamp(vigor, 0, MaxCondition)
}
