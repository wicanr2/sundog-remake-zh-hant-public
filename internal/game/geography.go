package game

import "github.com/wicanr2/sundog-remake/internal/world"

// defaultStartLabel and defaultMissionLabel are the source-name fallbacks shown
// before a localization overlay is applied. They are derived from the embedded
// Atari ST world tables so they cannot drift away from the registry; the
// literals below are only used if the embedded data fails to load, and that
// failure is separately surfaced by NewReducer.
var (
	defaultStartLabel   = "Jondd／Jondd／Drahew"
	defaultMissionLabel = "Jondd／Jondd／Banville"

	geographyRegistry   world.Registry
	geographyRegistryOK bool
)

func init() {
	registry, err := world.NewSourceRegistry()
	if err != nil {
		return
	}
	geographyRegistry = registry
	geographyRegistryOK = true
	if start, ok := registry.Destination(world.StartDestinationID); ok {
		defaultStartLabel = start.Label
	}
	if mission, ok := registry.Destination(world.MissionDestinationID); ok {
		defaultMissionLabel = mission.Label
	}
}

// destinationLabel returns a destination's source-name label, or the supplied
// fallback when the identity is not part of the source-backed world.
func destinationLabel(id, fallback string) string {
	if !geographyRegistryOK {
		return fallback
	}
	destination, ok := geographyRegistry.Destination(id)
	if !ok {
		return fallback
	}
	return destination.Label
}

// prototypeGeographyRemap names the only two retired prototype destinations
// that ever claimed a real counterpart. Every other retired identity has no
// place in the source-backed world and folds back to the start city instead of
// being matched to an invented equivalent.
var prototypeGeographyRemap = map[string]string{
	"drahew-spaceport": world.StartDestinationID,
	"jond-banville":    world.MissionDestinationID,
}

// migratePrototypeGeography rewrites a schema 6 save onto the source-backed
// world. Identities are checked against the registry rather than against a list
// of retired names, so anything that is not part of the source tables is
// remapped whether or not it was one of the prototype's own inventions.
// Balances held at retired systems are merged into the start system, so the
// migration never costs the player money.
func migratePrototypeGeography(state *State) {
	knownSystem := func(id string) bool {
		if !geographyRegistryOK {
			return true
		}
		for _, system := range geographyRegistry.Systems() {
			if system.ID == id {
				return true
			}
		}
		return false
	}
	knownPlanet := func(id string) bool {
		if !geographyRegistryOK {
			return true
		}
		for _, planet := range geographyRegistry.Planets() {
			if planet.ID == id {
				return true
			}
		}
		return false
	}
	remapDestination := func(id string) string {
		if replacement, ok := prototypeGeographyRemap[id]; ok {
			return replacement
		}
		if !geographyRegistryOK {
			return id
		}
		if _, ok := geographyRegistry.Destination(id); ok {
			return id
		}
		return world.StartDestinationID
	}

	state.CityID = remapDestination(state.CityID)
	state.NavigationTargetID = remapDestination(state.NavigationTargetID)

	visited := make([]string, 0, len(state.VisitedDestinations))
	seen := make(map[string]struct{}, len(state.VisitedDestinations))
	for _, id := range state.VisitedDestinations {
		replacement := remapDestination(id)
		if _, exists := seen[replacement]; exists {
			continue
		}
		seen[replacement] = struct{}{}
		visited = append(visited, replacement)
	}
	state.VisitedDestinations = visited

	if !knownSystem(state.SystemID) {
		state.SystemID = world.StartSystemID
	}
	if !knownPlanet(state.PlanetID) {
		state.PlanetID = world.StartPlanetID
	}

	if state.BankBalances != nil {
		merged := make(map[string]int, len(state.BankBalances))
		for systemID, balance := range state.BankBalances {
			if !knownSystem(systemID) {
				merged[world.StartSystemID] += balance
				continue
			}
			merged[systemID] += balance
		}
		state.BankBalances = merged
		// Bank mirrors the balance held at the current system. Re-reading it
		// after the merge keeps the two views consistent, so the money folded
		// in from retired systems is not overwritten later by normalization.
		if balance, ok := merged[state.SystemID]; ok {
			state.Bank = balance
		}
	}

	// The stored labels were prototype display strings. Clearing them lets
	// normalizeWorldFields rebuild them from the source-backed registry.
	state.Location = ""
	state.NavigationTarget = ""
}

// geographyPlanet resolves a planet through the shared registry, which the
// rules layer needs for the ship's place in its own star system.
func geographyPlanet(id string) (world.Planet, bool) {
	if !geographyRegistryOK {
		return world.Planet{}, false
	}
	return geographyRegistry.Planet(id)
}
