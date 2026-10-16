package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Export the production Navigate entry, including the remaining atomic
// world-leg approximation. The oracle comparison isolates the mode2 ETA
// prepayment; it does not certify those retained world/warp timings.
func TestExportTalosFlightSetupClock(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("原版對拍匯出未啟用")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	type input struct {
		ID                                     string
		Time, Angle, Radius, ToAngle, ToRadius int
	}
	type row struct {
		Input                                                            input
		TimeAfter, AtomicMinutes, ArcETA, Ticks, DeltaAngle, DeltaRadius int
		Screen                                                           Screen
	}
	rows := []row{}
	base := NewState()
	base.Screen = ScreenNavigation
	base.Fuel = 255
	scenarios := 0
	for _, d := range r.World.Destinations() {
		if d.SystemID != base.SystemID || d.PlanetID == base.PlanetID || !PrimaryCity(d.CityID) {
			continue
		}
		p, ok := r.World.Planet(d.PlanetID)
		if !ok {
			t.Fatal("missing planet")
		}
		for _, fromAngle := range []int{3, 64, 128, 250} {
			for _, initialTime := range []int{0, 39, 59, 60, 1439, 2879} {
				s := base
				s.FlightAngle, s.FlightRadius = fromAngle, 20
				s.TimeMinutes = initialTime
				s.NavigationTargetID = d.ID
				orbit := r.PlanetOrbit(s, p)
				legs := routeTicks(s, orbit, false)
				cost, ok := r.RouteCost(s)
				if !ok {
					t.Fatal("missing route")
				}
				arcETA := legs.ArcTicks + legs.ArcTicks/flightEtaDivisor
				after, err := r.Step(s, ActionNavigate)
				if err != nil {
					t.Fatal(err)
				}
				if after.Screen != ScreenFlight {
					t.Fatalf("navigation refused %s: %s", d.ID, after.MessageKey)
				}
				rows = append(rows, row{Input: input{d.ID, initialTime, s.FlightAngle, s.FlightRadius, orbit.Angle, orbit.Radius}, TimeAfter: after.TimeMinutes,
					AtomicMinutes: cost.TotalMinutes - arcETA, ArcETA: arcETA, Ticks: after.FlightTicks, DeltaAngle: after.FlightDeltaAngle,
					DeltaRadius: after.FlightDeltaRadius, Screen: after.Screen})
			}
		}
		scenarios = 4
		break
	}
	if scenarios != 4 {
		t.Fatal("need a primary same-system destination on another planet")
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-flight-setup.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式Navigate初始化%d案", len(rows))
}

// Keep the original answers in the ordinary suite so reintroducing ETA
// prepayment fails without requiring a local original disk or Talos install.
func TestFlightSetupClockNativeParity(t *testing.T) {
	data, err := os.ReadFile("../../docs/atari-st/flight-setup-clock-parity-20261004.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Native struct {
			Rows []struct {
				Input struct {
					ID                                     string
					Time, Angle, Radius, ToAngle, ToRadius int
				}
				Period, TimeAfter, ETA, Ticks, DeltaAngle, DeltaRadius int
			}
		}
	}
	if err = json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Native.Rows) != 72 {
		t.Fatal("missing original72 input coverage")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, row := range receipt.Native.Rows {
		if row.Period != 32 {
			continue
		}
		i := row.Input
		s := NewState()
		s.Screen, s.NavigationTargetID, s.TimeMinutes = ScreenNavigation, i.ID, i.Time
		s.Fuel, s.FlightAngle, s.FlightRadius = 255, i.Angle, i.Radius
		d, ok := r.World.Destination(i.ID)
		if !ok {
			t.Fatal("missing source destination")
		}
		p, ok := r.World.Planet(d.PlanetID)
		if !ok {
			t.Fatal("missing source planet")
		}
		// Each oracle row records its controlled target angle. The exporter
		// shares the base orbit slice across cases, so earlier clock advances
		// can change that angle; replay the recorded geometry explicitly.
		s.PlanetAngles[p.Index] = i.ToAngle
		orbit := r.PlanetOrbit(s, p)
		if orbit.Angle != i.ToAngle || orbit.Radius != i.ToRadius {
			t.Fatal("source geometry drift")
		}
		cost, ok := r.RouteCost(s)
		if !ok || cost.TotalMinutes != 2*planetaryLandingTicks+row.ETA-row.TimeAfter {
			t.Fatal("ETA display no longer matches original record")
		}
		a, err := r.Step(s, ActionNavigate)
		if err != nil {
			t.Fatal(err)
		}
		// The two atomic world legs remain a documented approximation. The
		// original mode2 entry itself leaves its main clock unchanged.
		want := row.TimeAfter + 2*planetaryLandingTicks
		if a.TimeMinutes != want || a.FlightTicks != row.Ticks || a.FlightDeltaAngle != row.DeltaAngle || a.FlightDeltaRadius != row.DeltaRadius || a.Screen != ScreenFlight {
			t.Fatalf("native mode2 setup %+v: actual clock/ticks/deltas=%d/%d/%d/%d; native clock plus retained world minutes/ticks/deltas=%d/%d/%d/%d", i, a.TimeMinutes, a.FlightTicks, a.FlightDeltaAngle, a.FlightDeltaRadius, want, row.Ticks, row.DeltaAngle, row.DeltaRadius)
		}
		checked++
	}
	if checked != 24 {
		t.Fatal("missing formal24 input coverage")
	}
}

