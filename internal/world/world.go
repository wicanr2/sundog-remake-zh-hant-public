// Package world holds the remake's geography.
//
// The star systems, planets and cities come from the Atari ST source tables
// decoded by tools/atari_st_world_data.py and validated in
// internal/source/atarist: names, ordering and the planet/city grouping are
// source-proven. What a route costs is source-proven too and lives in
// internal/game, which knows where the ship is; see routeFuelCost there. What
// is still explicit remake-owned prototype data, and is marked as such, is the
// facility tiles, the market prices and the ground map.
package world

import (
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

const (
	// StartDestinationID is Drahew, the first city of the first planet in the
	// source table and the region the manual names as the starting area.
	StartDestinationID = "drahew"
	// MissionDestinationID is Banville, the colony stored outside the
	// contiguous per-planet runs in the source city table.
	MissionDestinationID = "banville"

	StartSystemID = "jondd"
	StartPlanetID = "jondd"
)

type System struct {
	ID   string
	Name string
	// Coordinates are the three packed fields the original measures jumps
	// with. They come straight from the star system record.
	Coordinates [3]int
	// EncounterRating is the fourth field of the same word. A higher value
	// means the original generates fewer enemies in this system.
	EncounterRating int
	// PriceLevel is what every shop in the system multiplies its prices by.
	// It comes from the record's own tenth word.
	PriceLevel int
	// StreetDealRating is the next field of that same word. It is the
	// threshold that suppresses the street encounter which opens with an offer
	// of a deal: a higher value means fewer of them.
	StreetDealRating int
}

type Planet struct {
	ID       string
	SystemID string
	Name     string
	// CityStart is the zero-based start of the source planet's city run.
	// XDOUNITE's picker uses that run and reserves one slot for the current city.
	CityStart int
	CityCount int
	// Orbit is where the planet sits in its own star system: an angle on a
	// 256-step turn and a radius. In-system flight interpolates between two of
	// these, so this is the only geometry that leg has. It is the factory
	// value: a planet's angle is live state, and Registry.PlanetOrbit reports
	// where it has moved to.
	Orbit Orbit
	// OrbitRate is how far the planet moves each day (XMOVEONS:0x07
	// check_planets). Six of the eighteen planets have rate 0.
	OrbitRate int
	// Index is the planet's slot in the source table, which is what the live
	// orbit angles are keyed by.
	Index int
}

// Orbit is a polar position around a star.
type Orbit struct {
	Radius int
	Angle  int
}

type Destination struct {
	// SourceIndex is the original one-based city record number, including gaps.
	SourceIndex int
	ID          string
	SystemID    string
	SystemName  string
	PlanetID    string
	PlanetName  string
	CityID      string
	CityName    string
	Label       string
	// Hidden marks the colony stored outside the contiguous per-planet runs.
	// ST navigation omits it, and Tube's source run cannot select record 60.
	// Navigation target selection and other readers retain its identity.
	Hidden bool
	// MapX and MapY are where the original's planet map draws this city. Both
	// come from the city record's own packed position through the original's
	// change_xy; see docs/atari-st/ship-rules.md rule map.city-pixel.
	MapX int
	MapY int
	// MapColumn and MapRow are the packed fields MapX and MapY are derived
	// from. The tube fare is measured in those rather than in pixels.
	MapColumn int
	MapRow    int
}

type Registry struct {
	systems      []System
	planets      []Planet
	destinations []Destination
	cargoKinds   []CargoKind
	cargoOrigins map[uint8]string
	byID         map[string]Destination
	facilities   map[string]map[TilePosition]map[Facility]struct{}
	markets      map[string]MarketQuote
	fuelPrices   map[string]int
}

// FactorySystems is the star system table as the source ships it, in record
// order. New-game code that has no registry in hand starts from these.
func FactorySystems() []System {
	registry, err := NewSourceRegistry()
	if err != nil {
		return nil
	}
	return registry.Systems()
}

// NewSourceRegistry builds the registry from the embedded Atari ST world
// tables. It never reads the original disk image at runtime.
func NewSourceRegistry() (Registry, error) {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		return Registry{}, fmt.Errorf("load Atari ST world data: %w", err)
	}

	sourceSystems := data.Systems()
	sourcePlanets := data.Planets()
	sourceCities := data.Cities()

	systems := make([]System, 0, len(sourceSystems))
	for _, system := range sourceSystems {
		systems = append(systems, System{
			ID:               system.ID,
			Name:             system.Name,
			Coordinates:      system.Coordinates,
			EncounterRating:  system.EncounterRating,
			PriceLevel:       system.PriceLevel,
			StreetDealRating: system.StreetDealRating,
		})
	}

	planets := make([]Planet, 0, len(sourcePlanets))
	for _, planet := range sourcePlanets {
		planets = append(planets, Planet{
			ID:        planet.ID,
			SystemID:  sourceSystems[planet.SystemIndex].ID,
			Name:      planet.Name,
			CityStart: planet.CityStart,
			CityCount: planet.CityCount,
			Orbit:     Orbit{Radius: planet.Orbit.Radius, Angle: planet.Orbit.Angle},
			OrbitRate: planet.OrbitRate,
			Index:     planet.Index,
		})
	}

	destinations := make([]Destination, 0, len(sourceCities))
	for _, city := range sourceCities {
		planet := sourcePlanets[city.PlanetIndex]
		system := sourceSystems[planet.SystemIndex]
		destinations = append(destinations, Destination{
			SourceIndex: city.Index + 1,
			ID:          city.ID,
			SystemID:    system.ID,
			SystemName:  system.Name,
			PlanetID:    planet.ID,
			PlanetName:  planet.Name,
			CityID:      city.ID,
			CityName:    city.Name,
			Label:       fmt.Sprintf("%s／%s／%s", system.Name, planet.Name, city.Name),
			Hidden:      city.Hidden,
			MapX:        city.MapPosition.X,
			MapColumn:   city.MapPosition.Column,
			MapRow:      city.MapPosition.Row,
			MapY:        city.MapPosition.Y,
		})
	}

	cargoKinds, err := sourceCargoKinds()
	if err != nil {
		return Registry{}, err
	}
	registry := Registry{
		systems:      systems,
		planets:      planets,
		destinations: destinations,
		cargoKinds:   cargoKinds,
		cargoOrigins: make(map[uint8]string),
		byID:         make(map[string]Destination, len(destinations)),
		facilities:   prototypeFacilities(destinations),
		markets:      sourceMarkets(destinations, cargoKinds),
		fuelPrices:   prototypeFuelPrices(destinations),
	}
	for _, destination := range destinations {
		registry.byID[destination.ID] = destination
	}
	for _, planet := range sourcePlanets {
		for offset := 0; offset < planet.CityCount; offset++ {
			registry.cargoOrigins[uint8(planet.CityStart+offset+1)] = planet.ID
		}
	}
	if err := registry.Validate(); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

// RouteCost is the jump half of a trip: the distance between two star systems
// and the time the original charges for it. FuelCost and the in-system half of
// TotalMinutes are left at zero here and filled in by internal/game, which
// knows where the ship actually is and can run the original's own arc estimate.
// Nothing about a route is remake-owned any more.
type RouteCost struct {
	// JumpDistance is 0 for a destination in the player's own system.
	JumpDistance int
	// JumpMinutes is the original's warp charge for that distance.
	JumpMinutes  int
	FuelCost     int
	TotalMinutes int
	// WithinRange reports whether the ship's jump range covers the distance.
	WithinRange bool
}

// System returns one star system by identity.
func (r Registry) System(id string) (System, bool) {
	for _, system := range r.systems {
		if system.ID == id {
			return system, true
		}
	}
	return System{}, false
}

// SystemCoordinates are the three packed coordinate fields of every star
// system as they stand in one game. The original re-rolls them at new-game
// (XSTARTUP:0x26 init_systems, see internal/game/sector.go), so the factory
// values in the table are only the starting point. A nil map means the
// factory values.
type SystemCoordinates map[string][3]int

// SystemCoordinate is where a system sits in this game: the per-game value
// when the map carries one, the factory value otherwise.
func (r Registry) SystemCoordinate(coords SystemCoordinates, id string) ([3]int, bool) {
	for _, system := range r.systems {
		if system.ID == id {
			if c, ok := coords[id]; ok {
				return c, true
			}
			return system.Coordinates, true
		}
	}
	return [3]int{}, false
}

// JumpDistance measures a jump between two star systems on the factory map,
// exactly as SHIPLIB:0x0a dist does. Two destinations in the same system are
// not a jump. A running game measures on its own map with JumpDistanceWith.
func (r Registry) JumpDistance(fromSystemID, toSystemID string) (int, bool) {
	return r.JumpDistanceWith(nil, fromSystemID, toSystemID)
}

// JumpDistanceWith measures a jump on the given per-game map (nil for the
// factory values). The adjusted-system rule keys on the record index, which
// is the position in the source table, so it survives the re-roll.
func (r Registry) JumpDistanceWith(coords SystemCoordinates, fromSystemID, toSystemID string) (int, bool) {
	if fromSystemID == toSystemID {
		return 0, true
	}
	var from, to atarist.System
	foundFrom, foundTo := false, false
	for index, system := range r.systems {
		c, _ := r.SystemCoordinate(coords, system.ID)
		switch system.ID {
		case fromSystemID:
			from, foundFrom = atarist.System{Index: index, Coordinates: c}, true
		case toSystemID:
			to, foundTo = atarist.System{Index: index, Coordinates: c}, true
		}
	}
	if !foundFrom || !foundTo {
		return 0, false
	}
	return atarist.JumpDistance(from, to), true
}

// MaxJumpDistance is how far the ship can jump at the start of a game. The
// number is read from the source rather than chosen here.
func MaxJumpDistance() int {
	rules, err := atarist.EmbeddedRuleData()
	if err != nil {
		return 0
	}
	value, ok := rules.RuleValue("flight.initial-jump-range")
	if !ok {
		return 0
	}
	return value
}

// RouteCost reports the jump half of a trip. jumpRange is the ship's own
// maximum jump, which the depot can raise.
func (r Registry) RouteCost(fromSystemID, destinationID string, jumpRange int, coords SystemCoordinates) (RouteCost, bool) {
	destination, ok := r.Destination(destinationID)
	if !ok {
		return RouteCost{}, false
	}
	distance, ok := r.JumpDistanceWith(coords, fromSystemID, destination.SystemID)
	if !ok {
		return RouteCost{}, false
	}
	cost := RouteCost{
		JumpDistance: distance,
		WithinRange:  distance <= jumpRange,
	}
	if distance > 0 {
		cost.JumpMinutes = atarist.JumpDuration(distance)
		cost.TotalMinutes += cost.JumpMinutes
	}
	return cost, true
}

func (r Registry) Validate() error {
	if len(r.systems) != 12 {
		return fmt.Errorf("world has %d systems, want 12", len(r.systems))
	}
	if len(r.planets) != 18 {
		return fmt.Errorf("world has %d planets, want 18", len(r.planets))
	}
	if len(r.destinations) != 58 {
		return fmt.Errorf("world has %d destinations, want 58", len(r.destinations))
	}
	systemIDs := make(map[string]struct{}, len(r.systems))
	for _, system := range r.systems {
		if system.ID == "" || system.Name == "" {
			return fmt.Errorf("system has empty identity or name")
		}
		systemIDs[system.ID] = struct{}{}
	}
	for _, planet := range r.planets {
		if planet.ID == "" || planet.Name == "" {
			return fmt.Errorf("planet has empty identity or name")
		}
		if _, ok := systemIDs[planet.SystemID]; !ok {
			return fmt.Errorf("planet %q references unknown system %q", planet.ID, planet.SystemID)
		}
	}
	seen := make(map[string]struct{}, len(r.destinations))
	hidden := 0
	for _, destination := range r.destinations {
		if destination.ID == "" || destination.Label == "" {
			return fmt.Errorf("destination has empty identity or label")
		}
		if _, ok := seen[destination.ID]; ok {
			return fmt.Errorf("duplicate destination %q", destination.ID)
		}
		seen[destination.ID] = struct{}{}
		if _, ok := r.byID[destination.ID]; !ok {
			return fmt.Errorf("destination %q missing index", destination.ID)
		}
		if destination.Hidden {
			hidden++
		}
	}
	if hidden != 1 {
		return fmt.Errorf("world has %d cities stored outside the contiguous runs, want 1", hidden)
	}
	if _, ok := r.byID[StartDestinationID]; !ok {
		return fmt.Errorf("missing start destination")
	}
	mission, ok := r.byID[MissionDestinationID]
	if !ok {
		return fmt.Errorf("missing mission destination")
	}
	if !mission.Hidden {
		return fmt.Errorf("mission destination %q is not the hidden colony", MissionDestinationID)
	}
	for _, destination := range r.destinations {
		quote, ok := r.markets[destination.CityID]
		if !ok {
			return fmt.Errorf("destination %q has no market quote", destination.ID)
		}
		if !r.knownCargoKey(quote.ItemKey) {
			return fmt.Errorf("destination %q has unknown cargo item %q", destination.ID, quote.ItemKey)
		}
		fuelPrice, ok := r.fuelPrices[destination.CityID]
		if !ok || fuelPrice <= 0 {
			return fmt.Errorf("destination %q has no positive fuel price", destination.ID)
		}
		// Every city has to be usable the moment the player lands on it, so the
		// check is against the landing cell rather than a fixed coordinate.
		landing := LandingPosition(destination.CityID)
		for _, facility := range []Facility{FacilityUniteller, FacilityExchange, FacilitySpaceport, FacilityWarehouse} {
			if !r.HasFacility(destination.CityID, landing.X, landing.Y, facility) {
				return fmt.Errorf("destination %q has no %s facility on its landing cell", destination.ID, facility)
			}
		}
	}
	return nil
}

// CargoKinds returns the tradable cargo from the source table, in table order.
func (r Registry) CargoKinds() []CargoKind {
	result := make([]CargoKind, len(r.cargoKinds))
	copy(result, r.cargoKinds)
	return result
}

// CargoKind resolves a market item key back to the source record behind it, so
// a consumer can reach the record's own name and value instead of parsing the
// key string.
func (r Registry) CargoKind(key string) (CargoKind, bool) {
	for _, kind := range r.cargoKinds {
		if kind.Key == key {
			return kind, true
		}
	}
	return CargoKind{}, false
}

func (r Registry) knownCargoKey(key string) bool {
	_, ok := r.CargoKind(key)
	return ok
}

func (r Registry) Destination(id string) (Destination, bool) {
	destination, ok := r.byID[id]
	return destination, ok
}

func (r Registry) Destinations() []Destination {
	result := make([]Destination, len(r.destinations))
	copy(result, r.destinations)
	return result
}

// Systems returns the star systems in source-table order.
func (r Registry) Systems() []System {
	result := make([]System, len(r.systems))
	copy(result, r.systems)
	return result
}

// Planets returns the planets in source-table order.
func (r Registry) Planets() []Planet {
	result := make([]Planet, len(r.planets))
	copy(result, r.planets)
	return result
}

// PlanetsInSystem returns one system's planets in source-table order.
func (r Registry) PlanetsInSystem(systemID string) []Planet {
	result := make([]Planet, 0, 3)
	for _, planet := range r.planets {
		if planet.SystemID == systemID {
			result = append(result, planet)
		}
	}
	return result
}

// DestinationsOnPlanet returns one planet's cities in source-table order.
func (r Registry) DestinationsOnPlanet(planetID string) []Destination {
	result := make([]Destination, 0, 6)
	for _, destination := range r.destinations {
		if destination.PlanetID == planetID {
			result = append(result, destination)
		}
	}
	return result
}

// CycleTargetSystem moves the selection to the neighbouring star system and
// returns that system's first city. With 58 destinations, walking the whole
// table one city at a time is not a usable route selector; the source tables
// group cities by planet and planets by system, so the selector follows that
// same hierarchy.
func (r Registry) CycleTargetSystem(current string, delta int) (Destination, error) {
	destination, ok := r.byID[current]
	if !ok {
		return Destination{}, fmt.Errorf("unknown destination %q", current)
	}
	index := 0
	for candidateIndex, system := range r.systems {
		if system.ID == destination.SystemID {
			index = candidateIndex
			break
		}
	}
	index = wrapIndex(index, delta, len(r.systems))
	return r.firstDestinationInSystem(r.systems[index].ID)
}

// CycleTargetPlanet moves the selection to the neighbouring planet inside the
// currently selected system and returns that planet's first city.
func (r Registry) CycleTargetPlanet(current string, delta int) (Destination, error) {
	destination, ok := r.byID[current]
	if !ok {
		return Destination{}, fmt.Errorf("unknown destination %q", current)
	}
	planets := r.PlanetsInSystem(destination.SystemID)
	if len(planets) == 0 {
		return Destination{}, fmt.Errorf("system %q has no planets", destination.SystemID)
	}
	index := 0
	for candidateIndex, planet := range planets {
		if planet.ID == destination.PlanetID {
			index = candidateIndex
			break
		}
	}
	index = wrapIndex(index, delta, len(planets))
	return r.firstDestinationOnPlanet(planets[index].ID)
}

// CycleTargetCity moves the selection within the currently selected planet.
func (r Registry) CycleTargetCity(current string, delta int) (Destination, error) {
	destination, ok := r.byID[current]
	if !ok {
		return Destination{}, fmt.Errorf("unknown destination %q", current)
	}
	cities := r.DestinationsOnPlanet(destination.PlanetID)
	if len(cities) == 0 {
		return Destination{}, fmt.Errorf("planet %q has no cities", destination.PlanetID)
	}
	index := 0
	for candidateIndex, city := range cities {
		if city.ID == destination.ID {
			index = candidateIndex
			break
		}
	}
	return cities[wrapIndex(index, delta, len(cities))], nil
}

func (r Registry) firstDestinationInSystem(systemID string) (Destination, error) {
	for _, destination := range r.destinations {
		if destination.SystemID == systemID {
			return destination, nil
		}
	}
	return Destination{}, fmt.Errorf("system %q has no destination", systemID)
}

func (r Registry) firstDestinationOnPlanet(planetID string) (Destination, error) {
	for _, destination := range r.destinations {
		if destination.PlanetID == planetID {
			return destination, nil
		}
	}
	return Destination{}, fmt.Errorf("planet %q has no destination", planetID)
}

func wrapIndex(index, delta, length int) int {
	if length == 0 {
		return 0
	}
	index = (index + delta) % length
	if index < 0 {
		index += length
	}
	return index
}

// CycleDestination walks the whole destination table in source order.
func (r Registry) CycleDestination(current string, delta int) (Destination, error) {
	if len(r.destinations) == 0 {
		return Destination{}, fmt.Errorf("world has no destinations")
	}
	index := 0
	for candidateIndex, destination := range r.destinations {
		if destination.ID == current {
			index = candidateIndex
			break
		}
	}
	if delta == 0 {
		return r.destinations[index], nil
	}
	index = (index + delta) % len(r.destinations)
	if index < 0 {
		index += len(r.destinations)
	}
	return r.destinations[index], nil
}

// Planet returns one planet by identity.
func (r Registry) Planet(id string) (Planet, bool) {
	for _, planet := range r.planets {
		if planet.ID == id {
			return planet, true
		}
	}
	return Planet{}, false
}

// CargoKindByCode is the tradable kind with that item-table index.
func (r Registry) CargoKindByCode(code int) (CargoKind, bool) {
	for _, kind := range r.cargoKinds {
		if kind.Code == code {
			return kind, true
		}
	}
	return CargoKind{}, false
}

// CargoOrigin uses find_location's contiguous source ranges. The trailing
// colony has a navigation destination but is outside those ranges.
func (r Registry) CargoOrigin(code uint8) (Planet, bool) {
	return r.Planet(r.cargoOrigins[code])
}
