// Package render produces deterministic, framework-neutral preview images.
// A future interactive renderer can consume the same theme groups without
// changing game rules or save data.
package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strings"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

const (
	panelWidth  = 320
	panelHeight = 220
	previewTile = 24
)

// RenderComparison renders the same game state with every registered theme.
// The result is a review artifact, not proof that any pattern is decoded
// original art.
func RenderComparison(state game.State, registry theme.Registry) (*image.RGBA, error) {
	ids := registry.IDs()
	if len(ids) == 0 {
		return nil, fmt.Errorf("theme registry is empty")
	}
	out := image.NewRGBA(image.Rect(0, 0, panelWidth*len(ids), panelHeight))
	for index, id := range ids {
		group, ok := registry.Group(id)
		if !ok {
			return nil, fmt.Errorf("theme %q is not registered", id)
		}
		if err := renderPanel(out, index*panelWidth, 0, state, group); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// WritePNG writes a deterministic preview artifact.
func WritePNG(w io.Writer, img image.Image) error {
	if err := png.Encode(w, img); err != nil {
		return fmt.Errorf("encode preview PNG: %w", err)
	}
	return nil
}

func renderPanel(dst *image.RGBA, originX, originY int, state game.State, group theme.Group) error {
	background := group.Palette[group.UI.Background]
	draw.Draw(dst, image.Rect(originX, originY, originX+panelWidth, originY+panelHeight), &image.Uniform{C: background}, image.Point{}, draw.Src)

	drawRect(dst, originX+8, originY+8, panelWidth-16, 20, group.Palette[group.UI.Foreground])
	drawText(dst, originX+14, originY+13, "SUNDOG", group.Palette[group.UI.Background], 1)
	drawText(dst, originX+82, originY+13, strings.ToUpper(strings.ReplaceAll(string(group.ID), "-", " ")), group.Palette[group.UI.Background], 1)

	// The city map is 20 × 12; the preview shows the window the shells show,
	// centred on the player.
	mapX, mapY := originX+12, originY+36
	originCellX, originCellY := world.ViewportOrigin(state.Position.X, state.Position.Y)
	for y := 0; y < world.ViewportHeight(); y++ {
		for x := 0; x < world.ViewportWidth(); x++ {
			terrain, ok := game.GroundTerrainAt(state, state.CityID, originCellX+x, originCellY+y)
			if !ok {
				return fmt.Errorf("city %q map coordinate (%d,%d) is missing",
					state.CityID, originCellX+x, originCellY+y)
			}
			tileID := string(terrain)
			pattern, ok := group.Patterns[tileID]
			if !ok {
				return fmt.Errorf("theme %q lacks preview tile %q", group.ID, tileID)
			}
			drawPattern(dst, mapX+x*previewTile, mapY+y*previewTile, pattern, group.TilePalette, 3)
		}
	}
	playerX, playerY := state.Position.X-originCellX, state.Position.Y-originCellY
	if playerX >= 0 && playerX < world.ViewportWidth() && playerY >= 0 && playerY < world.ViewportHeight() {
		pattern := group.Patterns[theme.Player]
		drawPattern(dst, mapX+playerX*previewTile, mapY+playerY*previewTile, pattern, group.TilePalette, 3)
	}

	infoX := originX + 218
	drawText(dst, infoX, originY+42, "SCREEN", group.Palette[group.UI.Foreground], 1)
	drawText(dst, infoX, originY+54, strings.ToUpper(strings.ReplaceAll(string(state.Screen), "-", " ")), group.Palette[group.UI.Foreground], 1)
	drawText(dst, infoX, originY+72, fmt.Sprintf("POS %d,%d", state.Position.X, state.Position.Y), group.Palette[group.UI.Foreground], 1)
	drawText(dst, infoX, originY+84, fmt.Sprintf("CASH %d", state.Cash), group.Palette[group.UI.Foreground], 1)
	drawText(dst, infoX, originY+96, fmt.Sprintf("BANK %d", state.Bank), group.Palette[group.UI.Foreground], 1)
	drawText(dst, infoX, originY+108, fmt.Sprintf("GEMS %d", state.Gems), group.Palette[group.UI.Foreground], 1)
	drawText(dst, infoX, originY+132, "F8 THEME", group.Palette[group.UI.Accent], 1)
	drawText(dst, originX+12, originY+190, "APPLE II PIXEL SEMANTICS PROTOTYPE", group.Palette[group.UI.Foreground], 1)
	return nil
}

func drawPattern(dst *image.RGBA, x, y int, pattern theme.PixelPattern, palette []color.RGBA, scale int) {
	for rowIndex, row := range pattern.Rows {
		for columnIndex := 0; columnIndex < len(row); columnIndex++ {
			colorIndex, ok := theme.PaletteIndex(row[columnIndex])
			if !ok || colorIndex >= len(palette) {
				continue
			}
			drawRectColor(dst, x+columnIndex*scale, y+rowIndex*scale, scale, scale, palette[colorIndex])
		}
	}
}

func drawRect(dst *image.RGBA, x, y, width, height int, c color.RGBA) {
	drawRectColor(dst, x, y, width, height, c)
}

func drawRectColor(dst *image.RGBA, x, y, width, height int, c color.RGBA) {
	draw.Draw(dst, image.Rect(x, y, x+width, y+height), &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func drawText(dst *image.RGBA, x, y int, text string, c color.RGBA, scale int) {
	for _, runeValue := range strings.ToUpper(text) {
		glyph, ok := font5x7[runeValue]
		if !ok {
			glyph = font5x7['?']
		}
		for rowIndex, row := range glyph {
			for columnIndex := 0; columnIndex < len(row); columnIndex++ {
				if row[columnIndex] == '1' {
					drawRectColor(dst, x+columnIndex*scale, y+rowIndex*scale, scale, scale, c)
				}
			}
		}
		x += 6 * scale
	}
}

var font5x7 = map[rune][]string{
	' ': {"00000", "00000", "00000", "00000", "00000", "00000", "00000"},
	'?': {"11110", "00001", "00110", "00100", "00100", "00000", "00100"},
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"01110", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "01110"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I': {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'J': {"00111", "00010", "00010", "00010", "00010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "11011", "10001"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
	',': {"00000", "00000", "00000", "00000", "00110", "00100", "01000"},
	'-': {"00000", "00000", "00000", "11111", "00000", "00000", "00000"},
}
