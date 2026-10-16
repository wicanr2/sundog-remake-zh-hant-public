package game

import "testing"

// TestEnemyAnglesDriftAndWrap: a new encounter has both axes somewhere on
// the circle, every tick moves them by drift × 4 and wraps at 0x400, and
// set_course never leaves a drift outside ±4.
func TestEnemyAnglesDriftAndWrap(t *testing.T) {
	state := NewState()
	state.Screen = ScreenTactical
	beginEncounter(&state, 8)
	if state.EnemyBearing < 0 || state.EnemyBearing >= enemyAngleTurn || state.EnemyPitch < 0 || state.EnemyPitch >= enemyAngleTurn {
		t.Fatalf("angles %d/%d outside the circle", state.EnemyBearing, state.EnemyPitch)
	}
	for mode := 0; mode < 8; mode++ {
		state.EnemyCourseMode = mode
		for i := 0; i < 20; i++ {
			setEnemyDrift(&state)
			if state.EnemyDriftBearing < -4 || state.EnemyDriftBearing > 4 || state.EnemyDriftPitch < -4 || state.EnemyDriftPitch > 4 {
				t.Fatalf("mode %d drift %d/%d outside ±4", mode, state.EnemyDriftBearing, state.EnemyDriftPitch)
			}
		}
	}
	state.EnemyBearing, state.EnemyDriftBearing = 2, 1
	state.EnemyPitch, state.EnemyDriftPitch = 0x3fe, 1
	steerEnemy(&state)
	if state.EnemyBearing != 0x3fe || state.EnemyPitch != 2 {
		t.Fatalf("after a tick bearing %d pitch %d, want 0x3fe and 2", state.EnemyBearing, state.EnemyPitch)
	}
}

// TestSlewRatesFollowTheOriginalTable pins XDOFIGHT:0x0b slew: the screen
// centre is the dead zone, the right edge turns +8, the top edge pitches +8.
func TestSlewRatesFollowTheOriginalTable(t *testing.T) {
	cases := []struct{ x, y, bearing, pitch int }{
		{160, 92, 0, 0},
		{319, 0, 8, 8},
		{0, 199, -8, -8},
		{150, 86, 0, 0},
		{169, 97, 0, 0},
		{170, 98, 1, -1},
		{149, 85, -1, 1},
		// The screen centre itself sits one entry below the dead zone: the
		// original's table is offset, not centred, and the remake keeps it.
		{160, 100, 0, -1},
	}
	for _, c := range cases {
		bearing, pitch := SlewRates(c.x, c.y)
		if bearing != c.bearing || pitch != c.pitch {
			t.Errorf("SlewRates(%d,%d) = %d,%d want %d,%d", c.x, c.y, bearing, pitch, c.bearing, c.pitch)
		}
	}
}

// TestSlewMovesTheEnemyAcrossTheSky pins replot_screen: turning right (cursor
// at the right edge) takes 2×8 off the bearing per pass, plus the enemy's own
// drift weighted by range; nothing moves outside the tactical move.
func TestSlewMovesTheEnemyAcrossTheSky(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := State{Screen: ScreenTactical, CombatActive: true, CombatRange: 64,
		EnemyBearing: aimCentre, EnemyPitch: aimCentre, EnemyDriftBearing: 1, EnemyDriftPitch: -1}
	if moved := reducer.Slew(state, 319, 0); moved.EnemyBearing != aimCentre {
		t.Fatalf("slew moved the enemy without the tactical move engaged: %+v", moved)
	}
	state.TacticalSlewing = true
	moved := reducer.Slew(state, 319, 0)
	// weight = 3 - 64/32 = 1: bearing -= 16 + 1×1, pitch += 16 + 1×(-1).
	if moved.EnemyBearing != aimCentre-17 || moved.EnemyPitch != aimCentre+15 {
		t.Fatalf("slew: bearing %d pitch %d, want %d %d", moved.EnemyBearing, moved.EnemyPitch, aimCentre-17, aimCentre+15)
	}
	if moved.SlewBearing != 8 || moved.SlewPitch != 8 {
		t.Fatalf("slew rates not recorded: %+v", moved)
	}
	// The tactical move is entered and left through the reducer.
	fresh := NewState()
	fresh.Screen = ScreenTactical
	entered, err := reducer.Step(fresh, ActionTacticalMove)
	if err != nil || !entered.TacticalSlewing {
		t.Fatalf("tactical move did not engage the mouse: %v %+v", err, entered)
	}
	left, err := reducer.Step(entered, ActionTacticalStop)
	if err != nil || left.TacticalSlewing {
		t.Fatalf("tactical stop did not release the mouse: %v %+v", err, left)
	}
}
