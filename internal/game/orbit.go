package game

import (
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// Planets move. XMOVEONS:0x07 check_planets runs whenever the day changes and
// walks all eighteen records:
//
//	angle := angle + rate × elapsed days
//	while angle < 0 { angle += 256 }
//
// stored back into a byte, so the wrap is free. The rate is the high byte of
// the planet record's word +5 (rule planet.orbit-rate); six planets have rate 0
// and never move.
//
// The angle is live state, not disk data: XSTARTUP:init_planets re-rolls all
// eighteen at the start of a new game (rule planet.orbit-angle-is-live-state),
// so what is in the source table is only a factory value. That is why the
// angles live in State rather than in the registry.

// orbitSpreadFloor is the gap init_planets insists on between one planet's
// fresh angle and the previous planet's: it re-rolls until the two differ by
// more than fifteen steps, which keeps the eighteen from clustering.
const orbitSpreadFloor = 15

// OrbitTurnSteps is one full turn of a planet's orbit.
func OrbitTurnSteps() int { return atarist.OrbitTurnSteps }

// rollPlanetAngles is init_planets: each planet's angle is rand(0, 255), rolled
// again while it sits within fifteen steps of the previous planet's.
func rollPlanetAngles(state *State, count int) []int {
	angles := make([]int, count)
	turn := OrbitTurnSteps()
	previous := -1
	for index := range angles {
		angle := combatRoll(state, turn)
		if previous >= 0 {
			// The original loops until the gap is wide enough. Bound the tries
			// so a degenerate roll sequence cannot spin: the original's own
			// random source cannot stall, but this one is a counter.
			for tries := 0; tries < turn && abs(angle-previous) <= orbitSpreadFloor; tries++ {
				angle = combatRoll(state, turn)
			}
		}
		angles[index] = angle
		previous = angle
	}
	return angles
}

// advancePlanetOrbits is check_planets. It is driven by the day count rather
// than by minutes, because that is what the original compares: it keeps the day
// it last ran on and does nothing until that number changes.
func advancePlanetOrbits(state *State, planets []world.Planet) {
	day := state.TimeMinutes / minutesPerDay
	if day == state.OrbitDay {
		return
	}
	elapsed := day - state.OrbitDay
	if elapsed < 0 {
		elapsed = -elapsed
	}
	state.OrbitDay = day
	turn := OrbitTurnSteps()
	if turn <= 0 || len(state.PlanetAngles) == 0 {
		return
	}
	for _, planet := range planets {
		if planet.Index < 0 || planet.Index >= len(state.PlanetAngles) {
			continue
		}
		angle := state.PlanetAngles[planet.Index] + planet.OrbitRate*elapsed
		angle %= turn
		if angle < 0 {
			angle += turn
		}
		state.PlanetAngles[planet.Index] = angle
	}
}

// normalizePlanetAngles seeds the live angles for a save that has none, which
// means every save written before schema 20. Seeding from the source table
// leaves that save's sky exactly where it was rather than shuffling it.
func normalizePlanetAngles(state *State) {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		return
	}
	if len(state.PlanetAngles) == len(data.Planets()) {
		return
	}
	angles := make([]int, len(data.Planets()))
	for index, planet := range data.Planets() {
		if index < len(state.PlanetAngles) {
			angles[index] = state.PlanetAngles[index]
			continue
		}
		angles[index] = planet.Orbit.Angle
	}
	state.PlanetAngles = angles
}

// PlanetOrbit is where a planet actually is now: the source's radius, which
// never changes, and the live angle.
func (r Reducer) PlanetOrbit(state State, planet world.Planet) world.Orbit {
	orbit := planet.Orbit
	if planet.Index >= 0 && planet.Index < len(state.PlanetAngles) {
		orbit.Angle = state.PlanetAngles[planet.Index]
	}
	return orbit
}
