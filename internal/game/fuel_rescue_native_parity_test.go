package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Original answers come from Talos executing XPILOTAG, MAINLIB and SHIPLIB.
// The producer comparison controls only the range-draw remainder; it does not
// assume the remake RNG and the original two-word RNG have identical streams.
type rescueNativeGate struct {
	Pending                bool
	Planet, City, HomeCity int
	Allowed                bool
}

type rescueNative struct {
	Gates, SpaceGates []rescueNativeGate
	Rows              []struct {
		Input struct {
			Time, Planet, City int
			Seed               [2]uint16
		}
		Due   int
		Trace struct{ RawRandom []int }
	}
}

type rescueComparison struct {
	GateCases, SpaceGateCases, ProducerCases int
	Differences                              []string
}

func compareRescueProducer(t *testing.T, n rescueNative) rescueComparison {
	t.Helper()
	x := rescueComparison{GateCases: len(n.Gates), SpaceGateCases: len(n.SpaceGates), ProducerCases: len(n.Rows)}
	if x.GateCases != 48 || x.SpaceGateCases != 6 || x.ProducerCases != 112 {
		t.Fatal("original rescue coverage drift")
	}
	for _, row := range append(append([]rescueNativeGate{}, n.Gates...), n.SpaceGates...) {
		if fuelRescueAllowed(row.Pending, row.City != 0, row.City == row.HomeCity) != row.Allowed {
			x.Differences = append(x.Differences, "gate")
		}
	}
	for index, row := range n.Rows {
		s := NewState()
		s.TimeMinutes, s.Fuel, s.ShipRefresh = row.Input.Time, (index%2)*255, true
		before := s
		calls := 0
		requestFuel(&s, row.Input.City != 0, row.Input.Planet != 0, func(span int) int {
			calls++
			if span != 51 || len(row.Trace.RawRandom) != 1 {
				t.Fatal("unmatched original range draw")
			}
			return row.Trace.RawRandom[0] % span
		})
		wantCalls := 0
		if row.Input.Planet == 0 {
			wantCalls = 1
		}
		if !s.FuelRescuePending || s.FuelRescueDue != row.Due || s.ShipRefresh || calls != wantCalls {
			x.Differences = append(x.Differences, "producer")
		}
		// Check every unrelated typed field, including fuel, main clock, ship
		// controls, money, cargo, RNG counters and background clock phase.
		s.FuelRescuePending, s.FuelRescueDue, s.ShipRefresh = before.FuelRescuePending, before.FuelRescueDue, before.ShipRefresh
		if !reflect.DeepEqual(s, before) {
			x.Differences = append(x.Differences, "unrelated-state")
		}
	}
	return x
}

func TestExportFuelRescue(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("original export disabled")
	}
	raw, err := os.ReadFile(filepath.Join(out, "native-fuel-rescue.json"))
	if err != nil {
		t.Fatal(err)
	}
	var n rescueNative
	if err = json.Unmarshal(raw, &n); err != nil {
		t.Fatal(err)
	}
	x := compareRescueProducer(t, n)
	raw, err = json.MarshalIndent(x, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-fuel-rescue.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if len(x.Differences) != 0 {
		t.Fatalf("rescue differences: %v", x.Differences)
	}
}

