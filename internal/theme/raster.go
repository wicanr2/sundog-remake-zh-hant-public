package theme

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
)

// RasterAtlas is a complete high-resolution presentation group. It remains
// separate from PixelPattern because the latter is the existing 8×8 semantic
// prototype/fallback contract. A raster atlas never changes map identity,
// collision, anchors, secrets, rules, or save data.
type RasterAtlas struct {
	Schema          int                     `json:"schema"`
	AssetVersion    string                  `json:"assetVersion"`
	Theme           ID                      `json:"theme"`
	Atlas           string                  `json:"atlas"`
	AtlasSHA256     string                  `json:"atlasSha256"`
	AtlasWidth      int                     `json:"atlasWidth"`
	AtlasHeight     int                     `json:"atlasHeight"`
	CellSize        int                     `json:"cellSize"`
	LogicalTileSize int                     `json:"logicalTileSize"`
	Filter          string                  `json:"filter"`
	Provenance      string                  `json:"provenance"`
	SourceStatus    string                  `json:"sourceStatus"`
	Boundaries      map[string]string       `json:"boundaries"`
	Build           RasterBuild             `json:"build"`
	SourceArtifacts map[string]RasterSource `json:"sourceArtifacts"`
	Tiles           map[string]RasterTile   `json:"tiles"`

	png []byte
}

// RasterBuild records the deterministic local transformation boundary. The
// image-generation inputs themselves remain external, but their individual
// hashes are retained in RasterSource for audit and drift detection.
type RasterBuild struct {
	Tool                       string `json:"tool"`
	ChromaKey                  string `json:"chromaKey"`
	ChromaKeyHelper            string `json:"chromaKeyHelper"`
	SourceArtifactAvailability string `json:"sourceArtifactAvailability"`
}

type RasterSource struct {
	Basename                string           `json:"basename"`
	SHA256                  string           `json:"sha256"`
	Kind                    string           `json:"kind"`
	Transform               string           `json:"transform"`
	ChromaKey               string           `json:"chromaKey,omitempty"`
	ChromaKeyResidualPixels int              `json:"chromaKeyResidualPixels,omitempty"`
	Alpha                   RasterAlphaStats `json:"alpha"`
	VisibleBounds           []int            `json:"visibleBounds,omitempty"`
}

type RasterAlphaStats struct {
	TransparentPixels  int `json:"transparentPixels"`
	PartialAlphaPixels int `json:"partialAlphaPixels"`
	OpaquePixels       int `json:"opaquePixels"`
}

