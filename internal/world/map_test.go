package world

import "testing"

func TestEveryDestinationHasAWalkableDecodedMap(t *testing.T) {
	// Twenty cells across, twelve down: the axis the source package calls the
	// row is the one the original's ground screen shows horizontally.
	if MapWidth() != 20 || MapHeight() != 12 {
		t.Fatalf("unexpected city map dimensions: %dx%d", MapWidth(), MapHeight())
	}
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, destination := range registry.Destinations() {
		cityMap, ok := CityMap(destination.CityID)
		if !ok {
			t.Fatalf("city %q has no decoded map", destination.CityID)
		}
		seen++
		// Every cell has to resolve to a semantic tile, and every city has to
		// be walkable somewhere — a map that decoded wrong would fail both.
		walkable := 0
		for y := 0; y < MapHeight(); y++ {
			for x := 0; x < MapWidth(); x++ {
				terrain, ok := TerrainAt(destination.CityID, x, y)
				if !ok || terrain == "" {
					t.Fatalf("city %q cell (%d,%d) has no terrain", destination.CityID, x, y)
				}
				if IsPassable(terrain) {
					walkable++
				}
			}
		}
		if walkable*2 < MapWidth()*MapHeight() {
			t.Fatalf("city %q has only %d walkable cells", destination.CityID, walkable)
		}
		_ = cityMap
	}
	// The registry drops the two placeholder city records, so 58 destinations
	// remain out of the 60 decoded maps.
	if seen != 58 {
		t.Fatalf("%d destinations have maps, want 58", seen)
	}
}

func TestTerrainAtRejectsCoordinatesOutsideTheMap(t *testing.T) {
	if _, ok := TerrainAt(StartDestinationID, -1, 0); ok {
		t.Fatal("negative x unexpectedly returned a terrain")
	}
	if _, ok := TerrainAt(StartDestinationID, MapWidth(), MapHeight()); ok {
		t.Fatal("out-of-bounds coordinate unexpectedly returned a terrain")
	}
	if _, ok := TerrainAt("unknown-city", 0, 0); ok {
		t.Fatal("an unknown city unexpectedly returned a terrain")
	}
}

// The landing square is the planet record's, not a walkable cell the remake
// picked: the original decides passability by pixel colour, so three of the
// cities land on a cell this shell classes as wall or water. What has to hold
// is that the cell is on the map and that the walker can step off it.
func TestLandingPositionIsInsideAndCanBeSteppedOffFrom(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	unwalkable := 0
	for _, destination := range registry.Destinations() {
		landing := LandingPosition(destination.CityID)
		if landing.X < 0 || landing.X >= MapWidth() || landing.Y < 0 || landing.Y >= MapHeight() {
			t.Fatalf("city %q lands at (%d,%d), outside the map", destination.CityID, landing.X, landing.Y)
		}
		if terrain, ok := TerrainAt(destination.CityID, landing.X, landing.Y); !ok || !IsPassable(terrain) {
			unwalkable++
		}
		neighbours := 0
		for _, delta := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			if terrain, ok := TerrainAt(destination.CityID, landing.X+delta[0], landing.Y+delta[1]); ok && IsPassable(terrain) {
				neighbours++
			}
		}
		if neighbours == 0 {
			t.Fatalf("city %q lands at (%d,%d) with no walkable neighbour", destination.CityID, landing.X, landing.Y)
		}
	}
	// Dramming and Aylika land on a wall, Tram-El on water. If that count
	// moves, the map or the record moved with it.
	if unwalkable != 3 {
		t.Fatalf("%d cities land on a cell this shell cannot stand on; want 3", unwalkable)
	}
}

func TestViewportOriginKeepsTheWindowInsideTheMap(t *testing.T) {
	cases := []struct{ x, y, wantX, wantY int }{
		{x: 0, y: 0, wantX: 0, wantY: 0},
		{x: 10, y: 6, wantX: 10 - ViewportWidth()/2, wantY: 6 - ViewportHeight()/2},
		{x: 19, y: 11, wantX: MapWidth() - ViewportWidth(), wantY: MapHeight() - ViewportHeight()},
	}
	for _, test := range cases {
		gotX, gotY := ViewportOrigin(test.x, test.y)
		if gotX != test.wantX || gotY != test.wantY {
			t.Fatalf("ViewportOrigin(%d,%d) = (%d,%d); want (%d,%d)",
				test.x, test.y, gotX, gotY, test.wantX, test.wantY)
		}
		if gotX+ViewportWidth() > MapWidth() || gotY+ViewportHeight() > MapHeight() {
			t.Fatalf("window at (%d,%d) runs past the map", gotX, gotY)
		}
	}
}

func TestCanEnterRequiresKnownCityAndPassableCell(t *testing.T) {
	registry, err := NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	landing := LandingPosition(StartDestinationID)
	if !registry.CanEnter(StartDestinationID, landing) {
		t.Fatal("the landing cell should be enterable")
	}
	blocked := TilePosition{}
	found := false
	for y := 0; y < MapHeight() && !found; y++ {
		for x := 0; x < MapWidth(); x++ {
			if terrain, ok := TerrainAt(StartDestinationID, x, y); ok && !IsPassable(terrain) {
				blocked = TilePosition{X: x, Y: y}
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("the start city has no blocked cell to test against")
	}
	if registry.CanEnter(StartDestinationID, blocked) {
		t.Fatalf("blocked cell (%d,%d) should not be enterable", blocked.X, blocked.Y)
	}
	if registry.CanEnter("unknown-city", landing) {
		t.Fatal("unknown city should not be enterable")
	}
}

// TestLandingIsThePlanetsLandingSquare: a new game lands where the planet
// record says (planet.landing-square), which for Drahew is the cell the
// original's live run started on.
func TestLandingIsThePlanetsLandingSquare(t *testing.T) {
	if got := LandingPosition(StartDestinationID); got != (TilePosition{X: 1, Y: 0}) {
		t.Fatalf("Drahew landing %+v, want (1,0)", got)
	}
}
