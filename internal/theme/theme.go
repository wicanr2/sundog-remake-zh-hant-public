// Package theme owns complete, atomically swappable presentation groups.
package theme

import (
	"fmt"
	"image/color"
)

type ID string

const (
	AppleIIOriginal ID = "apple-ii-original"
	AtariSTOriginal ID = "atari-st-original"
	// ModernApple is the identity the Remake Modern raster manifest declares.
	// It is not a chrome: the Modern Apple chromes were retired on 2026-08-30
	// (the interface colours are the two originals'), and the art batch keeps
	// its audited name so a mismatched manifest cannot be loaded silently. It
	// deliberately has no entry in chromes().
	ModernApple ID = "modern-apple"
)

// retiredChromes are chrome names an older save or settings file may still
// carry, and what they resolve to now.
var retiredChromes = map[ID]ID{
	"modern-apple":       AppleIIOriginal,
	"modern-apple-cream": AppleIIOriginal,
	"modern-apple-dark":  AppleIIOriginal,
	"modern-apple-port":  AppleIIOriginal,
}

// MigrateChrome maps a retired chrome name to the chrome that stands in for
// it; any other name comes back unchanged.
func MigrateChrome(id ID) ID {
	if to, ok := retiredChromes[id]; ok {
		return to
	}
	return id
}

// retiredTilesets are tileset names an older save may carry: the Modern
// Apple tiles became Remake Modern, and the spaceport batch was retired
// into it.
var retiredTilesets = map[TilesetID]TilesetID{
	"modern-apple-tiles":      ModernTiles,
	"modern-apple-port-tiles": ModernTiles,
}

// MigrateTileset maps a retired tileset name to the current one.
func MigrateTileset(id TilesetID) TilesetID {
	if to, ok := retiredTilesets[id]; ok {
		return to
	}
	return id
}

// TilesetID names a tile art set. It is a separate axis from the chrome: any
// chrome can be worn over any tileset, which is what lets a player keep the
// Apple II tiles under a modern panel or the other way round.
type TilesetID string

// LayoutID names the third presentation axis: where the boxes, buttons and
// readouts are and how they are drawn. The original's layout is the default;
// the remake's own is the 2026-08-24 arrangement kept as an option.
type LayoutID string

const (
	AtariSTLayout LayoutID = "atari-st"
	RemakeLayout  LayoutID = "remake"
)

// Layouts lists the layouts in options order.
func Layouts() []LayoutID { return []LayoutID{AtariSTLayout, RemakeLayout} }

// LayoutNameKey is the catalog key of a layout's display name.
func LayoutNameKey(id LayoutID) string {
	if id == RemakeLayout {
		return "ui.layout.remake"
	}
	return "ui.layout.atari_st"
}

// KnownLayout reports whether id is one of Layouts.
func KnownLayout(id LayoutID) bool {
	for _, known := range Layouts() {
		if known == id {
			return true
		}
	}
	return false
}

const (
	AppleIITiles   TilesetID = "apple-ii-tiles"
	AtariSTTiles   TilesetID = "atari-st-tiles"
	AtariSTHDTiles TilesetID = "atari-st-hd-tiles"
	// ModernTiles is the remake's own high-resolution art, drawn from the
	// Apple II semantics. Its raster manifest still declares the batch as
	// ModernApple, the name it was audited under.
	ModernTiles TilesetID = "remake-modern-tiles"
)

const (
	Grass  = "grass"
	Water  = "water"
	Road   = "road"
	Wall   = "wall"
	Ship   = "ship"
	Cargo  = "cargo"
	City   = "city"
	Player = "player"
)

// Chrome is the colour language of the interface: the palette and which of its
// slots the shared renderer uses for background, text, accents and panels.
type Chrome struct {
	ID             ID
	DisplayNameKey string
	Provenance     string
	SourceStatus   string
	UI             UIIndices
	Palette        []color.RGBA
}

