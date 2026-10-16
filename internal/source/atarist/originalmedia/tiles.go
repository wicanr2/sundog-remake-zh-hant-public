package originalmedia

import (
	"fmt"
	"image"
	"image/color"
)

// The city ground is drawn from three tile strips chosen per city, and the
// terrain code in the map goes through a translation table before it picks one.
// Both are the original's own; see docs/atari-st/city-maps.md.

const (
	cityMapFirstBlock = 0x22
	cityMapBytes      = 128
	cityMapsPerBlock  = 4
	cityCount         = 60
	// The three strips a city's header names, each with its own offset.
	stripOffsetA = 31
	stripOffsetB = 34
	stripOffsetC = 34
	// Tiles are 32 x 32 and a strip is one tile tall.
	tileSize = 32
	// The thresholds the city mode uses; the world map uses 17 and 1.
	citySegmentThreshold = 18
	citySegmentSubtract  = 2
)

// Palette is the sixteen colours the original writes once at start-up
// (XSTARTUP:0x02 set_palette). Nothing on the disk carries them: they are
// literals in the program, so they are literals here too.
//
// The values are Atari ST 0RGB with three bits per channel.
var palette = [16]uint16{
	0x000, 0x005, 0x040, 0x055, 0x500, 0x505, 0x430, 0x555,
	0x222, 0x127, 0x070, 0x077, 0x700, 0x707, 0x770, 0x777,
}

// PaletteRGBA is the same sixteen colours as the renderer needs them. Index 5
// is the one the city mode re-asserts; the rest are the start-up values.
// stChannelStep is one step of a three-bit ST colour channel as Hatari shows it.
const stChannelStep = 0x22

func PaletteRGBA() [16]color.RGBA {
	var out [16]color.RGBA
	for index, value := range palette {
		out[index] = color.RGBA{
			// Three bits per channel, spread the way Hatari renders an ST
			// (v × 0x22): the frames the remake is compared against come from
			// it, so 7 is 238 here, not 255.
			R: uint8(int(value>>8&0x7) * stChannelStep),
			G: uint8(int(value>>4&0x7) * stChannelStep),
			B: uint8(int(value&0x7) * stChannelStep),
			A: 255,
		}
	}
	return out
}

// screenCode turns a map cell's four-bit code into the code the drawing
// routine works with (XMOVEONG:0x22 draw_city_map, table at 0x18a5-0x18c7).
//
// roll picks between the two looks a terrain has. The original rolls a die; the
// remake passes a value derived from the cell so the same cell always looks the
// same, which a rolling map cannot do on a canvas that is redrawn every frame.
func screenCode(code, roll int) (int, bool) {
	switch {
	case code == 0 || code == 1:
		// The original never draws these.
		return 0, false
	case code == 15:
		return 32, true
	case code == 5:
		return 33, true
	case code == 11:
		options := [4]int{5, 11, 21, 27}
		return options[roll&3], true
	default:
		if roll&1 == 1 {
			return code + 16, true
		}
		return code, true
	}
}

// stripChoice is which of the three strips a screen code lands in, and which
// tile of that strip.
type stripChoice struct {
	Strip int // 0, 1 or 2, in the order the city header names them
	Tile  int
}

func chooseStrip(code int) stripChoice {
	switch {
	case code >= 32:
		return stripChoice{Strip: 0, Tile: code - 32}
	case code >= citySegmentThreshold:
		return stripChoice{Strip: 1, Tile: code - citySegmentThreshold}
	default:
		return stripChoice{Strip: 2, Tile: code - citySegmentSubtract}
	}
}

// CityStrips is the three image indices a city's map header names.
type CityStrips [3]int

// CityStripIndices reads a city's header and returns the three image indices.
// cityNumber is one-based, as the original numbers them.
func (d *Disk) CityStripIndices(cityNumber int) (CityStrips, error) {
	if cityNumber < 1 || cityNumber > cityCount {
		return CityStrips{}, fmt.Errorf("city number %d is outside 1..%d", cityNumber, cityCount)
	}
	block := cityMapFirstBlock + (cityNumber-1)/cityMapsPerBlock
	offset := logicalOffset(block) + ((cityNumber-1)%cityMapsPerBlock)*cityMapBytes
	header, err := d.at(offset, 2)
	if err != nil {
		return CityStrips{}, err
	}
	word := int(header[0])<<8 | int(header[1])
	// Four four-bit fields, field i starting at bit i*4.
	field := func(i int) int { return (word >> (i * 4)) & 0xF }
	return CityStrips{
		field(0) + stripOffsetA,
		field(1) + stripOffsetB,
		field(2) + stripOffsetC,
	}, nil
}

// TileSet is one city's three decoded strips, ready to draw from.
type TileSet struct {
	strips  [3]Picture
	palette [16]color.RGBA
}

