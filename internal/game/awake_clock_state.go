package game

import "fmt"

// Semantic state for awake-clock-integration-20261004 READY. The source uses
// a remake epoch, not an interpreter address or a snapshot of original RAM.
type AwakeClockState struct {
	Source      VBLClock        `json:"source"`
	Background  BackgroundClock `json:"background"`
	Ground      GroundClock     `json:"ground"`
	ShipActive  bool            `json:"shipActive"`
	MinutePause int             `json:"minutePause"`
	ShipPause   int             `json:"shipPause"`
}

func newAwakeClock() AwakeClockState {
	c := AwakeClockState{ShipActive: true}
	c.Source.Enable(true)
	// The higher-priority worker reaches WAIT on the initially empty table
	// before the foreground registers these two callbacks.
	c.Background.BeginPass()
	_ = c.Background.Start(ClockMinute, 60)
	_ = c.Background.Start(ClockIdleShip, 30)
	return c
}

func ValidateAwakeClockState(s State) error {
	c := s.AwakeClock
	if err := c.Source.Validate(); err != nil {
		return err
	}
	if !c.Source.Enabled {
		return fmt.Errorf("game clock source is disabled")
	}
	if err := c.Background.Validate(); err != nil {
		return err
	}
	if c.Background.WaitEvents < 1 || c.Background.WaitEvents > 8 {
		return fmt.Errorf("game clock is not at a foreground wait")
	}
	if err := c.Ground.Validate(); err != nil {
		return err
	}
	if c.Ground.Updates == 10 {
		return fmt.Errorf("unconsumed ground clock update")
	}
	for _, slot := range c.Background.Slots {
		if slot.Callback > ClockIdleShip {
			return fmt.Errorf("unsupported game clock callback %d", slot.Callback)
		}
		if slot.Callback == ClockMinute && slot.Period != 15 || slot.Callback == ClockIdleShip && slot.Period != 8 {
			return fmt.Errorf("invalid game clock callback period")
		}
	}
	if c.MinutePause == 0 && c.Background.PeriodTicks(ClockMinute) == -1 {
		return fmt.Errorf("minute clock is absent without a pause")
	}
	if c.ShipActive && c.ShipPause == 0 && c.Background.PeriodTicks(ClockIdleShip) == -1 {
		return fmt.Errorf("ship clock is absent without a pause")
	}
	for _, p := range []struct {
		id    ClockCallback
		value int
	}{{ClockMinute, c.MinutePause}, {ClockIdleShip, c.ShipPause}} {
		period := 60
		if p.id == ClockIdleShip {
			period = 32
		}
		if p.value != 0 && p.value != period {
			return fmt.Errorf("invalid paused clock period")
		}
		if p.value != 0 && c.Background.PeriodTicks(p.id) != -1 {
			return fmt.Errorf("paused clock still has a callback")
		}
	}
	if !c.ShipActive && (c.Background.PeriodTicks(ClockIdleShip) != -1 || c.ShipPause != 0) {
		return fmt.Errorf("ground scene has a ship clock")
	}
	return nil
}

// Only source/game scenes change slot 2. A navigation window still belongs
// to the ship scene. Library/status are overlays over their recorded return.
func awakeClockScene(s State) (bool, bool) {
	screen := s.Screen
	if screen == ScreenCharacterLibrary {
		screen = s.LibraryReturn
	}
	if screen == ScreenCharacterStatus {
		screen = s.StatusReturn
	}
	return sleepScene(screen)
}

func pauseClock(c *BackgroundClock, id ClockCallback, saved *int, paused bool) {
	if paused && *saved == 0 {
		*saved = c.PeriodTicks(id)
		c.Stop(id)
	} else if !paused && *saved != 0 {
		_ = c.Start(id, *saved)
		*saved = 0
	}
}

func syncAwakeClockLifecycle(s *State) {
	c := &s.AwakeClock
	if ship, valid := awakeClockScene(*s); valid && ship != c.ShipActive {
		c.ShipActive = ship
		c.Background.Stop(ClockIdleShip)
		c.ShipPause = 0
		if ship {
			_ = c.Background.Start(ClockIdleShip, 30)
		}
	}
	dragging := s.HandItem != 0 && s.HandOrigin.Kind != ""
	pauseClock(&c.Background, ClockMinute, &c.MinutePause, dragging || s.Sleep.Active)
	if c.ShipActive {
		pauseClock(&c.Background, ClockIdleShip, &c.ShipPause, s.Sleep.Active)
	}
}

func resetGroundClock(s *State) {
	s.AwakeClock.Ground.Updates = 0
	_ = s.AwakeClock.Ground.Reset(s.AwakeClock.Source.Ticks, 1)
}

// Foreground consumes a condition interrupt only after every callback and
// the higher-priority worker's next BeginPass. In particular, a minute that
// reaches Rest0 must not suppress the ship callback later in the same pass.
func advanceAwakeVBL(s *State) {
	syncAwakeClockLifecycle(s)
	if !s.AwakeClock.Source.Advance() {
		return
	}
	clockBefore := s.TimeMinutes
	completed := runAwakeEvent(s)
	if completed {
		s.AwakeClock.Background.BeginPass()
		if s.TimeMinutes/60 != clockBefore/60 && !s.Sleep.Active {
			interruptForConditions(s)
		}
	}
}

// Kept separate from foreground dispatch for the native worker-boundary
// comparison. Production calls BeginPass immediately after this returns.
func runAwakeEvent(s *State) bool {
	return s.AwakeClock.Background.AdvanceEvent(func(id ClockCallback) {
		switch id {
		case ClockMinute:
			advanceClockMinutes(s, 1)
		case ClockIdleShip:
			advanceShipClock(s)
		}
	})
}

func advanceGroundClock(s *State, moving bool) {
	advanceTime(s, s.AwakeClock.Ground.Update(s.AwakeClock.Source.Ticks, moving, s.GroundMode == GroundOnFoot))
}

// One parent callback: phase/movement, energy, then the enemy update. Sleep,
// awake slots and explicit tactical steps share this order.
func advanceShipClock(s *State) {
	if s.FlightTicks > 0 && s.FlightTick < s.FlightTicks {
		advanceInSystemFlight(s)
	} else {
		advanceShipPhase(s)
		updateShipEnergy(s, func(n int) int { return combatRoll(s, n) })
	}
	advanceEnemyCombat(s)
}
