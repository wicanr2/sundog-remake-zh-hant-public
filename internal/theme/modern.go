package theme

// redesignModernPatterns derives the disposable Modern Apple prototype from
// the Apple II semantic patterns. It deliberately works on pattern data, not
// on any original image file: the Apple II pattern is the source silhouette
// and the transformation is the remake's presentation rule.
//
// Evidence boundary: the Apple II pattern in this repository is itself a
// source-free semantic prototype, not decoded original art. Consequently the
// result remains a prototype until the source pixel consumer and a user visual
// review exist.
func redesignModernPatterns(source map[string]PixelPattern) map[string]PixelPattern {
	result := make(map[string]PixelPattern, len(source))
	for tile, pattern := range source {
		result[tile] = redesignModernPattern(tile, pattern)
	}
	return result
}

func redesignModernPattern(tile string, source PixelPattern) PixelPattern {
	rows := make([]string, len(source.Rows))
	for y, row := range source.Rows {
		redesigned := make([]byte, len(row))
		for x := range row {
			index, ok := PaletteIndex(row[x])
			if !ok {
				// The source pattern has already passed Group.Validate in the
				// normal construction path. Preserve malformed input for the
				// validator instead of silently inventing a pixel.
				redesigned[x] = row[x]
				continue
			}
			if isModernSprite(tile) && index == 0 {
				// Sprite index 0 is the transparent/empty silhouette slot.
				redesigned[x] = '0'
				continue
			}
			modernIndex := modernPaletteIndex(tile, index)
			// Modern sprites use a deliberate one-pixel contour language. The
			// contour is derived from the Apple II occupied mask, so the redraw
			// can change its visual hierarchy without changing collision-facing
			// occupancy. Interior light pixels receive a sparse cyan glint.
			if isModernSprite(tile) {
				if modernSpriteEdge(source, x, y) {
					modernIndex = modernEdgePalette(tile, x, y)
				} else if modernIndex == 1 && (x+y)%5 == 0 {
					modernIndex = 4
				}
			}
			redesigned[x] = "01234"[modernIndex]
		}
		rows[y] = string(redesigned)
	}
	return PixelPattern{Rows: rows}
}

func modernSpriteEdge(source PixelPattern, x, y int) bool {
	if !modernOccupied(source, x, y) {
		return false
	}
	for _, offset := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		if !modernOccupied(source, x+offset[0], y+offset[1]) {
			return true
		}
	}
	return false
}

func modernOccupied(source PixelPattern, x, y int) bool {
	if y < 0 || y >= len(source.Rows) || x < 0 || x >= len(source.Rows[y]) {
		return false
	}
	return source.Rows[y][x] != '0'
}

func modernEdgePalette(tile string, x, y int) int {
	switch tile {
	case Ship, Player:
		if (x+2*y)%3 == 0 {
			return 4
		}
		return 3
	case Cargo:
		return 4
	case City:
		if y < 3 || (x+y)%3 == 0 {
			return 4
		}
		return 3
	default:
		return 3
	}
}

func isModernSprite(tile string) bool {
	switch tile {
	case Ship, Cargo, City, Player:
		return true
	default:
		return false
	}
}

// modernPaletteIndex is the reviewable color-language table for the current
// five-color prototype palette:
//
//	0 near-black background/transparent
//	1 warm paper light
//	2 bio-teal
//	3 coral signal
//	4 electric cyan highlight
//
// Terrain index 0 is not universally transparent, so terrain mappings are
// kept separate from sprite mappings.
func modernPaletteIndex(tile string, sourceIndex int) int {
	switch tile {
	case Grass:
		if sourceIndex == 1 {
			return 4
		}
		return 2
	case Water:
		if sourceIndex == 1 {
			return 2
		}
		return 4
	case Road:
		if sourceIndex == 3 {
			return 1
		}
		return 0
	case Wall:
		if sourceIndex == 1 {
			return 3
		}
		return 2
	default:
		// Sprite pixels retain their semantic layers while moving to the
		// Modern Apple palette. The highlight pass above adds index 4 only
		// to selected light pixels.
		switch sourceIndex {
		case 0:
			return 0
		case 1:
			return 1
		case 2:
			return 2
		case 3:
			return 3
		default:
			return 0
		}
	}
}