// LoadPlanetTiles 依 XMOVEONG:0x25 的地表記錄選取三條原版圖像。
func (d *Disk) LoadPlanetTiles(index int) (*TileSet, error) {
	if index < 0 || index >= 18 {
		return nil, fmt.Errorf("planet index %d outside 0..17", index)
	}
	header, err := d.at(0xaa00+index*256, 16)
	if err != nil {
		return nil, err
	}
	set := &TileSet{palette: PaletteRGBA()}
	for slot, number := range []int{54 + int(header[2]), 46 + int(header[1]), 46 + int(header[0])} {
		picture, err := d.DecodeImage(number)
		if err != nil {
			return nil, err
		}
		if picture.Height != tileSize {
			return nil, fmt.Errorf("planet strip %d height %d", number, picture.Height)
		}
		set.strips[slot] = picture
	}
	return set, nil
}

func (s *TileSet) SurfaceTileIndices(code int) ([]byte, bool) {
	if code <= 0 {
		return nil, false
	}
	choice := stripChoice{Strip: 2, Tile: code - 1}
	if code >= 32 {
		choice = stripChoice{Strip: 0, Tile: code - 32}
	} else if code >= 17 {
		choice = stripChoice{Strip: 1, Tile: code - 17}
	}
	strip := s.strips[choice.Strip]
	left := choice.Tile * tileSize
	if left < 0 || left+tileSize > strip.Width {
		return nil, false
	}
	out := make([]byte, tileSize*tileSize)
	for y := 0; y < tileSize; y++ {
		for x := 0; x < tileSize; x++ {
			out[y*tileSize+x] = paletteIndex(strip.Pixels[y*strip.Width+left+x])
		}
	}
	return out, true
}

// LoadCityTiles decodes the three strips a city draws its ground from.
func (d *Disk) LoadCityTiles(cityNumber int) (*TileSet, error) {
	indices, err := d.CityStripIndices(cityNumber)
	if err != nil {
		return nil, err
	}
	set := &TileSet{palette: PaletteRGBA()}
	for slot, index := range indices {
		picture, err := d.DecodeImage(index)
		if err != nil {
			return nil, fmt.Errorf("city %d strip %d (image %d): %w", cityNumber, slot, index, err)
		}
		if picture.Height != tileSize {
			return nil, fmt.Errorf("city %d strip %d is %d pixels tall, want %d",
				cityNumber, slot, picture.Height, tileSize)
		}
		set.strips[slot] = picture
	}
	return set, nil
}

// Tile returns the 32x32 image a terrain code draws as, or false when the
// original draws nothing for that code.
func (s *TileSet) Tile(code, roll int) (*image.RGBA, bool) {
	screen, drawn := screenCode(code, roll)
	if !drawn {
		return nil, false
	}
	choice := chooseStrip(screen)
	if choice.Strip < 0 || choice.Strip > 2 || choice.Tile < 0 {
		return nil, false
	}
	strip := s.strips[choice.Strip]
	left := choice.Tile * tileSize
	if left+tileSize > strip.Width {
		// A city whose header picked a strip shorter than the code it uses.
		// Two of the sixty records are placeholders and do this; drawing
		// nothing is better than drawing a neighbouring tile.
		return nil, false
	}
	out := image.NewRGBA(image.Rect(0, 0, tileSize, tileSize))
	for y := 0; y < tileSize; y++ {
		for x := 0; x < tileSize; x++ {
			index := strip.Pixels[y*strip.Width+left+x]
			out.SetRGBA(x, y, s.palette[paletteIndex(index)])
		}
	}
	return out, true
}

// TileIndices is Tile as the palette indices behind it. GEMBIND:0x28 collide
// reads the screen's four bitplanes, so what decides whether a pixel can be
// stood on is the index, not the colour it is painted with — the city walker
// needs these rather than an RGBA tile.
func (s *TileSet) TileIndices(code, roll int) ([]byte, bool) {
	screen, drawn := screenCode(code, roll)
	if !drawn {
		return nil, false
	}
	choice := chooseStrip(screen)
	if choice.Strip < 0 || choice.Strip > 2 || choice.Tile < 0 {
		return nil, false
	}
	strip := s.strips[choice.Strip]
	left := choice.Tile * tileSize
	if left+tileSize > strip.Width {
		return nil, false
	}
	out := make([]byte, tileSize*tileSize)
	for y := 0; y < tileSize; y++ {
		for x := 0; x < tileSize; x++ {
			out[y*tileSize+x] = paletteIndex(strip.Pixels[y*strip.Width+left+x])
		}
	}
	return out, true
}

// TileSize is the side of one ground tile, so a caller composing the screen
// from indices does not have to repeat the number.
const TileSize = tileSize

// paletteIndexFlip is a probe hook: it swaps the nibble order used to look a
// pixel up in the palette. It exists so the two candidate orders can be
// compared against the real disk rather than argued about.
var paletteIndexFlip bool

// SetPaletteIndexFlip is only for the decoding probe.
func SetPaletteIndexFlip(on bool) { paletteIndexFlip = on }

func paletteIndex(value byte) byte {
	if paletteIndexFlip {
		return flip4(value & 0x0F)
	}
	return value & 0x0F
}
