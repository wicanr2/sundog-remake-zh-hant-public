package theme

// A presentation is one self-consistent look: the interface colours, the tile
// art and the layout that belong together.
//
// The three used to be independent axes with their own controls. Measuring the
// twelve combinations on 2026-09-03 showed why that did not hold: the Atari ST
// layout paints the cabin and the city from the player's own disk, with the
// original's palette and its 32×32 tiles, so under that layout the chrome and
// tileset axes changed nothing at all — four of the six combinations came out
// bit-identical on both screens. One combination was worse than inert: the
// Atari ST layout with the Remake Modern raster drew the remake's map as a
// framed rectangle floating over the original's full-screen city, and a player
// reached it with one keypress from the default.
//
// Collapsing the three into a short list of presets removes both problems by
// construction: every reachable combination is one someone chose to draw.

// PresentationID names a preset.
type PresentationID string

const (
	// AtariSTPresentation is the original Atari ST look: the disk's own
	// pictures, palette and tiles in the original's full-screen layout.
	AtariSTPresentation   PresentationID = "atari-st"
	AtariSTHDPresentation PresentationID = "atari-st-hd"
	// AppleIIPresentation is the Apple II colour language over the remake's
	// own layout, which is where a windowed layout has something to show.
	AppleIIPresentation PresentationID = "apple-ii"
	// ModernPresentation identifies the retired Apple II HD preset in old
	// preferences. It is not selectable.
	ModernPresentation PresentationID = "remake-modern"
)

// Presentation is a preset: the triple, plus how to name it to the player.
type Presentation struct {
	ID             PresentationID
	DisplayNameKey string
	Chrome         ID
	Tileset        TilesetID
	Layout         LayoutID
}

// Presentations lists the presets in cycle order. The Atari ST one is first
// because it is the default: with the player's disk in hand it is the closest
// thing to the original on screen.
func Presentations() []Presentation {
	return []Presentation{
		{
			ID: AtariSTPresentation, DisplayNameKey: "ui.presentation.atari_st",
			Chrome: AtariSTOriginal, Tileset: AtariSTTiles, Layout: AtariSTLayout,
		},
		{
			ID: AtariSTHDPresentation, DisplayNameKey: "ui.presentation.atari_st_hd",
			Chrome: AtariSTOriginal, Tileset: AtariSTHDTiles, Layout: AtariSTLayout,
		},
		{
			ID: AppleIIPresentation, DisplayNameKey: "ui.presentation.apple2",
			Chrome: AppleIIOriginal, Tileset: AppleIITiles, Layout: RemakeLayout,
		},
	}
}

// DefaultPresentation is what a player who has never chosen gets.
func DefaultPresentation() Presentation { return Presentations()[0] }

// PresentationByID looks a preset up by name.
func PresentationByID(id PresentationID) (Presentation, bool) {
	for _, preset := range Presentations() {
		if preset.ID == id {
			return preset, true
		}
	}
	return Presentation{}, false
}

// NextPresentation is the one after current in cycle order, wrapping. An
// unknown current gives the default, so a corrupt value cannot stick.
func NextPresentation(current PresentationID) Presentation {
	presets := Presentations()
	for i, preset := range presets {
		if preset.ID == current {
			return presets[(i+1)%len(presets)]
		}
	}
	return DefaultPresentation()
}

// SnapPresentation is the preset a stored triple names. An exact triple wins;
// otherwise the layout decides, because the layout is what changes who paints
// the screen, and a tileset that the layout ignores is not evidence of intent.
// Anything else falls back to the default rather than being drawn as-is: a
// settings file written by an older version, or edited by hand, must not be
// able to reach a combination nobody designed.
func SnapPresentation(chrome ID, tileset TilesetID, layout LayoutID) Presentation {
	for _, preset := range Presentations() {
		if preset.Chrome == chrome && preset.Tileset == tileset && preset.Layout == layout {
			return preset
		}
	}
	if layout == AtariSTLayout {
		return DefaultPresentation()
	}
	for _, preset := range Presentations() {
		if preset.Layout == RemakeLayout && preset.Tileset == tileset {
			return preset
		}
	}
	return DefaultPresentation()
}

// PresentationNameKey is the catalog key of a preset's display name.
func PresentationNameKey(id PresentationID) string {
	if preset, ok := PresentationByID(id); ok {
		return preset.DisplayNameKey
	}
	return DefaultPresentation().DisplayNameKey
}
