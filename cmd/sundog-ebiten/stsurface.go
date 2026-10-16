package main

import (
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

// buildSTSurfaceScene 將來源地形、當次外觀與城市覆蓋分開組合。
// 亂數仍採 remake 的可重播來源，不宣稱與原版同一亂數狀態。
func (a *app) buildSTSurfaceScene(scrollX, scrollY, rebuilds int) *stGroundScene {
	surface, ok := game.GroundSurface(a.state)
	if !ok || a.originalDisk == nil {
		return nil
	}
	cacheKey := -surface.PlanetIndex - 1
	cache := a.originalTiles[cacheKey]
	if cache == nil {
		set, err := a.originalDisk.LoadPlanetTiles(surface.PlanetIndex)
		if err != nil {
			log.Printf("行星地表讀取失敗：%v", err)
			return nil
		}
		cache = &originalTileCache{set: set, images: map[int]*ebiten.Image{}}
		a.originalTiles[cacheKey] = cache
	}
	if cache.set == nil {
		return nil
	}
	roll := newGroundRoller(a.state.PlanetID, 0, 0, 0)
	codes := [24][40]int{}
	for y := 0; y < 24; y++ {
		for x := 0; x < 40; x++ {
			code := surface.States[y][x]
			if code == 0 && surface.Header[4]*256+surface.Header[5] != 0 {
				code = 0
			}
			if code == 0 && surface.Header[4]*256+surface.Header[5] != 0 {
				// 原版保留空白，城市覆蓋仍須繼續。
			} else if code == 0 || code == 13 {
				r := (roll()*16 + roll()*4 + roll()) % 17
				switch {
				case r < 4:
					code = 13
				case r < 8:
					code = 29
				default:
					code = r + 26
				}
			} else if roll()&1 == 0 {
				code += 16
			}
			if city, yes := game.GroundSurfaceCity(a.state, game.Position{X: x, Y: y}); yes {
				code = 33
				if game.PrimaryCity(city.CityID) {
					code = 32
				}
			}
			codes[y][x] = code
		}
	}
	indices := make([]byte, logicalWidth*logicalHeight)
	background := a.stGroundBackgroundIndices()
	plain := make([]byte, 32*32)
	for y := 0; y < 32; y++ {
		copy(plain[y*32:(y+1)*32], background[y*40:y*40+32])
	}
	for row := 0; row < 7; row++ {
		for col := 0; col < 10; col++ {
			p := game.GroundWrap(a.state, game.Position{X: scrollX + col, Y: scrollY + row})
			if codes[p.Y][p.X] == 0 {
				stGroundBlit(indices, plain, col*32, row*32, 32, 32)
				continue
			}
			if codes[p.Y][p.X] == 1 {
				stGroundFill(indices, col*32, row*32, 32, 32, 2)
				continue
			}
			if tile, ok := cache.set.SurfaceTileIndices(codes[p.Y][p.X]); ok {
				stGroundBlit(indices, tile, col*32, row*32, 32, 32)
			}
		}
	}
	colours := originalmedia.PaletteRGBA()
	// XMOVEONG:0x0b 的地表分支將 record+6 寫入色盤第 5 格。
	value := surface.Header[6]*256 + surface.Header[7]
	colours[5] = color.RGBA{R: uint8((value >> 8 & 7) * 0x22), G: uint8((value >> 4 & 7) * 0x22), B: uint8((value & 7) * 0x22), A: 255}
	rgba := image.NewRGBA(image.Rect(0, 0, logicalWidth, logicalHeight))
	for i, index := range indices {
		rgba.SetRGBA(i%logicalWidth, i/logicalWidth, colours[index&15])
	}
	return &stGroundScene{surfaceCodes: codes, world: true, pod: a.state.GroundMode == game.GroundInPod, cityID: a.state.CityID, scrollX: scrollX, scrollY: scrollY, rebuilds: rebuilds,
		shipCity: a.state.ShipCityID, shipPosition: a.state.ShipPosition, colonySite: a.state.ColonySite,
		blocked: uint16(surface.Header[8]*256 + surface.Header[9]), indices: indices, picture: ebiten.NewImageFromImage(rgba)}
}

// 其他呈現採地表總覽，尚未製作的主題美術不冒稱原版素材。
func (a *app) drawSurfaceOverview(screen *ebiten.Image, group theme.Group) {
	screen.Fill(group.Palette[group.UI.Background])
	surface, ok := game.GroundSurface(a.state)
	if !ok {
		return
	}
	for y, row := range surface.States {
		for x, code := range row {
			ink := group.Palette[group.UI.Foreground]
			if code%2 == 0 {
				ink = group.Palette[group.UI.Accent]
			}
			uiFillRect(screen, float32(x*8+2), float32(y*7+2), 2, 2, ink, false)
			if _, ok := game.GroundSurfaceCity(a.state, game.Position{X: x, Y: y}); ok {
				uiStrokeRect(screen, float32(x*8), float32(y*7), 7, 6, 1, group.Palette[group.UI.Foreground], false)
			}
		}
	}
	uiFillRect(screen, float32(a.state.Position.X*8+2), float32(a.state.Position.Y*7+1), 4, 5, group.Palette[group.UI.Accent], false)
	a.drawTextWithin(screen, a.lookup(a.state.MessageKey), 4, 178, 312, group.Palette[group.UI.Foreground])
}
