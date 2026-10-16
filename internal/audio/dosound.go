package audio

import "fmt"

// FrameRate is how often Dosound advances: once per video frame on a 50 Hz
// machine.
const FrameRate = 50

// maxSweepCycles cuts off a sweep that never reaches its end value. A list can
// ask for one on purpose — a siren, say — and it would otherwise run forever.
const maxSweepCycles = 256

// Render plays one Dosound list and returns it as 16-bit mono samples.
//
// The command set is small:
//
//	0x00..0x0F v     write v into that register of the sound chip
//	0x80 v           load v into the temporary register
//	0x81 r s e       write the temporary register into register r once per
//	                 frame, adding s each time, until it reaches e
//	0x82..0xFF n     wait n frames; n of zero ends the list
//
// Only the last two take time: a run of register writes all lands on the same
// frame.
//
// trailingFrames keeps the chip running after the list ends, which lets an
// envelope finish rather than stopping mid-decay.
func Render(list []byte, rate int, trailingFrames int) ([]int16, error) {
	return RenderWithClock(list, rate, trailingFrames, AtariSTClock)
}

// RenderWithClock is Render for a machine whose sound chip runs at another
// speed. Everything the list itself times is in video frames and does not
// move; the clock sets the pitch and how fast an envelope sweeps.
func RenderWithClock(list []byte, rate int, trailingFrames int, clock int) ([]int16, error) {
	if rate <= 0 {
		return nil, fmt.Errorf("sample rate %d", rate)
	}
	chip := NewYM2149WithClock(rate, clock)
	samplesPerFrame := rate / FrameRate

	var out []int16
	generate := func(frames int) {
		for index := 0; index < frames*samplesPerFrame; index++ {
			out = append(out, chip.Sample())
		}
	}

	cursor := 0
	var temp byte
walk:
	for cursor < len(list) {
		command := list[cursor]
		cursor++
		switch {
		case command < 0x10:
			if cursor >= len(list) {
				return nil, fmt.Errorf("register write at %d has no value", cursor-1)
			}
			chip.Write(int(command), list[cursor])
			cursor++
		case command == 0x80:
			if cursor >= len(list) {
				return nil, fmt.Errorf("0x80 at %d has no value", cursor-1)
			}
			temp = list[cursor]
			cursor++
		case command == 0x81:
			if cursor+2 >= len(list) {
				return nil, fmt.Errorf("0x81 at %d is truncated", cursor-1)
			}
			register := int(list[cursor])
			delta := list[cursor+1]
			end := list[cursor+2]
			cursor += 3

			cycles := 0
			for {
				chip.Write(register, temp)
				temp += delta
				generate(1)
				cycles++
				if temp == end || cycles >= maxSweepCycles {
					break
				}
			}
			if cycles >= maxSweepCycles {
				break walk
			}
		case command >= 0x82:
			if cursor >= len(list) {
				return nil, fmt.Errorf("wait at %d has no count", cursor-1)
			}
			frames := list[cursor]
			cursor++
			if frames == 0 {
				break walk
			}
			generate(int(frames))
		default:
			return nil, fmt.Errorf("unknown command 0x%02x at %d", command, cursor-1)
		}
	}
	generate(trailingFrames)
	return out, nil
}
