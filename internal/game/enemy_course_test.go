package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func enemyCourseFixture() State {
	s := consumerState(true, [2]int{1, 1})
	s.Screen = ScreenTactical
	s.ShipControls = ShipControls{Warp: true, Subc: true, Guns: true, Tactical: true, Pilotage: true}
	s.ShieldsActive = true
	s.ShipSystems = IntactShipSystems()
	for n := range s.ShipSystems {
		if n == 2 || n == 4 {
			s.ShipSystems[n].Rows[2] = make([]int, 4)
			s.ShipSystems[n].Rows[3] = make([]int, 4)
		}
		s.ShipSystems[n].LastHitSlot = 0
	}
	normalizeShipSystems(&s)
	s.Fuel, s.ShipPhase, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = 0, 0, 0, 255, 255
	s.HullCondition, s.FlightTicks = 16, 1000
	s.FlightFromAngle, s.FlightFromRadius, s.FlightAngle, s.FlightRadius = 250, 20, 250, 20
	s.FlightDeltaAngle, s.FlightDeltaRadius = 0, 0
	s.EnemyType, s.EnemyCourseMode, s.EnemyFireCountdown = 0, 1, 1
	s.CombatRange, s.EnemyHull, s.EnemyMaxHull = 70, 200, 200
	s.EnemyBearing, s.EnemyPitch, s.EnemyDriftBearing, s.EnemyDriftPitch = 512, 512, 3, -3
	s.EnemyClosing, s.CombatActive, s.CombatRolls = -1, true, 0
	s.EnemyCourse = EnemyCourseState{Ticks: 1, CanPrepareFire: true, Maneuver: [2]int{2, 1}}
	return s
}

func enemyCourseCore(s State) any {
	return struct {
		Phase, Fuel, Weapon, Shield, Setting, Hull, Counter, Timer, Range, Bearing, Pitch, BearingDrift, PitchDrift, Closing int
		Active                                                                                                               bool
		Course                                                                                                               EnemyCourseState
		Holds                                                                                                                [2]Hold
		Systems                                                                                                              []ShipSystem
	}{s.ShipPhase, s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting, s.HullCondition, s.CombatRolls, s.EnemyFireCountdown,
		s.CombatRange, s.EnemyBearing, s.EnemyPitch, s.EnemyDriftBearing, s.EnemyDriftPitch, s.EnemyClosing, s.CombatActive,
		s.EnemyCourse, s.Holds, s.ShipSystems}
}

func TestCombatParentPreparationDoesNotFireImmediately(t *testing.T) {
	s := enemyCourseFixture()
	advanceShipClock(&s)
	// First original controlled parent vector: mode1, phase0, fuel0, not ready.
	if s.EnemyFireCountdown != -8 || s.EnemyCourse.FireReady || s.EnemyCourse.Ticks != 0 || s.CombatRange != 69 || s.CombatRolls != 2 || s.ShieldLevel != 255 || s.Fuel != 254 {
		t.Fatalf("source parent preparation vector changed: %+v", enemyCourseCore(s))
	}
}

