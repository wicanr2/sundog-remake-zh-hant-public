package render

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

func TestComparisonRendersAllThemesFromOneState(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := game.NewState()
	state, err = reducer.Step(state, game.ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	img, err := RenderComparison(state, reducer.Themes)
	if err != nil {
		t.Fatal(err)
	}
	// The comparison is one panel per registered chrome, so its width follows
	// the registry rather than a number written down here.
	ids := reducer.Themes.IDs()
	if got, want := img.Bounds().Dx(), 320*len(ids); got != want {
		t.Fatalf("width=%d, want %d", got, want)
	}
	if got, want := img.Bounds().Dy(), 220; got != want {
		t.Fatalf("height=%d, want %d", got, want)
	}
	sampled := map[color.Color]bool{}
	for index := range ids {
		sampled[img.At(index*320+20, 50)] = true
	}
	if len(sampled) < 2 {
		t.Fatal("all theme panels have the same sample pixel")
	}

	var encoded bytes.Buffer
	if err := WritePNG(&encoded, img); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(encoded.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds() != img.Bounds() {
		t.Fatalf("decoded bounds=%v, want %v", decoded.Bounds(), img.Bounds())
	}
}

func TestSpaceSceneIsDeterministicAndThemeAware(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := game.NewState()
	state, err = reducer.Step(state, game.ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	apple, ok := reducer.Themes.Group(theme.AppleIIOriginal)
	if !ok {
		t.Fatal("Apple II theme is not registered")
	}
	first, err := RenderSpaceScene(state, apple)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderSpaceScene(state, apple)
	if err != nil {
		t.Fatal(err)
	}
	if first.Bounds().Dx() != SpaceSceneWidth || first.Bounds().Dy() != SpaceSceneHeight {
		t.Fatalf("space scene bounds=%v, want %dx%d", first.Bounds(), SpaceSceneWidth, SpaceSceneHeight)
	}
	var firstPNG, secondPNG bytes.Buffer
	if err := WritePNG(&firstPNG, first); err != nil {
		t.Fatal(err)
	}
	if err := WritePNG(&secondPNG, second); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstPNG.Bytes(), secondPNG.Bytes()) {
		t.Fatal("same state and theme produced different space-scene PNGs")
	}
	background := first.At(0, 0)
	nonBackground := 0
	for y := 0; y < SpaceSceneHeight; y++ {
		for x := 0; x < SpaceSceneWidth; x++ {
			if first.At(x, y) != background {
				nonBackground++
			}
		}
	}
	if nonBackground < 1000 {
		t.Fatalf("space scene has only %d non-background pixels", nonBackground)
	}
	atari, ok := reducer.Themes.Group(theme.AtariSTOriginal)
	if !ok {
		t.Fatal("Atari ST theme is not registered")
	}
	atariScene, err := RenderSpaceScene(state, atari)
	if err != nil {
		t.Fatal(err)
	}
	var atariPNG bytes.Buffer
	if err := WritePNG(&atariPNG, atariScene); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(firstPNG.Bytes(), atariPNG.Bytes()) {
		t.Fatal("Apple II and Atari ST space-scene PNGs are identical")
	}
	comparison, err := RenderSpaceComparison(state, reducer.Themes)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := comparison.Bounds().Dx(), SpaceSceneWidth*len(reducer.Themes.IDs()); got != want {
		t.Fatalf("space comparison width=%d, want %d", got, want)
	}
}
