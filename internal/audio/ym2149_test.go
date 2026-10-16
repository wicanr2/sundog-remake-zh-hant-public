package audio

import "testing"

// A square wave at a known register setting should come out at the frequency
// the datasheet gives: the clock over sixteen times the period.
func TestToneFrequencyMatchesThePeriod(t *testing.T) {
	for _, period := range []int{100, 256, 1000} {
		chip := NewYM2149(44100)
		chip.Write(regToneAFine, byte(period&0xFF))
		chip.Write(regToneACoar, byte(period>>8))
		chip.Write(regMixer, 0x3E) // channel A tone on, everything else off
		chip.Write(regVolumeA, 15)

		const seconds = 1
		samples := make([]int16, 44100*seconds)
		for index := range samples {
			samples[index] = chip.Sample()
		}

		// Count the rising edges after the output has settled.
		crossings := 0
		middle := int16(0)
		for index := 4410; index < len(samples); index++ {
			if samples[index-1] <= middle && samples[index] > middle {
				crossings++
			}
		}
		measured := float64(crossings) / 0.9
		// The datasheet's figure for the ST's 2 MHz chip.
		want := 2_000_000.0 / float64(16*period)
		if difference := measured - want; difference > want*0.02 || difference < -want*0.02 {
			t.Errorf("period %d: measured %.1f Hz, want about %.1f Hz", period, measured, want)
		}
	}
}

// The mixer's bits switch sources off, and a channel with nothing switched on
// holds a level rather than falling silent.
func TestMixerSwitchesTheToneOff(t *testing.T) {
	chip := NewYM2149(44100)
	chip.Write(regToneAFine, 100)
	chip.Write(regVolumeA, 15)
	chip.Write(regMixer, 0x3F) // everything off

	// The output filter takes a moment to settle on the held level.
	for index := 0; index < 4410; index++ {
		chip.Sample()
	}
	moved := false
	first := chip.Sample()
	for index := 0; index < 4410; index++ {
		if chip.Sample() != first {
			moved = true
		}
	}
	if moved {
		t.Error("a channel with no source switched on should hold its level")
	}
}

// Volume is taken from the envelope when bit four of the volume register is
// set, and from the register itself otherwise.
func TestVolumeFollowsTheEnvelopeOnlyWhenAsked(t *testing.T) {
	fixed := NewYM2149(44100)
	fixed.Write(regMixer, 0x3E)
	fixed.Write(regToneAFine, 50)
	fixed.Write(regVolumeA, 0x0F)
	if fixed.volume[0]&32 != 0 {
		t.Error("volume 0x0F should not select the envelope")
	}

	swept := NewYM2149(44100)
	swept.Write(regVolumeA, 0x10)
	if swept.volume[0]&32 == 0 {
		t.Error("volume 0x10 should select the envelope")
	}
}

// Writing the shape register restarts the sweep, which is how a list retriggers
// an envelope without changing anything else.
func TestWritingTheShapeRestartsTheSweep(t *testing.T) {
	chip := NewYM2149(44100)
	chip.Write(regEnvFine, 0x00)
	chip.Write(regEnvCoarse, 0x10)
	chip.Write(regEnvShape, 0x08) // repeating downward sweep
	if chip.envPointer != 0x1F {
		t.Fatalf("a downward sweep should start at the top, not %d", chip.envPointer)
	}
	for index := 0; index < 5000; index++ {
		chip.Sample()
	}
	moved := chip.envPointer
	chip.Write(regEnvShape, 0x08)
	if chip.envPointer != 0x1F {
		t.Errorf("rewriting the shape should restart the sweep; it was at %d and is at %d",
			moved, chip.envPointer)
	}
}

// A shape without the continue bit runs once and stays down.
func TestOneShotEnvelopeStopsAtTheBottom(t *testing.T) {
	chip := NewYM2149(44100)
	chip.Write(regEnvFine, 0x40)
	chip.Write(regEnvCoarse, 0x00)
	chip.Write(regEnvShape, 0x00) // one downward sweep, then silence
	for index := 0; index < 44100; index++ {
		chip.Sample()
	}
	if !chip.envPaused {
		t.Error("a one-shot envelope should have stopped")
	}
	if chip.envPointer != 0 {
		t.Errorf("it should rest at zero, not %d", chip.envPointer)
	}
}

// The envelope generator's sweep takes the time the datasheet says, which is
// also the time the original's startup sound takes on a real machine: an
// envelope period of 0x0d00 makes the swell that plays under the FTL logo, and
// that swell reaches full volume in a little over four tenths of a second.
func TestEnvelopeSweepTakesTheTimeTheDatasheetGives(t *testing.T) {
	const rate = 44100
	// Envelope period 0x0d00, rising and then held, on a tone channel.
	list := []byte{
		0x0B, 0x00, 0x0C, 0x0D, 0x0D, 0x0D,
		0x08, 0x10, 0x00, 0x64, 0x07, 0x3E,
		0xFF, 0x50, 0xFF, 0x00,
	}
	samples, err := Render(list, rate, 0)
	if err != nil {
		t.Fatal(err)
	}

	window := rate / 100
	loudest := 0
	levels := make([]int, 0, len(samples)/window)
	for start := 0; start+window <= len(samples); start += window {
		level := 0
		for _, value := range samples[start : start+window] {
			if int(value) > level {
				level = int(value)
			}
		}
		levels = append(levels, level)
		if level > loudest {
			loudest = level
		}
	}
	reached := -1
	for index, level := range levels {
		if level*100 >= loudest*95 {
			reached = index
			break
		}
	}
	if reached < 0 {
		t.Fatal("the envelope never reached full volume")
	}

	got := float64(reached) / 100
	want := 256 * 0x0D00 / 2_000_000.0
	if got < want*0.9 || got > want*1.1 {
		t.Errorf("the sweep took %.3fs, want about %.3fs", got, want)
	}
}
