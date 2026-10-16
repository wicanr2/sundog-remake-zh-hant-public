package main

// Native HD composition. The source slots and reducers still own all input,
// item flags, drag state and saves; the PNGs are independent HD artwork.
import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
)

var hdStoresRasterCache = map[string]*ebiten.Image{}

func hdStoresTitleSafe(w stZoomWindow) [4]int {
	return [4]int{w.x0 + stZoomInner, w.y0 + stZoomTitleTextY, stZoomW - 2*stZoomInner, stZoomTitleH - 4}
}

func hdStoresFooterSafe(w stZoomWindow) [4]int {
	return [4]int{w.x0 + 3, w.y0 + 89, stZoomW - 6, 8}
}

func hdStoresRaster(key string) *ebiten.Image {
	return hdStoredAsset(key)
}

func (a *app) drawHDStoresSlot(screen *ebiten.Image, atlas *ebiten.Image, window stZoomWindow, slot stSlot, item int, selected bool) {
	scale := a.viewScale()
	code := game.ItemCode(item)
	x, y := stRepairSlotRect(window, slot)
	sx, sy := code%8*16*scale, code/8*16*scale
	sprite := atlas.SubImage(image.Rect(sx, sy, sx+16*scale, sy+16*scale)).(*ebiten.Image)
	hdWorldBlit(screen, sprite, a.originX+x*scale, a.originY+y*scale)
	if selected {
		p := hdReviewPainter{a: a, screen: screen, scale: float32(scale)}
		p.stroke(float32(x-1), float32(y-1), 18, 18, .6, 0xf3f6fa)
		for _, op := range p.operations {
			op()
		}
	}
}

func (a *app) drawHDStores(screen *ebiten.Image) bool {
	if a.state.Screen != game.ScreenStores || !a.usesSTRepair() {
		return false
	}
	scale := a.viewScale()
	chrome := hdStoresRaster(fmt.Sprintf("window/%d", scale))
	atlas := hdStoresRaster(fmt.Sprintf("icons/%d", scale))
	background := a.hdCabinRaster("cabin-closed", 320, 200)
	if chrome == nil || atlas == nil || background == nil {
		return false
	}
	window := stZoomFor(game.ScreenStores, 0, 0, 0)
	if chrome.Bounds().Dx() != stZoomW*scale || chrome.Bounds().Dy() != (window.y1-window.y0+1)*scale || atlas.Bounds().Dx() != 128*scale || atlas.Bounds().Dy() != 64*scale {
		panic("HD stores raster dimensions")
	}
	hdWorldBlit(screen, background, a.originX, a.originY)
	hdWorldBlit(screen, chrome, a.originX+window.x0*scale, a.originY+window.y0*scale)
	for i, slot := range stRepairPockets {
		item := 0
		if i < len(a.state.Parts) {
			item = a.state.Parts[i]
		}
		a.drawHDStoresSlot(screen, atlas, window, slot, item, false)
	}
	for i, item := range a.state.Stores {
		a.drawHDStoresSlot(screen, atlas, window, storeGridSlot(i), item, i == a.state.SelectedPartIndex)
	}
	a.drawHDStoresSlot(screen, atlas, window, stRepairHand, 0, false)
	p := hdReviewPainter{a: a, screen: screen, scale: float32(scale)}
	title, footer := hdStoresTitleSafe(window), hdStoresFooterSafe(window)
	p.text(a.lookup("ui.ship_stores"), float32(title[0]), float32(title[1]), float32(title[2]), float32(title[3]), 8, 8, 0x1762ff, true)
	p.text(a.lookup("ui.stores_short_keys"), float32(footer[0]), float32(footer[1]), float32(footer[2]), float32(footer[3]), 8, 8, 0xf3f6fa, false)
	if p.err != "" {
		panic(p.err)
	}
	for _, op := range p.operations {
		op()
	}
	// Match the formal composition: held item is last and follows the cursor.
	if a.state.HandItem != 0 {
		cx, cy := a.toLayout(ebiten.CursorPosition())
		slot := stRepairHand
		if cx >= window.x0 && cx <= window.x1 && cy >= window.y0 && cy <= window.y1 {
			slot = stSlot{cx - window.x0 - 1, cy - window.y0 - 1}
		}
		a.drawHDStoresSlot(screen, atlas, window, slot, a.state.HandItem, false)
	}
	return true
}
