package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
)

// Only independently authored HD art is embedded. Original media remains a
// read-only optional player input, including the original collision indices.
//
//go:embed hdassets/*
var hdAssets embed.FS

//go:embed hdassets/world.json
var hdWorldArtJSON []byte

var hdBitmapCache = map[string]*ebiten.Image{}
var hdVectorCache = map[string]hdIconArt{}

func hdBitmap(name string) *ebiten.Image {
	if v := hdBitmapCache[name]; v != nil {
		return v
	}
	raw, err := hdAssets.ReadFile("hdassets/" + name + ".png")
	if err != nil {
		return nil
	}
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		panic(err)
	}
	v := ebiten.NewImageFromImage(decoded)
	hdBitmapCache[name] = v
	return v
}

type hdIconArt struct {
	Window  []hdMapNode            `json:"windowNodes"`
	Icons   map[string][]hdMapNode `json:"icons"`
	Details map[string][]hdMapNode `json:"detailNodes"`
}

func hdVectorAsset(name string) hdIconArt {
	if art, ok := hdVectorCache[name]; ok {
		return art
	}
	raw, err := hdAssets.ReadFile("hdassets/" + name + ".json")
	if err != nil {
		panic(err)
	}
	var art hdIconArt
	if err := json.Unmarshal(raw, &art); err != nil {
		panic(err)
	}
	hdVectorCache[name] = art
	return art
}

func hdNodesImage(nodes []hdMapNode, w, h, scale int) *ebiten.Image {
	a := &app{scale: scale}
	return a.hdWorldSprite(fmt.Sprintf("nodes/%p/%d/%d", nodes, w, h), w, h, nodes)
}

func (a *app) hdCabinRaster(key string, w, h int) *ebiten.Image {
	if key != "cabin-closed" && key != "cabin-lit" {
		return nil
	}
	if w != 320 || h != 200 {
		return nil
	}
	if img := hdBitmap(fmt.Sprintf("cabin-%s-%d", key, a.viewScale())); img != nil {
		return img
	}
	art := hdWorldAssets()
	nodes := art.CabinClosed
	if key == "cabin-lit" {
		nodes = art.CabinLit
	}
	return a.hdWorldSprite("vector/"+key, 320, 200, nodes)
}

func hdStoredAsset(key string) *ebiten.Image {
	name := "stores-" + strings.ReplaceAll(key, "/", "-")
	if img := hdBitmap(name); img != nil {
		return img
	}
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return nil
	}
	scale, err := strconv.Atoi(parts[1])
	if err != nil || scale < 1 {
		return nil
	}
	if img := hdBitmapCache[name]; img != nil {
		return img
	}
	art := hdVectorAsset("stores")
	if parts[0] == "window" {
		img := hdNodesImage(art.Window, 160, 100, scale)
		hdBitmapCache[name] = img
		return img
	}
	if parts[0] != "icons" {
		return nil
	}
	atlas := ebiten.NewImage(128*scale, 64*scale)
	for code := 0; code < 32; code++ {
		nodes := art.Icons[strconv.Itoa(code)]
		base := hdNodesImage(nodes, 16, 16, scale)
		if base == nil {
			panic(fmt.Sprintf("HD icon %d missing", code))
		}
		cell := ebiten.NewImage(16*scale, 16*scale)
		cell.DrawImage(base, nil)
		if detail := art.Details[strconv.Itoa(code)]; len(detail) > 0 {
			op := &ebiten.DrawImageOptions{Blend: ebiten.BlendSourceAtop}
			cell.DrawImage(hdNodesImage(detail, 16, 16, scale), op)
		}
		if code != 0 {
			back := ebiten.NewImage(16*scale, 16*scale)
			back.Fill(hdReviewColour(0x020508))
			back.DrawImage(cell, nil)
			cell.Dispose()
			cell = back
		}
		hdWorldBlit(atlas, cell, code%8*16*scale, code/8*16*scale)
		cell.Dispose()
	}
	hdBitmapCache[name] = atlas
	return atlas
}

func hdItemImage(code, scale int) *ebiten.Image {
	if code < 0 || code > 31 {
		return nil
	}
	atlas := hdStoredAsset(fmt.Sprintf("icons/%d", scale))
	x, y := code%8*16*scale, code/8*16*scale
	return atlas.SubImage(image.Rect(x, y, x+16*scale, y+16*scale)).(*ebiten.Image)
}

func hdEngineeringAsset(scale int) *ebiten.Image {
	if img := hdBitmap(fmt.Sprintf("engineering-%d", scale)); img != nil {
		return img
	}
	return hdNodesImage(hdVectorAsset("engineering").Window, 160, 100, scale)
}

func (a *app) drawHDCachedStars(screen *ebiten.Image) {
	const w, h = frameViewportW - 6, frameViewportH - 6
	key := fmt.Sprintf("starfield/%d", a.viewScale())
	stars := hdBitmapCache[key]
	if stars == nil {
		nodes := make([]hdMapNode, 0, 76)
		for i := 0; i < 76; i++ {
			seed := uint32(i)*2654435761 + 2246822519
			x, y := float32(seed%uint32(w)), float32(seed>>16%uint32(h))
			ink, r := uint32(0x8295a4), float32(.35)
			if i%7 == 0 {
				ink, r = 0xeaf4fa, .65
			}
			nodes = append(nodes, hdMapNode{Kind: "ellipse", Box: [4]float32{x - r, y - r, 2 * r, 2 * r}, Colour: ink, Width: .45, Clip: [4]int{0, 0, w, h}})
		}
		stars = a.hdWorldSprite("starfield", w, h, nodes)
		hdBitmapCache[key] = stars
	}
	scale := a.viewScale()
	x, y := a.originX+(frameViewportX+3)*scale, a.originY+(frameViewportY+3)*scale
	clip := screen.SubImage(image.Rect(x, y, x+w*scale, y+h*scale)).(*ebiten.Image)
	dx, dy := 0, 0
	if a.state.Screen == game.ScreenTactical {
		dx = ((a.starScrollX % w) + w) % w
		dy = ((a.starScrollY % h) + h) % h
	}
	if dx == 0 && dy == 0 {
		hdWorldBlit(clip, stars, x, y)
		return
	}
	for _, off := range [][2]int{{dx - w, dy - h}, {dx, dy - h}, {dx - w, dy}, {dx, dy}} {
		hdWorldBlit(clip, stars, x+off[0]*scale, y+off[1]*scale)
	}
}
