package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Restore during an existing worker wait, not before wait selection. This
// distinguishes normal resume from the artificial 65536-event input.
func TestClockResumeDuringSavedWait(t *testing.T) {
	c := BackgroundClock{WaitEvents: 5}
	if err := c.Start(ClockMinute, 60); err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded BackgroundClock
	if err := json.Unmarshal(saved, &reloaded); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatal(err)
	}
	for event := 1; event <= 80; event++ {
		var direct, resumed []ClockCallback
		c.AdvanceEvent(func(id ClockCallback) { direct = append(direct, id) })
		reloaded.AdvanceEvent(func(id ClockCallback) { resumed = append(resumed, id) })
		if !reflect.DeepEqual(c, reloaded) || !reflect.DeepEqual(direct, resumed) {
			t.Fatalf("saved worker differs at event %d", event)
		}
		if event == 5 && !reflect.DeepEqual(direct, []ClockCallback{ClockMinute}) || event < 5 && len(direct) != 0 {
			t.Fatalf("resume did not consume saved wait: event%d %v", event, direct)
		}
	}
}

func TestClockDispatchRefillAndLaterSlotMutation(t *testing.T) {
	c := BackgroundClock{WaitEvents: 1, Slots: [8]ClockSlot{
		{Callback: ClockMinute, Period: 15}, {Callback: ClockIdleShip, Period: 8},
	}}
	var fired []ClockCallback
	c.AdvanceEvent(func(id ClockCallback) {
		fired = append(fired, id)
		if id == ClockMinute {
			if c.Slots[0].Remaining != 15 {
				t.Fatal("period must refill before callback")
			}
			c.Stop(ClockIdleShip)
		}
	})
	if !reflect.DeepEqual(fired, []ClockCallback{ClockMinute}) || c.Slots[1] != (ClockSlot{}) {
		t.Fatalf("later slot was dispatched after stop: %v %+v", fired, c)
	}
}

func TestClockJSONPreservesAllLayers(t *testing.T) {
	type clocks struct {
		Source     VBLClock
		Background BackgroundClock
		Ground     GroundClock
	}
	for phase := 1; phase <= 4; phase++ {
		for wait := 1; wait <= 8; wait++ {
			direct := clocks{
				Source: VBLClock{Ticks: 0xfffffff8, Enabled: true, EventCountdown: phase},
				Background: BackgroundClock{WaitEvents: wait, Slots: [8]ClockSlot{
					{Callback: ClockMinute, Remaining: 0, Period: 15},
					{Callback: ClockIdleShip, Remaining: 0, Period: 8},
				}},
				Ground: GroundClock{Deadline: 0xfffffff9, Updates: 9},
			}
			data, err := json.Marshal(direct)
			if err != nil {
				t.Fatal(err)
			}
			var resumed clocks
			if err := json.Unmarshal(data, &resumed); err != nil {
				t.Fatal(err)
			}
			for tick := 0; tick < 400; tick++ {
				advance := func(c *clocks) (callbacks []ClockCallback, minutes int) {
					if c.Source.Advance() {
						c.Background.AdvanceEvent(func(id ClockCallback) { callbacks = append(callbacks, id) })
					}
					minutes = c.Ground.Update(c.Source.Ticks, tick%3 != 0, tick%2 == 0)
					return
				}
				left, leftMinutes := advance(&direct)
				right, rightMinutes := advance(&resumed)
				if direct != resumed || leftMinutes != rightMinutes || !reflect.DeepEqual(left, right) {
					t.Fatalf("phase%d wait%d tick%d differs after JSON resume", phase, wait, tick)
				}
			}
		}
	}
}

func TestClockRejectsInvalidSavedCountdowns(t *testing.T) {
	g := GroundClock{Deadline: 17, Updates: 9}
	if err := g.Reset(100, 32768); err == nil || g != (GroundClock{Deadline: 17, Updates: 9}) {
		t.Fatal("unsupported signed16 deadline delta mutated state")
	}
	for _, c := range []BackgroundClock{
		{WaitEvents: -1}, {WaitEvents: 65537},
		{Slots: [8]ClockSlot{{Remaining: 1}}},
		{Slots: [8]ClockSlot{{Callback: 8, Period: 1}}},
		{Slots: [8]ClockSlot{{Callback: 1, Period: 1}, {Callback: 1, Period: 2}}},
		{Slots: [8]ClockSlot{{Callback: 1, Period: 0}}},
		{Slots: [8]ClockSlot{{Callback: 1, Period: 1, Remaining: 32768}}},
	} {
		if c.Validate() == nil {
			t.Fatalf("accepted invalid saved clock: %+v", c)
		}
	}
	for _, c := range []VBLClock{{}, {Enabled: true}, {Enabled: true, EventCountdown: 5}, {EventCountdown: 4}} {
		if c.Validate() == nil {
			t.Fatalf("accepted invalid event phase: %+v", c)
		}
	}
	// Every intermediate counter in the artificial zero wait can be saved.
	if err := (BackgroundClock{WaitEvents: 65535}).Validate(); err != nil {
		t.Fatal(err)
	}
}
