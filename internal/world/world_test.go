package world

import "testing"

func TestSourceWorldShapeAndMission(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(registry.Systems()); got != 12 {
		t.Fatalf("systems = %d, want 12", got)
	}
	if got := len(registry.Planets()); got != 18 {
		t.Fatalf("planets = %d, want 18", got)
	}
	if got := len(registry.Destinations()); got != 58 {
		t.Fatalf("destinations = %d, want 58", got)
	}
	start, ok := registry.Destination(StartDestinationID)
	if !ok || start.CityName != "Drahew" || start.PlanetName != "Jondd" || start.SystemName != "Jondd" {
		t.Fatalf("start destination is not the source-backed Drahew: %+v", start)
	}
	mission, ok := registry.Destination(MissionDestinationID)
	if !ok || mission.CityName != "Banville" || !mission.Hidden {
		t.Fatalf("mission destination is not the hidden colony: %+v", mission)
	}
}

func TestSourceWorldGroupingMatchesSourceTables(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Ferr owns three planets in the source table; Jondd owns two.
	if got := len(registry.PlanetsInSystem("ferr")); got != 3 {
		t.Fatalf("planets in Ferr = %d, want 3", got)
	}
	if got := len(registry.PlanetsInSystem("jondd")); got != 2 {
		t.Fatalf("planets in Jondd = %d, want 2", got)
	}
	// The Jondd planet owns five contiguous cities plus the hidden colony.
	cities := registry.DestinationsOnPlanet("jondd")
	if len(cities) != 6 {
		t.Fatalf("cities on Jondd = %d, want 6", len(cities))
	}
	hidden := 0
	for _, city := range cities {
		if city.Hidden {
			hidden++
		}
	}
	if hidden != 1 {
		t.Fatalf("hidden cities on Jondd = %d, want 1", hidden)
	}
	systems := make(map[string]struct{}, 12)
	for _, system := range registry.Systems() {
		systems[system.ID] = struct{}{}
	}
	for _, planet := range registry.Planets() {
		if _, ok := systems[planet.SystemID]; !ok {
			t.Fatalf("planet %q references unknown system %q", planet.ID, planet.SystemID)
		}
	}
}

func TestCycleDestinationWrapsAndCopies(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// The hidden colony is the last record in the source city table.
	first, err := registry.CycleDestination(StartDestinationID, -1)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != MissionDestinationID {
		t.Fatalf("previous destination did not wrap: %+v", first)
	}
	list := registry.Destinations()
	list[0].Label = "mutated"
	original, _ := registry.Destination(StartDestinationID)
	if original.Label == "mutated" {
		t.Fatal("destination list leaked registry storage")
	}
}

