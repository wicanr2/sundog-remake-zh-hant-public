package game

import "github.com/wicanr2/sundog-remake/internal/world"

// The tube station is a building of its own on the city map (terrain code 18,
// the door check_building dispatches): a ride to another city of the same
// planet, paid at the station instead of flown. XDOUNITE:0x09 do_tube_station
// runs it and its move_it prices the ride (rule tube.fare):
//
//	fare = 2 × price level × isqrt(Δcolumn² + Δrow²)
//
// The two deltas are the cities' packed positions on the planet map — the same
// fields change_xy draws them from — not their pixel positions. The price level
// is the star system's (rule tube.levels).
//
// The window is the original's, read off a run of the port under the p-system
// debugger (docs/atari-st/parity-oracle-log.md, the tube-station probe): the
// title is "Tube Station", "Select destination city" is centred over two rows
// of the fill, and the destinations sit in pick_location's boxes — four cities
// measured at style 9's four narrow boxes, two to a row. A planet holds at
// most six cities, so the list is at most five long and the wide OTHERS box
// that style 9 keeps for a sixth entry never comes up.
//
// The ride itself is the original's too: it puts the player in the destination
// city without a flight, which is what makes it worth paying for.

const tubeFareFactor = 2

// tubePickActions are the station's boxes in the order pick_location fills
// them; a planet's list is never longer than these.
var tubePickActions = []Action{ActionTubePick1, ActionTubePick2, ActionTubePick3, ActionTubePick4, ActionTubePick5, ActionTubePick6}

// do_tube_station has two pages. The first is the list; picking a city clears
// the window and puts up the second, which names the ride and asks for it:
//
//	Destination
//	City:   <city>
//	Planet: <planet>
//	System: <system>
//	Cost:   <fare> Cr
//
// with menu 6's box 1 reading TRANSPORT — but only when the fare is affordable.
// When it is not, row 7 says "Insufficient funds" and there is no box to press
// (XDOUNITE:0x03b6: `more(cost, cash)` decides which). The right button on
// either page goes back a step.
const (
	TubePageList    = 0
	TubePageConfirm = 1
)

// isqrt is MAINLIB:0x09: the integer square root, rounded down.
func isqrt(value int) int {
	if value <= 0 {
		return 0
	}
	root := 0
	for (root+1)*(root+1) <= value {
		root++
	}
	return root
}

// TubeFare is what a ride to the named destination costs, and whether the tube
// goes there at all. It only runs between cities of the planet the player is
// standing on.
func (r Reducer) TubeFare(state State, destinationID string) (int, bool) {
	if destinationID == state.CityID {
		return 0, false
	}
	here, ok := r.Destination(state, state.CityID)
	if !ok {
		return 0, false
	}
	there, ok := r.Destination(state, destinationID)
	if !ok || there.PlanetID != here.PlanetID {
		return 0, false
	}
	planet, ok := r.World.Planet(here.PlanetID)
	if !ok || planet.CityCount < 2 {
		return 0, false
	}
	first := planet.CityStart + 1
	if there.SourceIndex < first || there.SourceIndex >= first+planet.CityCount {
		return 0, false
	}
	// pick_location emits count-1 labels, even when the current city is
	// Banville's record 60 outside the planet's ordinary city run.
	pick := there.SourceIndex - first
	if here.SourceIndex >= first && here.SourceIndex < there.SourceIndex {
		pick--
	}
	if pick >= planet.CityCount-1 {
		return 0, false
	}
	system, ok := r.World.System(state.SystemID)
	if !ok {
		return 0, false
	}
	dx := there.MapColumn - here.MapColumn
	dy := there.MapRow - here.MapRow
	return tubeFareFactor * system.PriceLevel * isqrt(dx*dx+dy*dy), true
}

// rideTube takes the fare and puts the player in the other city. It reports
// whether the ride happened.
//
// **The fare comes out of the bank, not the purse.** xdo_uniteller lifts the
// system's balance out of its star-system record into a local when it opens
// (the record reads 0 while the player is inside, measured under the debugger),
// and do_tube_station both compares against that local (`more(&cost, &balance)`
// at XDOUNITE:0x03b6) and subtracts from it (`0x03ec`). The purse never enters
// into it: an oracle run with 1 Cr in hand and 36,470 in the bank still offered
// TRANSPORT, and the same run refused once the balance was set to 1.
func (r Reducer) rideTube(state *State, destination world.Destination, fare int) bool {
	if state.Bank < fare {
		return false
	}
	state.Bank -= fare
	storeCurrentBank(state)
	arriveByTube(state, destination)
	return true
}

// TubeDestination is one city the station sells a ride to.
type TubeDestination struct {
	ID   string
	Name string
	Fare int
}

// TubeDestinations follows XDOUNITE's source city run and count-1 label limit.
// The order is the source table's; record 60 is never an inbound destination.
func (r Reducer) TubeDestinations(state State) []TubeDestination {
	here, ok := r.World.Destination(state.CityID)
	if !ok {
		return nil
	}
	list := []TubeDestination{}
	for _, there := range r.World.Destinations() {
		if there.PlanetID != here.PlanetID || there.CityID == here.CityID {
			continue
		}
		fare, ok := r.TubeFare(state, there.ID)
		if !ok {
			continue
		}
		list = append(list, TubeDestination{ID: there.ID, Name: there.CityName, Fare: fare})
	}
	return list
}

// tubePick takes box n of the station's list: the confirmation page comes up
// for that city. It reports the message key for what happened.
func (r Reducer) tubePick(state *State, n int) string {
	list := r.TubeDestinations(*state)
	if n < 0 || n >= len(list) {
		return "ui.message.tube_no_destination"
	}
	state.TubeTargetID = list[n].ID
	state.TubePage = TubePageConfirm
	return "ui.message.tube_confirm"
}

// TubeSelection is what the confirmation page shows: the ride the list picked
// and whether the player can pay for it.
type TubeSelection struct {
	Destination TubeDestination
	CityName    string
	PlanetName  string
	SystemName  string
	Affordable  bool
}

// TubeSelected is the confirmation page's content, or false when no city is
// picked.
func (r Reducer) TubeSelected(state State) (TubeSelection, bool) {
	if state.TubeTargetID == "" {
		return TubeSelection{}, false
	}
	there, ok := r.World.Destination(state.TubeTargetID)
	if !ok {
		return TubeSelection{}, false
	}
	fare, ok := r.TubeFare(state, there.ID)
	if !ok {
		return TubeSelection{}, false
	}
	return TubeSelection{
		Destination: TubeDestination{ID: there.ID, Name: there.CityName, Fare: fare},
		CityName:    there.CityName,
		PlanetName:  there.PlanetName,
		SystemName:  there.SystemName,
		// more(cost, balance) at XDOUNITE:0x03b6: TRANSPORT only appears when
		// the fare is covered, and what it is covered from is the **bank**.
		Affordable: state.Bank >= fare,
	}, true
}

// tubeTransport is the TRANSPORT box: pay and go.
func (r Reducer) tubeTransport(state *State) string {
	selection, ok := r.TubeSelected(*state)
	if !ok {
		return "ui.message.tube_no_destination"
	}
	if !selection.Affordable {
		return "ui.message.tube_insufficient"
	}
	destination, ok := r.World.Destination(selection.Destination.ID)
	if !ok {
		return "ui.message.tube_no_destination"
	}
	if !r.rideTube(state, destination, selection.Destination.Fare) {
		return "ui.message.tube_insufficient"
	}
	state.TubePage, state.TubeTargetID = TubePageList, ""
	return "ui.message.tube_arrived"
}
