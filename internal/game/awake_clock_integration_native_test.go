package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestExportTalosAwakeClockIntegration(t *testing.T) {
	root := os.Getenv("SUNDOG_AWAKE_CLOCK_NATIVE")
	if root == "" {
		t.Skip("fresh native clock receipts absent")
	}
	read := func(folder, name string, into any) {
		data, err := os.ReadFile(filepath.Join(root, folder, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, into); err != nil {
			t.Fatal(err)
		}
	}
	report := struct {
		BackgroundCases, GroundCases, ResearchOnly, Differences int
		Scope                                                   string
	}{Scope: "formal worker/ground minute consumers and four conditions; worker fixture defers foreground until final pass; not original GUI/complete flight/combat"}
	var background struct {
		Rows []struct {
			Location, Phase, Passes, MinuteAfter, HourAfter, SceneAfter int
			Before, After                                               [4]int
			SlotsBefore, SlotsAfter                                     [2][3]int
			Trace                                                       struct{ Pulses, ShipUpdates int }
		}
	}
	read("background", "native-food-awake-background.json", &background)
	for _, row := range background.Rows {
		if row.Phase == 0 {
			report.ResearchOnly++
			continue
		}
		s := NewState()
		s.Screen = ScreenGround
		if row.Location == 2 {
			s.Screen = ScreenShipboard
		}
		s.GroundWorld = row.Location == 4
		s.TimeMinutes = 59
		s.Vigor, s.Rest, s.Health, s.Nourishment = row.Before[0], row.Before[1], row.Before[2], row.Before[3]
		s.AwakeClock.Background = BackgroundClock{}
		if row.Phase < 0 {
			s.AwakeClock.Background.BeginPass()
			_ = s.AwakeClock.Background.Start(ClockMinute, 60)
			if row.Location == 2 {
				_ = s.AwakeClock.Background.Start(ClockIdleShip, 30)
			}
		} else {
			for i, v := range row.SlotsBefore {
				s.AwakeClock.Background.Slots[i] = ClockSlot{ClockCallback(v[0]), v[1], v[2]}
			}
		}
		pulses, passes := 0, 0
		for passes < row.Passes && pulses < 70000 {
			pulses++
			if runAwakeEvent(&s) {
				passes++
			}
		}
		// The bounded native worker fixture has no foreground between passes.
		// Consume its pending condition mode once, at the same final boundary.
		if s.TimeMinutes >= 60 {
			interruptForConditions(&s)
		}
		var slots [2][3]int
		for i := range slots {
			v := s.AwakeClock.Background.Slots[i]
			slots[i] = [3]int{int(v.Callback), v.Remaining, v.Period}
		}
		mode := row.SceneAfter >> 8 & 15
		interruptOK := mode != 10 && mode != 12 && !s.Sleep.Active && s.Screen != ScreenOutcome || mode == 10 && s.Sleep.Active && s.Sleep.Forced || mode == 12 && s.Screen == ScreenOutcome
		if [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment} != row.After || s.TimeMinutes != row.HourAfter*60+row.MinuteAfter || slots != row.SlotsAfter || pulses != row.Trace.Pulses || !interruptOK {
			report.Differences++
			t.Logf("background location%d phase%d passes%d time%d readings%v interrupt%t", row.Location, row.Phase, row.Passes, s.TimeMinutes, [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment}, interruptOK)
		}
		report.BackgroundCases++
	}
	var ground struct {
		Ground []struct {
			Foot                                                                               bool
			Record5, CounterBefore, CounterAfter, NowDelta, MinuteAfter, HourAfter, SceneAfter int
			Before, After                                                                      [4]int
			DeadlineAfter                                                                      [2]int
		}
	}
	read("ground", "native-food-ground-clock.json", &ground)
	for _, row := range ground.Ground {
		if row.CounterBefore == 10 {
			report.ResearchOnly++
			continue
		}
		s := NewState()
		s.Screen, s.TimeMinutes, s.GroundMode = ScreenGround, 59, GroundInPod
		if row.Foot {
			s.GroundMode = GroundOnFoot
		}
		s.Vigor, s.Rest, s.Health, s.Nourishment = row.Before[0], row.Before[1], row.Before[2], row.Before[3]
		s.AwakeClock.Source.Ticks = uint32(100 + row.NowDelta)
		s.AwakeClock.Ground = GroundClock{100, row.CounterBefore}
		advanceGroundClock(&s, row.Record5 == 1)
		mode := row.SceneAfter >> 8 & 15
		interruptOK := mode != 10 && mode != 12 && !s.Sleep.Active && s.Screen != ScreenOutcome || mode == 10 && s.Sleep.Active && s.Sleep.Forced || mode == 12 && s.Screen == ScreenOutcome
		deadline := uint32(row.DeadlineAfter[0])<<16 | uint32(row.DeadlineAfter[1])
		if [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment} != row.After || s.TimeMinutes != row.HourAfter*60+row.MinuteAfter || s.AwakeClock.Ground.Updates != row.CounterAfter || s.AwakeClock.Ground.Deadline != deadline || !interruptOK {
			report.Differences++
		}
		report.GroundCases++
	}
	if report.BackgroundCases != 240 || report.GroundCases != 216 || report.ResearchOnly != 73 {
		t.Fatalf("incomplete integration comparison: %+v", report)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "remake-awake-clock-integration.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 {
		t.Fatalf("formal clock consumer differences: %+v", report)
	}
}
