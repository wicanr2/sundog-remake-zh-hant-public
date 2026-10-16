package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

// SpaceSceneWidth and SpaceSceneHeight match the logical shape of the Apple II
// oracle captures after preserving their 560x192 pixel-aspect output as a
// 280x192 logical scene. The renderer is a source-free semantic prototype:
// it borrows the observed composition, not original pixel data.
const (
	SpaceSceneWidth  = 280
	SpaceSceneHeight = 192
)

// RenderSpaceScene creates one deterministic space-view scene for a complete
// presentation group. All themes share the same semantic masks so a theme
// switch cannot change game state, geometry, or collision meaning.
func RenderSpaceScene(state game.State, group theme.Group) (*image.RGBA, error) {
	return renderSpaceScene(state, group, false)
}

// RenderSpaceStarfield excludes the decorative preview objects from combat.
// Star positions remain remake approximations, not original frame parity.
func RenderSpaceStarfield(state game.State, group theme.Group) (*image.RGBA, error) {
	return renderSpaceScene(state, group, true)
}

func renderSpaceScene(state game.State, group theme.Group, starsOnly bool) (*image.RGBA, error) {
	if err := group.Validate(); err != nil {
		return nil, fmt.Errorf("validate space-scene theme: %w", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, SpaceSceneWidth, SpaceSceneHeight))
	background := group.Palette[group.UI.Background]
	if starsOnly {
		background = color.RGBA{A: 255}
	}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: background}, image.Point{}, draw.Src)

	scene := spaceColorsFor(group)
	drawStars(img, scene, state)
	if starsOnly {
		return img, nil
	}
	drawPlanetCloud(img, scene, state)
	drawShipSilhouette(img, scene, state)
	drawInstrumentPixels(img, scene, state)
	return img, nil
}

// RenderSpaceComparison places the same state in each registered theme. It is
// intended for review screenshots and deterministic art regression tests.
func RenderSpaceComparison(state game.State, registry theme.Registry) (*image.RGBA, error) {
	ids := registry.IDs()
	if len(ids) == 0 {
		return nil, fmt.Errorf("theme registry is empty")
	}
	out := image.NewRGBA(image.Rect(0, 0, SpaceSceneWidth*len(ids), SpaceSceneHeight))
	for index, id := range ids {
		group, ok := registry.Group(id)
		if !ok {
			return nil, fmt.Errorf("theme %q is not registered", id)
		}
		scene, err := RenderSpaceScene(state, group)
		if err != nil {
			return nil, err
		}
		draw.Draw(out, image.Rect(index*SpaceSceneWidth, 0, (index+1)*SpaceSceneWidth, SpaceSceneHeight), scene, image.Point{}, draw.Src)
	}
	return out, nil
}

type spaceColors struct {
	background color.RGBA
	light      color.RGBA
	mid        color.RGBA
	accent     color.RGBA
	highlight  color.RGBA
	panel      color.RGBA
}

func spaceColorsFor(group theme.Group) spaceColors {
	get := func(index int) color.RGBA {
		if index < 0 || index >= len(group.Palette) {
			return group.Palette[group.UI.Background]
		}
		return group.Palette[index]
	}
	lightIndex := group.UI.Foreground
	midIndex := group.UI.Panel
	accentIndex := group.UI.Accent
	highlightIndex := accentIndex
	if len(group.Palette) > 4 {
		highlightIndex = 4
	}
	return spaceColors{
		background: get(group.UI.Background),
		light:      get(lightIndex),
		mid:        get(midIndex),
		accent:     get(accentIndex),
		highlight:  get(highlightIndex),
		panel:      get(accentIndex),
	}
}

func drawStars(img *image.RGBA, colors spaceColors, state game.State) {
	// Fixed points keep screenshots deterministic. Position and clock only
	// select a stable phase; they do not introduce a random source.
	stars := [...]image.Point{
		{12, 18}, {31, 42}, {54, 12}, {76, 61}, {98, 28}, {122, 17},
		{147, 39}, {171, 12}, {193, 73}, {214, 25}, {246, 16}, {268, 91},
		{18, 106}, {143, 96}, {178, 118}, {251, 128},
	}
	phase := (state.Position.X + state.Position.Y + state.TimeMinutes/60) & 3
	for index, point := range stars {
		if (index+phase)%5 == 0 {
			setPixel(img, point.X, point.Y, colors.highlight)
		} else {
			setPixel(img, point.X, point.Y, colors.light)
		}
	}
}

