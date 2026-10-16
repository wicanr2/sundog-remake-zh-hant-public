package game

import "testing"

func TestAwakeVBLNewGameAndForegroundInterrupt(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen = ScreenShipboard
	s.TimeMinutes = 59
	s.Rest = 1
	for i := 0; i < 31; i++ {
		s = sleepStep(t, r, s, ActionVBLTick)
	}
	if s.TimeMinutes != 59 || s.Sleep.Active || s.ShipPhase != 0 {
		t.Fatalf("callback before first empty-table wait: %+v", s.AwakeClock)
	}
	s = sleepStep(t, r, s, ActionVBLTick)
	if s.TimeMinutes != 60 || !s.Sleep.Active || !s.Sleep.Forced || s.ShipPhase != 1 {
		t.Fatalf("foreground interrupted before the same-pass ship callback: time%d sleep%+v phase%d", s.TimeMinutes, s.Sleep, s.ShipPhase)
	}
	if s.AwakeClock.Background.WaitEvents != 8 || s.AwakeClock.MinutePause != 60 || s.AwakeClock.ShipPause != 32 {
		t.Fatalf("next wait/pause contract: %+v", s.AwakeClock)
	}
	if err := ValidateAwakeClockState(s); err != nil {
		t.Fatal(err)
	}
	before := s.TimeMinutes
	for i := 0; i < 59; i++ {
		s = sleepStep(t, r, s, ActionVBLTick)
	}
	if s.TimeMinutes != before || s.ShipPhase != 1 {
		t.Fatal("sleep double-counted awake callbacks")
	}
}

func TestAwakeClockDraggingAndSceneLifecycle(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen = ScreenStores
	for i := 0; i < 9; i++ {
		s = sleepStep(t, r, s, ActionVBLTick)
	}
	wait := s.AwakeClock.Background.WaitEvents
	s = r.SelectStoreItem(s, 0)
	if s.HandItem == 0 || s.AwakeClock.MinutePause != 60 || s.AwakeClock.Background.PeriodTicks(ClockMinute) != -1 {
		t.Fatal("direct UI drag did not pause minute callback")
	}
	if s.AwakeClock.Background.WaitEvents != wait || s.AwakeClock.Background.PeriodTicks(ClockIdleShip) != 32 {
		t.Fatal("drag altered worker wait or ship callback")
	}
	for i := 0; i < 32; i++ {
		s = sleepStep(t, r, s, ActionVBLTick)
	}
	if s.TimeMinutes != 0 || s.ShipPhase != 1 {
		t.Fatal("drag stopped wrong callback")
	}
	wait = s.AwakeClock.Background.WaitEvents
	s = r.CancelHand(s)
	if s.AwakeClock.MinutePause != 0 || s.AwakeClock.Background.WaitEvents != wait {
		t.Fatal("restore reselected waiting pass")
	}
	for i := 0; i < wait*4; i++ {
		s = sleepStep(t, r, s, ActionVBLTick)
	}
	if s.TimeMinutes != 1 || s.AwakeClock.Background.WaitEvents == 65536 {
		t.Fatal("restore entered artificial zero-wait boundary")
	}
	s.Screen = ScreenShipboard
	s = sleepStep(t, r, s, ActionOpenGround)
	if s.AwakeClock.ShipActive || s.AwakeClock.Background.PeriodTicks(ClockIdleShip) != -1 || s.AwakeClock.Ground.Deadline != s.AwakeClock.Source.Ticks+1 {
		t.Fatal("ground entry clock contract")
	}
}

func TestGroundMovementClockUsesDeadlineAndCommand(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []GroundMode{GroundOnFoot, GroundInPod} {
		s := NewState()
		s.Screen, s.GroundMode = ScreenGround, mode
		syncAwakeClockLifecycle(&s)
		resetGroundClock(&s)
		// Pixel movement may be blocked and the character cell stays fixed.
		position := s.Position
		for i := 0; i < 10; i++ {
			s = sleepStep(t, r, s, ActionVBLTick)
			s = sleepStep(t, r, s, ActionVBLTick)
			s = sleepStep(t, r, s, ActionGroundClockTick)
			s = sleepStep(t, r, s, ActionGroundClockTick) // Same timestamp must not count twice.
		}
		want := 4
		if mode == GroundInPod {
			want = 1
		}
		if s.TimeMinutes != want || s.Position != position || s.AwakeClock.Ground.Updates != 0 || s.StreetRolls != 0 {
			t.Fatalf("mode%s time%d updates%d rolls%d", mode, s.TimeMinutes, s.AwakeClock.Ground.Updates, s.StreetRolls)
		}
	}
}
