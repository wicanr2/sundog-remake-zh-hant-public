package originalmedia

import "fmt"

// GEMBIND:0x28 collide(pattern, x, y) is native code: it takes a sample-pattern
// number and a position, subtracts four from both axes, and reads the screen's
// four bitplanes at every point of that pattern. A point whose colour has a
// non-zero flag in GEMBIND_G230 (the table set_coll_color fills from a sixteen
// character string) makes it return 1 — "cannot stand here".
//
// The ten patterns live in GEMBIND's native data area, next to the sprite
// bitmaps GroundSprites reads: an offset table of ten words, a count table of
// ten bytes, then the point data. Each group is
//
//	1 byte  starting y
//	1 byte  the first dx
//	2 bytes per point: (newline flag, dx)
//
// walking a run-length outline — a non-zero flag drops to the next scan line
// and puts x back at that dx, a zero flag stays on the line and steps right.
// The group lengths chain exactly onto the next group's offset, so the format
// validates itself; ReadCollidePatterns refuses a disk where it does not.
//
// Cross-check: pattern 0 comes out as the 3×6 outline of the parked-vehicle
// bar, which is the character XMOVEONG:0x0c park_pods draws with collide(0,…).
// Patterns 6–9 are the 7×7 walker in its four directions.
const (
	collideOffsetsOffset = 0x304E0
	collideCountsOffset  = 0x304F4
	collideDataOffset    = 0x304FE
	// CollidePatternCount is how many sample patterns the table holds.
	CollidePatternCount = 10
	// CollideAnchorOffset is the four collide subtracts from both axes, so a
	// pattern cell (cx, cy) is sampled at (x-4+cx, y-4+cy).
	CollideAnchorOffset = 4
	// CollideWalkerPattern is the pattern the cabin walker collides with. The
	// four walker patterns differ only in stride, and the original picks by
	// direction; 6 is the one birth_sprite starts it on.
	CollideWalkerPattern = 6
)

// CollidePoint is one sample point, relative to the pattern's own origin.
type CollidePoint struct{ X, Y int }

// CollidePatterns reads the ten sample patterns. The returned points are the
// pattern's own cells; the caller applies CollideAnchorOffset.
func (d *Disk) CollidePatterns() ([CollidePatternCount][]CollidePoint, error) {
	var patterns [CollidePatternCount][]CollidePoint
	offsetSpan, err := d.at(collideOffsetsOffset, CollidePatternCount*2)
	if err != nil {
		return patterns, err
	}
	countSpan, err := d.at(collideCountsOffset, CollidePatternCount)
	if err != nil {
		return patterns, err
	}
	offsets := make([]int, CollidePatternCount)
	counts := make([]int, CollidePatternCount)
	for i := 0; i < CollidePatternCount; i++ {
		offsets[i] = int(offsetSpan[i*2])<<8 | int(offsetSpan[i*2+1])
		counts[i] = int(countSpan[i])
	}
	// The chain check: group i occupies 2 + 2×count bytes and the next group
	// starts right after it. A wrong base or a different disk breaks this.
	for i := 0; i < CollidePatternCount-1; i++ {
		if want := offsets[i] + 2 + 2*counts[i]; want != offsets[i+1] {
			return patterns, fmt.Errorf(
				"collide pattern %d ends at %d but pattern %d starts at %d; wrong offset or disk",
				i, want, i+1, offsets[i+1])
		}
	}
	total := offsets[CollidePatternCount-1] + 2 + 2*counts[CollidePatternCount-1]
	data, err := d.at(collideDataOffset, total)
	if err != nil {
		return patterns, err
	}
	for i := 0; i < CollidePatternCount; i++ {
		cursor := offsets[i]
		y := int(data[cursor])
		x := int(data[cursor+1])
		points := []CollidePoint{{X: x, Y: y}}
		for k := 0; k < counts[i]; k++ {
			flag := data[cursor+2+2*k]
			dx := int(data[cursor+3+2*k])
			if flag != 0 {
				y++
				x = dx
			} else {
				x += dx
			}
			points = append(points, CollidePoint{X: x, Y: y})
		}
		patterns[i] = points
	}
	// Pattern 0 is the parked-vehicle outline: three wide, six tall. A disk
	// that reads back as anything else is not the table this code expects.
	if width, height := patternExtent(patterns[0]); width != 3 || height != 6 {
		return patterns, fmt.Errorf(
			"collide pattern 0 is %d×%d, want the 3×6 vehicle outline; wrong offset or disk",
			width, height)
	}
	return patterns, nil
}

func patternExtent(points []CollidePoint) (width, height int) {
	if len(points) == 0 {
		return 0, 0
	}
	minX, maxX := points[0].X, points[0].X
	minY, maxY := points[0].Y, points[0].Y
	for _, p := range points[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return maxX - minX + 1, maxY - minY + 1
}
