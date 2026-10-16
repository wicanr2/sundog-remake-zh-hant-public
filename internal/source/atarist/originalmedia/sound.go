package originalmedia

import (
	"encoding/binary"
	"fmt"
)

// The original's sounds are XBIOS 32 (Dosound) command lists.
// MAINLIB:0x03 fore_sound takes a sound number, works out its length from an
// offset table (MAINLIB_G53f), copies that many bytes out of the data that
// follows it (MAINLIB_G563), and hands the buffer to GEMBIND:0x08 do_sound,
// whose native code is `move.w #$20,-(sp); trap #14` — Dosound.
//
// XSTARTUP loads the offset table with a 36-word moveblock. Those 36 words and
// the data behind them are on the disk at the offsets below.

const (
	soundTableOffset = 0x04400
	soundTableWords  = 36
	soundDataOffset  = soundTableOffset + soundTableWords*2
	// A Dosound list longer than this is not one; the whole bank is under a
	// kilobyte.
	maxSoundBytes = 2048
)

// SoundBank is the original's sound effects, each one a Dosound command list.
type SoundBank struct {
	sounds [][]byte
}

// Count is how many sounds the bank holds.
func (b *SoundBank) Count() int { return len(b.sounds) }

// Sound returns one command list. The slice is the bank's own; callers read it.
func (b *SoundBank) Sound(index int) ([]byte, bool) {
	if b == nil || index < 0 || index >= len(b.sounds) {
		return nil, false
	}
	return b.sounds[index], true
}

// SoundBank decodes the sound table. Every gate here is fail-closed: a wrong
// offset gives a table that is not monotonic, or lists that do not parse as
// Dosound, and either way the caller gets an error instead of noise.
func (d *Disk) SoundBank() (*SoundBank, error) {
	raw, err := d.at(soundTableOffset, soundTableWords*2)
	if err != nil {
		return nil, err
	}
	offsets := make([]int, soundTableWords)
	for index := range offsets {
		offsets[index] = int(binary.BigEndian.Uint16(raw[index*2:]))
	}
	if offsets[0] != 0 {
		return nil, fmt.Errorf("sound table starts at %d, want 0", offsets[0])
	}
	// The table runs until it stops climbing; the words after the last sound
	// are zero.
	last := 0
	for last+1 < len(offsets) && offsets[last+1] > offsets[last] {
		last++
	}
	if last < 8 {
		return nil, fmt.Errorf("sound table holds only %d entries", last)
	}
	total := offsets[last]
	if total <= 0 || total > maxSoundBytes {
		return nil, fmt.Errorf("sound data is %d bytes", total)
	}
	data, err := d.at(soundDataOffset, total)
	if err != nil {
		return nil, err
	}

	bank := &SoundBank{sounds: make([][]byte, 0, last)}
	for index := 0; index < last; index++ {
		start, end := offsets[index], offsets[index+1]
		list := data[start:end]
		if err := validateDosound(list); err != nil {
			return nil, fmt.Errorf("sound %d does not parse as Dosound: %w", index, err)
		}
		bank.sounds = append(bank.sounds, list)
	}
	return bank, nil
}

// validateDosound walks a command list. The command set is small, so a list
// read from the wrong place almost always runs off the end or hits a command
// that does not exist.
func validateDosound(list []byte) error {
	if len(list) == 0 {
		return fmt.Errorf("empty list")
	}
	cursor := 0
	for cursor < len(list) {
		command := list[cursor]
		cursor++
		switch {
		case command < 0x10:
			// Write the next byte to that sound-chip register.
			if cursor >= len(list) {
				return fmt.Errorf("register write at %d has no value", cursor-1)
			}
			cursor++
		case command == 0x80:
			// Load the temporary register.
			if cursor >= len(list) {
				return fmt.Errorf("0x80 at %d has no value", cursor-1)
			}
			cursor++
		case command == 0x81:
			// Sweep a register: register, step, end value.
			if cursor+2 >= len(list) {
				return fmt.Errorf("0x81 at %d is truncated", cursor-1)
			}
			if list[cursor] > 0x0F {
				return fmt.Errorf("0x81 at %d names register %d", cursor-1, list[cursor])
			}
			cursor += 3
		case command >= 0x82:
			// Wait; a count of zero ends the list.
			if cursor >= len(list) {
				return fmt.Errorf("wait at %d has no count", cursor-1)
			}
			if list[cursor] == 0 {
				return nil
			}
			cursor++
		default:
			return fmt.Errorf("unknown command 0x%02x at %d", command, cursor-1)
		}
	}
	return nil
}

