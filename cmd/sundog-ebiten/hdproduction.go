package main

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	ebitentext "github.com/hajimehoshi/ebiten/v2/text"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"golang.org/x/image/font"
)

func (a *app) drawHDTextWithin(value string, x, y, maxWidth int, c color.RGBA) {
	if value == "" {
		return
	}
	p := hdReviewPainter{a: a, scale: float32(a.viewScale())}
	for size := float32(8); size >= 5.5; size -= .25 {
		face := p.face(size)
		if font.MeasureString(face, value).Ceil() <= maxWidth*a.viewScale() {
			ebitentext.Draw(a.textLayer, value, face, x*a.viewScale()+a.originX, (y+8)*a.viewScale()+a.originY, c)
			return
		}
	}
	panic(fmt.Sprintf("HD text exceeds safe width %d: %s", maxWidth, value))
}

func (a *app) usesHD() bool { return a.state.Tileset == theme.AtariSTHDTiles && a.usesSTLayout() }

func hdPaint(a *app, screen *ebiten.Image, f func(*hdReviewPainter)) {
	p := &hdReviewPainter{a: a, screen: screen, scale: float32(a.viewScale())}
	f(p)
	if p.err != "" {
		panic(p.err)
	}
	for _, op := range p.operations {
		op()
	}
}

// drawHD uses the established layout, menus and view models. The bitmap entry
// points below have independent HD implementations; collision construction
// remains in the original read-only scene path.
func (a *app) drawHD(screen *ebiten.Image, group theme.Group) {
	previous := a.uiLayer
	a.uiLayer = screen
	if a.textLayer == nil || a.textLayer.Bounds() != screen.Bounds() {
		a.textLayer = ebiten.NewImage(screen.Bounds().Dx(), screen.Bounds().Dy())
	}
	a.textLayer.Clear()
	previousCanvas := hdCanvas
	hdCanvas = nativeCanvas{screen, float32(a.viewScale()), float32(a.originX), float32(a.originY)}
	defer func() { screen.DrawImage(a.textLayer, nil); a.uiLayer = previous; hdCanvas = previousCanvas }()
	screen.Fill(hdReviewColour(0x020508))
	if a.state.Landing.Phase != game.LandingIdle && !a.state.Sleep.Active {
		a.drawHDLanding(screen, group)
		return
	}
	if a.showHelp {
		a.drawHeader(screen, group)
		a.drawHelp(screen, group)
		a.drawFooter(screen, group)
		return
	}
	if a.showOptions {
		if !a.options.diskOnly {
			a.drawHeader(screen, group)
		}
		a.drawOptions(screen, group)
		if !a.options.diskOnly {
			a.drawFooter(screen, group)
		}
		return
	}
	if a.state.Screen == game.ScreenTitle {
		a.drawHDTitle(screen)
		return
	}
	if a.state.Screen == game.ScreenNavigation {
		if !a.drawHDNavigation(screen) {
			panic("HD navigation composition failed")
		}
		return
	}
	if a.state.Screen == game.ScreenStores && a.drawHDStores(screen) {
		return
	}
	if a.state.Screen == game.ScreenEngineering && a.drawHDEngineering(screen) {
		return
	}
	if isMenuScreen(a.state.Screen) {
		a.drawHDCover(screen, false)
		a.drawMenu(screen, group)
		a.drawHeader(screen, group)
		a.drawFooter(screen, group)
		return
	}
	if a.state.Screen == game.ScreenOutcome {
		a.drawHDCover(screen, false)
		a.drawOutcome(screen, group)
		a.drawHeader(screen, group)
		a.drawFooter(screen, group)
		return
	}
	switch {
	case a.usesSTShipboard():
		a.drawSTShipboard(screen)
	case a.usesSTShipboardBackdrop():
		a.drawSTShipboard(screen)
		a.drawSTZoom(screen, group, a.lookup(screenTitleKeys[a.state.Screen]))
	case a.usesSTRepair():
		a.drawSTRepair(screen, group)
	case a.usesSTGround():
		if !a.drawSTGround(screen, true) {
			panic("HD ground scene unavailable")
		}
	case a.usesSTGroundBackdrop():
		if !a.drawSTGround(screen, false) {
			panic("HD ground backdrop unavailable")
		}
		switch a.state.Screen {
		case game.ScreenTrade:
			a.drawSTTrade(screen)
		case game.ScreenUniteller:
			a.drawSTTeller(screen)
		case game.ScreenTubeStation:
			a.drawSTTubeStation(screen)
		default:
			a.drawSTZoom(screen, group, a.lookup(screenTitleKeys[a.state.Screen]))
		}
	default:
		a.drawSTFrame(screen)
		a.drawMap(screen, group)
		a.drawScreenPanel(screen, group)
		a.drawSTReadouts(screen)
		if a.state.Screen == game.ScreenTactical {
			a.drawSTTacticalStatus(screen)
		} else {
			a.drawMessageStrip(screen, group)
		}
	}
}

