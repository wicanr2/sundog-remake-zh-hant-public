package game

import (
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// In-system flight is polar. Every planet has an angle on a 256-step turn and a
// radius around its star, and a trip is a straight interpolation between two of
// those over a computed number of ticks. Both the interpolation and the
// projection that puts it on a screen are the original's:
//
//   - XSHOWMOV:0x0a do_in_system works out the two deltas and the trip length.
//   - SHIPLIB:0x1f update_movement advances one tick and redraws.
//   - SHIPLIB:0x0c polar_to_rect projects, using the quarter sine table decoded
//     into docs/atari-st/sine-table.json.
//
// Each rule has a byte signature in docs/atari-st/ship-rules.json.
const (
	// flightRadiusDivisor and flightArcDivisor weight the two halves of the
	// trip length: the radial distance counts for a third of itself, and the
	// angular distance is scaled by the mean radius of the two ends.
	flightRadiusDivisor = 3
	flightArcDivisor    = 0x100
	// flightTickScale and flightTickBase turn that estimate into ticks.
	flightTickScale = 4
	flightTickBase  = 1
	// Retained world-leg approximation only. Mode2 uses its saved phase and
	// pays on phase1; interpolation progress is not a parent-call count.
	flightFuelPhases = 3
	// XSHOWMOV:0x0d do_on_world flies the legs between a city and orbit. It
	// charges 11 ticks for a city flown straight to another city of the same
	// planet and 10 for a take-off or a landing (0x0791), and takes one more
	// unit of fuel at the end of each (0x07ff) — the cost of the hop itself.
	// Its own branches settle how the legs compose: the landing mode requires
	// the ship to already be in orbit, which only a take-off puts it in, so a
	// trip to another planet is take-off, then do_in_system, then landing.
	planetaryHopTicks     = 11
	planetaryLandingTicks = 10
	planetaryHopExtraFuel = 1
	// The sub-c engine gates original phases 0 and 2: the tick only advances when
	// random() mod 16 is under the engine's status (0x08f5). A half-wrecked
	// engine therefore takes longer in expectation. The actual loop consumes
	// its own draws; damaged-engine estimates are not an original exact total.
	flightSubcScale = 16
	// The time estimate of a trip is the original's own ETA: do_in_system stamps
	// the current time into a record and adds ticks + ticks/4 to it (0x057a),
	// and add_time's unit is minutes (SHIPLIB:0x13, 0x0128). do_on_world adds
	// its tick count unscaled (0x07a6). Mode2 does not prepay these minutes to
	// the main clock; the awake minute callback advances that independently.
	flightEtaDivisor = 4
	// flightScreenOriginX and flightScreenOriginY are where the star sits in
	// the original's own 320x200 frame. A screen that draws the plot at another
	// size scales around this point rather than replacing it.
	flightScreenOriginX = 0xB0 + 1
	flightScreenOriginY = 0x3D + 2
	// The two quadrant bounds polar_to_rect uses to sign each component. They
	// are the source's, including the fact that the first one is not symmetric.
	flightNegateXLow  = 0x40
	flightNegateXHigh = 0xC4
	flightNegateYHigh = 0x80
	// flightQuarterTurn is what the x component adds before reading the table,
	// which is what turns a sine into a cosine.
	flightQuarterTurn = 0x40
)

var (
	sineSource   atarist.SineTableData
	sineSourceOK bool
)

func init() {
	data, err := atarist.EmbeddedSineTableData()
	if err != nil {
		return
	}
	sineSource = data
	sineSourceOK = true
}

// FlightTurnSteps is how many angle units make one revolution.
func FlightTurnSteps() int {
	if !sineSourceOK {
		return 0
	}
	return sineSource.TurnSteps()
}

// FlightScreenOrigin is where the star sits in the original's frame.
func FlightScreenOrigin() (int, int) { return flightScreenOriginX, flightScreenOriginY }

// PolarToRect projects a polar position the way SHIPLIB:0x0c does: one quarter
// sine table, read twice, with the sign of each component decided by which
// quadrant the angle falls in.
func PolarToRect(angle, radius int) (int, int) {
	if !sineSourceOK {
		return 0, 0
	}
	turn := sineSource.TurnSteps()
	angle %= turn
	if angle < 0 {
		angle += turn
	}
	scale := sineSource.Scale()

	x := radius * sineSource.Sine(angle+flightQuarterTurn) / scale
	if angle >= flightNegateXLow && angle <= flightNegateXHigh {
		x = -x
	}
	// The y branch at SHIPLIB:0x0c 0x0116 skips the negation while the angle
	// is at or below 0x80 and negates above it, so the turn runs clockwise on
	// a screen whose y grows downward. Confirmed against the original's own
	// scanner dots (docs/atari-st/hatari-live-run.md, round nine).
	y := radius * sineSource.Sine(angle) / scale
	if angle > flightNegateYHigh {
		y = -y
	}
	return x, y
}

// FlightPosition is where the ship is right now, in the source's own units.
func FlightPosition(state State) (angle, radius int) {
	return state.FlightAngle, state.FlightRadius
}

// FlightProgress is how far along the trip the ship is, as a percentage, for a
// meter. The tick counts themselves are the source's and are much smaller.
func FlightProgress(state State) int {
	if state.FlightTicks <= 0 {
		return 100
	}
	return clamp(state.FlightTick, 0, state.FlightTicks) * 100 / state.FlightTicks
}

// FlightArrived reports whether the trip has run out of ticks.
func FlightArrived(state State) bool {
	return state.FlightTicks <= 0 || state.FlightTick >= state.FlightTicks
}

// shortWayRound folds an angle difference into the half turn either side of
// zero, which is what makes the ship take the short way round rather than
// winding the long way to the same place.
func shortWayRound(delta, turn int) int {
	half := turn / 2
	if delta > half {
		delta -= turn
	} else if delta < -half {
		delta += turn
	}
	return delta
}

// beginInSystemFlight sets up a trip to a planet. The ship's position carries
// across trips rather than being reset, which is what the original does: it
// interpolates from wherever the ship already is, and a jump between stars does
// not move it in polar terms.
//
// The tick count is the original's arc estimate: the radial distance plus the
// angular distance weighted by the sum of the two radii.
func beginInSystemFlight(state *State, to world.Orbit) {
	turn := FlightTurnSteps()
	if turn <= 0 {
		return
	}
	from := world.Orbit{Radius: state.FlightRadius, Angle: state.FlightAngle}
	state.FlightFromAngle = from.Angle
	state.FlightFromRadius = from.Radius
	state.FlightDeltaRadius = to.Radius - from.Radius
	state.FlightDeltaAngle = shortWayRound(to.Angle-from.Angle, turn)

	state.FlightTicks = inSystemFlightTicks(from, to, turn)
	state.FlightTick = 0
	state.ShipControls.Subc = true
}

// Draw once on every active movement call, including phase1/intact engines.
// Phases0/2 consume the result for progress. Natural RNG parity is not claimed.
func subcTickPasses(state *State) bool {
	status := shipSystemStatusByID(*state, ShipSystemSubcEngine)
	return combatRoll(state, flightSubcScale) < status
}

// inSystemFlightTicks is do_in_system's own arc estimate (0x052e): the radial
// distance counts for a third of itself, the angular distance is weighted by
// the sum of the two radii, and the whole thing is scaled and offset.
func inSystemFlightTicks(from, to world.Orbit, turn int) int {
	span := abs(to.Radius-from.Radius)/flightRadiusDivisor +
		abs(shortWayRound(to.Angle-from.Angle, turn))*(to.Radius+from.Radius)/flightArcDivisor
	return span*flightTickScale + flightTickBase
}

// routeLegs is a trip broken into the legs the original flies it in. Ticks is
// what the whole trip runs to; Hops is how many do_on_world legs it contains,
// which is what the fuel surcharge is charged per; Minutes is the ETA, which
// each kind of leg computes differently.
type routeLegs struct {
	Ticks    int
	ArcTicks int
	Hops     int
	Minutes  int
}

// routeTicks breaks a trip into its legs. Two cities of one planet are a single
// do_on_world hop; anywhere else is a take-off, the in-system arc, and a
// landing.
func routeTicks(state State, to world.Orbit, samePlanet bool) routeLegs {
	if samePlanet {
		return routeLegs{Ticks: planetaryHopTicks, Hops: 1, Minutes: planetaryHopTicks}
	}
	turn := FlightTurnSteps()
	if turn <= 0 {
		return routeLegs{}
	}
	from := world.Orbit{Radius: state.FlightRadius, Angle: state.FlightAngle}
	arc := inSystemFlightTicks(from, to, turn)
	return routeLegs{
		Ticks:    planetaryLandingTicks*2 + arc,
		ArcTicks: arc,
		Hops:     2,
		Minutes:  planetaryLandingTicks*2 + arc + arc/flightEtaDivisor,
	}
}

// The two numbers XMOVEONS:0x03 applies to the polar radius to get the warp
// success percentage. Radius 43 -- the ring the system map draws -- gives 100.
const (
	warpSuccessBias  = 7
	warpSuccessScale = 2
)

// Mode2 movement fuel uses the saved initial phase. At intact status this is
// exact for movement alone; damaged-engine draws and active energy systems can
// change the actual total. World-leg composition remains an explicit estimate.
func routeFuelCost(state State, legs routeLegs) int {
	arcTicks := legs.ArcTicks
	status := shipSystemStatusByID(state, ShipSystemSubcEngine)
	if status > 0 && status < flightSubcScale {
		arcTicks = arcTicks * flightSubcScale / status
	}
	return routeDepartureFuelCost(state, legs) + mode2MovementFuel(state.ShipPhase, arcTicks)
}

func mode2MovementFuel(initialPhase, progressTicks int) int {
	if progressTicks <= 0 {
		return 0
	}
	switch initialPhase {
	case 0:
		return (progressTicks + 1) / 2
	case 1:
		return (progressTicks - 1) / 2
	default:
		return progressTicks / 2
	}
}

// Only the existing world-leg approximation is charged at departure. Mode2
// pays during its persistent three-phase loop; charging its estimate here too
// would charge the same arc twice. World/warp composition remains Issue #6.
func routeDepartureFuelCost(state State, legs routeLegs) int {
	ticks := legs.Ticks - legs.ArcTicks
	status := shipSystemStatusByID(state, ShipSystemSubcEngine)
	if status > 0 && status < flightSubcScale {
		ticks = ticks * flightSubcScale / status
	}
	return ticks/flightFuelPhases + legs.Hops*planetaryHopExtraFuel
}

// One parent update: advance the saved phase, run mode2 movement, then energy.
// Phases0/2 can progress; phase1 pays one fuel. Interpolation uses the original
// endpoints instead of accumulating drift. Energy uses the attached <=0
// consumer, independently of the <5 pod-reentry consumer.
// Returns whether any energy consumer spent an auxiliary tank.
// Evidence and full remaining gates: docs/atari-st/ship-rules.md.
func advanceInSystemFlight(state *State) bool {
	turn := FlightTurnSteps()
	if turn <= 0 || state.FlightTicks <= 0 {
		return false
	}
	oldFuel := state.Fuel
	advanceShipPhase(state)
	if state.FlightTick < state.FlightTicks && state.ShipControls.Subc {
		passes := subcTickPasses(state) // Also drawn during phase1/full status.
		if state.ShipPhase&1 == 0 && passes {
			state.FlightTick++
		}
		if state.ShipPhase == 1 {
			state.Fuel--
		}
		if state.FlightTick == state.FlightTicks {
			state.ShipControls.Subc = false
		}
	}
	state.FlightRadius = state.FlightTick*state.FlightDeltaRadius/state.FlightTicks + state.FlightFromRadius
	angle := state.FlightTick*state.FlightDeltaAngle/state.FlightTicks + state.FlightFromAngle
	if angle < 0 {
		angle += turn
	}
	state.FlightAngle = angle % turn
	return updateShipEnergyFromFuel(state, oldFuel, func(n int) int { return combatRoll(state, n) })
}

// WarpSuccessPercent is the manual's PROBABILITY WARP SUCCESS, the reading in
// the small box at the top right of the panel. The original works it out in
// XMOVEONS:0x03 (ship rule move.warp-probability) as (radius + 7) * 2, where
// the radius is the ship's own polar radius -- the high field of G49, the same
// value it hands to polar_to_rect. So the number is distance from the star:
// a jump is safest at the edge of the system, and radius 43 (the system map's
// ring) gives exactly 100.
//
// The original also has two gates in front of it. One is a flag that hides the
// reading entirely; this shell has no equivalent state, so it is not modelled.
// The other returns zero when the ship has no planet set, which is the case
// this function's zero radius stands in for.
func WarpSuccessPercent(state State) int {
	if state.FlightRadius <= 0 {
		return 0
	}
	return (state.FlightRadius + warpSuccessBias) * warpSuccessScale
}
