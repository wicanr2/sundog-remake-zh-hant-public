package theme

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// AtlasManifest describes a user-supplied, normalized theme atlas. The atlas
// is deliberately separate from the original disk/image formats: a researcher
// must first decode and map each candidate frame to a semantic tile, then can
// opt into this runtime presentation contract.
type AtlasManifest struct {
	Schema        int                  `json:"schema"`
	Theme         ID                   `json:"theme"`
	Atlas         string               `json:"atlas"`
	TileSize      int                  `json:"tileSize"`
	PixelScale    int                  `json:"pixelScale"`
	Palette       []string             `json:"palette"`
	UI            *UIIndices           `json:"ui"`
	Tiles         map[string]AtlasTile `json:"tiles"`
	Provenance    string               `json:"provenance"`
	SourceStatus  string               `json:"sourceStatus"`
	EvidenceNotes map[string]string    `json:"evidenceNotes,omitempty"`
}

// AtlasTile identifies one crop in the normalized atlas and preserves the
// evidence boundary that led to the mapping.
type AtlasTile struct {
	X             int    `json:"x"`
	Y             int    `json:"y"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Source        string `json:"source"`
	EvidenceLevel string `json:"evidenceLevel"`
}

// LoadAtlasGroup validates and decodes an external PNG atlas into a complete
// theme group. The source image is never copied or embedded in the binary.
// Current presentation code uses 8×8 logical patterns at a 2× scale; the
// manifest makes that normalization explicit instead of silently resizing.
func LoadAtlasGroup(manifestPath string, base Group) (Group, error) {
	manifestFile, err := os.Open(manifestPath)
	if err != nil {
		return Group{}, fmt.Errorf("open atlas manifest %s: %w", manifestPath, err)
	}
	defer manifestFile.Close()

	var manifest AtlasManifest
	decoder := json.NewDecoder(manifestFile)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Group{}, fmt.Errorf("decode atlas manifest %s: %w", manifestPath, err)
	}
	if err := manifest.validate(base); err != nil {
		return Group{}, err
	}

	root := filepath.Dir(manifestPath)
	atlasPath, err := resolveAtlasPath(root, manifest.Atlas)
	if err != nil {
		return Group{}, err
	}
	atlasFile, err := os.Open(atlasPath)
	if err != nil {
		return Group{}, fmt.Errorf("open atlas image %s: %w", atlasPath, err)
	}
	defer atlasFile.Close()
	atlas, format, err := image.Decode(atlasFile)
	if err != nil {
		return Group{}, fmt.Errorf("decode atlas image %s: %w", atlasPath, err)
	}
	if format != "png" {
		return Group{}, fmt.Errorf("atlas %s has format %q; only PNG is accepted", atlasPath, format)
	}

	palette, err := parseAtlasPalette(manifest.Palette)
	if err != nil {
		return Group{}, fmt.Errorf("atlas %s palette: %w", manifestPath, err)
	}
	patterns := make(map[string]PixelPattern, len(base.SemanticTiles))
	for _, semanticTile := range base.SemanticTiles {
		spec := manifest.Tiles[semanticTile]
		pattern, err := decodeAtlasTile(atlas, palette, spec, semanticTile, manifest.PixelScale)
		if err != nil {
			return Group{}, fmt.Errorf("atlas tile %q: %w", semanticTile, err)
		}
		patterns[semanticTile] = pattern
	}

	group := base
	group.TileSize = manifest.TileSize
	// An external atlas brings its own colours for the tiles and its own choice
	// of which of them the interface uses. Both stay together in the loaded
	// group; installing it as a tileset keeps the tile half only.
	group.Palette = palette
	group.TilePalette = palette
	group.UI = *manifest.UI
	group.Patterns = patterns
	if manifest.Provenance != "" {
		group.Provenance = manifest.Provenance
	}
	if manifest.SourceStatus != "" {
		group.SourceStatus = manifest.SourceStatus
	}
	if err := group.Validate(); err != nil {
		return Group{}, fmt.Errorf("loaded atlas group: %w", err)
	}
	return group, nil
}

// WithAtariSTAtlas prepares a registry whose Atari ST tiles come from a
// caller-supplied normalized atlas. The returned registry is new; a failure
// leaves the caller's prototype registry unchanged. Only the tileset changes —
// the atlas is art, and which chrome the player is wearing is a separate axis.
func (r Registry) WithAtariSTAtlas(manifestPath string) (Registry, error) {
	base, ok := r.Group(AtariSTOriginal)
	if !ok {
		return Registry{}, fmt.Errorf("Atari ST theme is not registered")
	}
	group, err := LoadAtlasGroup(manifestPath, base)
	if err != nil {
		return Registry{}, err
	}
	return r.ReplaceTileset(Tileset{
		ID:             AtariSTTiles,
		DisplayNameKey: "ui.tileset.atari_st",
		Provenance:     group.Provenance,
		SourceStatus:   group.SourceStatus,
		TileSize:       group.TileSize,
		SemanticTiles:  group.SemanticTiles,
		Patterns:       group.Patterns,
		Raster:         group.Raster,
		RasterThemeID:  group.RasterThemeID,
		Palette:        group.Palette,
		PaletteFloor:   len(group.Palette),
	})
}

func (manifest AtlasManifest) validate(base Group) error {
	if manifest.Schema != 1 {
		return fmt.Errorf("atlas manifest schema %d is unsupported", manifest.Schema)
	}
	if manifest.Theme == "" || manifest.Theme != base.ID {
		return fmt.Errorf("atlas theme %q does not match base theme %q", manifest.Theme, base.ID)
	}
	if manifest.Atlas == "" {
		return fmt.Errorf("atlas image path is empty")
	}
	if manifest.TileSize != 16 || manifest.PixelScale != 2 {
		return fmt.Errorf("atlas tileSize and pixelScale must be 16 and 2")
	}
	if len(manifest.Palette) == 0 || len(manifest.Palette) > 16 {
		return fmt.Errorf("atlas palette size %d is outside 1..16", len(manifest.Palette))
	}
	if manifest.UI == nil {
		return fmt.Errorf("atlas manifest must declare ui palette indices")
	}
	if err := manifest.UI.validate(len(manifest.Palette)); err != nil {
		return fmt.Errorf("atlas ui: %w", err)
	}
	if len(manifest.Tiles) != len(base.SemanticTiles) {
		return fmt.Errorf("atlas has %d tile mappings; want exactly %d", len(manifest.Tiles), len(base.SemanticTiles))
	}
	for _, semanticTile := range base.SemanticTiles {
		spec, ok := manifest.Tiles[semanticTile]
		if !ok {
			return fmt.Errorf("atlas is missing semantic tile %q", semanticTile)
		}
		basePattern, ok := base.Patterns[semanticTile]
		if !ok || len(basePattern.Rows) == 0 {
			return fmt.Errorf("base theme is missing pattern %q", semanticTile)
		}
		if spec.Width != len(basePattern.Rows[0]) || spec.Height != len(basePattern.Rows) {
			return fmt.Errorf("tile %q crop is %dx%d; normalized base pattern is %dx%d", semanticTile, spec.Width, spec.Height, len(basePattern.Rows[0]), len(basePattern.Rows))
		}
		if spec.Width*manifest.PixelScale != manifest.TileSize || spec.Height*manifest.PixelScale != manifest.TileSize {
			return fmt.Errorf("tile %q crop %dx%d does not match tileSize=%d at pixelScale=%d", semanticTile, spec.Width, spec.Height, manifest.TileSize, manifest.PixelScale)
		}
		if spec.Source == "" || !validEvidenceLevel(spec.EvidenceLevel) {
			return fmt.Errorf("tile %q must retain source and a valid evidenceLevel", semanticTile)
		}
	}
	return nil
}

func validEvidenceLevel(value string) bool {
	switch strings.TrimSpace(value) {
	case "proven", "strong-inference", "hypothesis", "unknown":
		return true
	default:
		return false
	}
}

func resolveAtlasPath(root, relative string) (string, error) {
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("atlas path %q must be relative to its manifest", relative)
	}
	clean := filepath.Clean(relative)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("atlas path %q escapes its manifest directory", relative)
	}
	return filepath.Join(root, clean), nil
}

func parseAtlasPalette(values []string) ([]color.RGBA, error) {
	palette := make([]color.RGBA, len(values))
	for index, value := range values {
		clean := strings.TrimPrefix(value, "#")
		if len(clean) == 6 {
			clean += "ff"
		}
		if len(clean) != 8 {
			return nil, fmt.Errorf("color %q must be #RRGGBB or #RRGGBBAA", value)
		}
		decoded, err := hex.DecodeString(clean)
		if err != nil {
			return nil, fmt.Errorf("color %q: %w", value, err)
		}
		palette[index] = color.RGBA{R: decoded[0], G: decoded[1], B: decoded[2], A: decoded[3]}
	}
	return palette, nil
}

func decodeAtlasTile(atlas image.Image, palette []color.RGBA, spec AtlasTile, semanticTile string, pixelScale int) (PixelPattern, error) {
	if spec.X < 0 || spec.Y < 0 || spec.Width <= 0 || spec.Height <= 0 {
		return PixelPattern{}, fmt.Errorf("invalid crop rectangle")
	}
	if spec.X+spec.Width > atlas.Bounds().Dx() || spec.Y+spec.Height > atlas.Bounds().Dy() {
		return PixelPattern{}, fmt.Errorf("crop %dx%d+%d+%d is outside atlas bounds %dx%d", spec.Width, spec.Height, spec.X, spec.Y, atlas.Bounds().Dx(), atlas.Bounds().Dy())
	}
	if pixelScale <= 0 {
		return PixelPattern{}, fmt.Errorf("invalid pixelScale %d", pixelScale)
	}
	rows := make([]string, spec.Height)
	const digits = "0123456789abcdef"
	for y := 0; y < spec.Height; y++ {
		row := make([]byte, spec.Width)
		for x := 0; x < spec.Width; x++ {
			red, green, blue, alpha := atlas.At(spec.X+x, spec.Y+y).RGBA()
			actual := color.RGBA{R: uint8(red >> 8), G: uint8(green >> 8), B: uint8(blue >> 8), A: uint8(alpha >> 8)}
			index := -1
			for candidate, expected := range palette {
				if actual == expected {
					index = candidate
					break
				}
			}
			if index < 0 {
				return PixelPattern{}, fmt.Errorf("pixel (%d,%d) color %#v is not in declared palette", x, y, actual)
			}
			row[x] = digits[index]
		}
		rows[y] = string(row)
	}
	return PixelPattern{Rows: rows}, nil
}
