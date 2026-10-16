package originalmedia

import "testing"

// TestFontStripReadsBackTheSignature builds a disk-sized buffer with just the
// strip in place and checks the cut: 'A' must read as the signature, a glyph
// next to it must not bleed in, and a strip without the signature is refused.
func TestFontStripReadsBackTheSignature(t *testing.T) {
	bytes := make([]byte, expectedDiskBytes)
	put := func(ch int, rows [GlyphHeight]uint16) {
		for y, row := range rows {
			for x := 0; x < GlyphWidth; x++ {
				if row>>(uint(GlyphWidth-1-x))&1 == 1 {
					px := ch*GlyphWidth + x
					bytes[fontStripOffset+y*fontStripStride+px/8] |= 1 << uint(7-px%8)
				}
			}
		}
	}
	put('A', fontStripSignature)
	put('B', [GlyphHeight]uint16{0b111110, 0b110011, 0b110011, 0b111110, 0b110011, 0b110011, 0b111110, 0})
	disk := &Disk{bytes: bytes, path: "synthetic"}
	font, err := disk.Font()
	if err != nil {
		t.Fatal(err)
	}
	if got := font.Glyph('A'); got.Rows != fontStripSignature || got.Width != GlyphWidth {
		t.Fatalf("A reads %v", got)
	}
	if !font.Glyph('B').Set(0, 0) || font.Glyph('B').Set(5, 0) || font.Glyph('C').Rows != ([GlyphHeight]uint16{}) {
		t.Fatalf("neighbouring glyphs bled: B=%v C=%v", font.Glyph('B'), font.Glyph('C'))
	}
	bytes[fontStripOffset+int('A')*GlyphWidth/8] ^= 0xff
	if _, err := disk.Font(); err == nil {
		t.Fatal("a strip without the signature was accepted")
	}
}
