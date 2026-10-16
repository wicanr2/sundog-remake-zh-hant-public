package render

import (
	"bytes"
	"encoding/json"
	"image"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

func TestTacticalStarfieldExcludesPreviewObjects(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	group, ok := r.Themes.Group(theme.AtariSTOriginal)
	if !ok {
		t.Fatal("missing Atari ST theme")
	}
	s := game.NewState()
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	stars, err := RenderSpaceStarfield(s, group)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := RenderSpaceScene(s, group)
	if err != nil {
		t.Fatal(err)
	}
	lit := func(img *image.RGBA) int {
		n := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				c := img.RGBAAt(x, y)
				if c.A != 255 {
					t.Fatal("background is not opaque")
				}
				if c.R != 0 || c.G != 0 || c.B != 0 {
					n++
				}
			}
		}
		return n
	}
	if n := lit(stars); n == 0 || n > 64 {
		t.Fatalf("expected sparse stars, got %d lit pixels", n)
	}
	if lit(preview) <= 64 {
		t.Fatal("preview lost its decorative scene")
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("render changed game state")
	}
}
