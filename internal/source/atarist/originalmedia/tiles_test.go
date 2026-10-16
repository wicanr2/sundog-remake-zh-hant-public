package originalmedia

import "testing"

func TestCodesZeroAndOneAreNeverDrawn(t *testing.T) {
	for _, code := range []int{0, 1} {
		for roll := 0; roll < 4; roll++ {
			if _, drawn := screenCode(code, roll); drawn {
				t.Fatalf("code %d was drawn on roll %d", code, roll)
			}
		}
	}
}

func TestTheTwoSpecialCodesAlwaysPickTheSameTile(t *testing.T) {
	for code, want := range map[int]int{15: 32, 5: 33} {
		for roll := 0; roll < 4; roll++ {
			got, drawn := screenCode(code, roll)
			if !drawn || got != want {
				t.Fatalf("code %d roll %d gave %d (%t), want %d", code, roll, got, drawn, want)
			}
		}
	}
}

func TestCodeElevenRollsBetweenFourLooks(t *testing.T) {
	seen := map[int]bool{}
	for roll := 0; roll < 4; roll++ {
		got, drawn := screenCode(11, roll)
		if !drawn {
			t.Fatalf("roll %d drew nothing", roll)
		}
		seen[got] = true
	}
	for _, want := range []int{5, 11, 21, 27} {
		if !seen[want] {
			t.Fatalf("roll never produced %d; saw %v", want, seen)
		}
	}
}

func TestEveryOtherCodeHasTwoLooks(t *testing.T) {
	for code := 2; code < 15; code++ {
		if code == 5 || code == 11 {
			continue
		}
		plain, ok1 := screenCode(code, 0)
		other, ok2 := screenCode(code, 1)
		if !ok1 || !ok2 {
			t.Fatalf("code %d was not drawn", code)
		}
		if plain != code || other != code+16 {
			t.Fatalf("code %d gave %d and %d", code, plain, other)
		}
	}
}

func TestScreenCodesLandInTheStripTheirRangeNames(t *testing.T) {
	cases := map[int]stripChoice{
		32: {Strip: 0, Tile: 0},
		39: {Strip: 0, Tile: 7},
		18: {Strip: 1, Tile: 0},
		30: {Strip: 1, Tile: 12},
		2:  {Strip: 2, Tile: 0},
		14: {Strip: 2, Tile: 12},
	}
	for code, want := range cases {
		if got := chooseStrip(code); got != want {
			t.Fatalf("code %d chose %+v, want %+v", code, got, want)
		}
	}
}

func TestThePaletteIsTheSixteenStartUpColours(t *testing.T) {
	colours := PaletteRGBA()
	// Index 0 is black and index 15 is white in the start-up table; those two
	// anchor the three-bit-per-channel conversion.
	if colours[0].R != 0 || colours[0].G != 0 || colours[0].B != 0 {
		t.Fatalf("index 0 = %+v, want black", colours[0])
	}
	// White is 238 per channel: Hatari's spread of a three-bit channel,
	// which is what every pixel comparison against the original uses.
	if colours[15].R != 238 || colours[15].G != 238 || colours[15].B != 238 {
		t.Fatalf("index 15 = %+v, want Hatari's white", colours[15])
	}
	// Index 5 is the one the city mode re-asserts: 0x505.
	if colours[5].R != 170 || colours[5].G != 0 || colours[5].B != 170 {
		t.Fatalf("index 5 = %+v, want 0x505", colours[5])
	}
}
