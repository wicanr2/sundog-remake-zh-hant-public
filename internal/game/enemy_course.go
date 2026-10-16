package game

import "fmt"

// EnemyCourseState stores game values, never interpreter memory. Mode3 uses
// Maneuver as a turn countdown/polarity; mode7 uses pitch/bearing steps.
// Evidence, scope and schema45 decision: docs/atari-st/ship-rules.md.
type EnemyCourseState struct {
	Ticks          int    `json:"ticks"`
	FireReady      bool   `json:"fireReady"`
	CanPrepareFire bool   `json:"canPrepareFire"`
	Maneuver       [2]int `json:"maneuver"`
}

func initializeEnemyManeuver(s *State, c *EnemyCourseState) {
	s.EnemyFireCountdown = 20
	switch s.EnemyCourseMode {
	case 3:
		c.Maneuver[1] = 0
		if s.CombatRange <= 60 {
			c.Maneuver[1] = 1
		}
		c.Maneuver[0] = rollBetween(s, 2, 10)
	case 4:
		s.EnemyDriftBearing, s.EnemyDriftPitch = 3, -3
	case 5:
		s.EnemyDriftBearing = 2
		if combatRoll(s, 2) != 0 {
			s.EnemyDriftBearing = -3
		}
		s.EnemyDriftPitch = 3
		if combatRoll(s, 2) != 0 {
			s.EnemyDriftPitch = -2
		}
	case 7:
		s.EnemyDriftBearing, s.EnemyDriftPitch = -4, 4
		c.Maneuver = [2]int{-1, 1}
	}
}

// Returns false at the unproven mode0/cloak helper boundary. No local-frame
// residue or invented replacement enters typed state.
func selectEnemyCourse(s *State, c *EnemyCourseState) bool {
	distance := combatRange(*s)
	rand := func(low, high int) int { return rollBetween(s, low, high) }
	switch s.EnemyCourseMode {
	case 0:
		if s.ShipPhase == 0 && s.ShipControls.Secondary[4] {
			return false
		}
		s.EnemyDriftBearing = rand(-2, 2)
		s.EnemyDriftPitch = rand(-2, 2)
		s.EnemyClosing, c.Ticks = -2, distance/16
	case 1:
		s.EnemyDriftPitch = rand(-1, 1)
		s.EnemyDriftBearing = rand(-1, 1)
		s.EnemyClosing = -1
		if distance < 70 {
			s.EnemyClosing = rand(-2, 2)
		}
		c.Ticks = distance / 10
	case 2:
		s.EnemyClosing = -2
		if distance <= 90 {
			s.EnemyClosing = -1
			if distance < 60 {
				s.EnemyClosing = 1
			}
		}
		s.EnemyDriftBearing++
		if s.EnemyDriftBearing > 4 {
			s.EnemyDriftBearing = -4
		}
		s.EnemyDriftPitch--
		if s.EnemyDriftPitch < -4 {
			s.EnemyDriftPitch = 4
		}
		c.Ticks = 20
	case 3:
		s.EnemyDriftBearing, s.EnemyDriftPitch = rand(-4, 4), rand(-4, 4)
		if distance > 90 {
			s.EnemyClosing = -3
		} else if distance < 50 || distance > 80 {
			s.EnemyClosing = 0
		}
		if s.EnemyClosing == 0 {
			c.Maneuver[0]--
		}
		if c.Maneuver[0] < 1 {
			if c.Maneuver[1] == 0 {
				s.EnemyClosing = rand(-3, -1)
				c.Maneuver[1] = 1
			} else {
				s.EnemyClosing = rand(1, 3)
				c.Maneuver[1] = 0
			}
			c.Maneuver[0] = rand(2, 10)
		}
		c.Ticks = 20
	case 4:
		s.EnemyDriftBearing = -s.EnemyDriftBearing
		if combatRoll(s, 5) == 0 {
			s.EnemyDriftBearing++
			s.EnemyDriftPitch = -s.EnemyDriftPitch + 1
		}
		c.Ticks, s.EnemyClosing = 40, -1
		if distance <= 80 {
			s.EnemyClosing = rand(-2, 1)
		}
	case 5:
		s.EnemyClosing = -2
		if distance <= 70 {
			s.EnemyClosing = rand(-2, 1)
		}
		if combatRoll(s, 2) != 0 {
			s.EnemyDriftBearing = -s.EnemyDriftBearing
		} else {
			s.EnemyDriftPitch = -s.EnemyDriftPitch
		}
		c.Ticks = rand(5, 20)
	case 6:
		s.EnemyDriftBearing, s.EnemyDriftPitch = rand(-4, 0), rand(-4, 4)
		c.Ticks, s.EnemyClosing = 30, -2
		if distance <= 60 {
			s.EnemyClosing = 0
		}
	case 7:
		if s.EnemyDriftBearing == 4 || s.EnemyDriftBearing == -4 {
			c.Maneuver[1] = -c.Maneuver[1]
		}
		if s.EnemyDriftPitch == 4 || s.EnemyDriftBearing == -4 {
			c.Maneuver[0] = -c.Maneuver[0]
		}
		s.EnemyDriftBearing += c.Maneuver[1]
		s.EnemyDriftPitch += c.Maneuver[0]
		if distance > 100 {
			s.EnemyClosing = -3
		} else if distance > 60 {
			s.EnemyClosing = rand(-2, 2)
		}
		c.Ticks = 20
	default:
		return false
	}
	s.EnemyDriftBearing = clamp(s.EnemyDriftBearing, -4, 4)
	s.EnemyDriftPitch = clamp(s.EnemyDriftPitch, -4, 4)
	if distance < 40 {
		s.EnemyClosing = 1
	}
	if s.ShipPhase != 0 {
		return true
	}
	retreat, closing, ticks := false, 0, 0
	switch s.EnemyCourseMode {
	case 0:
		return true // flag12=false was checked above; no undefined read
	case 1:
		retreat = s.EnemyHull < rand(10, 120)
		closing, ticks = 3, 45
	case 2:
		retreat = s.EnemyHull < 80
		closing, ticks = 1, 130
	case 3:
		retreat = s.EnemyHull < rand(30, 60)
		closing, ticks = 3, 40
	case 4, 5, 6:
		retreat = s.EnemyHull < rand(60, 100)
		closing, ticks = 2, 65
	case 7:
		retreat = s.EnemyHull < rand(40, 80)
		closing, ticks = 1, 130
	}
	if retreat || s.ShipControls.Secondary[4] {
		s.EnemyClosing, c.Ticks = closing, ticks
		c.CanPrepareFire = false
	}
	return true
}
func prepareEnemyFire(s *State, c *EnemyCourseState) {
	ship, ok := enemyShip(s.EnemyType)
	if !ok {
		return
	}
	if s.EnemyFireCountdown > 0 {
		s.EnemyFireCountdown -= ship.FireRate
	}
	if s.EnemyFireCountdown < 1 && c.CanPrepareFire && !s.ShipControls.Secondary[4] {
		switch s.EnemyCourseMode {
		case 0:
			c.FireReady = combatRoll(s, 100) > combatRange(*s)
		case 2:
			odd := combatRoll(s, 2) != 0
			c.FireReady = combatRange(*s) < 80 && odd && s.ShipPhase == 0
		case 6:
			limit := rollBetween(s, 40, 70)
			c.FireReady = combatRange(*s) < limit && (s.EnemyDriftPitch > 3 || s.EnemyDriftPitch < -3)
		default:
			c.FireReady = combatRange(*s) < rollBetween(s, 40, 80)
		}
	}
	// 0x0ce1..0x0ced only sets the redraw request when ready is true.
	// Clearing it belongs to the omitted display consumer, not set_fire.
}