// soundNames names the sounds in table order. The names come from laanwj's
// SunDog port, which worked them out by playing each one in context.
var soundNames = [...]string{
	"off",
	"tractor",
	"nme_laser",
	"jackpot",
	"menu_click",
	"hit_shield",
	"hit_hull",
	"part_dies",
	"laser",
	"cannon",
	"fuel_here",
	"stinger",
	"ship_arrives",
	"pod",
	"hit_pirate",
	"disk_dead",
	"attack_siren",
	"distress",
	"jettison",
	"repair_bay",
	"warp_fail",
	"teleport_up",
	"teleport_down",
	"explosion",
	"phase_done",
	"hail",
	"pirate_laser",
	"disk_warn",
}

// SoundName is the name for a numbered sound, or its number if the table runs
// out.
func SoundName(index int) string {
	if index >= 0 && index < len(soundNames) {
		return fmt.Sprintf("%02d-%s", index, soundNames[index])
	}
	return fmt.Sprintf("%02d-sound", index)
}

// The warp sounds are not in the sound table. SHIPLIB keeps them as three
// literal command lists, one after another; the first has no terminator
// because it runs until the sweep cap stops it.
const (
	warpSoundOffset = 0x3441a
	warpOneLength   = 28
	warpTwoLength   = 26
	warpThreeLength = 28
	// The byte the game rewrites to pick a warp speed.
	warpSpeedByte = 26
)

// NamedSound is a sound the export knows a name for.
type NamedSound struct {
	Name string
	List []byte
}

// WarpSounds reads the three warp lists.
//
// The first drives the envelope shape register through a sweep that never
// reaches its end value, so it runs until the interpreter's cap: the step size
// is what makes one warp speed sound different from another, and the game
// writes 2, 4 or 8 there.
func (d *Disk) WarpSounds() ([]NamedSound, error) {
	total := warpOneLength + warpTwoLength + warpThreeLength
	block, err := d.at(warpSoundOffset, total)
	if err != nil {
		return nil, err
	}
	one := block[:warpOneLength]
	two := block[warpOneLength : warpOneLength+warpTwoLength]
	three := block[warpOneLength+warpTwoLength:]

	for name, list := range map[string][]byte{"warp1": one, "warp2": two, "warp3": three} {
		if err := validateDosound(list); err != nil {
			return nil, fmt.Errorf("%s does not parse as Dosound: %w", name, err)
		}
	}
	// The two that end normally have to end exactly where they are said to;
	// otherwise the block is not where it is expected to be.
	for name, list := range map[string][]byte{"warp2": two, "warp3": three} {
		tail := list[len(list)-2:]
		if tail[0] != 0xFF || tail[1] != 0x00 {
			return nil, fmt.Errorf("%s does not end with a terminator", name)
		}
	}

	sounds := make([]NamedSound, 0, 5)
	for _, speed := range []byte{2, 4, 8} {
		variant := append([]byte(nil), one...)
		variant[warpSpeedByte] = speed
		sounds = append(sounds, NamedSound{
			Name: fmt.Sprintf("28-warp1_speed%d", speed),
			List: variant,
		})
	}
	sounds = append(sounds,
		NamedSound{Name: "31-warp2", List: append([]byte(nil), two...)},
		NamedSound{Name: "32-warp3", List: append([]byte(nil), three...)},
	)
	return sounds, nil
}

// SoundNames lists every sound the export can produce, under the names the
// game asks for.
func SoundNames() []string {
	names := make([]string, 0, len(soundNames)+5)
	names = append(names, soundNames[:]...)
	names = append(names, "warp1_speed2", "warp1_speed4", "warp1_speed8", "warp2", "warp3")
	return names
}