func (a *app) drawHDFrame(screen *ebiten.Image) {
	hdPaint(a, screen, func(p *hdReviewPainter) {
		p.rect(0, 0, 320, 200, 0xa7afb8)
		for _, r := range [][4]float32{{frameLeftX, frameLeftY, frameLeftW, frameLeftH}, {frameGaugeX, frameGaugeY, frameGaugeW, frameGaugeH}, {frameMessageX, frameMessageY, frameMessageW, frameMessageH}, {frameMenuX, frameMenuY, frameMenuW, frameMenuH}} {
			p.panel(r[0], r[1], r[2], r[3], 0xf1df00)
		}
		p.panel(frameViewportX, frameViewportY, frameViewportW, frameViewportH, 0x1258ff)
		p.panel(15, 5, 59, 13, 0x1258ff)
		p.panel(277, 5, 28, 13, 0x1258ff)
		p.text("CONDITION", 16, 0, 56, 5, 5, 4.5, 0xf3f6fa, true)
		p.text("WARP", 278, 0, 25, 5, 5, 4.5, 0xf3f6fa, true)
		for i, label := range [4]string{"FUEL", "GUNS", "SHLD", "WARP"} {
			c := [4]uint32{0x1762ff, 0xf4242d, 0x20ef14, 0xed22ee}[i]
			for j, v := range label {
				p.text(string(v), float32(stGaugeX[i]), 88+float32(j)*8, 7, 7.8, 7, 6, c, true)
			}
		}
		for i, v := range "HULL" {
			p.text(string(v), 274, 142+float32(i)*8, 4.5, 7, 6, 5, 0xf3f6fa, true)
		}
		for i, label := range [6]string{"WARP", "SUB-C", "GUNS", "TAC", "SHLDS", "PILOT"} {
			p.text(label, 288, 131+float32(i)*11, 28, 6, 5.5, 5, 0xf3f6fa, true)
		}
	})
}

func (a *app) drawHDCabin(screen *ebiten.Image) {
	key := "cabin-closed"
	if a.frames/8%5 == 1 {
		key = "cabin-lit"
	}
	hdWorldBlit(screen, a.hdCabinRaster(key, 320, 200), a.originX, a.originY)
	sweep := a.frames / 8 % 10
	white := stPen(1)
	if left := 132 - 6*sweep; left >= 0 {
		uiFillRect(screen, float32(left), 126, 1, 2, white, false)
	}
	if right := 218 + 6*sweep; right <= 319 {
		uiFillRect(screen, float32(right), 126, 1, 2, white, false)
	}
	a.drawSTShipWalker(screen)
	a.drawSTStationLabel(screen)
	if a.usesSTShipboard() && a.stSceneMessageFresh() {
		uiFillRect(screen, stShipMessageX0, stShipMessageY0, stShipMessageW, stShipMessageH, hdReviewColour(0x020508), false)
		uiStrokeLine(screen, stShipMessageX0, stShipMessageY0, stShipMessageX0+stShipMessageW, stShipMessageY0, .6, white, true)
		uiStrokeLine(screen, stShipMessageX0, stShipMessageY0+stShipMessageH-1, stShipMessageX0+stShipMessageW, stShipMessageY0+stShipMessageH-1, .6, white, true)
		a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), stShipMessageX0+3, stShipMessageY0+3, stShipMessageW-6, 2, white)
	}
}

