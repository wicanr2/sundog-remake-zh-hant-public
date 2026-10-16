package originalmedia

import (
	"fmt"
	"image"
)

// The item icons are one 496x16 picture in the image directory: 31 cells of
// 16x16, one per item code from 0x01 up. The item table itself holds 32
// records, and the first of them is "(nothing)" — a code with no picture. So
// icon i belongs to item code i+1.
//
// The alignment is not guesswork from the count. Seven independent cells give
// the same offset: a floppy disk on "(freeze/quit)", a waste bin on "(trash)",
// a watch on "(time)", a money bag on "(money)", a burger on "burger", a beer
// glass on "beer", and a resistor on "shunt". The nine ship parts at the end
// are drawn as the electronic components they are named after — a fuse for
// "cryofuse", a diode junction for "j-junc module", a bridge rectifier for
// "photon bridge", a tube for "plasma tube". One cell out and none of that
// lines up. See docs/atari-st/item-icons.md.
const (
	itemIconImageIndex = 4
	// ItemIconSize is how many pixels a cell is on each side.
	ItemIconSize = 16
	// FirstItemIconCode is the item code the first cell belongs to.
	FirstItemIconCode = 1
	// itemIconBackground is the palette index the cells leave as their ground.
	// The remake draws icons over its own panels, so it comes out transparent.
	itemIconBackground = 0
)

// ItemIcons is the decoded strip, addressed by item code.
type ItemIcons struct {
	first int
	cells []*image.RGBA
}

// Count is how many cells the strip holds.
func (s *ItemIcons) Count() int { return len(s.cells) }

// Icon returns the picture for an item code, or reports that this code has
// none. Code 0 is "(nothing)" and never has one.
func (s *ItemIcons) Icon(code int) (*image.RGBA, bool) {
	index := code - s.first
	if index < 0 || index >= len(s.cells) {
		return nil, false
	}
	return s.cells[index], true
}

// ItemIcons decodes the strip from the player's own disk.
func (d *Disk) ItemIcons() (*ItemIcons, error) {
	picture, err := d.DecodeImage(itemIconImageIndex)
	if err != nil {
		return nil, fmt.Errorf("item icons: %w", err)
	}
	cells, err := sliceItemIcons(picture)
	if err != nil {
		return nil, err
	}
	return &ItemIcons{first: FirstItemIconCode, cells: cells}, nil
}

// sliceItemIcons cuts the strip into square cells. A strip that is not exactly
// one cell tall, or whose width does not divide, is not the strip this expects:
// picking a cutting anyway would silently hand back the wrong pictures.
func sliceItemIcons(picture Picture) ([]*image.RGBA, error) {
	if picture.Height != ItemIconSize {
		return nil, fmt.Errorf("item icon strip is %d tall, want %d", picture.Height, ItemIconSize)
	}
	if picture.Width <= 0 || picture.Width%ItemIconSize != 0 {
		return nil, fmt.Errorf("item icon strip is %d wide, not a multiple of %d",
			picture.Width, ItemIconSize)
	}
	if len(picture.Pixels) != picture.Width*picture.Height {
		return nil, fmt.Errorf("item icon strip holds %d pixels, want %d",
			len(picture.Pixels), picture.Width*picture.Height)
	}
	colours := PaletteRGBA()
	cells := make([]*image.RGBA, 0, picture.Width/ItemIconSize)
	for left := 0; left < picture.Width; left += ItemIconSize {
		cell := image.NewRGBA(image.Rect(0, 0, ItemIconSize, ItemIconSize))
		for row := 0; row < ItemIconSize; row++ {
			for column := 0; column < ItemIconSize; column++ {
				value := paletteIndex(picture.Pixels[row*picture.Width+left+column])
				if int(value) == itemIconBackground {
					continue
				}
				cell.SetRGBA(column, row, colours[value&0x0f])
			}
		}
		cells = append(cells, cell)
	}
	return cells, nil
}