// Tileset is the art the map is drawn from. It carries no colours of its own
// beyond palette indices, which is what makes it independent of the chrome.
type Tileset struct {
	ID             TilesetID
	DisplayNameKey string
	Provenance     string
	SourceStatus   string
	TileSize       int
	SemanticTiles  []string
	Patterns       map[string]PixelPattern
	Raster         *RasterAtlas
	// Palette is the tile art's own colours. A tileset that brings its own —
	// an external 16-colour atlas, say — is showable under any chrome, because
	// the chrome only colours the interface. A tileset without one borrows the
	// chrome's palette, which is what the hand-written prototype tiles do.
	Palette []color.RGBA
	// RasterThemeID is the identity the raster manifest declares. The manifest
	// is an audited asset and predates the split into two axes, so the tileset
	// carries the name the manifest uses rather than the manifest being edited.
	RasterThemeID ID
	// PaletteFloor is how many palette entries the patterns index into. A
	// chrome with a shorter palette cannot show this tileset, so the registry
	// requires every chrome to reach the tallest floor rather than letting an
	// unshowable pair exist.
	PaletteFloor int
}

// Group is a resolved chrome and tileset. Everything that draws takes one of
// these, so the two axes are a registry concern rather than a renderer one.
type Group struct {
	ID            ID
	TilesetID     TilesetID
	RasterThemeID ID
	// DisplayNameKey names the chrome and TilesetNameKey the tile art. Both are
	// carried because the player picks them separately and the footer has to say
	// which of each is on.
	DisplayNameKey string
	TilesetNameKey string
	Provenance     string
	SourceStatus   string
	TileSize       int
	UI             UIIndices
	SemanticTiles  []string
	// Palette is the chrome's; UI indexes into it.
	Palette []color.RGBA
	// TilePalette is what the patterns index into. It is the tileset's own when
	// it has one and the chrome's otherwise.
	TilePalette []color.RGBA
	Patterns    map[string]PixelPattern
	Raster      *RasterAtlas
}

// Resolve pairs a chrome with a tileset. It is the only way a Group is built,
// so an unshowable pair cannot reach a renderer.
func Resolve(chrome Chrome, tileset Tileset) (Group, error) {
	group := Group{
		ID:             chrome.ID,
		TilesetID:      tileset.ID,
		RasterThemeID:  tileset.RasterThemeID,
		DisplayNameKey: chrome.DisplayNameKey,
		TilesetNameKey: tileset.DisplayNameKey,
		Provenance:     chrome.Provenance + " / " + tileset.Provenance,
		SourceStatus:   tileset.SourceStatus,
		TileSize:       tileset.TileSize,
		UI:             chrome.UI,
		SemanticTiles:  tileset.SemanticTiles,
		Palette:        chrome.Palette,
		TilePalette:    tileset.Palette,
		Patterns:       tileset.Patterns,
		Raster:         tileset.Raster,
	}
	if len(group.TilePalette) == 0 {
		group.TilePalette = chrome.Palette
	}
	if err := group.Validate(); err != nil {
		return Group{}, err
	}
	return group, nil
}

// UIIndices names the palette slots used by the shared renderer for chrome.
// Source atlas colors remain independent; an Atari ST 16-colour palette may
// use different indices for readable text and panel accents.
type UIIndices struct {
	Background int `json:"background"`
	Foreground int `json:"foreground"`
	Accent     int `json:"accent"`
	Panel      int `json:"panel"`
}