// RasterTile is one explicitly mapped high-resolution cell. Source and
// evidenceLevel are retained alongside the crop so a polished redraw does not
// silently become a claim about unknown original graphics bytes.
type RasterTile struct {
	X             int    `json:"x"`
	Y             int    `json:"y"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Source        string `json:"source"`
	EvidenceLevel string `json:"evidenceLevel"`
	Layer         string `json:"layer"`
}

// PNG returns immutable-by-convention atlas bytes for a presentation shell to
// decode. Callers must not mutate the returned slice.
func (a *RasterAtlas) PNG() []byte {
	if a == nil {
		return nil
	}
	return a.png
}

func (a *RasterAtlas) Tile(semantic string) (RasterTile, bool) {
	if a == nil {
		return RasterTile{}, false
	}
	tile, ok := a.Tiles[semantic]
	return tile, ok
}

func (a *RasterAtlas) Validate(themeID ID, semanticTiles []string) error {
	if a == nil {
		return nil
	}
	if a.Schema != 1 {
		return fmt.Errorf("raster atlas schema %d is unsupported", a.Schema)
	}
	if a.Theme != themeID || a.Theme == "" {
		return fmt.Errorf("raster atlas theme %q does not match group theme %q", a.Theme, themeID)
	}
	if a.AssetVersion == "" || a.Atlas == "" || a.Provenance == "" || a.SourceStatus == "" {
		return fmt.Errorf("raster atlas has incomplete identity or provenance")
	}
	if len(a.Boundaries) == 0 || a.Build.Tool == "" || len(a.SourceArtifacts) != len(semanticTiles) {
		return fmt.Errorf("raster atlas has incomplete audit metadata")
	}
	if a.AtlasWidth <= 0 || a.AtlasHeight <= 0 || a.CellSize <= 0 || a.LogicalTileSize <= 0 {
		return fmt.Errorf("raster atlas has invalid dimensions")
	}
	if a.Filter != "linear" {
		return fmt.Errorf("raster atlas filter %q is unsupported", a.Filter)
	}
	if len(a.png) == 0 {
		return fmt.Errorf("raster atlas has no embedded PNG")
	}
	actualHash := sha256.Sum256(a.png)
	if got := hex.EncodeToString(actualHash[:]); got != a.AtlasSHA256 {
		return fmt.Errorf("raster atlas SHA-256 mismatch: got %s", got)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(a.png))
	if err != nil {
		return fmt.Errorf("decode raster atlas: %w", err)
	}
	if format != "png" || config.Width != a.AtlasWidth || config.Height != a.AtlasHeight {
		return fmt.Errorf("raster atlas PNG dimensions are %dx%d %q, want %dx%d png", config.Width, config.Height, format, a.AtlasWidth, a.AtlasHeight)
	}
	if len(a.Tiles) != len(semanticTiles) {
		return fmt.Errorf("raster atlas has %d semantic tiles, want %d", len(a.Tiles), len(semanticTiles))
	}
	for _, semantic := range semanticTiles {
		tile, ok := a.Tiles[semantic]
		if !ok {
			return fmt.Errorf("raster atlas is missing semantic tile %q", semantic)
		}
		if tile.X < 0 || tile.Y < 0 || tile.Width != a.CellSize || tile.Height != a.CellSize {
			return fmt.Errorf("raster tile %q has invalid cell %dx%d+%d+%d", semantic, tile.Width, tile.Height, tile.X, tile.Y)
		}
		if tile.X+tile.Width > a.AtlasWidth || tile.Y+tile.Height > a.AtlasHeight {
			return fmt.Errorf("raster tile %q exceeds atlas bounds", semantic)
		}
		if tile.Source == "" || !validEvidenceLevel(tile.EvidenceLevel) {
			return fmt.Errorf("raster tile %q has incomplete provenance", semantic)
		}
		if tile.Layer != "terrain" && tile.Layer != "sprite" {
			return fmt.Errorf("raster tile %q has unsupported layer %q", semantic, tile.Layer)
		}
		source, ok := a.SourceArtifacts[semantic]
		if !ok || source.Basename == "" || len(source.SHA256) != 64 || source.Transform == "" {
			return fmt.Errorf("raster tile %q has incomplete source audit data", semantic)
		}
		if source.Kind != tile.Layer {
			return fmt.Errorf("raster tile %q layer %q does not match source kind %q", semantic, tile.Layer, source.Kind)
		}
		if tile.Layer == "sprite" && source.ChromaKey == "" {
			return fmt.Errorf("raster sprite %q has no chroma-key audit data", semantic)
		}
	}
	return nil
}

//go:embed assets/modern-apple/modern-apple-atlas.png
var modernAppleAtlasPNG []byte

//go:embed assets/modern-apple/modern-apple-atlas-manifest.json
var modernAppleAtlasManifest []byte

func embeddedModernAppleRaster(semanticTiles []string) (*RasterAtlas, error) {
	return decodeEmbeddedRaster(modernAppleAtlasManifest, modernAppleAtlasPNG, ModernApple, semanticTiles)
}

func decodeEmbeddedRaster(manifest, png []byte, themeID ID, semanticTiles []string) (*RasterAtlas, error) {
	var atlas RasterAtlas
	decoder := json.NewDecoder(bytes.NewReader(manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&atlas); err != nil {
		return nil, fmt.Errorf("decode embedded %s raster manifest: %w", themeID, err)
	}
	atlas.png = png
	if err := atlas.Validate(themeID, semanticTiles); err != nil {
		return nil, fmt.Errorf("validate embedded %s raster atlas: %w", themeID, err)
	}
	return &atlas, nil
}
