package theme

import "testing"

func TestPrototypeRegistryCyclesCompleteGroups(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Cycling has to visit every registered chrome once and come back to where
	// it started, whatever the registry happens to hold.
	ids := registry.IDs()
	current := ids[0]
	for step := 1; step <= len(ids); step++ {
		current, err = registry.Cycle(current)
		if err != nil {
			t.Fatal(err)
		}
		want := ids[step%len(ids)]
		if current != want {
			t.Fatalf("cycled to %q, want %q", current, want)
		}
		group, ok := registry.Group(current)
		if !ok || len(group.SemanticTiles) != 8 {
			t.Fatalf("theme %q is not a complete semantic group", current)
		}
	}

	// The tileset is the other axis and cycles on its own.
	tilesets := registry.TilesetIDs()
	tile := tilesets[0]
	for step := 1; step <= len(tilesets); step++ {
		tile, err = registry.CycleTileset(tile)
		if err != nil {
			t.Fatal(err)
		}
		if want := tilesets[step%len(tilesets)]; tile != want {
			t.Fatalf("cycled tileset to %q, want %q", tile, want)
		}
	}

	// Every pair has to resolve, which is what makes the two axes independent
	// rather than a list of blessed combinations.
	for _, chromeID := range ids {
		for _, tilesetID := range tilesets {
			if _, ok := registry.Pair(chromeID, tilesetID); !ok {
				t.Fatalf("chrome %q cannot show tileset %q", chromeID, tilesetID)
			}
		}
	}
}

func TestUnknownThemeRejected(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Cycle("unknown"); err == nil {
		t.Fatal("expected unknown theme error")
	}
}

func TestModernApplePreservesAppleIISemanticMasks(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	apple, ok := registry.Group(AppleIIOriginal)
	if !ok {
		t.Fatal("Apple II theme is missing")
	}
	modern, ok := registry.Pair(AppleIIOriginal, ModernTiles)
	if !ok {
		t.Fatal("Modern Apple theme is missing")
	}
	changed := false
	// Only these prototype sprites use palette index 0 as transparent background.
	// Terrain patterns may legitimately use index 0 as a visible palette color.
	spriteTiles := map[string]bool{Ship: true, Cargo: true, City: true, Player: true}
	for _, tile := range apple.SemanticTiles {
		applePattern := apple.Patterns[tile]
		modernPattern := modern.Patterns[tile]
		if len(applePattern.Rows) != len(modernPattern.Rows) {
			t.Fatalf("tile %q changed row count", tile)
		}
		for row := range applePattern.Rows {
			if len(applePattern.Rows[row]) != len(modernPattern.Rows[row]) {
				t.Fatalf("tile %q row %d changed width", tile, row)
			}
			for column := range applePattern.Rows[row] {
				appleOccupied := applePattern.Rows[row][column] != '0'
				modernOccupied := modernPattern.Rows[row][column] != '0'
				if applePattern.Rows[row][column] != modernPattern.Rows[row][column] {
					changed = true
				}
				if spriteTiles[tile] && appleOccupied != modernOccupied {
					t.Fatalf("tile %q pixel (%d,%d) changed semantic occupancy", tile, column, row)
				}
			}
		}
	}
	if !changed {
		t.Fatal("Modern Apple theme did not redraw any Apple II pixels")
	}
}

func TestModernAppleIsDeterministicallyDerivedFromAppleII(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	apple, ok := registry.Group(AppleIIOriginal)
	if !ok {
		t.Fatal("Apple II theme is missing")
	}
	modern, ok := registry.Pair(AppleIIOriginal, ModernTiles)
	if !ok {
		t.Fatal("Modern Apple theme is missing")
	}
	derived := redesignModernPatterns(apple.Patterns)
	for _, tile := range apple.SemanticTiles {
		got := modern.Patterns[tile]
		want := derived[tile]
		if len(got.Rows) != len(want.Rows) {
			t.Fatalf("tile %q row count = %d, want %d", tile, len(got.Rows), len(want.Rows))
		}
		for row := range want.Rows {
			if got.Rows[row] != want.Rows[row] {
				t.Fatalf("tile %q row %d = %q, want deterministic derivation %q", tile, row, got.Rows[row], want.Rows[row])
			}
		}
	}
}

