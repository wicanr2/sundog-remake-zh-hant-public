package game

import "fmt"

// The four clock layers use semantic values, never interpreter RAM. Source
// addresses and scope are in awake-clock-core-20261004 READY. GUI pacing and
// new-game process interleaving belong to the integration contract.
type VBLClock struct {
	Ticks          uint32 `json:"ticks"`
	Enabled        bool   `json:"enabled"`
	EventCountdown int    `json:"eventCountdown"`
}

func (c VBLClock) Validate() error {
	if c.Enabled && (c.EventCountdown < 1 || c.EventCountdown > 4) || !c.Enabled && c.EventCountdown != -1 {
		return fmt.Errorf("invalid VBL event phase")
	}
	return nil
}

func (c *VBLClock) Enable(enabled bool) {
	c.Enabled = enabled
	c.EventCountdown = -1
	if enabled {
		c.EventCountdown = 4
	}
}

// One original vertical-blank counter input, with no seconds conversion.
func (c *VBLClock) Advance() bool {
	c.Ticks++
	if !c.Enabled {
		return false
	}
	c.EventCountdown--
	if c.EventCountdown != 0 {
		return false
	}
	c.EventCountdown = 4
	return true
}

type ClockCallback int

const (
	ClockMinute   ClockCallback = 1
	ClockIdleShip ClockCallback = 2
)

// Other identities are preserved for the original eight-slot dispatcher;
// their consumer semantics are outside this core's verified scope.
type ClockSlot struct {
	Callback  ClockCallback `json:"callback"`
	Remaining int           `json:"remaining"`
	Period    int           `json:"period"`
}

type BackgroundClock struct {
	WaitEvents int          `json:"waitEvents"`
	Slots      [8]ClockSlot `json:"slots"`
}

func (c BackgroundClock) Validate() error {
	if c.WaitEvents < 0 || c.WaitEvents > 65536 {
		return fmt.Errorf("invalid background wait")
	}
	seen := [8]bool{}
	for _, s := range c.Slots {
		if s.Callback == 0 {
			if s != (ClockSlot{}) {
				return fmt.Errorf("empty callback has pending countdown")
			}
			continue
		}
		if s.Callback < 1 || s.Callback > 7 || seen[s.Callback] || s.Remaining < 0 || s.Remaining > 32767 || s.Period < 1 || s.Period > 8191 {
			return fmt.Errorf("invalid background slot")
		}
		seen[s.Callback] = true
	}
	return nil
}

func (c BackgroundClock) PeriodTicks(id ClockCallback) int {
	for _, s := range c.Slots {
		if s.Callback == id {
			return s.Period * 4
		}
	}
	return -1
}

func (c *BackgroundClock) Stop(id ClockCallback) {
	for i := range c.Slots {
		if c.Slots[i].Callback == id {
			c.Slots[i] = ClockSlot{}
			return
		}
	}
}

// Start recreates MAINLIB:30's period rounding and first-empty-slot rule.
// A nonpositive saved period means that the callback was absent.
func (c *BackgroundClock) Start(id ClockCallback, ticks int) error {
	if ticks <= 0 {
		return nil
	}
	if id < 1 || id > 7 || ticks > 32765 {
		return fmt.Errorf("invalid background callback period")
	}
	index := -1
	for i, s := range c.Slots {
		if s.Callback == id {
			index = i
			break
		}
	}
	if index < 0 {
		for i, s := range c.Slots {
			if s.Callback == 0 {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return fmt.Errorf("background callback table is full")
	}
	c.Slots[index] = ClockSlot{Callback: id, Period: max(1, (ticks+2)/4)}
	return nil
}

// BeginPass deducts before the wait. A save made during the wait must retain
// WaitEvents and the already-deducted slot values, without selecting again.
func (c *BackgroundClock) BeginPass() {
	if c.WaitEvents != 0 {
		return
	}
	delay := 8
	for _, s := range c.Slots {
		if s.Callback > 0 {
			delay = min(delay, s.Remaining)
		}
	}
	for i := range c.Slots {
		if c.Slots[i].Callback > 0 {
			c.Slots[i].Remaining -= delay
		}
	}
	c.WaitEvents = delay
	if delay == 0 {
		// Original do/wait loop compares a 16-bit counter after its first
		// wait. This is an artificial boundary, not a resume default.
		c.WaitEvents = 65536
	}
}

// AdvanceEvent consumes one event0 pulse. Refill precedes the callback;
// later slots are read after earlier callbacks, as in the original XJP loop.
func (c *BackgroundClock) AdvanceEvent(dispatch func(ClockCallback)) bool {
	if c.WaitEvents == 0 {
		c.BeginPass()
	}
	c.WaitEvents--
	if c.WaitEvents != 0 {
		return false
	}
	for i := range c.Slots {
		s := &c.Slots[i]
		if s.Callback > 0 && s.Remaining == 0 {
			id := s.Callback
			s.Remaining = s.Period
			if dispatch != nil {
				dispatch(id)
			}
		}
	}
	return true
}

type GroundClock struct {
	Deadline uint32 `json:"deadline"`
	Updates  int    `json:"updates"`
}

func (c GroundClock) Validate() error {
	if c.Updates < 0 || c.Updates > 10 {
		return fmt.Errorf("invalid ground clock counter")
	}
	return nil
}

// MAINLIB:28 compares an unsigned timestamp, but its high-word subtraction
// is signed16. It is not a modular elapsed-time comparison.
func GroundTimeReady(deadline, now uint32) bool {
	return deadline < now && int16(uint16(now>>16)-uint16(deadline>>16)) < 3
}

func (c *GroundClock) Reset(now uint32, delta uint16) error {
	// MAINLIB:26 detects a low-word carry using signed16 transitions.
	// Its observed callers use positive deltas in this safe range.
	if delta > 32767 {
		return fmt.Errorf("ground deadline delta exceeds verified range")
	}
	c.Deadline = now + uint32(delta)
	return nil
}

// moving is the move command flag, including movement blocked by collision.
// The returned minutes are additional to the background minute callback.
func (c *GroundClock) Update(now uint32, moving, onFoot bool) int {
	if !moving || !GroundTimeReady(c.Deadline, now) {
		return 0
	}
	c.Deadline = now + 1
	c.Updates++
	if c.Updates < 10 {
		return 0
	}
	c.Updates = 0
	if onFoot {
		return 4
	}
	return 1
}
