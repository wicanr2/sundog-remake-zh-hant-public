package game

// The original's tactical move is the mouse. XDOFIGHT:0x12 do_guns turns the
// cursor off and loops update_fight until a button is pressed; every pass
// XDOFIGHT:0x0b slew reads the mouse (GEMBIND get_mouse) and turns the two
// screen coordinates into a pair of slew rates through one 32-entry table:
//
//	Gd (pitch)   = table[min(y + 4, 0xbf) / 6]
//	Ge (bearing) = table[31 - x / 10]
//
// The table runs +8 .. -8 with a two-entry dead zone in the middle, so the
// cursor at the screen centre holds course and the edges turn fastest. It is
// static data on the disk (raw 0xd422, the same 64 bytes in every memory
// snapshot taken; rule fight.slew-table).
//
// XDOFIGHT:0x0a replot_screen then moves the enemy across the sky by the
// player's turn and by the enemy's own drift, the drift weighted by range:
//
//	Gd0 -= 2*Ge + (3 - range/32) * Gd3        (mod 0x400)
//	Gcf += 2*Gd + (3 - range/32) * Gd2        (mod 0x400)
//
// Turning right (cursor right of centre) makes the enemy slide left in the
// viewport, which is what puts it under the cross-hair. Dead ahead is 0x200
// on both axes; XDOFIGHT:0x06 fire_it lands a shot when
// |Gd0 - 0x200| + |Gcf - 0x200| is inside the gun's aiming window.
//
// The remake keeps the original's pass rate loosely: the front end calls Slew
// a fixed number of times per second while the player holds the tactical move
// (SlewTicksPerSecond); the original runs it as fast as its frame loop goes.

const (
	slewTableSize        = 32
	slewMouseYOffset     = 4
	slewMouseYCap        = 0xbf
	slewMouseYDivisor    = 6
	slewMouseXDivisor    = 10
	slewRateScale        = 2
	slewDriftWeightBase  = 3
	slewDriftRangeDivide = 32
	// aimCentre is dead ahead on both axes.
	aimCentre = 0x200
	// SlewTicksPerSecond is the remake's pass rate for Slew; the original has
	// no fixed clock here.
	SlewTicksPerSecond = 15
)

var slewTable = [slewTableSize]int{
	8, 7, 6, 5, 5, 4, 4, 3, 3, 2, 2, 2, 1, 1, 1, 0,
	0, -1, -1, -1, -2, -2, -2, -3, -3, -4, -4, -5, -5, -6, -7, -8,
}

// SlewRates is XDOFIGHT:0x0b slew: the mouse position on the 320x200 screen
// becomes a bearing rate and a pitch rate.
func SlewRates(x, y int) (bearing, pitch int) {
	y += slewMouseYOffset
	if y > slewMouseYCap {
		y = slewMouseYCap
	}
	pitch = slewTable[clamp(y/slewMouseYDivisor, 0, slewTableSize-1)]
	bearing = slewTable[clamp(slewTableSize-1-x/slewMouseXDivisor, 0, slewTableSize-1)]
	return bearing, pitch
}

// Slew is one pass of the original's tactical loop while the player holds the
// tactical move: the mouse sets the slew rates and replot_screen moves the
// enemy by them. Outside the tactical move it does nothing.
func (r Reducer) Slew(state State, x, y int) State {
	if state.Screen != ScreenTactical || !state.TacticalSlewing {
		return state
	}
	state.SlewBearing, state.SlewPitch = SlewRates(x, y)
	if !state.CombatActive {
		return state
	}
	weight := slewDriftWeightBase - combatRange(state)/slewDriftRangeDivide
	state.EnemyBearing = wrapAngle(state.EnemyBearing - (slewRateScale*state.SlewBearing + weight*state.EnemyDriftBearing))
	state.EnemyPitch = wrapAngle(state.EnemyPitch + slewRateScale*state.SlewPitch + weight*state.EnemyDriftPitch)
	return state
}

// aimError is fire_it's measure of how far off the cross-hair the enemy is:
// the two axes' distances from dead ahead, added.
func aimError(state State) int {
	return abs(state.EnemyBearing-aimCentre) + abs(state.EnemyPitch-aimCentre)
}

// AimError is aimError for the front end's readouts.
func AimError(state State) int { return aimError(state) }
