package originalmedia

import "fmt"

// The ground sprites are not pictures in the image directory: birth_sprite's
// pattern argument is a small number, and the bitmaps those numbers name live
// in GEMBIND's native data at raw 0x30802 — ten 7-row one-bit patterns on an
// 8-byte stride (docs/atari-st/ground-rules.json, spritePatterns). Patterns
// 0–3 and 6–9 are the walker by direction and stride frame; 4 and 5 are the
// dot and block marks.
const (
	groundSpritesOffset = 0x30802
	// GroundSpriteCount and GroundSpriteRows describe the table shape.
	GroundSpriteCount  = 10
	GroundSpriteRows   = 7
	groundSpriteStride = 8
)

// GroundSprites reads the ten patterns. Each byte is one row, bit 6 down to
// bit 0 left to right the way the native handler shifts them.
func (d *Disk) GroundSprites() ([GroundSpriteCount][GroundSpriteRows]byte, error) {
	var patterns [GroundSpriteCount][GroundSpriteRows]byte
	span, err := d.at(groundSpritesOffset, GroundSpriteCount*groundSpriteStride)
	if err != nil {
		return patterns, err
	}
	for index := 0; index < GroundSpriteCount; index++ {
		record := span[index*groundSpriteStride : (index+1)*groundSpriteStride]
		if record[GroundSpriteRows] != 0 {
			return patterns, fmt.Errorf("sprite pattern %d has padding byte %d, want 0", index, record[7])
		}
		copy(patterns[index][:], record[:GroundSpriteRows])
	}
	if patterns[0] != [GroundSpriteRows]byte{0x00, 0x38, 0x38, 0x38, 0x38, 0x38, 0x38} {
		return patterns, fmt.Errorf("sprite pattern 0 is not the upright walker; wrong offset")
	}
	return patterns, nil
}
