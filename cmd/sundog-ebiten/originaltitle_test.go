package main

import (
	"image/color"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// TestOriginalTitleNeedsDisk: the original title is independent of the
// in-game chrome, but still needs the player's disk and never covers a card.
func TestOriginalTitleNeedsDisk(t *testing.T) {
	a := newOptionsTestApp(t)
	if a.usesOriginalTitle() {
		t.Fatal("no disk, yet the original title")
	}
	a.originalDisk = &originalmedia.Disk{}
	if !a.usesOriginalTitle() {
		t.Fatal("disk present, no original title")
	}
	a.showOptions = true
	if a.usesOriginalTitle() {
		t.Fatal("the options card did not take over")
	}
	a.showOptions = false
	a.state.Screen = game.ScreenShipboard
	if a.usesOriginalTitle() {
		t.Fatal("the title followed the player aboard")
	}
}

// TestOriginalTitleRowsAreTheMenu: clicks land on the window rows in order,
// and nothing outside the window is a choice.
func TestOriginalTitleRowsAreTheMenu(t *testing.T) {
	for index, want := range originalTitleActions {
		if got := originalTitleActionAt(200, originalTitleRowY(index)+2); got != want {
			t.Fatalf("row %d gives %q, want %q", index, got, want)
		}
	}
	if got := originalTitleActionAt(60, originalTitleRowY(0)+2); got != "" {
		t.Fatalf("outside the window gives %q", got)
	}
	if got := originalTitleActionAt(200, originalTitleWindowY0+1); got != "" {
		t.Fatalf("the window frame gives %q", got)
	}
}

// TestTitleColourCycleShape pins the two triangles sampled on the original:
// the frame walks b over 0,2,4,6,4,2 and the letters walk g over 4..7 and back.
func TestTitleColourCycleShape(t *testing.T) {
	seenFrame, seenLetter := map[uint8]bool{}, map[uint8]bool{}
	for step := 0; step < titleColorSteps; step++ {
		f, l := titleFrameColour(step), titleLetterColour(step)
		if f.R != 255 || f.G != 255 || l.R != 255 || l.B != 0 {
			t.Fatalf("step %d: frame %v letters %v", step, f, l)
		}
		seenFrame[f.B], seenLetter[l.G] = true, true
	}
	if len(seenFrame) != 4 || len(seenLetter) != 4 {
		t.Fatalf("frame has %d blues, letters %d greens; want 4 and 4", len(seenFrame), len(seenLetter))
	}
	if titleFrameColour(0) != titleFrameColour(titleColorSteps) {
		t.Fatal("the cycle does not wrap")
	}
}

// TestTitleNoticeEndsOnTimeoutOrKey: the notice window shows after the logo
// and gives way to the menu on its own, or at once when a key ends it.
func TestTitleNoticeEndsOnTimeoutOrKey(t *testing.T) {
	a := newOptionsTestApp(t)
	art := &originalTitleArt{enteredFrame: 100}
	a.frames = 100 + titleLogoFrame + 1
	if !a.originalTitleShowsNotice(art) {
		t.Fatal("no notice right after the logo")
	}
	a.frames = 100 + titleLogoFrame + titleNoticeFrames
	if a.originalTitleShowsNotice(art) {
		t.Fatal("the notice outlived its timeout")
	}
	a.frames = 100 + titleLogoFrame + 1
	art.noticeDone = true
	if a.originalTitleShowsNotice(art) {
		t.Fatal("a key did not end the notice")
	}
}

// TestTitlePaletteIsTheStartUpPaletteWithThreeChanges: entries 4 and 5 are
// what init_screen sets, 1 and 12 cycle, the rest are set_palette's.
func TestTitlePaletteIsTheStartUpPaletteWithThreeChanges(t *testing.T) {
	base := originalmedia.PaletteRGBA()
	colours := titlePalette(0)
	for index := range colours {
		switch index {
		case 1, 4, 5, 12:
			continue
		}
		if colours[index] != base[index] {
			t.Fatalf("entry %d changed: %v vs %v", index, colours[index], base[index])
		}
	}
	if (colours[5] != color.RGBA{0, 0, 255, 255}) {
		t.Fatalf("entry 5 should be 0x007, got %v", colours[5])
	}
}