func TestFuelRescueNativeParity(t *testing.T) {
	raw, err := os.ReadFile("../../docs/atari-st/fuel-rescue-parity-20261005.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct{ Native rescueNative }
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if x := compareRescueProducer(t, receipt.Native); len(x.Differences) != 0 {
		t.Fatalf("rescue differences: %v", x.Differences)
	}
}

type rescueResumeInput struct {
	ID                                     string
	Time, Angle, Radius, ToAngle, ToRadius int
}
type rescueResumeRow struct {
	Input                                                 rescueResumeInput
	TimeAfter, Fuel, Tick, Ticks, DeltaAngle, DeltaRadius int
	Travel                                                bool
}

func TestExportRescueResume(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("original export disabled")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := r.World.Destination("shoothan")
	if !ok {
		t.Fatal("missing destination")
	}
	p, ok := r.World.Planet(d.PlanetID)
	if !ok {
		t.Fatal("missing planet")
	}
	rows := []rescueResumeRow{}
	for _, time := range []int{6436, 6500, 65530, 65545} {
		for _, from := range [][2]int{{119, 13}, {0, 20}, {128, 12}, {255, 1}} {
			s := NewState()
			s.Screen, s.FlightTargetID, s.Fuel, s.TimeMinutes = ScreenFlight, d.ID, 255, time
			s.ShipControls.Subc = false
			s.FlightAngle, s.FlightRadius, s.FlightTick, s.FlightTicks = from[0], from[1], 63, 69
			orbit := r.PlanetOrbit(s, p)
			a, err := r.Step(s, ActionFlightResume)
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, rescueResumeRow{rescueResumeInput{d.ID, time, from[0], from[1], orbit.Angle, orbit.Radius}, a.TimeMinutes, a.Fuel, a.FlightTick, a.FlightTicks, a.FlightDeltaAngle, a.FlightDeltaRadius, a.ShipControls.Subc})
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-rescue-resume.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRescueResumeNativeParity(t *testing.T) {
	raw, err := os.ReadFile("../../docs/atari-st/fuel-rescue-parity-20261005.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		NativeResume []struct {
			Input                                           rescueResumeInput
			TimeAfter, Tick, Ticks, DeltaAngle, DeltaRadius int
			Travel                                          bool
		}
	}
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.NativeResume) != 16 {
		t.Fatal("original resume coverage")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range receipt.NativeResume {
		i := n.Input
		s := NewState()
		s.Screen, s.FlightTargetID, s.Fuel, s.TimeMinutes = ScreenFlight, i.ID, 255, i.Time
		s.ShipControls.Subc = false
		s.FlightAngle, s.FlightRadius, s.FlightTick, s.FlightTicks = i.Angle, i.Radius, 63, 69
		d, ok := r.World.Destination(i.ID)
		if !ok {
			t.Fatal("destination")
		}
		p, ok := r.World.Planet(d.PlanetID)
		if !ok {
			t.Fatal("planet")
		}
		s.PlanetAngles[p.Index] = i.ToAngle
		a, err := r.Step(s, ActionFlightResume)
		if err != nil {
			t.Fatal(err)
		}
		if a.TimeMinutes != n.TimeAfter || a.FlightTick != n.Tick || a.FlightTicks != n.Ticks || a.FlightDeltaAngle != n.DeltaAngle || a.FlightDeltaRadius != n.DeltaRadius || a.ShipControls.Subc != n.Travel || a.Fuel != 255 {
			t.Fatalf("native resume mismatch %+v", i)
		}
	}
}

func TestFuelRescuePlayerActions(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range r.World.Destinations() {
		p, ok := r.World.Planet(d.PlanetID)
		if !ok {
			t.Fatal("missing planet")
		}
		for _, fuel := range []int{0, 255} {
			s := NewState()
			s.Screen, s.ShipCityID, s.Fuel, s.TimeMinutes = ScreenShipboard, d.CityID, fuel, 6436
			a, err := r.Step(s, ActionDistressCall)
			if err != nil {
				t.Fatal(err)
			}
			allowed := d.SourceIndex != p.CityStart+1
			if a.FuelRescuePending != allowed || a.Fuel != fuel || a.TimeMinutes != s.TimeMinutes || a.CombatRolls != s.CombatRolls || allowed && a.FuelRescueDue != 6451 {
				t.Fatalf("source city gate %s fuel%d", d.ID, fuel)
			}
		}
	}
	s := NewState()
	s.Screen, s.FlightTargetID, s.Fuel, s.TimeMinutes = ScreenFlight, "shoothan", 0, 6436
	s.ShipControls.Subc = false
	s.FlightAngle, s.FlightRadius, s.FlightTick, s.FlightTicks = 119, 13, 63, 69
	a, err := r.Step(s, ActionDistressCall)
	if err != nil {
		t.Fatal(err)
	}
	if !a.FuelRescuePending || a.FuelRescueDue < 6496 || a.FuelRescueDue > 6546 || a.CombatRolls != s.CombatRolls+1 || a.Fuel != 0 || a.TimeMinutes != s.TimeMinutes {
		t.Fatal("space producer")
	}
	// JSON restores the pending action and its exact deadline, without reroll.
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	b, err := r.Step(restored, ActionDistressCall)
	if err != nil {
		t.Fatal(err)
	}
	if b.FuelRescueDue != a.FuelRescueDue || b.CombatRolls != a.CombatRolls || !b.FuelRescuePending {
		t.Fatal("repeat rerolled pending deadline")
	}
	c, err := r.Step(b, ActionFlightResume)
	if err != nil {
		t.Fatal(err)
	}
	if c.ShipControls.Subc || c.FlightTick != b.FlightTick {
		t.Fatal("empty fuel resumed")
	}
	// Existing source-verified consumer: one update before Due, then at Due.
	c.TimeMinutes = c.FuelRescueDue - 1
	updateShipEnergy(&c, func(n int) int { return combatRoll(&c, n) })
	if c.Fuel != 0 || !c.FuelRescuePending || c.ShipControls.Subc {
		t.Fatal("early rescue")
	}
	c.TimeMinutes++
	updateShipEnergy(&c, func(n int) int { return combatRoll(&c, n) })
	if c.Fuel != 255 || c.FuelRescuePending || c.ShipControls.Subc {
		t.Fatal("expiry changed engine control")
	}
	d, err := r.Step(c, ActionFlightResume)
	if err != nil {
		t.Fatal(err)
	}
	if !d.ShipControls.Subc || d.FlightTick != 0 || d.Fuel != c.Fuel || d.TimeMinutes != c.TimeMinutes || d.ShipPhase != c.ShipPhase || d.FlightFromAngle != 119 || d.FlightFromRadius != 13 {
		t.Fatal("resume repeated world/warp charges or lost current polar position")
	}
}
