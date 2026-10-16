package main

// The normal Atari geometry remains in logical coordinates. HD primitives are
// rasterised directly at the window resolution, before the text layer.
import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

type nativeCanvas struct {
	target      *ebiten.Image
	scale, x, y float32
}

var hdCanvas nativeCanvas

func canvasCoordinates(dst *ebiten.Image, x, y float32) (float32, float32, float32) {
	if dst == hdCanvas.target {
		return hdCanvas.x + x*hdCanvas.scale, hdCanvas.y + y*hdCanvas.scale, hdCanvas.scale
	}
	return x, y, 1
}

func uiFillRect(dst *ebiten.Image, x, y, w, h float32, c color.Color, aa bool) {
	x, y, s := canvasCoordinates(dst, x, y)
	vector.FillRect(dst, x, y, w*s, h*s, c, aa)
}

func uiStrokeRect(dst *ebiten.Image, x, y, w, h, width float32, c color.Color, aa bool) {
	x, y, s := canvasCoordinates(dst, x, y)
	vector.StrokeRect(dst, x, y, w*s, h*s, width*s, c, aa || s > 1)
}

func uiStrokeLine(dst *ebiten.Image, x0, y0, x1, y1, width float32, c color.Color, aa bool) {
	x0, y0, s := canvasCoordinates(dst, x0, y0)
	x1, y1, _ = canvasCoordinates(dst, x1, y1)
	vector.StrokeLine(dst, x0, y0, x1, y1, width*s, c, aa || s > 1)
}

func uiStrokeCircle(dst *ebiten.Image, x, y, radius, width float32, c color.Color, aa bool) {
	x, y, s := canvasCoordinates(dst, x, y)
	vector.StrokeCircle(dst, x, y, radius*s, width*s, c, aa || s > 1)
}
