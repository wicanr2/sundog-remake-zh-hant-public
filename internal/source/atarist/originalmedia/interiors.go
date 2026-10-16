package originalmedia

import (
	"fmt"
	"image"
)

// The interior backdrop is one 160x100 picture drawn across the pop-up room
// window, exactly the size make_zoom gives an interior. Which picture is a
// two-step rule read from the original's p-code and pinned in
// docs/atari-st/city-subsystems.json:
//
//   - interior.backdrop-case: WINDOWLI:0x1b draw_zoom maps the per-building
//     window style to a picture range: style 5 (bar) -> base 14 of 6,
//     6 (parts) -> base 20 of 4, 7 (equipment) -> base 24 of 3,
//     8 (hotel) -> base 27 of 3; style 9 (the ship's own interior) always
//     draws picture 13.
//   - interior.backdrop-pick: within a range the picture is
//     base + 1 + (cityHash mod (count-1)) -- the same city keeps the same
//     room -- and in Enlie it is always the base picture.
//
// The hash is the original's G395 (interior.city-hash): the first word of the
// city's record plus the coordinates the player entered the city at.

// Window styles as get_building hands them to make_zoom
// (city-subsystems.json, interior.layout-*).
const (
	StyleBar       = 5
	StylePartsShop = 6
	StyleEquipment = 7
	StyleHotel     = 8
	StyleShip      = 9
)

// backdropRanges is draw_zoom's case table (interior.backdrop-case).
var backdropRanges = map[int]struct{ base, count int }{
	StyleBar:       {14, 6},
	StylePartsShop: {20, 4},
	StyleEquipment: {24, 3},
	StyleHotel:     {27, 3},
}

// shipInteriorPicture is the style-9 fixed picture.
const shipInteriorPicture = 13

// BackdropIndex picks the picture number for a room, following pick_pic
// (interior.backdrop-pick). The bool reports whether the style has a backdrop
// at all.
func BackdropIndex(style, cityHash int, enlie bool) (int, bool) {
	if style == StyleShip {
		return shipInteriorPicture, true
	}
	span, ok := backdropRanges[style]
	if !ok {
		return 0, false
	}
	if enlie {
		return span.base, true
	}
	hash := cityHash % (span.count - 1)
	if hash < 0 {
		hash += span.count - 1
	}
	return span.base + 1 + hash, true
}

// City table layout: raw 0x05a10, sixty 12-byte big-endian records
// (docs/atari-st/world-data.json). Word 0 is the field check_exit folds into
// the hash.
const (
	cityTableOffset = 0x05A10
	cityRecordBytes = 12
)

// CityRecordWord0 reads the first word of a city's record, the city-dependent
// part of the original's G395 hash. cityNumber is the 1-based index into the
// source table.
func (d *Disk) CityRecordWord0(cityNumber int) (int, error) {
	if cityNumber < 1 || cityNumber > cityCount {
		return 0, fmt.Errorf("city number %d is outside the 1..%d table", cityNumber, cityCount)
	}
	offset := cityTableOffset + (cityNumber-1)*cityRecordBytes
	span, err := d.at(offset, 2)
	if err != nil {
		return 0, err
	}
	return int(span[0])<<8 | int(span[1]), nil
}

// RoomBackdropSize is the fixed size of every interior backdrop; a picture of
// any other size is not one of them.
const (
	RoomBackdropWidth  = 160
	RoomBackdropHeight = 100
)

// RoomBackdrop decodes an interior backdrop into an opaque RGBA image.
func (d *Disk) RoomBackdrop(pictureIndex int) (*image.RGBA, error) {
	picture, err := d.DecodeImage(pictureIndex)
	if err != nil {
		return nil, err
	}
	if picture.Width != RoomBackdropWidth || picture.Height != RoomBackdropHeight {
		return nil, fmt.Errorf("picture %d is %dx%d, not an interior backdrop",
			pictureIndex, picture.Width, picture.Height)
	}
	colours := PaletteRGBA()
	out := image.NewRGBA(image.Rect(0, 0, picture.Width, picture.Height))
	for y := 0; y < picture.Height; y++ {
		for x := 0; x < picture.Width; x++ {
			out.SetRGBA(x, y, colours[paletteIndex(picture.Pixels[y*picture.Width+x])&0x0f])
		}
	}
	return out, nil
}
