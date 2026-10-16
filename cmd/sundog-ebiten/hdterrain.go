package main

import (
	"encoding/json"
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
)

type hdCompleteArt struct {
	StripSizes map[int]int            `json:"stripSizes"`
	Strips     map[string][]hdMapNode `json:"strips"`
	Rooms      map[int][]hdMapNode    `json:"rooms"`
}

var hdCompleteCache *hdCompleteArt

func hdCompleteAssets() *hdCompleteArt {
	if hdCompleteCache != nil {
		return hdCompleteCache
	}
	raw, err := hdAssets.ReadFile("hdassets/complete.json")
	if err != nil {
		panic(err)
	}
	var art hdCompleteArt
	if err := json.Unmarshal(raw, &art); err != nil {
		panic(err)
	}
	if len(art.Strips) != 330 || len(art.Rooms) != 17 {
		panic("incomplete Atari ST HD asset group")
	}
	for strip, count := range art.StripSizes {
		for cell := 0; cell < count; cell++ {
			key := fmt.Sprintf("%d/%d", strip, cell)
			if len(art.Strips[key]) == 0 {
				panic("missing HD tile " + key)
			}
		}
	}
	for index := 13; index <= 29; index++ {
		if len(art.Rooms[index]) == 0 {
			panic(fmt.Sprintf("missing HD room %d", index))
		}
	}
	hdCompleteCache = &art
	return hdCompleteCache
}

func (a *app) hdRoomImage(index int) *ebiten.Image {
	nodes, ok := hdCompleteAssets().Rooms[index]
	if !ok {
		panic(fmt.Sprintf("HD room %d missing", index))
	}
	return a.hdWorldSprite(fmt.Sprintf("room/%d", index), 160, 100, nodes)
}

// hdCityTile is the proven renderer translation, kept separate from artwork.
// Invalid placeholder slots are deliberately blank, just as TileIndices is.
func hdCityTile(strips [3]int, code, roll int) (int, int, bool) {
	if code == 0 || code == 1 {
		return 0, 0, false
	}
	screen := code
	switch code {
	case 15:
		screen = 32
	case 5:
		screen = 33
	case 11:
		screen = [4]int{5, 11, 21, 27}[roll&3]
	default:
		if roll&1 == 1 {
			screen += 16
		}
	}
	slot, cell := 2, screen-2
	if screen >= 32 {
		slot, cell = 0, screen-32
	} else if screen >= 18 {
		slot, cell = 1, screen-18
	}
	strip := strips[slot]
	if cell < 0 || cell >= hdCompleteAssets().StripSizes[strip] {
		return 0, 0, false
	}
	return strip, cell, true
}

func hdSurfaceTile(strips [3]int, code int) (int, int, bool) {
	if code <= 0 {
		return 0, 0, false
	}
	slot, cell := 2, code-1
	if code >= 32 {
		slot, cell = 0, code-32
	} else if code >= 17 {
		slot, cell = 1, code-17
	}
	strip := strips[slot]
	if cell < 0 || cell >= hdCompleteAssets().StripSizes[strip] {
		return 0, 0, false
	}
	return strip, cell, true
}

func (a *app) hdSourceTile(strip, cell int) *ebiten.Image {
	key := fmt.Sprintf("%d/%d", strip, cell)
	nodes, ok := hdCompleteAssets().Strips[key]
	if !ok {
		panic("HD tile missing: " + key)
	}
	return a.hdWorldSprite("strip/"+key, 32, 32, nodes)
}

func (a *app) hdSurfaceArt(strip, cell, planetColour int) *ebiten.Image {
	key := fmt.Sprintf("%d/%d", strip, cell)
	nodes, ok := hdCompleteAssets().Strips[key]
	if !ok {
		panic("HD surface tile missing: " + key)
	}
	copyNodes := append([]hdMapNode(nil), nodes...)
	// A material role in our own vector art follows the proven planet palette
	// record. No original bitmap or mask is transformed into HD pixels.
	r, g, b := uint32(planetColour>>8&7)*0x22, uint32(planetColour>>4&7)*0x22, uint32(planetColour&7)*0x22
	for i := range copyNodes {
		if copyNodes[i].Material == "planet-ground" {
			copyNodes[i].Colour = r<<16 | g<<8 | b
		}
	}
	return a.hdWorldSprite(fmt.Sprintf("surface/%s/%03x", key, planetColour), 32, 32, copyNodes)
}