func TestManualAwakeAndSleepShareOneCombatParent(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	base := enemyCourseFixture()
	want := enemyCourseFixture()
	advanceShipClock(&want)
	manual, err := r.Step(base, ActionRechargeWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(enemyCourseCore(manual), enemyCourseCore(want)) {
		t.Fatal("manual tactical step bypassed or repeated the parent")
	}
	awake := enemyCourseFixture()
	for i := 0; i < 1000 && awake.ShipPhase == base.ShipPhase; i++ {
		advanceAwakeVBL(&awake)
	}
	if !reflect.DeepEqual(enemyCourseCore(awake), enemyCourseCore(want)) {
		t.Fatalf("awake callback differs: %+v", enemyCourseCore(awake))
	}
	sleep := enemyCourseFixture()
	sleep.Screen, sleep.StatusReturn = ScreenCharacterStatus, ScreenTactical
	sleep.Sleep = SleepState{Active: true, ShipActive: true}
	sleep.SleepTheftProtected = true
	for i := 0; i < 32; i++ {
		r.advanceSleep(&sleep)
	}
	if !reflect.DeepEqual(enemyCourseCore(sleep), enemyCourseCore(want)) {
		t.Fatalf("sleep callback repeated or skipped the enemy update: %+v", enemyCourseCore(sleep))
	}
}

func TestPreparedShotIsConsumedAfterCloak(t *testing.T) {
	s := enemyCourseFixture()
	s.EnemyCourse.FireReady, s.EnemyCourse.CanPrepareFire = true, false
	s.ShipControls.Secondary[4] = true
	advanceShipClock(&s)
	if s.EnemyCourse.FireReady || s.EnemyFireCountdown < 5 || s.EnemyFireCountdown > 10 || s.CombatRolls < 3 {
		t.Fatal("prepared shot was blocked by later cloak/eligibility state")
	}
}

func TestCurrentEnemyCourseNativeParity(t *testing.T) {
	in := os.Getenv("SUNDOG_COMBAT_INIT_INPUT")
	if in == "" {
		t.Skip("native input supplied only for original parity")
	}
	type input struct {
		Mode, Range, BearingDrift, PitchDrift, Aux8, Aux9, Closing, Phase, Hull, Seed, HelperLocal2, HelperLocal3 int
		Flag12                                                                                                    bool
	}
	// Display-only flags29/46 do not map to portable game state.
	type row struct {
		Input                                                         input
		BearingDrift, PitchDrift, Aux8, Aux9, Closing, Timer, Counter int
		Flag44                                                        bool
	}
	var native struct{ Native []row }
	raw, err := os.ReadFile(filepath.Join(in, "native-course.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Native) != 6336 {
		t.Fatal("native6336 required")
	}
	excluded := 0
	for _, n := range native.Native {
		i := n.Input
		s := State{EnemyCourseMode: i.Mode, CombatRange: i.Range, EnemyDriftBearing: i.BearingDrift, EnemyDriftPitch: i.PitchDrift,
			EnemyClosing: i.Closing, ShipPhase: i.Phase, EnemyHull: i.Hull, CombatRolls: i.Seed,
			EnemyCourse: EnemyCourseState{Maneuver: [2]int{i.Aux8, i.Aux9}, CanPrepareFire: true}}
		s.ShipControls.Secondary[4] = i.Flag12
		before := s
		if !selectEnemyCourse(&s, &s.EnemyCourse) {
			if i.Mode != 0 || !i.Flag12 || i.Phase != 0 || !reflect.DeepEqual(before, s) {
				t.Fatal("unproven boundary changed state", i)
			}
			excluded++
			continue
		}
		x := row{Input: i, BearingDrift: s.EnemyDriftBearing, PitchDrift: s.EnemyDriftPitch, Aux8: s.EnemyCourse.Maneuver[0], Aux9: s.EnemyCourse.Maneuver[1],
			Closing: s.EnemyClosing, Timer: s.EnemyCourse.Ticks, Counter: s.CombatRolls, Flag44: s.EnemyCourse.CanPrepareFire}
		if !reflect.DeepEqual(x, n) {
			t.Fatalf("current course differs input%+v native%+v actual%+v", i, n, x)
		}
	}
	if excluded != 192 {
		t.Fatalf("expected exactly192 undefined helper cases, got%d", excluded)
	}
}

func TestCurrentEncounterInitNativeParity(t *testing.T) {
	in := os.Getenv("SUNDOG_COMBAT_INIT_INPUT")
	if in == "" {
		t.Skip("native input supplied only for original parity run")
	}
	type input struct{ System, Rating, Phase, BearingDrift, PitchDrift, Aux8, Aux9, Seed int }
	type row struct {
		Input                                                                                                                   input
		Enemy, Mode, Range, Hull, Bearing, Pitch, BearingDrift, PitchDrift, Aux8, Aux9, Closing, CourseTimer, Cooldown, Counter int
		Active, Ready, Flag44                                                                                                   bool
	}
	var receipt struct{ Native []row }
	raw, err := os.ReadFile(filepath.Join(in, "native-encounter-init.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Native) != 2304 {
		t.Fatal("native2304 required")
	}
	for _, n := range receipt.Native {
		i := n.Input
		s := State{ShipPhase: i.Phase, EnemyDriftBearing: i.BearingDrift, EnemyDriftPitch: i.PitchDrift, CombatRolls: i.Seed,
			EnemyCourse: EnemyCourseState{Maneuver: [2]int{i.Aux8, i.Aux9}}}
		beginEncounter(&s, i.Rating)
		x := row{Input: i, Enemy: s.EnemyType, Mode: s.EnemyCourseMode, Range: s.CombatRange, Hull: s.EnemyHull, Bearing: s.EnemyBearing, Pitch: s.EnemyPitch,
			BearingDrift: s.EnemyDriftBearing, PitchDrift: s.EnemyDriftPitch, Aux8: s.EnemyCourse.Maneuver[0], Aux9: s.EnemyCourse.Maneuver[1], Closing: s.EnemyClosing,
			CourseTimer: s.EnemyCourse.Ticks, Cooldown: s.EnemyFireCountdown, Counter: s.CombatRolls, Active: s.CombatActive, Ready: s.EnemyCourse.FireReady, Flag44: s.EnemyCourse.CanPrepareFire}
		if !reflect.DeepEqual(x, n) {
			t.Fatalf("current init differs input%+v native%+v actual%+v", i, n, x)
		}
	}
}