func (a *app) drawHDPerson(screen *ebiten.Image, pattern, x, y, scale int, c color.RGBA) {
	// Independent 7x7 silhouette, preserving the source anchor and mode bounds.
	s := float32(scale)
	xf, yf := float32(x), float32(y)
	if pattern < 4 {
		angle := [4]float64{0, -math.Pi / 4, math.Pi / 2, math.Pi / 4}[pattern]
		rotate := func(x, y float32) (float32, float32) {
			dx, dy := float64(x-3.5), float64(y-3.5)
			return xf + (3.5+float32(dx*math.Cos(angle)-dy*math.Sin(angle)))*s, yf + (3.5+float32(dx*math.Sin(angle)+dy*math.Cos(angle)))*s
		}
		for _, line := range [][4]float32{{2.2, .8, 2.2, 6}, {4.8, .8, 4.8, 6}, {1.2, 1.5, 1.2, 5.5}, {5.8, 1.5, 5.8, 5.5}, {2.2, .8, 4.8, .8}, {2.2, 6, 4.8, 6}} {
			x0, y0 := rotate(line[0], line[1])
			x1, y1 := rotate(line[2], line[3])
			uiStrokeLine(screen, x0, y0, x1, y1, .75*s, c, true)
		}
		x0, y0 := rotate(3.5, 1.5)
		x1, y1 := rotate(3.5, 3)
		uiStrokeLine(screen, x0, y0, x1, y1, 1.5*s, hdReviewColour(0x3ec2d5), true)
		return
	}
	uiFillRect(screen, xf+2*s, yf, 3*s, 2*s, c, true)
	uiFillRect(screen, xf+2*s, yf+2*s, 3*s, 2.5*s, c, true)
	uiFillRect(screen, xf+2.7*s, yf+.5*s, 1.6*s, .5*s, hdReviewColour(0x243745), true)
	uiStrokeLine(screen, xf+.7*s, yf+2.3*s, xf+6*s, yf+3.8*s, .8*s, c, true)
	stride := float32(.8)
	if (a.frames/4)%2 == 1 {
		stride = -.3
	}
	uiStrokeLine(screen, xf+2.7*s, yf+4*s, xf+(2.7-stride)*s, yf+6.4*s, s, c, true)
	uiStrokeLine(screen, xf+4.3*s, yf+4*s, xf+(4.3+stride)*s, yf+6.4*s, s, c, true)
}

func (a *app) drawHDGlyph(screen *ebiten.Image, ch, x, y int, c color.RGBA) {
	xf, yf := float32(x), float32(y)
	switch ch {
	case stCharShipGroundNose:
		hdPaint(a, screen, func(p *hdReviewPainter) { p.parkedShip(x, y) })
	case stCharShipGroundTail:
	case 0:
		a.drawHDPerson(screen, 0, x-3, y-4, 1, c)
	case stCharScannerDot:
		uiStrokeCircle(screen, xf, yf, 1.1, .7, c, true)
	default:
		// Navigation glyphs are painted by hdmaps. Room figures use their own
		// 14x14 bodies. Remaining UI marks preserve their small anchor bounds.
		uiStrokeLine(screen, xf-2, yf, xf+2, yf, .7, c, true)
		uiStrokeLine(screen, xf, yf-3, xf, yf+3, .7, c, true)
	}
}

func (a *app) drawHDStars(screen *ebiten.Image) {
	uiFillRect(screen, frameViewportX+2, frameViewportY+2, frameViewportW-4, frameViewportH-4, hdReviewColour(0x020508), true)
	a.drawHDCachedStars(screen)
}

