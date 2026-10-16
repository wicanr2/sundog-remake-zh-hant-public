package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// Stores READY uses selector6's direct 4x5 item block and make_man body slots.
// Item high bits are flags/charges, not the repair window's blasted marker.
func (a *app) drawSTStoredSlot(screen *ebiten.Image, window stZoomWindow, slot stSlot, item int, selected bool) {
	a.drawSTRepairSlot(screen, window, slot, game.ItemCode(item), false)
	if selected {
		x, y := stRepairSlotRect(window, slot)
		uiStrokeRect(screen, float32(x-1), float32(y-1), originalmedia.ItemIconSize+2, originalmedia.ItemIconSize+2, 1, stColour(stColourWhite), false)
	}
}

func storeGridSlot(index int) stSlot {
	return stSlot{stRepairGridX0 + stRepairGridDX*(index%5), stRepairGridY0 + stRepairGridDY*(index/5)}
}

func (a *app) drawSTStoresWindow(screen *ebiten.Image, window stZoomWindow) {
	bx, by := float32(window.x0+1), float32(window.y0+1)
	for i, p := range stManOutline {
		q := stManOutline[(i+1)%len(stManOutline)]
		uiStrokeLine(screen, bx+float32(p[0]), by+float32(p[1]), bx+float32(q[0]), by+float32(q[1]), 1, stColour(1), false)
	}
	for _, box := range []stRoomRect{stRepairPartsBox, stRepairStrip} {
		uiStrokeRect(screen, bx+float32(box.x0), by+float32(box.y0), float32(box.x1-box.x0+1), float32(box.y1-box.y0+1), 1, stColour(1), false)
	}
	for i, slot := range stRepairPockets {
		item := 0
		if i < len(a.state.Parts) {
			item = a.state.Parts[i]
		}
		a.drawSTStoredSlot(screen, window, slot, item, false)
	}
	for i, item := range a.state.Stores {
		a.drawSTStoredSlot(screen, window, storeGridSlot(i), item, i == a.state.SelectedPartIndex)
	}
	a.drawSTStoredSlot(screen, window, stRepairHand, 0, false)
	a.drawTextWithin(screen, a.lookup("ui.stores_short_keys"), window.x0+3, window.y0+89, stZoomW-6, stColour(stColourWhite))
	// The dragged object is drawn last so the source remains visibly empty.
	if a.state.HandItem != 0 {
		cx, cy := a.toLayout(ebiten.CursorPosition())
		slot := stRepairHand
		if cx >= window.x0 && cx <= window.x1 && cy >= window.y0 && cy <= window.y1 {
			slot = stSlot{cx - window.x0 - 1, cy - window.y0 - 1}
		}
		a.drawSTStoredSlot(screen, window, slot, a.state.HandItem, false)
	}
}

func (a *app) stStoresClick(x, y int) (game.Action, bool) {
	w := stZoomFor(game.ScreenStores, 0, 0, 0)
	hit := func(slot stSlot) bool {
		sx, sy := stRepairSlotRect(w, slot)
		return x >= sx && x < sx+originalmedia.ItemIconSize && y >= sy && y < sy+originalmedia.ItemIconSize
	}
	if hit(stRepairHand) {
		return game.ActionEat, true
	}
	for i, slot := range stRepairPockets {
		if hit(slot) {
			a.state = a.reducer.PickPocketPart(a.state, i)
			return "", true
		}
	}
	for i := range a.state.Stores {
		if hit(storeGridSlot(i)) {
			a.state = a.reducer.SelectStoreItem(a.state, i)
			return "", true
		}
	}
	return "", false
}

func (a *app) storeItemLabel() string {
	i := a.state.SelectedPartIndex
	if i >= 0 && i < len(a.state.Stores) && a.state.Stores[i] != 0 {
		if name, ok := game.ItemName(a.state.Stores[i]); ok {
			return name
		}
	}
	return a.lookup("ui.hand_empty")
}
