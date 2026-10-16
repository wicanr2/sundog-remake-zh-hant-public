package game

import "github.com/wicanr2/sundog-remake/internal/world"

// Every new game re-rolls the star chart. XSTARTUP:0x26 init_systems walks the
// twelve star system records and nudges each of the three packed coordinate
// fields by set_random (v := v + random() mod 3 - 1, clamped to x 1-12,
// y 1-9, z 0-10), then asks the spacing check at XSTARTUP:0x28 whether the
// system landed too close to one already placed -- |dX| < 2 and |dY| < 2
// against every system with a lower index, plus the tenth -- and rolls the
// whole triple again while it does. Two systems are special: the tenth
// (Woremed) is written to (6, 5, 6) and never moves, and the third (Enlie)
// is skipped by the main loop and placed last on its own terms: x and y nudged
// and spaced against every system, z written to 12 + random() mod 4, and
// the placement redone until Enlie's nearest neighbour sits at jump distance
// exactly 9 -- one more than a new ship's jump range, which is why Enlie is
// always just out of reach at the start.
//
// The rule was read from the p-code and checked against the original running
// (docs/atari-st/hatari-live-run.md, round ten): the live records of two
// separate games both fit it, and the drawn star chart matched them 12/12.
// The rolls here come from the new-game seed rather than the original's
// clock-seeded LCG; the shape of the result is the source's.

const (
	// sectorFixedSystem is the tenth record, Woremed, written to
	// sectorFixedCoordinates and never nudged.
	sectorFixedSystem      = 9
	sectorFarSystem        = 2 // the third record, Enlie
	sectorFarZBase         = 12
	sectorFarZSpan         = 4
	sectorFarNearestJump   = 9
	sectorSpacing          = 2
	sectorRetryLimit       = 4096
	sectorFarTries         = 25 // Enlie placements per pass before the whole chart is nudged again
	sectorPassLimit        = 400
	sectorJitterSpan       = 3
	sectorRecordCount      = 12
	sectorCoordinateFields = 3
)

var (
	sectorFixedCoordinates = [3]int{6, 5, 6}
	// sectorBounds are the clamps set_random's condition() applies per field.
	sectorBounds = [sectorCoordinateFields][2]int{{1, 12}, {1, 9}, {0, 10}}
)

// sectorRoll is a small deterministic generator for the new-game re-roll. It
// only has to be well spread over a few hundred draws.
type sectorRoll struct{ state uint32 }

func newSectorRoll(seed int64) *sectorRoll {
	return &sectorRoll{state: uint32(seed)*2654435761 + 0x9e3779b9}
}

func (r *sectorRoll) next() int {
	// xorshift32
	x := r.state
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	r.state = x
	return int(x >> 1)
}

// JitterSystemCoordinates rolls the star chart for one game from the factory
// coordinates of the systems, in source-table order. It is what NewState
// stores in State.SystemCoordinates.
func JitterSystemCoordinates(seed int64, systems []world.System) world.SystemCoordinates {
	if len(systems) != sectorRecordCount {
		// Anything but the source's twelve records is not the map this rule
		// was written for; keep the factory values rather than guess.
		out := world.SystemCoordinates{}
		for _, system := range systems {
			out[system.ID] = system.Coordinates
		}
		return out
	}
	roll := newSectorRoll(seed)
	coords := make([][3]int, len(systems))
	for i, system := range systems {
		coords[i] = system.Coordinates
	}
	coords[sectorFixedSystem] = sectorFixedCoordinates

	nudge := func(c [3]int, fields int) [3]int {
		for f := 0; f < fields; f++ {
			v := c[f] + roll.next()%sectorJitterSpan - 1
			v = clamp(v, sectorBounds[f][0], sectorBounds[f][1])
			c[f] = v
		}
		return c
	}
	// tooClose is XSTARTUP:0x28: the systems a placement is checked against
	// are those with a lower index and the fixed tenth; Enlie is checked
	// against everyone.
	tooClose := func(i int) bool {
		for j := range coords {
			if j == i {
				continue
			}
			if !(i == sectorFarSystem || j < i || j == sectorFixedSystem) {
				continue
			}
			dx, dy := coords[i][0]-coords[j][0], coords[i][1]-coords[j][1]
			if abs(dx) < sectorSpacing && abs(dy) < sectorSpacing {
				return true
			}
		}
		return false
	}

	// The outer loop is the original's too: when Enlie cannot be placed in
	// sectorFarTries attempts, init_systems goes back and nudges every system
	// again (on top of where they are, not from the factory values) before
	// trying Enlie afresh.
	for pass := 0; pass < sectorPassLimit; pass++ {
		for i := range coords {
			if i == sectorFarSystem || i == sectorFixedSystem {
				continue
			}
			// set_random adds to the value it is given, so each retry walks on
			// from the last one rather than restarting at the factory value --
			// that is how a system ends up several squares from where it shipped.
			for try := 0; try < sectorRetryLimit; try++ {
				coords[i] = nudge(coords[i], sectorCoordinateFields)
				if !tooClose(i) {
					break
				}
			}
		}
		// Enlie: nudged on x and y, pushed to the far edge on z, and redone
		// until its nearest neighbour is exactly one jump past a new ship's
		// range.
		placed := false
		for try := 0; try < sectorFarTries; try++ {
			for spacing := 0; spacing < sectorRetryLimit; spacing++ {
				c := nudge(coords[sectorFarSystem], 2)
				c[2] = sectorFarZBase + roll.next()%sectorFarZSpan
				coords[sectorFarSystem] = c
				if !tooClose(sectorFarSystem) {
					break
				}
			}
			if sectorNearestJump(coords, sectorFarSystem) == sectorFarNearestJump {
				placed = true
				break
			}
		}
		if placed {
			break
		}
	}

	out := world.SystemCoordinates{}
	for i, system := range systems {
		out[system.ID] = coords[i]
	}
	return out
}

// sectorNearestJump is the last loop of init_systems: the smallest
// SHIPLIB:0x0a dist from the given system to any other, starting the search
// at 10 so anything further than that reads as 10.
func sectorNearestJump(coords [][3]int, i int) int {
	nearest := 10
	for j := range coords {
		if j == i {
			continue
		}
		if d := sectorJumpDistance(coords, i, j); d < nearest {
			nearest = d
		}
	}
	return nearest
}

// sectorJumpDistance is SHIPLIB:0x0a dist on the working table: the integer
// square root of the summed squares, plus one when either end is Enlie.
func sectorJumpDistance(coords [][3]int, i, j int) int {
	sum := 0
	for f := 0; f < sectorCoordinateFields; f++ {
		d := coords[i][f] - coords[j][f]
		sum += d * d
	}
	distance := isqrtInt(sum)
	if i == sectorFarSystem || j == sectorFarSystem {
		distance++
	}
	return distance
}

func isqrtInt(value int) int {
	if value <= 0 {
		return 0
	}
	r := 0
	for (r+1)*(r+1) <= value {
		r++
	}
	return r
}

// SystemCoordinate is where a system sits in this game.
func (r Reducer) SystemCoordinate(state State, systemID string) ([3]int, bool) {
	return r.World.SystemCoordinate(state.SystemCoordinates, systemID)
}

// factorySystemCoordinates is what an older save gets: the game it belongs to
// was already being played on the factory chart, so that is its chart.
func factorySystemCoordinates() world.SystemCoordinates {
	out := world.SystemCoordinates{}
	for _, system := range world.FactorySystems() {
		out[system.ID] = system.Coordinates
	}
	return out
}
