package originalmedia

import "testing"

func TestBackdropRangesFollowDrawZoomsCaseTable(t *testing.T) {
	// Non-Enlie picks live in base+1 .. base+count-1; Enlie always the base.
	cases := []struct {
		style     int
		enlieWant int
		low, high int
	}{
		{StyleBar, 14, 15, 19},
		{StylePartsShop, 20, 21, 23},
		{StyleEquipment, 24, 25, 26},
		{StyleHotel, 27, 28, 29},
	}
	for _, c := range cases {
		if got, ok := BackdropIndex(c.style, 0, true); !ok || got != c.enlieWant {
			t.Fatalf("style %d in Enlie picked %d/%v, want %d", c.style, got, ok, c.enlieWant)
		}
		for hash := 0; hash < 40; hash++ {
			got, ok := BackdropIndex(c.style, hash, false)
			if !ok || got < c.low || got > c.high {
				t.Fatalf("style %d hash %d picked %d/%v, want %d..%d", c.style, hash, got, ok, c.low, c.high)
			}
		}
	}
}

func TestTheSameCityHashAlwaysPicksTheSamePicture(t *testing.T) {
	first, _ := BackdropIndex(StyleBar, 1234, false)
	second, _ := BackdropIndex(StyleBar, 1234, false)
	if first != second {
		t.Fatalf("hash 1234 picked %d then %d", first, second)
	}
}

func TestTheShipStyleIsAlwaysPictureThirteen(t *testing.T) {
	for _, hash := range []int{0, 7, 999} {
		if got, ok := BackdropIndex(StyleShip, hash, false); !ok || got != 13 {
			t.Fatalf("ship style picked %d/%v, want 13", got, ok)
		}
	}
}

func TestAnUnknownStyleHasNoBackdrop(t *testing.T) {
	if _, ok := BackdropIndex(11, 0, false); ok {
		t.Fatal("style 11 (the arena) claimed a backdrop")
	}
}

func TestCityRecordWord0IsTheBigEndianFirstWord(t *testing.T) {
	bytes := make([]byte, cityTableOffset+cityCount*cityRecordBytes)
	// City 3's record starts two records past the table base.
	offset := cityTableOffset + 2*cityRecordBytes
	bytes[offset] = 0x12
	bytes[offset+1] = 0x34
	disk := &Disk{bytes: bytes}
	got, err := disk.CityRecordWord0(3)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0x1234 {
		t.Fatalf("word 0 = 0x%04x, want 0x1234", got)
	}
	if _, err := disk.CityRecordWord0(0); err == nil {
		t.Fatal("city number 0 was accepted")
	}
	if _, err := disk.CityRecordWord0(cityCount + 1); err == nil {
		t.Fatal("city number 61 was accepted")
	}
}