type flightArrivalInput struct {
	Phase, Fuel, Tick, Ticks, Status, Seed int
	Angle, Radius, DeltaAngle, DeltaRadius int
	Travel                                 bool
}

type flightArrivalNative struct {
	Input                                                    flightArrivalInput
	Phase, Fuel, Tick, Mode, Angle, Radius, Counter, Updates int
	Travel                                                   bool
	Ship, Location                                           []uint16
}

// Replay each controlled original movement input through production background
// callbacks. The intact engine makes the compared movement independent of RND;
// the two RNG sequences are recorded, not claimed to be identical algorithms.
func replayFlightArrival(t *testing.T, r Reducer, i flightArrivalInput, scene string) (State, int) {
	t.Helper()
	var err error
	s := NewState()
	s.Screen, s.TimeMinutes = ScreenFlight, 120
	s.FlightTargetID = "lightning"
	s.FlightTicks, s.FlightTick = i.Ticks, i.Tick
	s.FlightFromAngle, s.FlightFromRadius = i.Angle, i.Radius
	s.FlightDeltaAngle, s.FlightDeltaRadius = i.DeltaAngle, i.DeltaRadius
	s.FlightAngle = (i.Angle + i.Tick*i.DeltaAngle/i.Ticks + 256) % 256
	s.FlightRadius = i.Radius + i.Tick*i.DeltaRadius/i.Ticks
	s.Fuel, s.ShipPhase, s.CombatRolls = i.Fuel, i.Phase, i.Seed
	s.ShipSystems = []ShipSystem{{ID: ShipSystemSubcEngine, Status: i.Status}}
	s.ShieldsActive, s.ShieldLevel, s.ShieldSetting, s.WeaponCharge = false, 0, 0, 0
	s.ShipControls = ShipControls{Subc: true}
	if scene != "flight" {
		// Flight's manual status command is intentionally unavailable.
		// Replay the controlled status return used by a condition interrupt;
		// this fixture does not certify normal manual access to that view.
		s.Screen, s.StatusReturn = ScreenCharacterStatus, ScreenFlight
	}
	if scene == "sleep" {
		s, err = r.Step(s, ActionRest)
		if err != nil {
			t.Fatal(err)
		}
	}
	updates := 0
	for vbl := 0; vbl < 4000 && s.FlightTick < i.Ticks; vbl++ {
		before := s.ShipPhase
		action := ActionVBLTick
		if scene == "sleep" {
			action = ActionSleepTick
		}
		s, err = r.Step(s, action)
		if err != nil {
			t.Fatal(err)
		}
		if s.ShipPhase != before {
			updates++
		}
	}
	return s, updates
}