// All non-cloaked modes share the source persistent course/preparation lifecycle.
// Cloaked mode0 retains its legacy projection; helper residue is unverified.
func advanceEnemyCombat(s *State) {
	if !s.CombatActive {
		return
	}
	if s.EnemyCourseMode == 0 && s.ShipControls.Secondary[4] {
		steerEnemy(s)
		resolveEnemyFire(s)
	} else {
		c := &s.EnemyCourse
		if c.Ticks > 0 {
			c.Ticks--
			s.EnemyBearing = wrapAngle(s.EnemyBearing - s.EnemyDriftBearing*4)
			s.EnemyPitch = wrapAngle(s.EnemyPitch + s.EnemyDriftPitch*4)
			s.CombatRange = max(10, s.CombatRange+s.EnemyClosing)
		} else if !selectEnemyCourse(s, c) {
			return
		}
		if c.FireReady {
			c.FireReady = false
			s.EnemyFireCountdown = rollBetween(s, 5, 10)
			resolveEnemyShot(s)
		} else {
			prepareEnemyFire(s, c)
			s.MessageKey = "ui.message.enemy_holding"
		}
	}
	if s.HullCondition <= 0 {
		s.Sleep = SleepState{}
		s.Screen, s.OutcomeKey = ScreenOutcome, "ui.outcome.failure"
		s.CombatActive = false
		s.MessageKey = "ui.message.defeat"
	}
}

func ValidateEnemyCourseState(s State) error {
	c := s.EnemyCourse
	if c.Ticks < 0 || c.Ticks > 32767 {
		return fmt.Errorf("invalid enemy course countdown")
	}
	for _, v := range c.Maneuver {
		if v < -32768 || v > 32767 {
			return fmt.Errorf("invalid enemy maneuver value")
		}
	}
	return nil
}