func TestPrototypeFacilitiesAndMarketQuotesAreStable(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// The landing cell is the city's own exchange building, and the remake keeps
	// its facility cluster there so a city is usable on arrival.
	landing := LandingPosition(StartDestinationID)
	startFacilities := registry.FacilitiesAt(StartDestinationID, landing)
	for _, facility := range []Facility{FacilityWarehouse, FacilityUniteller, FacilityExchange, FacilitySpaceport} {
		if !containsFacility(startFacilities, facility) {
			t.Fatalf("start facilities = %v, want %s", startFacilities, facility)
		}
	}
	// The city's own depot building is a second warehouse cell, and it is the
	// original's position rather than the remake's.
	cityMap, ok := CityMap(StartDestinationID)
	if !ok {
		t.Fatal("the start city has no decoded map")
	}
	for _, building := range cityMap.Buildings() {
		if building.Facility != "depot" {
			continue
		}
		if !registry.HasFacility(StartDestinationID, building.Row, building.Column, FacilityWarehouse) {
			t.Fatalf("the decoded depot at (%d,%d) is not a warehouse cell", building.Row, building.Column)
		}
	}
	missionLanding := LandingPosition(MissionDestinationID)
	if !registry.HasFacility(MissionDestinationID, missionLanding.X, missionLanding.Y, FacilityColony) {
		t.Fatal("mission colony facility is missing")
	}
	kinds := registry.CargoKinds()
	if len(kinds) == 0 {
		t.Fatal("registry exposes no tradable cargo")
	}
	startQuote, ok := registry.Market(StartDestinationID)
	if !ok || startQuote.ItemKey != kinds[0].Key {
		t.Fatalf("start market quote = %+v, want the first source cargo %q", startQuote, kinds[0].Key)
	}
	// The price starts from the source record's own value rather than an
	// invented number; the spread below it is remake-owned.
	if startQuote.BuyPrice != kinds[0].BaseValue || startQuote.SellPrice != kinds[0].BaseValue-remakeCargoSpread {
		t.Fatalf("start market quote = %+v, want the source base value %d", startQuote, kinds[0].BaseValue)
	}
	otherQuote, ok := registry.Market("obburg")
	if !ok || otherQuote.ItemKey == startQuote.ItemKey {
		t.Fatalf("market quote did not vary its cargo identity: %+v", otherQuote)
	}
	if price, ok := registry.FuelPrice(StartDestinationID); !ok || price != 157 {
		t.Fatalf("start fuel unit price = %d, %v; want source price 157", price, ok)
	}
	seenItems := make(map[string]struct{})
	for _, destination := range registry.Destinations() {
		quote, ok := registry.Market(destination.CityID)
		if !ok {
			t.Fatalf("destination %q has no market quote", destination.ID)
		}
		seenItems[quote.ItemKey] = struct{}{}
	}
	if len(seenItems) != len(kinds) {
		t.Fatalf("market rotation exposed %d cargo kinds, want %d", len(seenItems), len(kinds))
	}
	// Every quoted cargo has to be one the source table actually prices.
	for _, kind := range kinds {
		if kind.BaseValue < 0 || kind.BaseValue > 255 || kind.Code == 0 {
			t.Fatalf("cargo %q reached the market without a source price input", kind.Key)
		}
	}
}

func containsFacility(values []Facility, target Facility) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestHierarchicalTargetSelection(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Cities cycle inside the selected planet only. Jondd owns five contiguous
	// cities plus the hidden colony.
	next, err := registry.CycleTargetCity(StartDestinationID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if next.PlanetID != "jondd" || next.ID == StartDestinationID {
		t.Fatalf("city selection left the planet or stood still: %+v", next)
	}
	seen := make(map[string]struct{})
	current := StartDestinationID
	for step := 0; step < 6; step++ {
		destination, err := registry.CycleTargetCity(current, 1)
		if err != nil {
			t.Fatal(err)
		}
		if destination.PlanetID != "jondd" {
			t.Fatalf("city cycling escaped the planet at step %d: %+v", step, destination)
		}
		seen[destination.ID] = struct{}{}
		current = destination.ID
	}
	if len(seen) != 6 || current != StartDestinationID {
		t.Fatalf("city cycling did not wrap through the planet's 6 cities: seen=%d current=%q", len(seen), current)
	}

	// Planet selection stays inside the system and lands on that planet's
	// first city.
	planet, err := registry.CycleTargetPlanet(StartDestinationID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if planet.SystemID != "jondd" || planet.PlanetID != "heavy" {
		t.Fatalf("planet selection = %+v, want the second Jondd planet", planet)
	}

	// System selection lands on the next system's first city.
	system, err := registry.CycleTargetSystem(StartDestinationID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if system.SystemID != "ferr" || system.PlanetID != "ferr" {
		t.Fatalf("system selection = %+v, want the first city of Ferr", system)
	}

	// Every system is reachable within one pass, and each lands on a real city.
	current = StartDestinationID
	systems := make(map[string]struct{})
	for step := 0; step < 12; step++ {
		destination, err := registry.CycleTargetSystem(current, 1)
		if err != nil {
			t.Fatal(err)
		}
		systems[destination.SystemID] = struct{}{}
		current = destination.ID
	}
	if len(systems) != 12 || current != StartDestinationID {
		t.Fatalf("system cycling did not wrap through 12 systems: seen=%d current=%q", len(systems), current)
	}

	if _, err := registry.CycleTargetSystem("no-such-city", 1); err == nil {
		t.Fatal("expected an error for an unknown destination")
	}
}