func (g Group) Validate() error {
	if g.ID == "" {
		return fmt.Errorf("theme id is empty")
	}
	if g.TilesetID == "" {
		return fmt.Errorf("theme %q has no tileset", g.ID)
	}
	if g.TileSize <= 0 {
		return fmt.Errorf("theme %q has invalid tile size", g.ID)
	}
	if len(g.SemanticTiles) == 0 {
		return fmt.Errorf("theme %q has no semantic tiles", g.ID)
	}
	if len(g.Palette) == 0 {
		return fmt.Errorf("theme %q has no palette", g.ID)
	}
	if err := g.UI.validate(len(g.Palette)); err != nil {
		return fmt.Errorf("theme %q: %w", g.ID, err)
	}
	if len(g.TilePalette) == 0 {
		return fmt.Errorf("theme %q has no tile palette", g.ID)
	}
	for _, tile := range g.SemanticTiles {
		pattern, ok := g.Patterns[tile]
		if !ok {
			return fmt.Errorf("theme %q is missing semantic tile %q", g.ID, tile)
		}
		if err := pattern.Validate(len(g.TilePalette)); err != nil {
			return fmt.Errorf("theme %q tile %q: %w", g.ID, tile, err)
		}
	}
	if err := g.Raster.Validate(g.RasterThemeID, g.SemanticTiles); err != nil {
		return fmt.Errorf("theme %q raster atlas: %w", g.ID, err)
	}
	return nil
}

func (ui UIIndices) validate(paletteSize int) error {
	for name, index := range map[string]int{
		"background": ui.Background,
		"foreground": ui.Foreground,
		"accent":     ui.Accent,
		"panel":      ui.Panel,
	} {
		if index < 0 || index >= paletteSize {
			return fmt.Errorf("UI %s palette index %d is outside palette size %d", name, index, paletteSize)
		}
	}
	return nil
}

type Registry struct {
	chromes     []Chrome
	tilesets    []Tileset
	byChrome    map[ID]Chrome
	byTileset   map[TilesetID]Tileset
	defaultTile map[ID]TilesetID
}

// appleIIPalette is the four colours the Apple II theme has always used. The
// fifth entry is not one of them: it exists only so a tileset that indexes five
// palette slots can still be shown under this chrome, and it is a darkened
// blend rather than a claim about the original.
func appleIIPalette() []color.RGBA {
	return []color.RGBA{
		{0, 0, 0, 255}, {255, 255, 255, 255}, {28, 108, 44, 255}, {201, 171, 62, 255},
		{20, 74, 110, 255},
	}
}

func atariSTPalette() []color.RGBA {
	return []color.RGBA{
		{8, 12, 24, 255}, {239, 239, 230, 255}, {42, 111, 154, 255}, {195, 82, 59, 255},
		{24, 62, 92, 255},
	}
}

// chromes are the interface colour languages: the two originals'. The Modern
// Apple chromes (base, cream, dark) were retired on 2026-08-30; the remake's
// own art lives on the tileset axis only.
func chromes() []Chrome {
	return []Chrome{
		{
			ID: AppleIIOriginal, DisplayNameKey: "ui.theme.apple2",
			Provenance: "Apple II pixel semantics", SourceStatus: "prototype; original tileset consumer pending",
			UI: UIIndices{Background: 0, Foreground: 1, Accent: 3, Panel: 2}, Palette: appleIIPalette(),
		},
		{
			ID: AtariSTOriginal, DisplayNameKey: "ui.theme.atari_st",
			Provenance: "Atari ST theme boundary", SourceStatus: "prototype; with -atari-disk the original tiles are decoded and drawn at runtime (internal/source/atarist/originalmedia)",
			UI: UIIndices{Background: 0, Foreground: 1, Accent: 3, Panel: 2}, Palette: atariSTPalette(),
		},
	}
}

