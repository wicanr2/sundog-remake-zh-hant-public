package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// --- The repair window: XDOREPAI over make_man's slots ---
//
// XDOREPAI:0x02 init_repairs opens the zoom by station (5: style 3 at (9,8))
// and the style takes WINDOWLI:0x1d make_man: the character's outline on
// the left with the hand at (24,29) and the four pockets at (27,57), (9,60),
// (9,77) and (27,77) -- each an item slot (add_item: code, x, y) that
// put_items draws with draw_icon and set_mark marks with '+' when empty.
// 0x03 show_system then adds the system's parts as a grid of slots: byte
// 5×row+column of the record at x = 51 + 22×column, y = 24 + 17×row (four
// rows, five bytes each on a ship system). 0x04 show_function colours the
// row's status square (windows 50–53, x 60..66, y 20 + 17×row) black, red,
// yellow or green by the status byte (0, 1–2, 3, 4), and the picked-up part
// is moved slot to slot with the mouse (0x0b move_item).
//
// Coordinates are relative to the zoom's inside (MAINLIB_G8 is the zoom's
// window). The icons are the disk's 16×16 item icons; the remake anchors
// them at the slot point's centre. The character outline (draw_man's
// polygon) is reconstructed from the measured points. The remake supports
// slot clicks and a cursor-following held icon; E remains the keyboard repair
// shortcut and the arrows change the system.

type stSlot struct{ x, y int }

var (
	stRepairHand    = stSlot{24, 29}
	stRepairPockets = []stSlot{{27, 57}, {9, 60}, {9, 77}, {27, 77}}
)

const (
	stRepairGridX0, stRepairGridDX     = 51, 22
	stRepairGridY0, stRepairGridDY     = 24, 17
	stRepairStatusX0, stRepairStatusX1 = 60, 66
	stRepairStatusY0, stRepairStatusH  = 20, 6
)

// stManOutline is draw_man's polygon (MAINLIB_G767, twelve points, read
// from the memory snapshots), the character standing at the left of the
// window; draw_outline frames window 64 (the strip along the bottom) and
// window 107 (the parts area) around it.
var stManOutline = [][2]int{{1, 20}, {6, 20}, {9, 23}, {9, 29}, {4, 33}, {10, 35}, {10, 36}, {21, 47}, {17, 51}, {9, 43}, {9, 52}, {1, 52}}

var (
	stRepairPartsBox = stRoomRect{38, 156, 1, 84}
	stRepairStrip    = stRoomRect{0, 157, 87, 96}
)

// stRepairStatusPen is show_function's colour for a row status.
func stRepairStatusPen(status int) int {
	switch {
	case status <= 0:
		return 0
	case status <= 2:
		return 12
	case status == 3:
		return 13
	default:
		return 11
	}
}

// stRepairSlotRect is the slot's icon square on the screen.
func stRepairSlotRect(window stZoomWindow, slot stSlot) (int, int) {
	return window.x0 + 1 + slot.x - originalmedia.ItemIconSize/2, window.y0 + 1 + slot.y - originalmedia.ItemIconSize/2
}

func (a *app) drawSTRepairSlot(screen *ebiten.Image, window stZoomWindow, slot stSlot, item int, selected bool) {
	x, y := stRepairSlotRect(window, slot)
	if item == 0 {
		// set_mark's cross, measured off the original's Ship's Locker
		// (parity-playtest-report.md, checkpoint 12): a 2×5 upright and a
		// 6×1 bar in the same 8×8 window the text mark used to sit in.
		white := stColour(stColourWhite)
		ox, oy := float32(x+4), float32(y+5)
		uiFillRect(screen, ox+3, oy, 2, 5, white, false)
		uiFillRect(screen, ox+1, oy+2, 6, 1, white, false)
		return
	}
	code := item
	if game.IsBlastedPart(item) {
		code = item & 0x7f
	}
	if icon, ok := a.itemIcon(code); ok {
		options := &ebiten.DrawImageOptions{}
		options.Filter = ebiten.FilterNearest
		options.GeoM.Translate(float64(x), float64(y))
		screen.DrawImage(icon, options)
	}
	if game.IsBlastedPart(item) {
		uiStrokeRect(screen, float32(x), float32(y), originalmedia.ItemIconSize, originalmedia.ItemIconSize, 1, stColour(stColourRed), false)
	}
	if selected {
		uiStrokeRect(screen, float32(x-1), float32(y-1), originalmedia.ItemIconSize+2, originalmedia.ItemIconSize+2, 1, stColour(stColourWhite), false)
	}
}

