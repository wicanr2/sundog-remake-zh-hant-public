package main

// Independent HD asset cache. Original collision indices remain separate.
import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

type hdWorldArt struct {
	CabinClosed    []hdMapNode              `json:"cabinClosed"`
	CabinLit       []hdMapNode              `json:"cabinLit"`
	GroundBackdrop []hdMapNode              `json:"groundBackdrop"`
	Tiles          map[string][][]hdMapNode `json:"tiles"`
}

var hdWorldArtCache *hdWorldArt
var hdWorldSpriteCache = map[string]*ebiten.Image{}
var hdWorldGroundImage *ebiten.Image
var hdWorldGroundKey string

func hdWorldAssets() *hdWorldArt {
	if hdWorldArtCache == nil {
		var art hdWorldArt
		if err := json.Unmarshal(hdWorldArtJSON, &art); err != nil {
			panic(err)
		}
		hdWorldArtCache = &art
	}
	return hdWorldArtCache
}

func (a *app) hdWorldSprite(key string, w, h int, nodes []hdMapNode) *ebiten.Image {
	if raster := a.hdCabinRaster(key, w, h); raster != nil {
		return raster
	}
	cacheKey := fmt.Sprintf("%s/%d", key, a.viewScale())
	if sprite := hdWorldSpriteCache[cacheKey]; sprite != nil {
		return sprite
	}
	if len(nodes) == 0 {
		return nil
	}
	sprite := ebiten.NewImage(w*a.viewScale(), h*a.viewScale())
	local := *a
	local.originX = 0
	local.originY = 0
	p := hdReviewPainter{a: &local, screen: sprite, scale: float32(a.viewScale())}
	p.drawHDMapScene(hdMapScene{Nodes: nodes})
	if p.err != "" {
		log.Printf("HD world review: %s", p.err)
		sprite.Dispose()
		return nil
	}
	for _, op := range p.operations {
		op()
	}
	hdWorldSpriteCache[cacheKey] = sprite
	return sprite
}

func hdWorldBlit(target, source *ebiten.Image, x, y int) {
	if source == nil {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	target.DrawImage(source, op)
}

func (p *hdReviewPainter) worldPerson(x, y int, pod bool, phase int) {
	if pod {
		p.rect(float32(x)+2, float32(y), 3, 6, 0xf3f6fa)
		p.rect(float32(x)+2.5, float32(y)+1, 2, 2, 0x203f5a)
		return
	}
	p.rect(float32(x)+2, float32(y), 3, 2, 0xf3f6fa)
	p.rect(float32(x)+2, float32(y)+2, 3, 2.5, 0xf3f6fa)
	p.line(float32(x)+.7, float32(y)+2.3, float32(x)+6, float32(y)+3.8, .8, 0xf3f6fa)
	if phase%2 == 0 {
		p.line(float32(x)+2.7, float32(y)+4, float32(x)+1.7, float32(y)+6.4, 1, 0xf3f6fa)
		p.line(float32(x)+4.3, float32(y)+4, float32(x)+5.3, float32(y)+6.4, 1, 0xf3f6fa)
	} else {
		p.line(float32(x)+2.7, float32(y)+4, float32(x)+3.1, float32(y)+6.4, 1, 0xf3f6fa)
		p.line(float32(x)+4.3, float32(y)+4, float32(x)+4, float32(y)+6.4, 1, 0xf3f6fa)
	}
}

func (p *hdReviewPainter) parkedShip(x, y int) {
	// Independent horizontal redraw inside the 12x8 source glyph pair bounds.
	xf, yf := float32(x)-3, float32(y)-4
	p.line(xf, yf+4, xf+3, yf+1, 1.2, 0xf3f6fa)
	p.line(xf, yf+4, xf+3, yf+7, 1.2, 0xf3f6fa)
	p.rect(xf+2, yf+2, 5, 4, 0xdde7ee)
	p.rect(xf+5, yf+1, 6, 1, 0xf3f6fa)
	p.rect(xf+5, yf+6, 6, 1, 0xf3f6fa)
	p.rect(xf+5, yf+3, 3, 2, 0x020508)
}
