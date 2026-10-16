// Package audio turns the original's sound data into waveforms.
//
// The Atari ST's sound chip is a YM2149: three square-wave channels, one noise
// generator and one envelope generator, all driven by writes to sixteen
// registers. The game never writes those registers itself; it hands a list of
// register writes to XBIOS 32 (Dosound), which applies them a video frame at a
// time. Rebuilding a sound therefore takes two pieces: a chip that turns
// register values into samples, and an interpreter that feeds it.
//
// The chip here is a port of emu2149 by Mitsutaka Okazaki, which is the
// emulation laanwj's SunDog port uses for the same job:
//
//	emu2149.c -- YM2149 emulator
//	Copyright (c) 2001-2021 Mitsutaka Okazaki
//	Distributed under the MIT licence.
//
// Porting it rather than writing a chip model from scratch means the remake's
// sounds can be checked sample-for-sample against an implementation that has
// been listened to for years; tools/compare_dosound_oracle.sh does exactly
// that.
package audio

// AtariSTClock is the clock this chip model wants for an Atari ST.
//
// The ST's YM2149 runs at 2 MHz, and the datasheet puts a channel's frequency
// at the clock over sixteen times the period. This model counts at a sixteenth
// of the clock it is given and flips the output once per period, which is one
// flip per half-cycle, so it needs twice the chip's own clock to land on the
// datasheet's figure. docs/atari-st/ym2149-clock-evidence.json holds the
// measurement that settles it against a real machine.
const AtariSTClock = 4_000_000

// Register numbers, named the way the datasheet names them.
const (
	regToneAFine = 0
	regToneACoar = 1
	regNoise     = 6
	regMixer     = 7
	regVolumeA   = 8
	regVolumeB   = 9
	regVolumeC   = 10
	regEnvFine   = 11
	regEnvCoarse = 12
	regEnvShape  = 13
	registerFile = 16
)

// Each register ignores the bits it does not have.
var registerMask = [registerFile]byte{
	0xFF, 0x0F, 0xFF, 0x0F, 0xFF, 0x0F, 0x1F, 0x3F,
	0x1F, 0x1F, 0x1F, 0xFF, 0xFF, 0x0F, 0xFF, 0xFF,
}

// volumeTable is the YM2149's measured output for each of its 32 steps. The
// chip's volume register holds four bits and the envelope five, and both index
// this same scale.
var volumeTable = [32]int32{
	0x00, 0x01, 0x01, 0x02, 0x02, 0x03, 0x03, 0x04,
	0x05, 0x06, 0x07, 0x09, 0x0B, 0x0D, 0x0F, 0x12,
	0x16, 0x1A, 0x1F, 0x25, 0x2D, 0x35, 0x3F, 0x4C,
	0x5A, 0x6A, 0x7F, 0x97, 0xB4, 0xD6, 0xFF, 0xFF,
}

// YM2149 is the sound chip's state.
type YM2149 struct {
	registers [registerFile]byte

	toneCount [3]int32
	toneFreq  [3]int32
	toneEdge  [3]bool
	toneMask  [3]bool // the mixer has this channel's tone switched off
	noiseMask [3]bool // ...and its noise

	noiseCount int32
	noiseFreq  int32
	noiseSeed  uint32

	envCount     int32
	envFreq      int32
	envPointer   int32
	envContinue  bool
	envAttack    bool
	envAlternate bool
	envHold      bool
	envRising    bool
	envPaused    bool

	volume  [3]int32
	channel [3]int32

	// Position within one output sample, used to step the chip at its own
	// rate while producing samples at the caller's.
	sampleRate int
	clock      int
	chipTime   int64
	realStep   int64
	chipStepQ  int64
}

// NewYM2149 returns a chip in its reset state, producing samples at rate.
func NewYM2149(rate int) *YM2149 {
	return NewYM2149WithClock(rate, AtariSTClock)
}

// NewYM2149WithClock is NewYM2149 for a machine other than the ST, and for
// reproducing another port's output.
func NewYM2149WithClock(rate, clock int) *YM2149 {
	if rate <= 0 {
		rate = 44100
	}
	if clock <= 0 {
		clock = AtariSTClock
	}
	chip := &YM2149{sampleRate: rate, clock: clock}
	chip.Reset()
	return chip
}