func (a *app) drawHDEnemy(screen *ebiten.Image, cx, cy int) {
	if !a.state.CombatActive || a.state.CombatRange >= stEnemyWindowD {
		return
	}
	dx, dy := a.state.EnemyBearing/2-0x100, a.state.EnemyPitch/2-0x100
	if absInt(dx) >= stEnemyWindowX || absInt(dy) >= stEnemyWindowY {
		return
	}
	x, y := float32(cx+dx), float32(cy+dy)
	// Seven distinct silhouettes and three aspect phases, inside the same
	// visible enemy bounds. This artwork does not participate in hit testing.
	k := a.state.EnemyType % 7
	phase := a.frames / stEnemyFrameStep % 3
	span := float32(3) + float32(k%3)*.4
	tilt := float32(phase-1) * .55
	c := hdReviewColour(0xf3f6fa)
	uiStrokeLine(screen, x-span, y+2, x-1, y-2-tilt, .8, c, true)
	uiStrokeLine(screen, x-1, y-2-tilt, x+1, y-2+tilt, .8, c, true)
	uiStrokeLine(screen, x+1, y-2+tilt, x+span, y+2, .8, c, true)
	uiStrokeLine(screen, x-span, y+2, x+span, y+2, .8, c, true)
	if k%2 == 0 {
		uiStrokeLine(screen, x-span, y, x+span, y, .7, c, true)
	}
	if k >= 3 {
		uiStrokeLine(screen, x, y-3, x, y+3, .7, c, true)
	}
	if k >= 5 {
		uiStrokeCircle(screen, x, y, .9, .5, hdReviewColour(0x3bd9e0), true)
	}
}

func (a *app) drawHDCover(screen *ebiten.Image, wordmark bool) {
	key := fmt.Sprintf("cover/%d", a.viewScale())
	cover := hdBitmapCache[key]
	if cover == nil {
		cover = ebiten.NewImage(320*a.viewScale(), 200*a.viewScale())
		local := *a
		local.originX = 0
		local.originY = 0
		local.drawHDCoverArt(cover)
		hdBitmapCache[key] = cover
	}
	hdWorldBlit(screen, cover, a.originX, a.originY)
	if wordmark {
		hdPaint(a, screen, func(p *hdReviewPainter) {
			p.text("SUNDOG", 16, 13, 224, 35, 30, 26, 0xde33de, false)
			p.text("FROZEN LEGACY", 18, 49, 215, 12, 9, 8, 0xf0df75, false)
		})
	}
}

func (a *app) drawHDCoverArt(screen *ebiten.Image) {
	hdPaint(a, screen, func(p *hdReviewPainter) {
		p.rect(0, 0, 320, 200, 0x020508)
		for i := 0; i < 120; i++ {
			seed := uint32(i)*2654435761 + 2246822519
			x, y := float32(seed%320), float32(seed>>16%200)
			p.rect(x, y, .35, .35, 0x9bafc0)
		}
		b := hdMapBuilder{clip: [4]int{0, 0, 320, 200}}
		b.ellipse(249, 46, 64, 64, 0x133d89, 0)
		b.ellipse(243, 42, 56, 56, 0x257dc0, 0)
		b.polygon(0x329989, [2]float32{210, 7}, [2]float32{232, 3}, [2]float32{254, 18}, [2]float32{248, 32}, [2]float32{267, 45}, [2]float32{256, 61}, [2]float32{233, 54}, [2]float32{221, 30})
		b.ellipse(28, 102, 14, 14, 0x8d9da9, 0)
		b.ellipse(24, 99, 10, 10, 0xd6dfe4, 0)
		hdTitleFreighter(&b)
		p.drawHDMapScene(b.scene)
	})
}

