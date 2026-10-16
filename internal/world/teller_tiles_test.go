package world

import "testing"

// Drahew's three teller cells (terrain code 15) are Unitellers, as the original
// dispatches them (parity-playtest-report.md, second batch).
func TestTellerTilesAreUnitellers(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	destination, ok := registry.Destination(StartDestinationID)
	if !ok {
		t.Fatal("no start destination")
	}
	cityMap, ok := CityMap(destination.CityID)
	if !ok {
		t.Fatal("no Drahew map")
	}
	found := 0
	for row := 0; row < cityMapRows; row++ {
		for column := 0; column < cityMapColumns; column++ {
			code, ok := cityMap.Terrain(row, column)
			if !ok || code != tellerTerrainCode {
				continue
			}
			found++
			if !containsFacility(registry.FacilitiesAt(destination.CityID, TilePosition{X: row, Y: column}), FacilityUniteller) {
				t.Fatalf("teller cell (%d,%d) has no uniteller", row, column)
			}
		}
	}
	if found == 0 {
		t.Fatal("Drahew has no terrain-15 cells; the rule was written for three")
	}
}
