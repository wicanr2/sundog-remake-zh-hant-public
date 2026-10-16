package theme

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAtlasGroupDecodesDeclaredPaletteAndCrops(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	base, ok := registry.Group(AtariSTOriginal)
	if !ok {
		t.Fatal("Atari ST prototype group is missing")
	}
	tempDir := t.TempDir()
	atlas := image.NewRGBA(image.Rect(0, 0, 32, 16))
	fill := color.RGBA{R: 0x88, G: 0x99, B: 0xAA, A: 0xFF}
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			atlas.SetRGBA(x, y, fill)
		}
	}
	atlasPath := filepath.Join(tempDir, "normalized-atlas.png")
	atlasFile, err := os.Create(atlasPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(atlasFile, atlas); err != nil {
		_ = atlasFile.Close()
		t.Fatal(err)
	}
	if err := atlasFile.Close(); err != nil {
		t.Fatal(err)
	}

	manifest := AtlasManifest{
		Schema:       1,
		Theme:        AtariSTOriginal,
		Atlas:        "normalized-atlas.png",
		TileSize:     16,
		PixelScale:   2,
		Palette:      []string{"#000000", "#AA5511", "#FFFFFF", "#123456", "#223344", "#334455", "#445566", "#556677", "#667788", "#778899", "#8899AA", "#99AABB", "#AABBCC", "#BBCCDD", "#CCDDEE", "#DDEEFF"},
		UI:           &UIIndices{Background: 0, Foreground: 15, Accent: 12, Panel: 7},
		Provenance:   "source-free loader test",
		SourceStatus: "synthetic normalized atlas; semantic mapping is test-only",
		Tiles:        makeAtlasTileSpecs(base.SemanticTiles),
	}
	manifestPath := filepath.Join(tempDir, "theme.json")
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadAtlasGroup(manifestPath, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Palette) != 16 || loaded.TileSize != 16 {
		t.Fatalf("loaded atlas metadata = palette %d, tileSize %d", len(loaded.Palette), loaded.TileSize)
	}
	if got := loaded.Patterns[Ship].Rows[0]; got != "aaaaaaaa" {
		t.Fatalf("loaded crop row = %q, want hexadecimal palette index", got)
	}
	replaced, err := registry.WithAtariSTAtlas(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := replaced.Group(AtariSTOriginal); got.SourceStatus != manifest.SourceStatus {
		t.Fatalf("replacement tileset did not install atomically: %+v", got)
	}
	if original, _ := registry.Group(AtariSTOriginal); original.SourceStatus == manifest.SourceStatus {
		t.Fatal("installing the atlas mutated the original registry")
	}
	// The atlas brings its own sixteen colours, and every chrome has to be able
	// to wear it — that is what the tileset carrying its own palette is for.
	for _, chrome := range replaced.Chromes() {
		group, ok := replaced.Pair(chrome.ID, AtariSTTiles)
		if !ok {
			t.Fatalf("chrome %q cannot show the external Atari ST atlas", chrome.ID)
		}
		if len(group.TilePalette) != 16 {
			t.Fatalf("chrome %q resolved a %d-colour tile palette", chrome.ID, len(group.TilePalette))
		}
		if len(group.Palette) != len(chrome.Palette) {
			t.Fatalf("chrome %q lost its own palette to the atlas", chrome.ID)
		}
	}
}

func TestLoadAtlasGroupRejectsEscapingAtlasPath(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	base, ok := registry.Group(AtariSTOriginal)
	if !ok {
		t.Fatal("Atari ST prototype group is missing")
	}
	tempDir := t.TempDir()
	manifest := AtlasManifest{
		Schema:     1,
		Theme:      AtariSTOriginal,
		Atlas:      "../outside.png",
		TileSize:   16,
		PixelScale: 2,
		Palette:    []string{"#000000"},
		UI:         &UIIndices{Background: 0, Foreground: 0, Accent: 0, Panel: 0},
		Tiles:      makeAtlasTileSpecs(base.SemanticTiles),
	}
	manifestPath := filepath.Join(tempDir, "theme.json")
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAtlasGroup(manifestPath, base); err == nil {
		t.Fatal("escaping atlas path was accepted")
	}
}

func makeAtlasTileSpecs(semanticTiles []string) map[string]AtlasTile {
	specs := make(map[string]AtlasTile, len(semanticTiles))
	for index, semanticTile := range semanticTiles {
		specs[semanticTile] = AtlasTile{
			X:             (index % 4) * 8,
			Y:             (index / 4) * 8,
			Width:         8,
			Height:        8,
			Source:        "synthetic://normalized-atlas",
			EvidenceLevel: "hypothesis",
		}
	}
	return specs
}
