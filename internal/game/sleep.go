package game

import "fmt"

// SleepState keeps semantic countdowns across saves. The original's inner
// loop is measured in cycles, not seconds. See sleep-status-20261004 READY.
type SleepState struct {
	Active         bool `json:"active"`
	Forced         bool `json:"forced"`
	WakeRequested  bool `json:"wakeRequested"`
	MinuteCycles   int  `json:"minuteCycles"`
	ShipCycles     int  `json:"shipCycles"`
	ElapsedMinutes int  `json:"elapsedMinutes"`
	ShipActive     bool `json:"shipActive"`
}

// The remake separates views that belong to one original scene. Returning
// from an interrupt preserves that view; only ship scenes have slot 2.
func sleepScene(screen Screen) (ship, valid bool) {
	switch screen {
	case ScreenShipboard, ScreenNavigation, ScreenFlight, ScreenTactical,
		ScreenEngineering, ScreenEquipment, ScreenCargo, ScreenStores:
		return true, true
	case ScreenGround, ScreenInterior, ScreenTrade, ScreenParts, ScreenUniteller,
		ScreenDialog, ScreenSlots, ScreenArena, ScreenTubeStation:
		return false, true
	default:
		return false, false
	}
}

func ValidateSleepState(s State) error {
	b := s.Sleep
	if b.MinuteCycles < 0 || b.MinuteCycles >= 60 || b.ShipCycles < 0 || b.ShipCycles >= 32 || b.ElapsedMinutes < 0 {
		return fmt.Errorf("invalid sleep countdown")
	}
	if !b.Active {
		if b != (SleepState{}) {
			return fmt.Errorf("inactive sleep has pending state")
		}
		return nil
	}
	if s.Screen != ScreenCharacterStatus && !(s.Screen == ScreenCharacterLibrary && s.LibraryReturn == ScreenCharacterStatus) {
		return fmt.Errorf("sleep requires character status")
	}
	ship, valid := sleepScene(s.StatusReturn)
	if !valid || b.ShipActive != ship {
		return fmt.Errorf("invalid sleep scene")
	}
	if !b.ShipActive && b.ShipCycles != 0 {
		return fmt.Errorf("ground sleep has ship countdown")
	}
	return nil
}

func beginSleep(s *State, forced bool) {
	ship, _ := sleepScene(s.StatusReturn)
	s.Sleep = SleepState{Active: true, Forced: forced, ShipActive: ship}
	s.MessageKey = "ui.message.sleeping"
}

// Called after the complete clock increment. MAINLIB:35 does not suspend
// remaining hours inside the call when Rest first becomes zero.
func interruptForConditions(s *State) {
	if s.Vigor <= 0 {
		s.Sleep = SleepState{}
		s.Screen, s.OutcomeKey = ScreenOutcome, "ui.outcome.failure"
		s.CombatActive = false
		s.MessageKey = "ui.message.defeat"
		return
	}
	if s.Rest != 0 {
		return
	}
	home := s.Screen
	if home == ScreenCharacterStatus {
		home = s.StatusReturn
	}
	if _, valid := sleepScene(home); !valid {
		return
	}
	s.StatusReturn = home
	s.Screen = ScreenCharacterStatus
	beginSleep(s, true)
}

// sleepTheft accepts raw controlled draws for same-state comparisons. Runtime
// uses the remake's saved RNG stream; matching seed numbers is not RNG parity.
func sleepTheft(s *State, minutes, risk int, draw func() int) {
	// The original admits city3 and interior5, but excludes world4.
	eligible := s.StatusReturn == ScreenGround && !s.GroundWorld || s.StatusReturn == ScreenInterior
	if !eligible || s.GroundMode != GroundOnFoot || s.SleepTheftProtected {
		return
	}
	for elapsed := minutes; elapsed > risk; elapsed -= 10 {
		if draw()%9*risk >= elapsed {
			continue
		}
		if draw()&1 != 0 && s.Cash != 0 {
			s.Cash = 0
			continue
		}
		first := draw() % CarrySlots
		for delta := 0; delta < CarrySlots; delta++ {
			slot := (first + delta) % CarrySlots
			if s.Parts[slot] != 0 {
				s.Parts = append([]int(nil), s.Parts...)
				s.Parts[slot] = 0
				break
			}
		}
	}
}

func (r Reducer) advanceSleep(s *State) {
	if !s.Sleep.Active || s.Screen != ScreenCharacterStatus {
		return
	}
	s.Sleep.MinuteCycles++
	callback := false
	if s.Sleep.MinuteCycles == 60 {
		s.Sleep.MinuteCycles = 0
		s.Sleep.ElapsedMinutes++
		// MAINLIB:35 reads last minute's Rest. The sleep writer follows it.
		rest := s.Rest
		advanceTime(s, 1)
		s.Rest = min(MaxCondition, rest+1)
		callback = true
	}
	if s.Sleep.ShipActive {
		s.Sleep.ShipCycles++
		if s.Sleep.ShipCycles == 32 {
			s.Sleep.ShipCycles = 0
			advanceShipClock(s)
			callback = true
		}
	}
	if !callback {
		if s.Sleep.Forced {
			s.Sleep.WakeRequested = false
		}
		return
	}
	if s.Sleep.Forced && s.Rest > 50 {
		s.Sleep.Forced = false
	}
	if s.Vigor <= 0 || s.HullCondition <= 0 {
		s.Sleep = SleepState{}
		s.Screen, s.OutcomeKey = ScreenOutcome, "ui.outcome.failure"
		s.CombatActive = false
		s.MessageKey = "ui.message.defeat"
		return
	}
	if s.Sleep.Forced {
		s.Sleep.WakeRequested = false
		return
	}
	if s.Sleep.WakeRequested && !s.Sleep.Forced {
		risk := 0
		if system, ok := r.World.System(s.SystemID); ok {
			risk = system.EncounterRating
		}
		sleepTheft(s, s.Sleep.ElapsedMinutes, risk, func() int { return combatRoll(s, 32768) })
		s.Sleep = SleepState{}
		s.MessageKey = "ui.message.rested"
	}
}
