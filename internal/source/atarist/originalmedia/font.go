package originalmedia

import "fmt"

// The original's character set is one 1-bit strip on the disk: 944 pixels
// wide (118 bytes a row), 8 rows, at raw 0x6600. GEMBIND:0x13 wchar copies
// character ch from x = ch×6 (six wide, eight tall) and GEMBIND:0x22
// draw_x_char copies the small figures from x = 0x300 + i×7. draw_char(ch,
// mode, x, y) is wchar(ch, x−3, y+3), so a character sits centred on (x, y):
// x−3..x+2, y−4..y+3. The strip was found by searching the disk for the
// city marker (character 20) as the running original draws it; docs/atari-st/
// font-strip.md.
const (
	fontStripOffset = 0x6600
	fontStripStride = 118
	fontStripRows   = 8
	GlyphWidth      = 6
	GlyphHeight     = 8
	SmallGlyphWidth = 7
	smallGlyphX     = 0x300
	glyphCount      = smallGlyphX / GlyphWidth
	smallGlyphCount = (fontStripStride*8 - smallGlyphX) / SmallGlyphWidth
)

// Glyph is one character as a mask: Rows[y] bit (5−x) is pixel x.
type Glyph struct {
	Width int
	Rows  [GlyphHeight]uint16
}

// Set reports whether pixel (x, y) of the glyph is ink.
func (g Glyph) Set(x, y int) bool {
	if x < 0 || x >= g.Width || y < 0 || y >= GlyphHeight {
		return false
	}
	return g.Rows[y]>>(uint(g.Width-1-x))&1 == 1
}

// fontStripSignature is character 'A' as the strip holds it; a disk whose
// strip does not read back this way is refused rather than drawn with.
var fontStripSignature = [GlyphHeight]uint16{0b011110, 0b110011, 0b110011, 0b111111, 0b110011, 0b110011, 0b110011, 0}

// Font is the decoded strip.
type Font struct {
	bits []byte
}

// Font reads the character strip and checks it against the signature.
func (d *Disk) Font() (*Font, error) {
	data, err := d.at(fontStripOffset, fontStripStride*fontStripRows)
	if err != nil {
		return nil, fmt.Errorf("font strip: %w", err)
	}
	font := &Font{bits: append([]byte(nil), data...)}
	if got := font.Glyph('A'); got.Rows != fontStripSignature {
		return nil, fmt.Errorf("font strip at 0x%x does not hold the character set (A reads %v)", fontStripOffset, got.Rows)
	}
	return font, nil
}

func (f *Font) pixel(x, y int) bool {
	if x < 0 || y < 0 || y >= fontStripRows || x >= fontStripStride*8 {
		return false
	}
	return f.bits[y*fontStripStride+x/8]>>(uint(7-x%8))&1 == 1
}

func (f *Font) cut(x0, width int) Glyph {
	glyph := Glyph{Width: width}
	for y := 0; y < GlyphHeight; y++ {
		for x := 0; x < width; x++ {
			if f.pixel(x0+x, y) {
				glyph.Rows[y] |= 1 << uint(width-1-x)
			}
		}
	}
	return glyph
}

// Glyph is character ch of the main set (0–127).
func (f *Font) Glyph(ch int) Glyph {
	if ch < 0 || ch >= glyphCount {
		return Glyph{Width: GlyphWidth}
	}
	return f.cut(ch*GlyphWidth, GlyphWidth)
}

// SmallGlyph is figure i of the small set drawn by draw_x_char.
func (f *Font) SmallGlyph(i int) Glyph {
	if i < 0 || i >= smallGlyphCount {
		return Glyph{Width: SmallGlyphWidth}
	}
	return f.cut(smallGlyphX+i*SmallGlyphWidth, SmallGlyphWidth)
}