func TestExportTalosFlightArrival(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("原版對拍匯出未啟用")
	}
	data, err := os.ReadFile(filepath.Join(out, "native-flight-arrival.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native []flightArrivalNative
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Input                                     flightArrivalInput
		Scene                                     string
		Updates, Phase, Fuel, Tick, Angle, Radius int
		Screen, StatusReturn                      Screen
		City, ShipCity, CargoCity, Target         string
		Subc, Sleeping                            bool
		Differences                               []string
	}
	rows := []row{}
	for _, n := range native {
		for _, scene := range []string{"flight", "status", "sleep"} {
			i := n.Input
			s, updates := replayFlightArrival(t, r, i, scene)
			x := row{Input: i, Scene: scene, Updates: updates, Phase: s.ShipPhase, Fuel: s.Fuel, Tick: s.FlightTick,
				Angle: s.FlightAngle, Radius: s.FlightRadius, Screen: s.Screen, StatusReturn: s.StatusReturn,
				City: s.CityID, ShipCity: s.ShipCityID, CargoCity: s.CargoCityID, Target: s.FlightTargetID, Subc: s.ShipControls.Subc, Sleeping: s.Sleep.Active}
			if updates != n.Updates || s.ShipPhase != n.Phase || s.Fuel != n.Fuel || s.FlightTick != n.Tick || s.FlightAngle != n.Angle || s.FlightRadius != n.Radius || s.ShipControls.Subc {
				x.Differences = append(x.Differences, "movement")
			}
			if s.CityID != "lightning" || s.ShipCityID != "lightning" || s.CargoCityID != "lightning" || s.FlightTargetID != "" {
				x.Differences = append(x.Differences, "destination")
			}
			if scene == "flight" && s.Screen != ScreenShipboard || scene != "flight" && (s.Screen != ScreenCharacterStatus || s.StatusReturn != ScreenShipboard) || scene == "sleep" && !s.Sleep.Active {
				x.Differences = append(x.Differences, "foreground")
			}
			rows = append(rows, x)
		}
	}
	data, err = json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-flight-arrival.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式背景抵達%d案", len(rows))
}

