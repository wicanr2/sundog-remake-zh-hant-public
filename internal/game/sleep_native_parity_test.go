package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Rule-level comparison of the real clock entry and subsequent interrupt.
// Normal player actions and saves are verified separately.
func TestExportTalosSleepInterrupt(t *testing.T) {
	root := os.Getenv("SUNDOG_SLEEP_INTERRUPT_NATIVE")
	if root == "" {
		t.Skip("native interrupt inputs absent")
	}
	data, err := os.ReadFile(filepath.Join(root, "native-food-sleep-interrupt-parent.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Rows []struct {
			Location, Delta, MinuteAfterClock, HourAfterClock, MinuteAfterSleep, HourAfterSleep int
			Before, AfterClock, AfterSleep                                                      [4]int
			SceneAfterClock                                                                     int
			Trace                                                                               struct{ Cycles, Minutes, ShipUpdates int }
		}
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		Cases, Differences int
		Examples           []string
	}{}
	for _, row := range native.Rows {
		s := NewState()
		s.Screen = ScreenGround
		if row.Location == 2 {
			s.Screen = ScreenShipboard
		}
		s.GroundWorld = row.Location == 4
		s.SleepTheftProtected = true
		s.Vigor, s.Rest, s.Health, s.Nourishment = row.Before[0], row.Before[1], row.Before[2], row.Before[3]
		s.TimeMinutes = 59
		advanceTime(&s, row.Delta)
		clockOK := [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment} == row.AfterClock && s.TimeMinutes == row.HourAfterClock*60+row.MinuteAfterClock
		mode := row.SceneAfterClock >> 8 & 15
		entryOK := !s.Sleep.Active
		if mode == 10 {
			entryOK = s.Sleep.Active && s.Sleep.Forced && s.Screen == ScreenCharacterStatus && s.Sleep.ShipActive == (row.Location == 2)
		}
		if mode == 12 {
			entryOK = !s.Sleep.Active && s.Screen == ScreenOutcome
		}
		cycles := 0
		for s.Sleep.Active && cycles < 8192 {
			cycles++
			s.Sleep.WakeRequested = true
			s = sleepStep(t, r, s, ActionSleepTick)
		}
		finalOK := [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment} == row.AfterSleep && s.TimeMinutes == row.HourAfterSleep*60+row.MinuteAfterSleep && cycles == row.Trace.Cycles
		report.Cases++
		if !clockOK || !entryOK || !finalOK {
			report.Differences++
			if len(report.Examples) < 12 {
				report.Examples = append(report.Examples, fmt.Sprintf("location%d before%v delta%d clock%t entry%t final%t", row.Location, row.Before, row.Delta, clockOK, entryOK, finalOK))
			}
		}
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "remake-food-sleep-interrupt.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 {
		t.Fatalf("formal interrupt differences: %+v", report)
	}
}

