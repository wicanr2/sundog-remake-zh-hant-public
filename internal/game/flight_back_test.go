package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

// XPILOTAG raw0x35200+04dd..04ec changes the return mode, preserving
// travel/subc and the original 14 navigation fields. See ship-rules.md.
func TestFlightBackPreservesRouteAndAllowsShipboardSleep(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"flightTargetId", "flightTick", "flightTicks", "flightAngle", "flightRadius", "flightFromAngle", "flightFromRadius", "flightDeltaAngle", "flightDeltaRadius", "shipControls", "shipPhase", "fuel", "timeMinutes", "awakeClock", "combatRolls", "holds"}
	asMap := func(s State) map[string]any {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, subc := range []bool{false, true} {
		for phase := 0; phase < 3; phase++ {
			s := NewState()
			s.Screen = ScreenFlight
			s.FlightTargetID, s.FlightTick, s.FlightTicks = "lightning", 5, 33
			s.ShipControls.Subc, s.ShipPhase = subc, phase
			if _, err := r.Step(s, ActionOpenStatus); err == nil {
				t.Fatal("Flight gained an unverified direct status shortcut")
			}
			next, err := r.Step(s, ActionBack)
			if err != nil {
				t.Fatal(err)
			}
			if next.Screen != ScreenShipboard {
				t.Fatalf("Back returned %s", next.Screen)
			}
			before, after := asMap(s), asMap(next)
			for _, k := range keys {
				if !reflect.DeepEqual(before[k], after[k]) {
					t.Errorf("Back changed %s", k)
				}
			}
			next, err = r.Step(next, ActionOpenStatus)
			if err != nil {
				t.Fatal(err)
			}
			if next.StatusReturn != ScreenShipboard {
				t.Fatal("wrong status return")
			}
			next, err = r.Step(next, ActionRest)
			if err != nil {
				t.Fatal(err)
			}
			if !next.Sleep.Active || !next.Sleep.ShipActive || next.FlightTargetID != s.FlightTargetID {
				t.Fatal("normal shipboard sleep lost its route")
			}
		}
	}
}
