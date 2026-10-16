package world

import "testing"

// The three route maps are drawn from the source tables, so the positions they
// draw have to be present and inside the shape the original's own routines
// assume. A table that decoded wrong would put cities on top of each other or
// off the map, which is exactly what these check.
func TestEveryCityCarriesItsPlanetMapPosition(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	destinations := registry.Destinations()
	if len(destinations) == 0 {
		t.Fatal("no destinations")
	}
	for _, destination := range destinations {
		// change_xy never produces a coordinate below these, whatever the
		// packed fields are: x is at least 4 and y at least 2.
		if destination.MapX < 4 || destination.MapY < 2 {
			t.Fatalf("city %q maps to (%d,%d), which change_xy cannot produce",
				destination.CityID, destination.MapX, destination.MapY)
		}
		// The packed fields are six and five bits, so these are the ceilings.
		if destination.MapX > 7*32+7 || destination.MapY > 4*31+2 {
			t.Fatalf("city %q maps to (%d,%d), past what the packed fields allow",
				destination.CityID, destination.MapX, destination.MapY)
		}
	}
}

func TestCitiesOnAPlanetNeverShareAPosition(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	planets := 0
	for _, planet := range registry.Planets() {
		cities := registry.DestinationsOnPlanet(planet.ID)
		if len(cities) == 0 {
			continue
		}
		planets++
		seen := make(map[[2]int]string, len(cities))
		for _, city := range cities {
			key := [2]int{city.MapX, city.MapY}
			if previous, clash := seen[key]; clash {
				t.Fatalf("planet %q draws %q and %q at the same spot (%d,%d)",
					planet.ID, previous, city.CityID, city.MapX, city.MapY)
			}
			seen[key] = city.CityID
		}
	}
	if planets == 0 {
		t.Fatal("no planet has any city")
	}
}

func TestEverySystemCarriesThreeChartCoordinates(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	systems := registry.Systems()
	if len(systems) != 12 {
		t.Fatalf("%d systems, want 12", len(systems))
	}
	seen := make(map[[3]int]string, len(systems))
	for _, system := range systems {
		for axis, value := range system.Coordinates {
			if value < 0 || value > 15 {
				t.Fatalf("system %q axis %d is %d, outside four bits", system.ID, axis, value)
			}
		}
		// Two systems at the same point would be one circle on the chart.
		if previous, clash := seen[system.Coordinates]; clash {
			t.Fatalf("systems %q and %q share chart coordinates %v", previous, system.ID, system.Coordinates)
		}
		seen[system.Coordinates] = system.ID
	}
}

func TestEveryPlanetSitsOnAnOrbitTheSystemMapCanDraw(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, planet := range registry.Planets() {
		if planet.Orbit.Radius <= 0 {
			t.Fatalf("planet %q has orbit radius %d, which would sit on the star", planet.ID, planet.Orbit.Radius)
		}
		if planet.Orbit.Angle < 0 || planet.Orbit.Angle > 0xFF {
			t.Fatalf("planet %q has orbit angle %d, outside a byte", planet.ID, planet.Orbit.Angle)
		}
	}
}