func TestFlightArrivalNativeParity(t *testing.T) {
	data, err := os.ReadFile("../../docs/atari-st/flight-arrival-parity-20261004.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct{ Native []flightArrivalNative }
	if err = json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Native) != 36 {
		t.Fatal("missing native36 arrival inputs")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range receipt.Native {
		for _, scene := range []string{"flight", "status", "sleep"} {
			s, updates := replayFlightArrival(t, r, n.Input, scene)
			if updates != n.Updates || s.ShipPhase != n.Phase || s.Fuel != n.Fuel || s.FlightTick != n.Tick || s.FlightAngle != n.Angle || s.FlightRadius != n.Radius || s.ShipControls.Subc {
				t.Fatalf("%s: native movement mismatch: input%+v", scene, n.Input)
			}
			if s.CityID != "lightning" || s.ShipCityID != "lightning" || s.CargoCityID != "lightning" || s.FlightTargetID != "" || s.ShipPosition != s.Position || s.CargoPosition != s.Position {
				t.Fatalf("%s: destination was not committed", scene)
			}
			if scene == "flight" && s.Screen != ScreenShipboard || scene != "flight" && (s.Screen != ScreenCharacterStatus || s.StatusReturn != ScreenShipboard) || scene == "sleep" && !s.Sleep.Active {
				t.Fatalf("%s: lost foreground/return/sleep", scene)
			}
		}
	}
}

func TestExportTalosFlightCargoGate(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("原版對拍匯出未啟用")
	}
	data, err := os.ReadFile(filepath.Join(out, "native-flight-cargo-gate.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native []struct {
		Codes   [2]int
		Allowed bool
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	type row struct {
		Codes   [2]int
		Allowed bool
	}
	rows := []row{}
	for _, n := range native {
		s := NewState()
		for i, code := range n.Codes {
			switch code {
			case 0:
				s.Holds[i] = Hold{}
			case 1:
				s.Holds[i] = Hold{Kind: HoldAuxFuel, Quality: 7, SourceCode: 0xab}
			default:
				s.Holds[i] = goodsHold(Lot{Goods: code, Quality: 7, Price: 0x1357, SourceCode: 0xab}, "fixture")
			}
		}
		rows = append(rows, row{n.Codes, PodDrawsEncounter(s)})
	}
	data, err = json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-flight-cargo-gate.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExportTalosFlightShipSleep(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("原版對拍匯出未啟用")
	}
	exportTalosFlightShipSleep(t, out)
}

func exportTalosFlightShipSleep(t *testing.T, out string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "native-flight-ship-sleep.json"))
	if err != nil {
		t.Fatal(err)
	}
	type input struct {
		Phase, Ticks, Tick, WakeCycle int
		Energy                        [4]int
	}
	var native []struct {
		Input                        input
		EnergyAfter, Conditions      [4]int
		TimeAfter, Phase, Tick, Mode int
		Travel                       bool
		Background                   [8][3]int
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		Input                   input
		EnergyAfter, Conditions [4]int
		TimeAfter, Phase, Tick  int
		Travel, Sleeping        bool
		StatusReturn            Screen
		Background              [8][3]int
		Differences             []string
	}
	rows := []row{}
	for _, n := range native {
		i := n.Input
		s := NewState()
		s.Screen, s.StatusReturn, s.TimeMinutes = ScreenCharacterStatus, ScreenFlight, 39
		s.FlightTargetID = "lightning"
		s.FlightTicks, s.FlightTick = i.Ticks, i.Tick
		s.FlightFromAngle, s.FlightFromRadius, s.FlightDeltaAngle, s.FlightDeltaRadius = 250, 20, 6, 10
		s.FlightAngle, s.FlightRadius = (250+i.Tick*6/i.Ticks)%256, 20+i.Tick*10/i.Ticks
		s.ShipPhase = i.Phase
		s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = i.Energy[0], i.Energy[1], i.Energy[2], i.Energy[3]
		// Status is derived from installed rows on every reducer step. Give
		// the two half-intact systems real rows instead of stale status-only
		// records, which normalizeShipSystems replaces with intact systems.
		for j := range s.ShipSystems {
			system := &s.ShipSystems[j]
			if system.ID == ShipSystemSubcEngine || system.ID == ShipSystemShipGuns || system.ID == ShipSystemForceShields {
				system.Rows = intactRows(system.ID)
				if system.ID != ShipSystemSubcEngine {
					system.Rows[2], system.Rows[3] = make([]int, 4), make([]int, 4)
				}
			}
		}
		normalizeShipSystems(&s)
		if shipSystemStatusByID(s, ShipSystemSubcEngine) != 16 || shipSystemStatusByID(s, ShipSystemShipGuns) != 8 || shipSystemStatusByID(s, ShipSystemForceShields) != 8 {
			t.Fatal("controlled system rows do not match the original statuses")
		}
		s.ShipControls = ShipControls{Subc: true, Guns: true}
		s.ShieldsActive = true
		s, err = r.Step(s, ActionRest)
		if err != nil {
			t.Fatal(err)
		}
		for cycle := 1; cycle <= i.WakeCycle; cycle++ {
			if cycle == i.WakeCycle {
				s, err = r.Step(s, ActionWake)
				if err != nil {
					t.Fatal(err)
				}
			}
			s, err = r.Step(s, ActionSleepTick)
			if err != nil {
				t.Fatal(err)
			}
		}
		x := row{Input: i, EnergyAfter: [4]int{s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting}, Conditions: [4]int{s.Vigor, s.Rest, s.Health, s.Nourishment}, TimeAfter: s.TimeMinutes, Phase: s.ShipPhase, Tick: s.FlightTick, Travel: s.ShipControls.Subc, Sleeping: s.Sleep.Active, StatusReturn: s.StatusReturn}
		for j, v := range s.AwakeClock.Background.Slots {
			x.Background[j] = [3]int{int(v.Callback), v.Remaining, v.Period}
		}
		if x.EnergyAfter != n.EnergyAfter || x.Conditions != n.Conditions || x.TimeAfter != n.TimeAfter || x.Phase != n.Phase || x.Tick != n.Tick || x.Travel != n.Travel || x.Background != n.Background || x.Sleeping {
			x.Differences = append(x.Differences, "full-callback")
		}
		if n.Mode == 0 && (s.CityID != "lightning" || x.StatusReturn != ScreenShipboard) || n.Mode == 2 && (s.CityID != "drahew" || x.StatusReturn != ScreenFlight) {
			x.Differences = append(x.Differences, "destination-return")
		}
		rows = append(rows, x)
	}
	data, err = json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-flight-ship-sleep.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

// Replay the independent original answers in the ordinary suite. This covers
// the complete sleep callback after the arrival writer, including charging.
func TestFlightShipSleepNativeParity(t *testing.T) {
	data, err := os.ReadFile("../../docs/atari-st/flight-ship-parent-parity-20261004.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		NativeSleep []json.RawMessage
	}
	if err = json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.NativeSleep) != 72 {
		t.Fatal("missing original72 sleep inputs")
	}
	out := t.TempDir()
	data, err = json.Marshal(receipt.NativeSleep)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "native-flight-ship-sleep.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	exportTalosFlightShipSleep(t, out)
	data, err = os.ReadFile(filepath.Join(out, "remake-flight-ship-sleep.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Input       json.RawMessage
		Differences []string
	}
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 72 {
		t.Fatal("missing formal72 sleep outputs")
	}
	for _, row := range rows {
		if len(row.Differences) != 0 {
			t.Fatalf("original sleep input %s: %v", row.Input, row.Differences)
		}
	}
}
