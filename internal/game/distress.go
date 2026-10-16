package game

// XPILOTAG:0a/0b: city0 waits30 minutes, otherwise15; planet0 adds one
// rand(30,80). The producer writes flags and an ETA, without paying elapsed
// time or filling fuel. SHIPLIB update_levels consumes the pending deadline.
// Evidence and typed mapping: docs/atari-st/ship-rules.md, rescue READY.

const (
	distressGroundWait = 15
	distressOrbitWait  = 30
	distressExtraLow   = 30
	distressExtraHigh  = 80
)

func fuelRescueAllowed(pending, inCity, atHome bool) bool {
	return !pending && (!inCity || !atHome)
}

// CanRequestFuel maps source city/planet identities to the current ship scene.
// Flight keeps the departure city in State, so it must be treated as space.
func (r Reducer) CanRequestFuel(s State) bool {
	if s.Screen == ScreenFlight {
		return fuelRescueAllowed(s.FuelRescuePending, false, false)
	}
	if s.Screen != ScreenShipboard {
		return false
	}
	d, ok := r.World.Destination(s.ShipCityID)
	if !ok {
		return false
	}
	p, ok := r.World.Planet(d.PlanetID)
	return ok && fuelRescueAllowed(s.FuelRescuePending, true, d.SourceIndex == p.CityStart+1)
}

// requestFuel accepts typed location predicates and a controlled range draw.
// It retains the existing deadline on repeated requests, including across saves.
func requestFuel(s *State, inCity, onPlanet bool, draw func(int) int) bool {
	if s.FuelRescuePending {
		return false
	}
	wait := distressOrbitWait
	if inCity {
		wait = distressGroundWait
	}
	if !onPlanet {
		wait += distressExtraLow + draw(distressExtraHigh-distressExtraLow+1)
	}
	s.FuelRescuePending = true
	s.FuelRescueDue = s.TimeMinutes + wait
	s.ShipRefresh = false
	return true
}