// Reset returns the chip to power-on state.
func (c *YM2149) Reset() {
	for channel := 0; channel < 3; channel++ {
		c.toneCount[channel] = 0x1000
		c.toneFreq[channel] = 0
		c.toneEdge[channel] = false
		c.volume[channel] = 0
		c.channel[channel] = 0
		c.toneMask[channel] = false
		c.noiseMask[channel] = false
	}
	c.registers = [registerFile]byte{}
	c.noiseSeed = 0xFFFF
	c.noiseCount = 0x40
	c.noiseFreq = 0
	c.envPointer = 0
	c.envFreq = 0
	c.envCount = 0
	c.envPaused = true

	const scale = 1 << 31
	c.realStep = scale / int64(c.sampleRate)
	c.chipStepQ = scale / int64(c.clock/16)
	c.chipTime = 0
}

// Write sets one register, applying the same masking the hardware applies.
func (c *YM2149) Write(register int, value byte) {
	if register < 0 || register >= registerFile {
		return
	}
	value &= registerMask[register]
	c.registers[register] = value

	switch register {
	case 0, 1, 2, 3, 4, 5:
		channel := register >> 1
		c.toneFreq[channel] = int32(c.registers[channel*2+1]&0x0F)<<8 |
			int32(c.registers[channel*2])
	case regNoise:
		c.noiseFreq = int32(value&0x1F) << 1
	case regMixer:
		for channel := 0; channel < 3; channel++ {
			c.toneMask[channel] = value&(1<<channel) != 0
			c.noiseMask[channel] = value&(1<<(channel+3)) != 0
		}
	case regVolumeA, regVolumeB, regVolumeC:
		// Shifted up one so the envelope flag lands on bit 5 and the four
		// volume bits share the envelope's 32-step scale.
		c.volume[register-regVolumeA] = int32(value) << 1
	case regEnvFine, regEnvCoarse:
		c.envFreq = int32(c.registers[regEnvCoarse])<<8 | int32(c.registers[regEnvFine])
	case regEnvShape:
		c.envContinue = value&0x08 != 0
		c.envAttack = value&0x04 != 0
		c.envAlternate = value&0x02 != 0
		c.envHold = value&0x01 != 0
		// Writing the shape restarts the sweep, even with the same value.
		c.envRising = c.envAttack
		c.envPaused = false
		c.envCount = 0x10000 - c.envFreq
		if c.envRising {
			c.envPointer = 0
		} else {
			c.envPointer = 0x1F
		}
	}
}

// step advances the chip by one of its own counter steps.
func (c *YM2149) step() {
	c.envCount++
	for c.envCount >= 0x10000 && c.envFreq != 0 {
		if !c.envPaused {
			if c.envRising {
				c.envPointer = (c.envPointer + 1) & 0x3F
			} else {
				c.envPointer = (c.envPointer + 0x3F) & 0x3F
			}
		}
		// Bit 5 sets when the sweep runs off either end.
		if c.envPointer&0x20 != 0 {
			if c.envContinue {
				if c.envAlternate != c.envHold {
					c.envRising = !c.envRising
				}
				if c.envHold {
					c.envPaused = true
				}
				if c.envRising {
					c.envPointer = 0
				} else {
					c.envPointer = 0x1F
				}
			} else {
				c.envPaused = true
				c.envPointer = 0
			}
		}
		c.envCount -= c.envFreq
	}

	c.noiseCount++
	if c.noiseCount&0x40 != 0 {
		if c.noiseSeed&1 != 0 {
			c.noiseSeed ^= 0x24000
		}
		c.noiseSeed >>= 1
		if c.noiseFreq != 0 {
			c.noiseCount -= c.noiseFreq
		} else {
			c.noiseCount -= 2
		}
	}
	noise := c.noiseSeed&1 != 0

	for channel := 0; channel < 3; channel++ {
		c.toneCount[channel]++
		if c.toneCount[channel]&0x1000 != 0 {
			if c.toneFreq[channel] > 1 {
				c.toneEdge[channel] = !c.toneEdge[channel]
				c.toneCount[channel] -= c.toneFreq[channel]
			} else {
				c.toneEdge[channel] = true
			}
		}

		// A masked source reads as high, so a channel with both switched off
		// simply holds its level.
		if (c.toneMask[channel] || c.toneEdge[channel]) && (c.noiseMask[channel] || noise) {
			if c.volume[channel]&32 == 0 {
				c.channel[channel] += volumeTable[c.volume[channel]&31] << 4
			} else {
				c.channel[channel] += volumeTable[c.envPointer&31] << 4
			}
		}
		// Halving each step is the chip model's output filter.
		c.channel[channel] >>= 1
	}
}

// Sample advances the chip far enough to produce one output sample.
func (c *YM2149) Sample() int16 {
	for c.realStep > c.chipTime {
		c.chipTime += c.chipStepQ
		c.step()
	}
	c.chipTime -= c.realStep
	return int16(c.channel[0] + c.channel[1] + c.channel[2])
}