func NewPrototypeRegistry() (Registry, error) {
	semanticTiles := []string{Grass, Water, Road, Wall, Ship, Cargo, City, Player}
	modernRaster, err := embeddedModernAppleRaster(semanticTiles)
	if err != nil {
		return Registry{}, err
	}
	tilesets := []Tileset{
		{
			ID: AppleIITiles, DisplayNameKey: "ui.tileset.apple2",
			Provenance: "Apple II pixel semantics", SourceStatus: "prototype; original tileset consumer pending",
			TileSize: 16, SemanticTiles: semanticTiles, Patterns: appleIIPatterns(), PaletteFloor: 4,
		},
		{
			ID: AtariSTTiles, DisplayNameKey: "ui.tileset.atari_st",
			Provenance: "Atari ST theme boundary", SourceStatus: "prototype; with -atari-disk the original tiles are decoded and drawn at runtime (internal/source/atarist/originalmedia)",
			TileSize: 16, SemanticTiles: semanticTiles, Patterns: atariSTPatterns(), PaletteFloor: 4,
		},
		{
			ID: AtariSTHDTiles, DisplayNameKey: "ui.presentation.atari_st_hd",
			Provenance: "Independent Atari ST HD redraw", SourceStatus: "native vector/raster presentation; original masks remain separate",
			TileSize: 16, SemanticTiles: semanticTiles, Patterns: atariSTPatterns(), PaletteFloor: 4,
		},
		{
			ID: ModernTiles, DisplayNameKey: "ui.tileset.remake_modern",
			Provenance: "Remake Modern: Apple II-informed redraw", SourceStatus: "official high-resolution raster asset; consumed by the normal ground screen (cmd/sundog-ebiten usesModernGroundRaster)",
			TileSize: 16, SemanticTiles: semanticTiles, Patterns: modernApplePatterns(),
			Raster: modernRaster, RasterThemeID: ModernApple, PaletteFloor: 5,
		},
	}

	registry := Registry{
		chromes:     chromes(),
		tilesets:    tilesets,
		byChrome:    make(map[ID]Chrome, len(chromes())),
		byTileset:   make(map[TilesetID]Tileset, len(tilesets)),
		defaultTile: make(map[ID]TilesetID, len(chromes())),
	}
	// Every chrome has to be able to show every tileset. Checking it here means
	// no combination the player can pick is one the renderer cannot draw.
	floor := 0
	for _, tileset := range tilesets {
		if len(tileset.Palette) == 0 && tileset.PaletteFloor > floor {
			floor = tileset.PaletteFloor
		}
		registry.byTileset[tileset.ID] = tileset
	}
	for _, chrome := range registry.chromes {
		if len(chrome.Palette) < floor {
			return Registry{}, fmt.Errorf("chrome %q has %d palette entries, fewer than the %d a tileset indexes",
				chrome.ID, len(chrome.Palette), floor)
		}
		registry.byChrome[chrome.ID] = chrome
	}
	// The chrome that came from a bundled theme keeps that theme's own tiles as
	// its default, so a save written before the two axes split still resolves to
	// what it used to look like.
	registry.defaultTile[AppleIIOriginal] = AppleIITiles
	registry.defaultTile[AtariSTOriginal] = AtariSTTiles

	for _, chrome := range registry.chromes {
		for _, tileset := range registry.tilesets {
			if _, err := Resolve(chrome, tileset); err != nil {
				return Registry{}, fmt.Errorf("chrome %q cannot show tileset %q: %w", chrome.ID, tileset.ID, err)
			}
		}
	}
	return registry, nil
}

// DefaultTileset is the tileset a chrome resolves to when none is chosen.
func (r Registry) DefaultTileset(id ID) (TilesetID, bool) {
	tileset, ok := r.defaultTile[id]
	return tileset, ok
}

// Chromes returns the interface colour languages in registration order.
func (r Registry) Chromes() []Chrome {
	result := make([]Chrome, len(r.chromes))
	copy(result, r.chromes)
	return result
}

// Tilesets returns the tile art sets in registration order.
func (r Registry) Tilesets() []Tileset {
	result := make([]Tileset, len(r.tilesets))
	copy(result, r.tilesets)
	return result
}

// Pair resolves a chrome and a tileset into something drawable.
func (r Registry) Pair(chromeID ID, tilesetID TilesetID) (Group, bool) {
	chrome, ok := r.byChrome[chromeID]
	if !ok {
		return Group{}, false
	}
	tileset, ok := r.byTileset[tilesetID]
	if !ok {
		fallback, known := r.defaultTile[chromeID]
		if !known {
			return Group{}, false
		}
		if tileset, ok = r.byTileset[fallback]; !ok {
			return Group{}, false
		}
	}
	group, err := Resolve(chrome, tileset)
	if err != nil {
		return Group{}, false
	}
	return group, true
}

