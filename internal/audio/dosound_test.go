package audio

import "testing"

const testRate = 44100

func samplesPerFrame() int { return testRate / FrameRate }

// A list of nothing but register writes takes no time at all: they all land on
// the same frame, so only the trailing frames produce samples.
func TestRegisterWritesTakeNoTime(t *testing.T) {
	samples, err := Render([]byte{0x08, 0x0F, 0x09, 0x0F, 0x0A, 0x0F}, testRate, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := 2 * samplesPerFrame(); len(samples) != want {
		t.Errorf("got %d samples, want %d", len(samples), want)
	}
}

// A wait runs the chip for that many frames.
func TestWaitRunsForTheFramesItAsksFor(t *testing.T) {
	samples, err := Render([]byte{0xFF, 0x05, 0xFF, 0x00}, testRate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := 5 * samplesPerFrame(); len(samples) != want {
		t.Errorf("got %d samples, want %d", len(samples), want)
	}
}

// Any command from 0x82 up is a wait, not just 0xFF.
func TestEveryHighCommandIsAWait(t *testing.T) {
	for _, command := range []byte{0x82, 0xC0, 0xFE, 0xFF} {
		samples, err := Render([]byte{command, 0x03, 0xFF, 0x00}, testRate, 0)
		if err != nil {
			t.Fatalf("command 0x%02x: %v", command, err)
		}
		if want := 3 * samplesPerFrame(); len(samples) != want {
			t.Errorf("command 0x%02x: got %d samples, want %d", command, len(samples), want)
		}
	}
}

// A zero count ends the list, and anything after it is not played.
func TestZeroCountEndsTheList(t *testing.T) {
	samples, err := Render([]byte{0xFF, 0x00, 0xFF, 0x09}, testRate, 1)
	if err != nil {
		t.Fatal(err)
	}
	if want := samplesPerFrame(); len(samples) != want {
		t.Errorf("got %d samples, want %d", len(samples), want)
	}
}

// A sweep steps once per frame from the temporary register until it reaches
// the end value, which is itself never written.
func TestSweepRunsOneFramePerStep(t *testing.T) {
	// Load 0, then step register 1 by one until the temporary register is 4:
	// four frames, writing 0, 1, 2 and 3.
	samples, err := Render([]byte{0x80, 0x00, 0x81, 0x01, 0x01, 0x04}, testRate, 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := 4 * samplesPerFrame(); len(samples) != want {
		t.Errorf("got %d samples, want %d", len(samples), want)
	}
}

// A sweep that can never reach its end value stops at the cap, and the
// trailing frames still play.
func TestUnreachableSweepStopsAtTheCap(t *testing.T) {
	// Stepping by two from zero passes one without ever landing on it.
	samples, err := Render([]byte{0x80, 0x00, 0x81, 0x01, 0x02, 0x01}, testRate, 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := (maxSweepCycles + 3) * samplesPerFrame(); len(samples) != want {
		t.Errorf("got %d samples, want %d", len(samples), want)
	}
}

// The commands between the register writes and the waits do not exist.
func TestUnknownCommandIsRejected(t *testing.T) {
	if _, err := Render([]byte{0x40, 0x00}, testRate, 0); err == nil {
		t.Error("0x40 is not a command and should not be played")
	}
}

// A truncated list is a list read from the wrong place.
func TestTruncatedListIsRejected(t *testing.T) {
	for _, list := range [][]byte{{0x08}, {0x80}, {0x81, 0x01, 0x02}, {0xFF}} {
		if _, err := Render(list, testRate, 0); err == nil {
			t.Errorf("%x should have been rejected", list)
		}
	}
}