// drawSTRepairWindow draws the repair window's inside: the hand, the
// pockets, the system's part grid and its row status squares.
func (a *app) drawSTRepairWindow(screen *ebiten.Image, window stZoomWindow) {
	black := color.RGBA{0, 0, 0, 255}
	baseX, baseY := float32(window.x0+1), float32(window.y0+1)
	for _, box := range []struct {
		rect stRoomRect
		pen  int
	}{{stRepairStrip, 1}, {stRepairPartsBox, 12}} {
		uiStrokeRect(screen, baseX+float32(box.rect.x0), baseY+float32(box.rect.y0),
			float32(box.rect.x1-box.rect.x0+1), float32(box.rect.y1-box.rect.y0+1), 1, stColour(box.pen), false)
	}
	for i := range stManOutline {
		p, q := stManOutline[i], stManOutline[(i+1)%len(stManOutline)]
		uiStrokeLine(screen, baseX+float32(p[0]), baseY+float32(p[1]), baseX+float32(q[0]), baseY+float32(q[1]), 1, stColour(1), false)
	}
	// The held part follows the cursor (move_item redraws it under the
	// mouse); the hand slot shows it when the cursor is off the window.
	if a.state.HandItem != 0 {
		cx, cy := a.toLayout(ebiten.CursorPosition())
		if cx >= window.x0 && cx <= window.x1 && cy >= window.y0 && cy <= window.y1 {
			a.drawSTRepairSlot(screen, window, stSlot{cx - window.x0 - 1, cy - window.y0 - 1}, a.state.HandItem, false)
		} else {
			a.drawSTRepairSlot(screen, window, stRepairHand, a.state.HandItem, false)
		}
	} else {
		a.drawSTRepairSlot(screen, window, stRepairHand, 0, false)
	}
	for i, slot := range stRepairPockets {
		item := 0
		if i < len(a.state.Parts) {
			item = a.state.Parts[i]
		}
		a.drawSTRepairSlot(screen, window, slot, item, i == a.state.SelectedPartIndex && item != 0)
	}
	system, ok := a.state.SelectedShipSystem()
	if !ok {
		return
	}
	for row, parts := range system.Rows {
		for column, part := range parts {
			a.drawSTRepairSlot(screen, window, stSlot{stRepairGridX0 + stRepairGridDX*column, stRepairGridY0 + stRepairGridDY*row}, part, false)
		}
		pen := stRepairStatusPen(game.ShipSystemRowStatus(system, row))
		x0 := float32(window.x0 + 1 + stRepairStatusX0)
		y0 := float32(window.y0 + 1 + stRepairStatusY0 + stRepairGridDY*row)
		fill := black
		if pen != 0 {
			fill = stColour(pen)
		}
		uiFillRect(screen, x0, y0, stRepairStatusX1-stRepairStatusX0+1, stRepairStatusH+1, fill, false)
	}
}

// stRepairClick is move_item on the remake: a pocket picks its part up (or
// takes the held one), a system slot swaps with the hand, the hand slot
// puts the held part away, the title bar's halves change the system.
func (a *app) stRepairClick(x, y int) (game.Action, bool) {
	if a.state.Screen == game.ScreenStores {
		return a.stStoresClick(x, y)
	}
	window := stZoomFor(game.ScreenEngineering, 0, 0, 0)
	if y >= window.y0 && y <= window.y0+stZoomTitleH {
		if x < window.x0+stZoomW/2 {
			return game.ActionPreviousSystem, true
		}
		return game.ActionNextSystem, true
	}
	hit := func(slot stSlot) bool {
		sx, sy := stRepairSlotRect(window, slot)
		return x >= sx && x < sx+originalmedia.ItemIconSize && y >= sy && y < sy+originalmedia.ItemIconSize
	}
	if hit(stRepairHand) {
		a.state = a.reducer.DropHandIntoPocket(a.state)
		return "", false
	}
	for i, slot := range stRepairPockets {
		if hit(slot) {
			if i < len(a.state.Parts) {
				a.state = a.reducer.PickPocketPart(a.state, i)
			} else {
				a.state = a.reducer.DropHandIntoPocket(a.state)
			}
			return "", false
		}
	}
	system, ok := a.state.SelectedShipSystem()
	if !ok {
		return "", false
	}
	for row := range system.Rows {
		for column := range system.Rows[row] {
			if hit(stSlot{stRepairGridX0 + stRepairGridDX*column, stRepairGridY0 + stRepairGridDY*row}) {
				a.state, _ = a.reducer.MoveHandToSlot(a.state, row, column)
				return "", false
			}
		}
	}
	return "", false
}