func (a *app) hdGroundScene(scene *stGroundScene) *ebiten.Image {
	key := fmt.Sprintf("full/%s/%t/%d/%d/%d/%d", scene.cityID, scene.world, scene.scrollX, scene.scrollY, scene.rebuilds, a.viewScale())
	if hdWorldGroundKey == key && hdWorldGroundImage != nil {
		return hdWorldGroundImage
	}
	scale := a.viewScale()
	canvas := ebiten.NewImage(320*scale, 200*scale)
	canvas.Fill(hdReviewColour(0x020508))
	background := a.hdWorldSprite("ground", 40, 40, hdWorldAssets().GroundBackdrop)
	if scene.world {
		surface, ok := game.GroundSurface(a.state)
		if !ok {
			panic("HD surface data unavailable")
		}
		strips := [3]int{54 + int(surface.Header[2]), 46 + int(surface.Header[1]), 46 + int(surface.Header[0])}
		for row := 0; row < 7; row++ {
			for col := 0; col < 10; col++ {
				cell := game.GroundWrap(a.state, game.Position{X: scene.scrollX + col, Y: scene.scrollY + row})
				code := scene.surfaceCodes[cell.Y][cell.X]
				x, y := col*32*scale, row*32*scale
				if code == 0 {
					hdWorldBlit(canvas, background, x, y)
					continue
				}
				if code == 1 {
					continue
				}
				if strip, tile, yes := hdSurfaceTile(strips, code); yes {
					hdWorldBlit(canvas, a.hdSurfaceArt(strip, tile, int(surface.Header[6])*256+int(surface.Header[7])), x, y)
				}
			}
		}
	} else {
		// The walker can be outside the city rectangle before the world-map
		// transition. Use the original scene builder's bounded source record;
		// out-of-map cells below remain background, exactly as that scene does.
		strips, err := a.originalDisk.CityStripIndices(scene.cityNumber)
		if err != nil {
			panic(err)
		}
		buildings := map[[2]int]int{}
		for _, b := range game.GroundCityBuildings(a.state, a.state.CityID) {
			buildings[[2]int{b.Row, b.Column}] = b.Code
		}
		roll := newGroundRoller(a.state.CityID, scene.scrollX, scene.scrollY, scene.rebuilds)
		for row := 0; row < stGroundRows; row++ {
			for col := 0; col < stGroundColumns; col++ {
				cell := game.GroundWrap(a.state, game.Position{X: scene.scrollX + col, Y: scene.scrollY + row})
				code, _, ok := game.GroundTerrainCode(a.state, a.state.CityID, cell.X, cell.Y)
				x, y := col*stGroundPitch, row*stGroundPitch
				if ok && code == stGroundSolidCode {
					continue
				}
				hdWorldBlit(canvas, background, x*scale, y*scale)
				if !ok {
					continue
				}
				look := roll()
				if b, found := buildings[[2]int{cell.X, cell.Y}]; found {
					code, look = b, 0
				}
				if strip, tile, yes := hdCityTile(strips, code, look); yes {
					hdWorldBlit(canvas, a.hdSourceTile(strip, tile), (x+4)*scale, (y+4)*scale)
				}
				if code == stGroundPodCode {
					a.rollSTGroundPods(nil, x, y, roll)
				}
			}
		}
	}
	if hdWorldGroundImage != nil {
		hdWorldGroundImage.Dispose()
	}
	hdWorldGroundKey = key
	hdWorldGroundImage = canvas
	return canvas
}

func (a *app) drawHDGround(screen *ebiten.Image, withMessage bool) bool {
	if !a.groundWalker.ready || a.groundWalker.cityID != a.state.CityID || a.groundWalker.world != a.state.GroundWorld {
		a.resetSTGroundWalker()
	}
	scene := a.stGroundSceneFor(a.state.CityID, a.groundWalker.scrollX, a.groundWalker.scrollY)
	if scene == nil {
		return false
	}
	hdWorldBlit(screen, a.hdGroundScene(scene), a.originX, a.originY)
	a.drawSTGroundMarks(screen, scene)
	a.drawSTGroundWalker(screen)
	if withMessage {
		a.drawSTGroundMessage(screen)
	}
	return true
}