// The runner supplies newly regenerated native answers. Mode12 artificial
// initial states and non-production attribute rates remain research only.
func TestExportTalosSleep(t *testing.T) {
	root := os.Getenv("SUNDOG_SLEEP_NATIVE")
	if root == "" {
		t.Skip("native sleep inputs absent")
	}
	read := func(name string, out any) {
		t.Helper()
		data, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(data, out); e != nil {
			t.Fatal(e)
		}
	}
	r, e := NewReducer()
	if e != nil {
		t.Fatal(e)
	}
	type counts struct{ Cases, Differences int }
	report := struct {
		Wake, Ship, Menu, Theft counts
		ResearchOnly            int
		Examples                []string
	}{}
	check := func(c *counts, ok bool, label string) {
		c.Cases++
		if !ok {
			c.Differences++
			if len(report.Examples) < 12 {
				report.Examples = append(report.Examples, label)
			}
		}
	}
	var wake struct {
		Rows []struct {
			Before, After                                                              [4]int
			Minute, Hour, Day, Mode, MinuteAfter, HourAfter, DayAfter, ModeAfter, Dead int
			Trace                                                                      struct{ WakePoll, Minutes, Cycles int }
		}
	}
	read("native-food-sleep-wake.json", &wake)
	for _, row := range wake.Rows {
		if row.Mode == 12 {
			report.ResearchOnly++
			continue
		}
		s := NewState()
		s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenGround
		s.GroundMode = GroundInPod
		s.Vigor, s.Rest, s.Health, s.Nourishment = row.Before[0], row.Before[1], row.Before[2], row.Before[3]
		s.TimeMinutes = row.Day*1440 + row.Hour*60 + row.Minute
		beginSleep(&s, row.Mode == 10)
		cycles := 0
		for s.Sleep.Active && cycles < 8192 {
			cycles++
			if cycles >= row.Trace.WakePoll*60 {
				s.Sleep.WakeRequested = true
			}
			s = sleepStep(t, r, s, ActionSleepTick)
		}
		check(&report.Wake, [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment} == row.After && s.TimeMinutes == row.DayAfter*1440+row.HourAfter*60+row.MinuteAfter && cycles == row.Trace.Cycles && (s.Screen == ScreenOutcome) == (row.Dead != 0), "wake")
	}
	var ship struct {
		Rows []struct {
			Mode, Rest, WakeCycle, Phase, RestAfter int
			EnergyBefore, EnergyAfter               [4]int
			Trace                                   struct{ Cycles, Minutes, ShipUpdates int }
		}
	}
	read("native-food-sleep-ship-parent.json", &ship)
	for _, row := range ship.Rows {
		s := NewState()
		s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenShipboard
		s.Vigor, s.Rest, s.Health, s.Nourishment = 255, row.Rest, 255, 255
		s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = row.EnergyBefore[0], row.EnergyBefore[1], row.EnergyBefore[2], row.EnergyBefore[3]
		s.ShipSystems = IntactShipSystems()
		for i := range s.ShipSystems {
			if s.ShipSystems[i].ID == ShipSystemShipGuns || s.ShipSystems[i].ID == ShipSystemForceShields {
				// Step derives status from the grid; a cache assignment alone
				// is overwritten. Two intact rows + two empty rows yield eight.
				for row := 2; row < 4; row++ {
					s.ShipSystems[i].Rows[row] = []int{0, 0, 0, 0}
				}
				s.ShipSystems[i].Status = shipSystemStatus(s.ShipSystems[i])
				if s.ShipSystems[i].Status != 8 {
					t.Fatal("ship fixture is not status8")
				}
			}
		}
		s.ShipControls.Guns = true
		beginSleep(&s, row.Mode == 10)
		cycles := 0
		for s.Sleep.Active && cycles < 8192 {
			cycles++
			if cycles >= row.WakeCycle {
				s.Sleep.WakeRequested = true
			}
			s = sleepStep(t, r, s, ActionSleepTick)
		}
		check(&report.Ship, cycles == row.Trace.Cycles && s.TimeMinutes == row.Trace.Minutes && s.Rest == row.RestAfter && s.ShipPhase == row.Phase && [4]int{s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting} == row.EnergyAfter, fmt.Sprintf("ship mode%d rest%d wake%d energy%v: native cycles%d phase%d energy%v; formal cycles%d phase%d energy%v", row.Mode, row.Rest, row.WakeCycle, row.EnergyBefore, row.Trace.Cycles, row.Phase, row.EnergyAfter, cycles, s.ShipPhase, [4]int{s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting}))
	}
	var menu struct {
		Rows []struct {
			Location, Mode, Nourishment, Rest, RestAfter, NourishmentAfter int
			Trace                                                          struct{ Cycles, Minutes int }
		}
	}
	read("native-food-sleep-menu-parent.json", &menu)
	for _, row := range menu.Rows {
		s := NewState()
		s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenGround
		s.GroundMode = GroundInPod
		if row.Location == 2 {
			s.StatusReturn = ScreenShipboard
		}
		s.Nourishment, s.Rest = row.Nourishment, row.Rest
		s = sleepStep(t, r, s, ActionRest)
		if row.Mode == 10 {
			s.Sleep.Forced = true
		}
		cycles := 0
		for s.Sleep.Active && cycles < 8192 {
			cycles++
			if cycles >= 60 {
				s.Sleep.WakeRequested = true
			}
			s = sleepStep(t, r, s, ActionSleepTick)
		}
		s = sleepStep(t, r, s, ActionBack)
		check(&report.Menu, cycles == row.Trace.Cycles && s.TimeMinutes == row.Trace.Minutes && s.Rest == row.RestAfter && s.Nourishment == row.NourishmentAfter && s.Screen == s.StatusReturn, "menu")
	}
	var theft struct {
		Rows []struct {
			LocationMode, Risk, Minutes, CashBefore, CashAfter int
			Bit13, Flag57                                      bool
			CarryBefore, CarryAfter                            [4]int
			StoresBefore, StoresAfter                          [20]int
			Trace                                              struct{ Random []int }
		}
	}
	read("native-food-sleep-theft.json", &theft)
	for _, row := range theft.Rows {
		s := NewState()
		s.StatusReturn = ScreenGround
		s.GroundMode = GroundOnFoot
		if row.LocationMode != 3 && row.LocationMode != 5 {
			s.StatusReturn = ScreenShipboard
		}
		if !row.Bit13 {
			s.GroundMode = GroundInPod
		}
		s.SleepTheftProtected = row.Flag57
		s.Cash = row.CashBefore
		s.Parts = append([]int(nil), row.CarryBefore[:]...)
		s.Stores = append([]int(nil), row.StoresBefore[:]...)
		at := 0
		sleepTheft(&s, row.Minutes, row.Risk, func() int {
			if at >= len(row.Trace.Random) {
				t.Fatal("extra RNG draw")
			}
			v := row.Trace.Random[at]
			at++
			return v
		})
		check(&report.Theft, s.Cash == row.CashAfter && reflect.DeepEqual(s.Parts, row.CarryAfter[:]) && reflect.DeepEqual(s.Stores, row.StoresAfter[:]) && at == len(row.Trace.Random), "theft")
	}
	data, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "remake-food-sleep.json"), append(data, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	t.Logf("formal sleep: %+v", report)
	if report.Wake.Differences+report.Ship.Differences+report.Menu.Differences+report.Theft.Differences != 0 {
		t.Fatal("formal sleep differences")
	}
}