func drawPlanetCloud(img *image.RGBA, colors spaceColors, state game.State) {
	// This is a deliberately chunky ellipse/cloud mask inspired by the
	// observed oracle composition. It is not copied from the original image.
	const (
		centerX = 225
		centerY = 52
		radiusX = 70
		radiusY = 42
	)
	phase := (state.Position.X*3 + state.Position.Y*5 + state.TimeMinutes/30) & 7
	for y := centerY - radiusY; y <= centerY+radiusY; y++ {
		for x := centerX - radiusX; x <= centerX+radiusX; x++ {
			dx := x - centerX
			dy := y - centerY
			if dx*dx*radiusY*radiusY+dy*dy*radiusX*radiusX > radiusX*radiusX*radiusY*radiusY {
				continue
			}
			band := (x*7 + y*11 + phase) & 15
			pixel := colors.mid
			if band < 6 || (y < centerY-10 && band < 11) {
				pixel = colors.accent
			}
			if band == 1 || band == 9 || (x+y+phase)%23 == 0 {
				pixel = colors.highlight
			}
			// Horizontal runs create the coarse scanline/dither language
			// without importing an original HGR bitmap.
			if (x+phase)%9 == 0 && y%3 != 0 {
				pixel = colors.background
			}
			setPixel(img, x, y, pixel)
		}
	}
	// A few angular cloud edges keep the silhouette readable at native size.
	for x := 164; x < 267; x += 3 {
		setPixel(img, x, centerY+radiusY-2, colors.accent)
	}
}

func drawShipSilhouette(img *image.RGBA, colors spaceColors, state game.State) {
	// The ship mask is a source-free 2x logical-pixel silhouette. Its occupied
	// shape is shared by every theme; only palette and dither change.
	const (
		originX = 27
		originY = 128
		scale   = 2
	)
	phase := (state.Position.X*2 + state.Position.Y + state.TimeMinutes/20) & 3
	for row := 0; row < 24; row++ {
		left, right := shipBounds(row)
		for column := left; column <= right; column++ {
			pixel := colors.light
			if (column+row+phase)%11 == 0 {
				pixel = colors.accent
			}
			if row > 5 && row < 19 && (column+row)%7 == 0 {
				pixel = colors.panel
			}
			fillPixelBlock(img, originX+column*scale, originY+row*scale, scale, pixel)
		}
	}
	// Wings and engine trails use separate semantic strokes, not an image crop.
	for column := 0; column < 43; column++ {
		if column%4 != 1 {
			fillPixelBlock(img, originX+column*scale, originY+38, scale, colors.light)
		}
	}
	for column := 0; column < 16; column++ {
		if (column+phase)%3 != 0 {
			fillPixelBlock(img, originX+column*scale, originY+48+(column%3)*scale, scale, colors.accent)
		}
	}
}

func shipBounds(row int) (int, int) {
	switch {
	case row < 2:
		return 20 - row, 24 + row
	case row < 5:
		return 15 - row, 29 + row
	case row < 9:
		return 9 - row/2, 35 + row/2
	case row < 17:
		return 3, 43
	case row < 21:
		return 7, 39
	default:
		return 13, 34
	}
}

func drawInstrumentPixels(img *image.RGBA, colors spaceColors, state game.State) {
	// A small lower-right instrument cluster gives the scene a stable UI anchor
	// without pretending to reproduce an original text/font layout.
	baseX, baseY := 205, 145
	for row := 0; row < 18; row++ {
		for column := 0; column < 31; column++ {
			if row == 0 || row == 17 || column == 0 || column == 30 || (row%4 == 0 && column%3 != 0) {
				setPixel(img, baseX+column, baseY+row, colors.panel)
			}
		}
	}
	value := state.Fuel + state.WeaponCharge + state.ShieldLevel + game.HullPercent(state)
	for column := 0; column < 25; column++ {
		if column*4 < value%101 {
			setPixel(img, baseX+3+column, baseY+6, colors.highlight)
		}
	}
}

func setPixel(img *image.RGBA, x, y int, value color.RGBA) {
	if image.Pt(x, y).In(img.Bounds()) {
		img.SetRGBA(x, y, value)
	}
}

func fillPixelBlock(img *image.RGBA, x, y, size int, value color.RGBA) {
	draw.Draw(img, image.Rect(x, y, x+size, y+size), &image.Uniform{C: value}, image.Point{}, draw.Src)
}
