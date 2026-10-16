package game

import (
	"reflect"
	"testing"
)

func sleepStep(t *testing.T, r Reducer, s State, a Action) State {
	t.Helper()
	next, err := r.Step(s, a)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestNormalActionEntersForcedSleepAndPreservesReturnView(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, scene := range []Screen{ScreenGround, ScreenTactical} {
		s := NewState()
		s.Screen = scene
		s.GroundWorld = true
		s.TimeMinutes, s.Rest = 59, 1
		action := ActionGroundClockTick
		if scene == ScreenTactical {
			action = ActionTacticalMove
		}
		next := s
		if scene == ScreenGround {
			for i := 0; i < 10; i++ {
				next = sleepStep(t, r, next, ActionVBLTick)
				next = sleepStep(t, r, next, ActionVBLTick)
				next = sleepStep(t, r, next, action)
			}
		} else {
			next = sleepStep(t, r, s, action)
		}
		if !next.Sleep.Active || !next.Sleep.Forced || next.Rest != 0 || next.Screen != ScreenCharacterStatus || next.StatusReturn != scene || next.TacticalSlewing {
			t.Fatalf("normal interrupt scene%s: %+v", scene, next.Sleep)
		}
		if next.Sleep.MinuteCycles != 0 || next.Sleep.ShipCycles != 0 || next.Sleep.ElapsedMinutes != 0 {
			t.Fatal("interrupt granted sleep time")
		}
		if err = ValidateSleepState(next); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3060; i++ {
			next = sleepStep(t, r, next, ActionWake)
			next = sleepStep(t, r, next, ActionSleepTick)
		}
		if next.Sleep.Active || next.Rest != 51 {
			t.Fatal("forced normal action did not wake at51")
		}
		next = sleepStep(t, r, next, ActionBack)
		if next.Screen != scene {
			t.Fatal("interrupt lost return view")
		}
	}
}

func TestAwakeClockFinishesBulkIncrementBeforeSleep(t *testing.T) {
	s := NewState()
	s.Screen = ScreenShipboard
	s.TimeMinutes, s.Rest = 59, 5
	advanceTime(&s, 121)
	if s.TimeMinutes != 180 || s.Rest != 0 || s.Vigor != 243 || s.Nourishment != 246 || !s.Sleep.Forced || s.Sleep.ElapsedMinutes != 0 {
		t.Fatalf("bulk clock stopped early or applied sleep during increment: %+v", s.Sleep)
	}
	s = NewState()
	s.Screen = ScreenGround
	s.Rest = 0
	advanceTime(&s, 59)
	if s.Sleep.Active {
		t.Fatal("Rest0 interrupted before an hour callback")
	}
}

func TestWorldSleepDoesNotStealCityItems(t *testing.T) {
	s := NewState()
	s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenGround
	s.GroundWorld = true
	s.GroundMode = GroundOnFoot
	s.Parts = []int{8, 1, 2, 3}
	cash, parts := s.Cash, append([]int(nil), s.Parts...)
	sleepTheft(&s, 65, 0, func() int { t.Fatal("world4 entered city3/interior5 theft"); return 0 })
	if s.Cash != cash || !reflect.DeepEqual(s.Parts, parts) {
		t.Fatal("world sleep lost carried items")
	}
}

func TestSleepIsInterruptibleWithoutImmediateCostOrGain(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenShipboard
	s.Vigor, s.Rest, s.Health, s.Nourishment = 100, 20, 0, 0
	next := sleepStep(t, r, s, ActionRest)
	if !next.Sleep.Active || next.Rest != 20 || next.Vigor != 100 || next.Health != 0 || next.Nourishment != 0 || next.TimeMinutes != 0 {
		t.Fatal("sleep applied immediate cost/gain or rejected nourishment zero")
	}
	next = sleepStep(t, r, next, ActionWake)
	for i := 0; i < 31; i++ {
		next = sleepStep(t, r, next, ActionSleepTick)
	}
	if !next.Sleep.Active {
		t.Fatal("woke before callback")
	}
	next = sleepStep(t, r, next, ActionSleepTick)
	if next.Sleep.Active || next.Screen != ScreenCharacterStatus || next.Rest != 20 || next.TimeMinutes != 0 || next.ShipPhase != 1 {
		t.Fatal("minute-free wake did not preserve readings or advance ship")
	}
}

func TestSleepClockUsesPriorRestAndForcesThroughFifty(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenGround
	s.GroundMode = GroundInPod
	s.TimeMinutes, s.Rest = 59, 0
	s = sleepStep(t, r, s, ActionRest)
	s = sleepStep(t, r, s, ActionWake)
	for i := 0; i < 60*50; i++ {
		s = sleepStep(t, r, s, ActionWake)
		s = sleepStep(t, r, s, ActionSleepTick)
	}
	if s.Sleep.WakeRequested {
		t.Fatal("forced sleep retained an ignored wake input")
	}
	if !s.Sleep.Active || !s.Sleep.Forced || s.Rest != 50 || s.Vigor != 251 {
		t.Fatalf("forced boundary: rest%d vigor%d sleep%+v", s.Rest, s.Vigor, s.Sleep)
	}
	for i := 0; i < 60; i++ {
		s = sleepStep(t, r, s, ActionWake)
		s = sleepStep(t, r, s, ActionSleepTick)
	}
	if s.Sleep.Active || s.Rest != 51 || s.TimeMinutes != 110 {
		t.Fatal("held wake did not unlock at Rest51")
	}
}

func TestSleepDeathClearsPendingState(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenGround
	s.TimeMinutes, s.Vigor, s.Rest, s.Health, s.Nourishment = 59, 1, 0, 0, 0
	s = sleepStep(t, r, s, ActionRest)
	for i := 0; i < 60; i++ {
		s = sleepStep(t, r, s, ActionSleepTick)
	}
	if s.Screen != ScreenOutcome || s.Vigor != 0 || s.Rest != 1 || s.Sleep != (SleepState{}) {
		t.Fatal("death did not terminate sleep after minute writer")
	}
}

func TestSleepLibraryPausesAndOtherActionsAreIgnored(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenShipboard
	s = sleepStep(t, r, s, ActionRest)
	s = sleepStep(t, r, s, ActionSleepTick)
	before := s.Sleep
	s = sleepStep(t, r, s, ActionEat)
	if s.Sleep != before {
		t.Fatal("sleep admitted gameplay action")
	}
	s = sleepStep(t, r, s, ActionOpenLibrary)
	s = sleepStep(t, r, s, ActionSleepTick)
	if s.Sleep != before {
		t.Fatal("library advanced sleep")
	}
	s = sleepStep(t, r, s, ActionBack)
	if s.Screen != ScreenCharacterStatus || s.Sleep != before {
		t.Fatal("library back reset sleep")
	}
}

func TestSleepTheftCopiesBodyAndPreservesHandAndStores(t *testing.T) {
	s := NewState()
	s.StatusReturn = ScreenGround
	s.GroundMode = GroundOnFoot
	s.Parts = []int{0, 44, 8, 255}
	s.HandItem = 8
	before := s
	before.Parts = append([]int(nil), s.Parts...)
	draws := []int{0, 0, 0}
	at := 0
	sleepTheft(&s, 1, 0, func() int { v := draws[at]; at++; return v })
	if s.Parts[1] != 0 || before.Parts[1] != 44 || s.HandItem != 8 || !reflect.DeepEqual(s.Stores, before.Stores) {
		t.Fatal("theft changed source/body anchor/hand/Stores")
	}
}

func TestSleepProtectionUsesEntryWritersAndPreservesGroundMode(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []GroundMode{GroundOnFoot, GroundInPod} {
		s := NewState()
		s.Screen = ScreenShipboard
		s.GroundMode = mode
		s.SleepTheftProtected = true
		ground := sleepStep(t, r, s, ActionOpenGround)
		if ground.SleepTheftProtected || ground.GroundMode != mode || ground.CargoAttached != s.CargoAttached {
			t.Fatal("airlock changed mode/attachment or retained protection")
		}
	}
	s := NewState()
	s.Screen = ScreenCargo
	s.SleepTheftProtected = true
	ground := sleepStep(t, r, s, ActionExitCargoPod)
	if !ground.SleepTheftProtected || ground.GroundMode != GroundInPod || ground.CargoAttached {
		t.Fatal("cargo exit overwrote protection")
	}
	ground.CityID, ground.Position = ground.ShipCityID, ground.ShipPosition
	ground.SleepTheftProtected = false
	if !BoardGroundShip(&ground, Position{23, 20}, true) || !ground.SleepTheftProtected || !ground.CargoAttached {
		t.Fatal("successful boarding did not set protection")
	}
}