func (a *app) drawHDTitle(screen *ebiten.Image) {
	if a.showCredits {
		a.drawHDCredits(screen)
		return
	}
	a.drawHDCover(screen, true)
	art := a.originalTitle()
	if art != nil && a.frames-art.enteredFrame < titleLogoFrame {
		return
	}
	x, y, w, h := float32(originalTitleWindowX0), float32(originalTitleWindowY0), float32(originalTitleWindowX1-originalTitleWindowX0+1), float32(originalTitleWindowY1-originalTitleWindowY0+1)
	uiFillRect(screen, x, y, w, h, titleFrameColour(a.titleColorStep()), true)
	uiFillRect(screen, x+2, y+2, w-4, h-4, hdReviewColour(0x020508), true)
	if art != nil && a.originalTitleShowsNotice(art) {
		for i, line := range originalTitleNotice {
			a.drawTextCentered(screen, line, 145, 302, 96+i*10, stPen(1))
		}
		return
	}
	for i, action := range originalTitleActions {
		y := originalTitleRowY(i)
		uiStrokeLine(screen, x+2, float32(y-3), x+w-2, float32(y-3), .6, hdReviewColour(0x1762ff), true)
		a.drawTextWithin(screen, a.lookup(originalTitleLabels[action]), originalTitleRowX, y, originalTitleRowW, stPen(1))
	}
	a.drawTextWithin(screen, a.lookup(a.state.MessageKey), originalTitleRowX, originalTitleRowY(len(originalTitleActions)), originalTitleRowW, titleFrameColour(a.titleColorStep()))
}

func (a *app) drawHDCredits(screen *ebiten.Image) {
	hdPaint(a, screen, func(p *hdReviewPainter) {
		p.rect(0, 0, 320, 200, 0x020508)
		p.text("SUNDOG", 16, 8, 288, 22, 20, 18, 0xe6cd4b, true)
		p.text(a.lookup("ui.credits.ftl_production"), 16, 34, 288, 10, 8, 7, 0x47db68, true)
		for i, row := range [][2]string{{"ui.credits.director", "Doug Bell"}, {"ui.credits.ship_sound_effects", "Mike Newton"}, {"ui.credits.graphics", "Andy Jaros"}, {"ui.credits.producer", "Wayne Holder"}} {
			y := float32(56 + i*17)
			p.text(a.lookup(row[0]), 16, y, 184, 10, 8, 7, 0x5ddde1, false)
			p.text(row[1], 205, y, 99, 10, 8, 7, 0xf1f4ed, false)
		}
		p.text(a.lookup("ui.credits.original_concept"), 16, 131, 288, 9, 8, 7, 0xe7e9e7, true)
		p.text("Bruce Webster / Wayne Holder", 16, 143, 288, 9, 8, 7, 0xe7e9e7, true)
		p.text(a.lookup("ui.credits.studios"), 16, 162, 288, 9, 8, 7, 0xe7e9e7, true)
		p.text("San Diego, CA.  (619) 453-5711", 16, 175, 288, 9, 8, 7, 0xe7e9e7, true)
	})
}

func (a *app) drawHDLanding(screen *ebiten.Image, group theme.Group) {
	horizon := a.state.Landing.Horizon
	if band, ok := a.state.Landing.StarBand(); ok && band > 0 {
		top := horizon - band
		if top < 0 {
			top = 0
		}
		for i := 0; i < 96; i++ {
			mixed := uint32(i)*2654435761 + 2246822519
			x := int(mixed % 320)
			y := top + int((mixed>>16)%uint32(band))
			if y >= 0 && y < 200 {
				uiStrokeCircle(screen, float32(x), float32(y), .4, .45, stPen(1), true)
			}
		}
	}
	if horizon >= 0 && horizon < 200 {
		a.drawSkyline(horizon, group)
		uiFillRect(screen, 0, float32(horizon+1), 320, float32(199-horizon), group.Palette[group.UI.Panel], true)
		uiStrokeLine(screen, 0, float32(horizon), 320, float32(horizon), .7, stPen(1), true)
	}
	a.drawTextWithin(screen, a.lookup("ui.message.landing"), 12, statusStripY+1, 296, stPen(1))
}

func hdSmoothCircle(screen *ebiten.Image, cx, cy, r int, c color.RGBA) bool {
	if screen != hdCanvas.target {
		return false
	}
	uiStrokeCircle(screen, float32(cx), float32(cy), float32(r), .65, c, true)
	return true
}