func TestModernAppleSpriteEdgesUseContourPalette(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	apple, ok := registry.Group(AppleIIOriginal)
	if !ok {
		t.Fatal("Apple II theme is missing")
	}
	modern, ok := registry.Pair(AppleIIOriginal, ModernTiles)
	if !ok {
		t.Fatal("Modern Apple theme is missing")
	}
	for _, tile := range []string{Ship, Cargo, City, Player} {
		source := apple.Patterns[tile]
		target := modern.Patterns[tile]
		contourCount := 0
		for y, row := range source.Rows {
			for x := range row {
				if !modernSpriteEdge(source, x, y) {
					continue
				}
				contourCount++
				if target.Rows[y][x] != '3' && target.Rows[y][x] != '4' {
					t.Fatalf("tile %q edge pixel (%d,%d) = %q, want contour palette 3 or 4", tile, x, y, target.Rows[y][x])
				}
			}
		}
		if contourCount == 0 {
			t.Fatalf("tile %q has no source edge pixels", tile)
		}
	}
}

func TestModernApplePrototypeUsesOnlyDeclaredPaletteIndices(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	modern, ok := registry.Pair(AppleIIOriginal, ModernTiles)
	if !ok {
		t.Fatal("Modern Apple theme is missing")
	}
	if err := modern.Validate(); err != nil {
		t.Fatal(err)
	}
	for tile, pattern := range modern.Patterns {
		for y, row := range pattern.Rows {
			for x := range row {
				index, ok := PaletteIndex(row[x])
				if !ok || index >= len(modern.Palette) {
					t.Fatalf("tile %q pixel (%d,%d) has invalid palette index %q", tile, x, y, row[x])
				}
			}
		}
	}
}

func TestModernAppleProvidesACompleteValidatedHighResolutionRasterAtlas(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	modern, ok := registry.Pair(AppleIIOriginal, ModernTiles)
	if !ok {
		t.Fatal("Modern Apple theme is missing")
	}
	if modern.Raster == nil {
		t.Fatal("Modern Apple official raster atlas is missing")
	}
	if err := modern.Raster.Validate(ModernApple, modern.SemanticTiles); err != nil {
		t.Fatalf("official raster atlas failed validation: %v", err)
	}
	if got, want := modern.Raster.CellSize, 128; got != want {
		t.Fatalf("raster cell size = %d, want %d", got, want)
	}
	if got, want := modern.Raster.LogicalTileSize, 20; got != want {
		t.Fatalf("raster logical tile size = %d, want %d", got, want)
	}
	if len(modern.Raster.PNG()) == 0 {
		t.Fatal("official raster PNG is empty")
	}
	for _, tile := range modern.SemanticTiles {
		spec, ok := modern.Raster.Tile(tile)
		if !ok {
			t.Fatalf("official raster is missing %q", tile)
		}
		if spec.Width != modern.Raster.CellSize || spec.Height != modern.Raster.CellSize {
			t.Fatalf("raster %q cell = %dx%d", tile, spec.Width, spec.Height)
		}
		if spec.EvidenceLevel != "hypothesis" {
			t.Fatalf("raster %q evidence = %q, want explicit original-semantic hypothesis", tile, spec.EvidenceLevel)
		}
	}
}

func TestBuiltInPrototypeTileSizeMatchesEightByEightRendererContract(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range registry.IDs() {
		group, ok := registry.Group(id)
		if !ok {
			t.Fatalf("theme %q is missing", id)
		}
		if group.TileSize != 16 {
			t.Fatalf("prototype theme %q advertises tile size %d; want 16 for 8×8 patterns at 2×", id, group.TileSize)
		}
		for _, tile := range group.SemanticTiles {
			pattern := group.Patterns[tile]
			if len(pattern.Rows) != 8 || len(pattern.Rows[0]) != 8 {
				t.Fatalf("prototype theme %q tile %q is not the current 8×8 contract", id, tile)
			}
		}
	}
}