// CycleTileset moves to the next tile art set, leaving the chrome alone.
func (r Registry) CycleTileset(current TilesetID) (TilesetID, error) {
	if len(r.tilesets) == 0 {
		return "", fmt.Errorf("tileset registry is empty")
	}
	index := -1
	for i, tileset := range r.tilesets {
		if tileset.ID == current {
			index = i
			break
		}
	}
	if index < 0 {
		return "", fmt.Errorf("unknown tileset %q", current)
	}
	return r.tilesets[(index+1)%len(r.tilesets)].ID, nil
}

// Group resolves a chrome with its own default tileset. It is what a caller
// that only knows about the one axis gets.
func (r Registry) Group(id ID) (Group, bool) {
	tileset, ok := r.defaultTile[id]
	if !ok {
		return Group{}, false
	}
	return r.Pair(id, tileset)
}

// ReplaceGroup returns a new registry with one complete, validated group
// swapped in. The original registry is untouched, so callers can prepare an
// external atlas before atomically installing it in a reducer or renderer.
// ReplaceTileset returns a new registry with one validated tileset swapped in.
// The original is untouched, so a caller can prepare an external atlas and then
// install it atomically. Every chrome is re-checked against the replacement,
// because a tileset that indexes more palette slots than a chrome has would
// otherwise become an unshowable pair the player could still select.
func (r Registry) ReplaceTileset(tileset Tileset) (Registry, error) {
	if _, ok := r.byTileset[tileset.ID]; !ok {
		return Registry{}, fmt.Errorf("tileset %q is not registered", tileset.ID)
	}
	replacement := Registry{
		chromes:     append([]Chrome(nil), r.chromes...),
		tilesets:    append([]Tileset(nil), r.tilesets...),
		byChrome:    make(map[ID]Chrome, len(r.byChrome)),
		byTileset:   make(map[TilesetID]Tileset, len(r.byTileset)),
		defaultTile: make(map[ID]TilesetID, len(r.defaultTile)),
	}
	for index, candidate := range replacement.tilesets {
		if candidate.ID == tileset.ID {
			replacement.tilesets[index] = tileset
		}
		replacement.byTileset[replacement.tilesets[index].ID] = replacement.tilesets[index]
	}
	for _, chrome := range replacement.chromes {
		replacement.byChrome[chrome.ID] = chrome
	}
	for id, value := range r.defaultTile {
		replacement.defaultTile[id] = value
	}
	for _, chrome := range replacement.chromes {
		if _, err := Resolve(chrome, replacement.byTileset[tileset.ID]); err != nil {
			return Registry{}, fmt.Errorf("chrome %q cannot show tileset %q: %w", chrome.ID, tileset.ID, err)
		}
	}
	return replacement, nil
}

func (r Registry) Cycle(current ID) (ID, error) {
	if len(r.chromes) == 0 {
		return "", fmt.Errorf("theme registry is empty")
	}
	index := -1
	for i, chrome := range r.chromes {
		if chrome.ID == current {
			index = i
			break
		}
	}
	if index < 0 {
		return "", fmt.Errorf("unknown theme %q", current)
	}
	return r.chromes[(index+1)%len(r.chromes)].ID, nil
}

func (r Registry) IDs() []ID {
	ids := make([]ID, len(r.chromes))
	for i, chrome := range r.chromes {
		ids[i] = chrome.ID
	}
	return ids
}

// TilesetIDs returns the tile art sets in registration order.
func (r Registry) TilesetIDs() []TilesetID {
	ids := make([]TilesetID, len(r.tilesets))
	for i, tileset := range r.tilesets {
		ids[i] = tileset.ID
	}
	return ids
}
