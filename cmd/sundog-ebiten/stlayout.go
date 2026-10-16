package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

// The Atari ST layout: the original's frame, drawn where the original draws
// it (docs/ui/atari-st-layout-plan.md, measured on the running original and
// on the frame picture, raw 0x14670). With the player's disk in hand the frame
// is the picture itself; without it the same boxes are drawn in the same
// colours. Text is the remake's own face, only its places are the original's.

const (
	stFramePicture = 10 // image 10, raw 0x14670

	// The left box menu: menu 4 of WINDOWLI, eight rows of 13 in window 17.
	stMenuX0    = 6
	stMenuX1    = 83
	stMenuY0    = 21
	stMenuPitch = 13
	stMenuH     = 12
	stMenuRows  = 8
	stMenuFrame = 2
	stMenuTextY = 2

	// Readouts.
	stConditionX0, stConditionX1 = 16, 72
	stConditionY0, stConditionY1 = 6, 16
	stWarpX0, stWarpX1           = 278, 303
	stWarpY0, stWarpY1           = 6, 16
	stClockX0, stClockX1         = 144, 209
	stClockY                     = 121

	// Gauges: window 11-14, bars up from y 85, one pixel per four points.
	stGaugeBottom      = 85
	stGaugeMaxH        = 60
	stGaugeW           = 7
	stHullX0, stHullX1 = 279, 282
	stHullY0, stHullY1 = 134, 165

	// The six bands down the right of the message box: label row, then four
	// lights, one band every 11 rows from 137.
	stBandY0    = 137
	stBandPitch = 11
	stLightW    = 4
	stLightGap  = 3
	stLightX0   = 289
	stLightY    = 0
	stLightH    = 4
)

var stGaugeX = [4]int{273, 283, 293, 303}

// Palette indices the frame uses (XSTARTUP:0x02 set_palette).
const (
	stColourGround   = 8
	stColourBox      = 14
	stColourViewport = 1
	stColourShade    = 9
	stColourMenu     = 9
	stColourWhite    = 15
	stColourGreen    = 10
	stColourRed      = 12
	stColourBlue     = 9
	stColourMagenta  = 13
	stColourYellow   = 14
)

func stColour(index int) color.RGBA { return originalmedia.PaletteRGBA()[index&15] }

func (a *app) usesSTLayout() bool { return a.state.Layout == theme.AtariSTLayout }

// stMenuRowY is the text row of menu row index (0-based) in the left box.
func stMenuRowY(index int) int { return stMenuY0 + index*stMenuPitch + stMenuTextY }

// stFrame is the frame picture from the disk, decoded once per disk; nil
// without a disk, and then the frame is drawn.
func (a *app) stFrame() *ebiten.Image {
	if a.stFrameRead {
		return a.stFrameImage
	}
	a.stFrameRead = true
	if a.originalDisk == nil {
		return nil
	}
	picture, err := a.originalDisk.DecodeImage(stFramePicture)
	if err != nil {
		log.Printf("原版介面框讀不出來，改用自繪的框：%v", err)
		return nil
	}
	a.stFrameImage = pictureImage(picture, originalmedia.PaletteRGBA())
	return a.stFrameImage
}

// drawSTFrame paints the frame: the picture, or the same boxes drawn.
func (a *app) drawSTFrame(screen *ebiten.Image) {
	if a.usesHD() {
		a.drawHDFrame(screen)
		return
	}

	if picture := a.stFrame(); picture != nil {
		screen.DrawImage(picture, &ebiten.DrawImageOptions{})
		return
	}
	screen.Fill(stColour(stColourGround))
	box := func(x, y, w, h int, border int) {
		uiFillRect(screen, float32(x), float32(y), float32(w), float32(h), stColour(border), false)
		uiFillRect(screen, float32(x+1), float32(y+1), float32(w-2), float32(h-2), color.RGBA{0, 0, 0, 255}, false)
	}
	box(frameLeftX, frameLeftY, frameLeftW, frameLeftH, stColourBox)
	box(frameGaugeX, frameGaugeY, frameGaugeW, frameGaugeH, stColourBox)
	box(frameMessageX, frameMessageY, frameMessageW, frameMessageH, stColourBox)
	// The viewport: two-pixel top/left in colour 1, right/bottom in colour 9.
	uiFillRect(screen, frameViewportX, frameViewportY, frameViewportW, frameViewportH, stColour(stColourViewport), false)
	uiFillRect(screen, frameViewportX+2, frameViewportY+2, frameViewportW-2, frameViewportH-2, stColour(stColourShade), false)
	uiFillRect(screen, frameViewportX+2, frameViewportY+2, frameViewportW-4, frameViewportH-4, color.RGBA{0, 0, 0, 255}, false)
	// The two readout boxes.
	box(stConditionX0-1, stConditionY0-1, stConditionX1-stConditionX0+3, stConditionY1-stConditionY0+3, stColourShade)
	box(stWarpX0-1, stWarpY0-1, stWarpX1-stWarpX0+3, stWarpY1-stWarpY0+3, stColourShade)
	// The clock band and its two arrows.
	uiFillRect(screen, 141, 120, 72, 10, color.RGBA{0, 0, 0, 255}, false)
	uiFillRect(screen, 141, 122, 3, 6, stColour(11), false)
	uiFillRect(screen, 210, 122, 3, 6, stColour(11), false)
	// Gauge letters and the band labels: the picture spells them one letter
	// per row; the drawn frame writes the same letters.
	for column, word := range []string{"FUEL", "GUNS", "SHLD", "WARP"} {
		colour := []int{stColourBlue, stColourRed, stColourGreen, stColourMagenta}[column]
		for row, letter := range word {
			a.drawText(screen, string(letter), stGaugeX[column], 88+row*9, stColour(colour))
		}
	}
	for row, letter := range "HULL" {
		a.drawText(screen, string(letter), stHullX0-1, 166+row*8, stColour(stColourWhite))
	}
	for band, label := range []string{"WARP", "SUB-C", "GUNS", "TAC", "SHLDS", "PILOT"} {
		a.drawText(screen, label, stLightX0, stBandY0+band*stBandPitch-7, stColour(stColourWhite))
	}
}

// drawSTReadouts fills the condition box, writes the warp setting and the
// clock, and lights the gauges: the parts of the frame that carry state.
func (a *app) drawSTReadouts(screen *ebiten.Image) {
	condition := stColourGreen
	if a.state.CombatActive {
		condition = stColourRed
	}
	uiFillRect(screen, stConditionX0, stConditionY0, stConditionX1-stConditionX0+1, stConditionY1-stConditionY0+1, stColour(condition), false)
	// The manual's page 8 calls this box PROBABILITY WARP SUCCESS, and the
	// original works it out in XMOVEONS:0x03 (screen rule move.warp-probability):
	// (polar radius + 7) * 2, printed as three digits with leading zeros. The
	// radius is how far the ship is from the star -- the same quantity the
	// original feeds to polar_to_rect -- so a jump gets safer the further out
	// you are, reaching 100 at radius 43, which is the system map's own ring.
	// The WARP gauge in the gauge box is a separate thing, the warp charge.
	uiFillRect(screen, stWarpX0, stWarpY0, stWarpX1-stWarpX0+1, stWarpY1-stWarpY0+1, color.RGBA{0, 0, 0, 255}, false)
	a.drawTextCentered(screen, fmt.Sprintf("%03d", game.WarpSuccessPercent(a.state)), stWarpX0, stWarpX1, stWarpY0+1, stColour(stColourWhite))
	// The fourth gauge is a separate reading from the box above: the original
	// fills it from Ge0, the warp charge (screen rule move.gauge-sources). This
	// shell has no charge to accumulate -- its jump is one step -- so the bar
	// carries the jump distance instead. WORKLIST B has the note.
	warp := 0
	if cost, ok := a.reducer.RouteCost(a.state); ok {
		warp = cost.JumpDistance
	}
	minutes := a.state.TimeMinutes
	clock := fmt.Sprintf("%03d/%02d:%02d", minutes/(24*60), minutes/60%24, minutes%60)
	uiFillRect(screen, stClockX0, 120, stClockX1-stClockX0+1, 10, color.RGBA{0, 0, 0, 255}, false)
	a.drawTextCentered(screen, clock, stClockX0, stClockX1, stClockY, stColour(stColourWhite))

	gauges := [4]struct {
		value, maximum, colour int
	}{
		{a.state.Fuel, 255, stColourBlue},
		{a.state.WeaponCharge, 255, stColourRed},
		{a.state.ShieldLevel, 255, stColourGreen},
		{warp, 10, stColourMagenta},
	}
	for index, gauge := range gauges {
		x := stGaugeX[index]
		uiFillRect(screen, float32(x), float32(stGaugeBottom-stGaugeMaxH+1), stGaugeW, stGaugeMaxH, color.RGBA{0, 0, 0, 255}, false)
		if gauge.maximum <= 0 {
			continue
		}
		height := clampInt(gauge.value*stGaugeMaxH/gauge.maximum, 0, stGaugeMaxH)
		if height > 0 {
			uiFillRect(screen, float32(x), float32(stGaugeBottom-height+1), stGaugeW, float32(height), stColour(gauge.colour), false)
		}
	}
	// HULL: SHIPLIB:0x11 bay_lights(6) fills window 115 from the bottom with
	// twice the hull value in rows, green above 30, yellow from 11 to 30,
	// red at 10 and under, and clears what is left above it.
	hullH := stHullY1 - stHullY0 + 1
	uiFillRect(screen, stHullX0, stHullY0, stHullX1-stHullX0+1, float32(hullH), color.RGBA{0, 0, 0, 255}, false)
	if height := clampInt(a.state.HullCondition*stHullRowsPerPoint, 0, hullH); height > 0 {
		uiFillRect(screen, stHullX0, float32(stHullY1-height+1), stHullX1-stHullX0+1, float32(height), stColour(stHullColour(a.state.HullCondition)), false)
	}
	// The six bands are the six ship systems in the original's record order
	// and the four lights are the four rows of each: bay_lights(0..5) reads
	// the status byte of every row and fills a 4×4 cell per row.
	for band := 0; band < 6; band++ {
		y := stBandY0 + band*stBandPitch + stLightY
		for cell := 0; cell < 4; cell++ {
			x := stLightX0 + cell*(stLightW+stLightGap)
			status := 0
			if band < len(a.state.ShipSystems) {
				status = game.ShipSystemRowStatus(a.state.ShipSystems[band], cell)
			}
			uiFillRect(screen, float32(x), float32(y), stLightW, stLightH, stColour(stLightColour(status)), false)
		}
	}
}

const stHullRowsPerPoint = 2

// stHullColour is bay_lights' choice for the hull bar, by the bar's height.
func stHullColour(hull int) int {
	switch rows := hull * stHullRowsPerPoint; {
	case rows > 30:
		return stColourGreen
	case rows >= 11:
		return stColourYellow
	default:
		return stColourRed
	}
}

// stLightColour is bay_lights' jump table on a row's status byte: 0 stays
// black, 1 and 2 are red, 3 is yellow, 4 (a working row) is green.
func stLightColour(status int) int {
	switch status {
	case 0:
		return 0
	case 1, 2:
		return stColourRed
	case 3:
		return stColourYellow
	default:
		return stColourGreen
	}
}

// drawSTMenuButton is one row of the left box menu, in the original's box
// style: a two-pixel blue frame, black inside, white text.
func (a *app) drawSTMenuButton(screen *ebiten.Image, row int, label string) {
	y := stMenuY0 + row*stMenuPitch
	if row >= stMenuRows {
		return
	}
	uiFillRect(screen, stMenuX0, float32(y), stMenuX1-stMenuX0+1, stMenuH, stColour(stColourMenu), false)
	uiFillRect(screen, stMenuX0+stMenuFrame, float32(y+stMenuFrame), stMenuX1-stMenuX0+1-2*stMenuFrame, stMenuH-2*stMenuFrame, color.RGBA{0, 0, 0, 255}, false)
	a.drawTextCentered(screen, label, stMenuX0+stMenuFrame+1, stMenuX1-stMenuFrame, stMenuRowY(row), stColour(stColourWhite))
}

// --- Tactical: the status window and the scanners (XDOFIGHT, screen-geometry.md) ---

const (
	stScannerY                       = 164
	stScannerBearingX                = 37
	stScannerPitchX                  = 102
	stScannerR                       = 31
	stScannerArm                     = 21
	stStatusX0, stStatusX1           = 135, 271
	stStatusY0, stStatusY1           = 132, 196
	stStatusOriginX, stStatusOriginY = 137, 133
	stStatusIDX, stStatusIDY         = 140, 134
	stStatusNameX                    = 166
	stStatusDamageX, stStatusDamageY = 148, 184
	stScannerDotDivisor              = 4
	stScannerDotLimit                = 28
	stScannerDotPen                  = 3
	stCharScannerDot                 = 4
)

// stScannerAngle is plot_dot's fold of a two-axis angle onto the sine
// table: divide by four (toward zero), negatives get 0x100, positives are
// taken mod 0xff.
func stScannerAngle(v int) int {
	a := v / 4
	if a < 0 {
		return a + 0x100
	}
	return a % 0xff
}

// VDI pen to hardware palette entry, the ST low-resolution default order. The
// drawing procedures name VDI pens; the frame picture and set_color name
// hardware entries. The scanner circles are pen 1 = entry 15 (white), the
// bearing vector pen 10 = entry 9 (blue), the pitch vector pen 12 = entry 12
// (red) — all three checked on the running original.
var vdiPen = [16]int{0, 15, 1, 2, 4, 6, 3, 5, 7, 8, 9, 10, 12, 14, 11, 13}

func stPen(pen int) color.RGBA { return stColour(vdiPen[pen&15]) }

// stLine and stCircle put ink on the same pixels VDI does: both ends of a
// line, and a midpoint circle of radius r that spans 2r+1 pixels. Ebiten's
// stroke primitives are anti-aliased float shapes and land one pixel short.
func stLine(screen *ebiten.Image, x0, y0, x1, y1 int, c color.RGBA) {
	if screen == hdCanvas.target {
		uiStrokeLine(screen, float32(x0), float32(y0), float32(x1), float32(y1), .7, c, true)
		return
	}
	if x0 > x1 {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	put := func(px, py int) { uiFillRect(screen, float32(px), float32(py), 1, 1, c, false) }
	dx, dy := x1-x0, y1-y0
	yinc := 1
	if dy < 0 {
		yinc, dy = -1, -dy
	}
	if dx >= dy {
		eps, y := 2*dy-dx, y0
		for x := x0; x <= x1; x++ {
			put(x, y)
			if eps >= 0 {
				y += yinc
				eps -= 2 * dx
			}
			eps += 2 * dy
		}
		return
	}
	eps, x := 2*dx-dy, x0
	for i := 0; i <= dy; i++ {
		put(x, y0+i*yinc)
		if eps >= 0 {
			x++
			eps -= 2 * dy
		}
		eps += 2 * dx
	}
}

// stCircle is VDI's v_ellarc as the original calls it: a 32-gon whose
// vertices are r·cos/r·sin truncated toward zero, joined by VDI's line
// rule (drawn left to right, x-major or y-major, error test ≥ 0). Checked
// against the running original's scanner circles: 2 pixels of 178 differ.
func stCircle(screen *ebiten.Image, cx, cy, r int, c color.RGBA) {
	if hdSmoothCircle(screen, cx, cy, r, c) {
		return
	}
	const sides = 32
	vertices := make([][2]int, sides)
	for i := range vertices {
		angle := 2 * math.Pi * float64(i) / sides
		vertices[i] = [2]int{cx + int(float64(r)*math.Cos(angle)), cy + int(float64(r)*math.Sin(angle))}
	}
	for i := range vertices {
		next := vertices[(i+1)%sides]
		stLine(screen, vertices[i][0], vertices[i][1], next[0], next[1], c)
	}
}

// drawSTTacticalStatus draws the bottom of the tactical screen: the two
// scanners in the message box and the status window with the enemy's outline.
func (a *app) drawSTTacticalStatus(screen *ebiten.Image) {
	white := stPen(1)
	for _, cx := range []int{stScannerBearingX, stScannerPitchX} {
		stCircle(screen, cx, stScannerY, stScannerR, white)
	}
	blue, red := stPen(10), stPen(12)
	bx, by := stScannerBearingX, stScannerY
	stLine(screen, bx, by, bx-stScannerArm, by-stScannerArm, blue)
	stLine(screen, bx, by, bx+stScannerArm, by-stScannerArm, blue)
	px, py := stScannerPitchX, stScannerY
	stLine(screen, px-stScannerArm, py-stScannerArm, px, py, red)
	stLine(screen, px-stScannerArm, py+stScannerArm, px, py, red)
	// The scanner dots (tactical.scanner-dot): radius distance/4, out of the
	// circle at 28 and over; the bearing scanner takes Gd0 + 0x100, the pitch
	// scanner −Gcf, each ÷4 and folded to 0–255 the way plot_dot does;
	// draw_char(4, 3, …) puts character 4 in pen 3 at polar_to_rect + centre.
	if a.state.CombatActive {
		if r := a.state.CombatRange / stScannerDotDivisor; r < stScannerDotLimit {
			dot := stPen(stScannerDotPen)
			ax, ay := game.PolarToRect(stScannerAngle(a.state.EnemyBearing+0x100), r)
			a.drawSTChar(screen, stCharScannerDot, bx+ax, by+ay, dot)
			ex, ey := game.PolarToRect(stScannerAngle(-a.state.EnemyPitch), r)
			a.drawSTChar(screen, stCharScannerDot, px+ex, py+ey, dot)
		}
	}

	uiFillRect(screen, stStatusX0, stStatusY0, stStatusX1-stStatusX0+1, stStatusY1-stStatusY0+1, red, false)
	uiFillRect(screen, stStatusX0+2, stStatusY0+1, stStatusX1-stStatusX0-3, stStatusY1-stStatusY0-1, color.RGBA{0, 0, 0, 255}, false)
	a.drawText(screen, "ID:", stStatusIDX, stStatusIDY, white)
	if !a.state.CombatActive {
		return
	}
	if a.state.CombatActive {
		a.drawTextWithin(screen, a.reducer.EnemyShipName(a.state), stStatusNameX, stStatusIDY, stStatusX1-4-stStatusNameX, red)
	}
	green := stPen(11)
	points := game.EnemyShipOutline(a.state)
	for index := 0; index+1 < len(points); index++ {
		stLine(screen, stStatusOriginX+points[index][0], stStatusOriginY+points[index][1],
			stStatusOriginX+points[index+1][0], stStatusOriginY+points[index+1][1], green)
	}
	damage := ""
	if a.state.EnemyMaxHull > 0 {
		damage = fmt.Sprintf("%d%%", 100-a.state.EnemyHull*100/a.state.EnemyMaxHull)
	}
	a.drawText(screen, a.lookup("ui.tactical.damage")+" "+damage, stStatusDamageX, stStatusDamageY, stPen(13))
}

// --- Navigation: the maps drawn in the viewport (XREADMAP) ---

const (
	stMapOriginX  = 96 // window 6
	stMapOriginY  = 6
	stSectorStepX = 12
	stSectorStepY = 10
	stSectorInset = 8
)

// drawSTSectorChart is map.sector-projection and map.sector-star-glyphs in
// the viewport: x = X×12 + 104, y = Y×10 + 14, r = 6 − Z/2.
func (a *app) drawSTSectorChart(screen *ebiten.Image) {
	// Window 6 is cleared to colour 0 first; the chart has no starfield.
	uiFillRect(screen, sceneX, sceneY, frameViewportW-4, frameViewportH-4, color.RGBA{0, 0, 0, 255}, false)
	white := stPen(1)
	target, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)
	for _, system := range a.reducer.World.Systems() {
		c, _ := a.reducer.SystemCoordinate(a.state, system.ID)
		x := stMapOriginX + stSectorInset + c[0]*stSectorStepX
		y := stMapOriginY + stSectorInset + c[1]*stSectorStepY
		r := 6 - c[2]/2
		if r <= 0 {
			// Character 0x40 at (x−1, y+1): its ink is a diamond around (x, y).
			a.drawSTChar(screen, stCharFarStar, x-1, y+1, white)
		} else {
			stCircle(screen, x, y, r, white)
		}
		// The box round the current system (pen 10) and the destination
		// (pen 12 flashing against 0): a five-point polyline one pixel
		// outside the circle, at least two out (XREADMAP:0x03 dispsect).
		box := func(colour color.RGBA) {
			rr := r + 1
			if rr < 2 {
				rr = 2
			}
			stLine(screen, x-rr, y-rr, x+rr, y-rr, colour)
			stLine(screen, x+rr, y-rr, x+rr, y+rr, colour)
			stLine(screen, x+rr, y+rr, x-rr, y+rr, colour)
			stLine(screen, x-rr, y+rr, x-rr, y-rr, colour)
		}
		if system.ID == a.state.SystemID {
			box(stPen(10))
		}
		if hasTarget && system.ID == target.SystemID && system.ID != a.state.SystemID {
			if a.frames/15%2 == 0 {
				box(stPen(12))
			} else {
				box(color.RGBA{0, 0, 0, 255})
			}
		}
	}
}

// drawSTViewportCross is the own-ship mark at the viewport centre: a red
// cross of five dots, as on the running original.
func (a *app) drawSTViewportCross(screen *ebiten.Image) {
	// Measured on the running original: centre (176,61), two dots each way.
	cx, cy := 176, 61
	red := stPen(12)
	for _, d := range [][2]int{{-3, 0}, {-2, 0}, {2, 0}, {3, 0}, {0, -3}, {0, -2}, {0, 2}, {0, 3}} {
		uiFillRect(screen, float32(cx+d[0]), float32(cy+d[1]), 1, 1, red, false)
	}
	a.drawSTViewportEnemy(screen, cx, cy)
}

// The enemy in the viewport (tactical.enemy-center, tactical.viewport-window):
// Δ = angle/2 − 0x100 on each axis, drawn only while |Δx| < 75, |Δy| < 51
// and the distance is under 108. The figure is one of the font strip's
// small glyphs: XDOFIGHT:0x04 set_up_status picks the base 3×type (types
// 0–6 → 0, 3, …, 18) and show_status steps through the three frames as the
// ship moves.
const (
	stEnemyWindowX   = 75
	stEnemyWindowY   = 51
	stEnemyWindowD   = 108
	stEnemyFrames    = 3
	stEnemyFrameStep = 8 // display frames per figure frame (remake pacing)
)

func (a *app) drawSTViewportEnemy(screen *ebiten.Image, cx, cy int) {
	if a.usesHD() {
		a.drawHDEnemy(screen, cx, cy)
		return
	}

	if !a.state.CombatActive || a.state.CombatRange >= stEnemyWindowD {
		return
	}
	dx, dy := a.state.EnemyBearing/2-0x100, a.state.EnemyPitch/2-0x100
	if absInt(dx) >= stEnemyWindowX || absInt(dy) >= stEnemyWindowY {
		return
	}
	font := a.stFont()
	if font == nil {
		return
	}
	glyph := font.SmallGlyph(a.state.EnemyType*stEnemyFrames + int(a.frames/stEnemyFrameStep)%stEnemyFrames)
	white := stPen(1)
	for gy := 0; gy < originalmedia.GlyphHeight; gy++ {
		for gx := 0; gx < glyph.Width; gx++ {
			if glyph.Set(gx, gy) {
				uiFillRect(screen, float32(cx+dx-3+gx), float32(cy+dy-4+gy), 1, 1, white, false)
			}
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// --- Characters and backdrops from the disk ---

// stFont is the original's character strip, read once per disk.
func (a *app) stFont() *originalmedia.Font {
	if a.stFontRead {
		return a.stFontStrip
	}
	a.stFontRead = true
	if a.originalDisk == nil {
		return nil
	}
	font, err := a.originalDisk.Font()
	if err != nil {
		log.Printf("原版字元條讀不出來，記號改用自繪：%v", err)
		return nil
	}
	a.stFontStrip = font
	return font
}

// drawSTChar is draw_char(ch, mode, x, y): the six-by-eight character centred
// on (x, y). Without the disk a two-by-two dot stands in.
func (a *app) drawSTChar(screen *ebiten.Image, ch, x, y int, colour color.RGBA) {
	if a.usesHD() {
		a.drawHDGlyph(screen, ch, x, y, colour)
		return
	}

	font := a.stFont()
	if font == nil {
		uiFillRect(screen, float32(x-1), float32(y-1), 2, 2, colour, false)
		return
	}
	glyph := font.Glyph(ch)
	for gy := 0; gy < originalmedia.GlyphHeight; gy++ {
		for gx := 0; gx < glyph.Width; gx++ {
			if glyph.Set(gx, gy) {
				uiFillRect(screen, float32(x-3+gx), float32(y-4+gy), 1, 1, colour, false)
			}
		}
	}
}

// stBackdrop is one of the disk's pictures as an image, cached by index.
func (a *app) stBackdrop(index int) *ebiten.Image {
	if a.stBackdrops == nil {
		a.stBackdrops = map[int]*ebiten.Image{}
	}
	if image, ok := a.stBackdrops[index]; ok {
		return image
	}
	var image *ebiten.Image
	if a.originalDisk != nil {
		if picture, err := a.originalDisk.DecodeImage(index); err == nil {
			image = pictureImage(picture, originalmedia.PaletteRGBA())
		} else {
			log.Printf("原版影像 %d 讀不出來：%v", index, err)
		}
	}
	a.stBackdrops[index] = image
	return image
}

// --- Navigation: the planet map and the system map (XREADMAP disp_planet, disp_system) ---

const (
	stPlanetBackdrop = 11
	stSystemBackdrop = 12
	stSystemClipRows = 96
	stSystemRingR    = 43
	stSystemCentreX  = 177
	stSystemCentreY  = 63
	stSystemRingN    = 16
	stCharCity       = 20
	stCharCityTarget = 0x3c
	stCharShip       = 9
	stCharPlanet     = 6
	stCharRingDot    = 21
	stCharFarStar    = 0x40
	stOrbitShipX     = 7*((40+1)/2) + 4 + 7 // grid (40,3), then seven to the right
	stOrbitShipY     = 4*3 + 2
)

// drawSTPlanetMap: image 11 in window 6, the cities as character 20 at their
// change_xy pixel (pen 5; the city the player is in pen 1), the chosen
// landing city as 0x3c, and the ship in orbit as character 9 at grid (40,3)
// moved seven right — all measured on the running original (font-strip.md).
func (a *app) drawSTPlanetMap(screen *ebiten.Image) {
	uiFillRect(screen, sceneX, sceneY, frameViewportW-4, frameViewportH-4, color.RGBA{0, 0, 0, 255}, false)
	if backdrop := a.stBackdrop(stPlanetBackdrop); backdrop != nil {
		options := &ebiten.DrawImageOptions{}
		options.GeoM.Translate(stMapOriginX, stMapOriginY)
		screen.DrawImage(backdrop, options)
	}
	target, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)
	planetID := a.state.PlanetID
	if hasTarget {
		planetID = target.PlanetID
	}
	shipShown := false
	for _, destination := range a.reducer.DestinationsOnPlanet(a.state, planetID) {
		if destination.Hidden {
			// The colony is not in the planet's run of city records, so the
			// original's loop never draws it (checked on the Jondd map).
			continue
		}
		x, y := stMapOriginX+destination.MapX, stMapOriginY+destination.MapY
		ch, colour := stCharCity, stPen(5)
		if destination.CityID == a.state.CityID {
			colour = stPen(1)
		}
		if hasTarget && destination.CityID == target.CityID && destination.CityID != a.state.CityID {
			ch = stCharCityTarget
		}
		a.drawSTChar(screen, ch, x, y, colour)
		// XREADMAP:0x06 disp_planet compares each city against G4a's field
		// (6,9) -- the city the ship is parked in -- and draws the ship glyph
		// over that same square in pen 8. So a landed ship shows next to its
		// city rather than in the orbit corner, which is what the manual's
		// page 8 says.
		if destination.CityID == a.state.ShipCityID {
			a.drawSTChar(screen, stCharShip, x, y, stPen(8))
			shipShown = true
		}
	}
	// The orbit corner is only for a ship this planet's cities did not claim.
	if planetID == a.state.PlanetID && a.state.Screen != game.ScreenGround && !shipShown {
		a.drawSTChar(screen, stCharShip, stMapOriginX+stOrbitShipX, stMapOriginY+stOrbitShipY, stPen(1))
	}
}

// drawSTSystemMap: image 12 (its first 96 rows) in window 6, the reference
// ring of sixteen character-21 dots at radius 43, the planets as character 6
// on their polar positions, the ship flashing on the planet it orbits. The
// ring centre is the viewport centre; no snapshot of this map exists yet, so
// the centre and the pens are the p-code's reading, not a measurement.
func (a *app) drawSTSystemMap(screen *ebiten.Image) {
	uiFillRect(screen, sceneX, sceneY, frameViewportW-4, frameViewportH-4, color.RGBA{0, 0, 0, 255}, false)
	if backdrop := a.stBackdrop(stSystemBackdrop); backdrop != nil {
		options := &ebiten.DrawImageOptions{}
		options.GeoM.Translate(stMapOriginX, stMapOriginY)
		clipped := backdrop.SubImage(image.Rect(0, 0, backdrop.Bounds().Dx(), stSystemClipRows)).(*ebiten.Image)
		screen.DrawImage(clipped, options)
	}
	// The ring's centre as the sysmap snapshot has it (round fourteen): the
	// sixteen dots' ink sits at (176,62) ± 43. drawSTChar puts character
	// 21's ink one pixel up and left of its anchor, so the anchor is (177,63).
	cx, cy := stSystemCentreX, stSystemCentreY
	white := stPen(1)
	for i := 0; i < stSystemRingN; i++ {
		dx, dy := game.PolarToRect(i*256/stSystemRingN, stSystemRingR)
		a.drawSTChar(screen, stCharRingDot, cx+dx, cy+dy, white)
	}
	target, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)
	systemID := a.state.SystemID
	if hasTarget {
		systemID = target.SystemID
	}
	for _, planet := range a.reducer.World.PlanetsInSystem(systemID) {
		orbit := a.reducer.PlanetOrbit(a.state, planet)
		dx, dy := game.PolarToRect(orbit.Angle, orbit.Radius)
		a.drawSTChar(screen, stCharPlanet, cx+dx, cy+dy, white)
	}
	// The ship goes on its own polar position, not on the planet's. It used to
	// be drawn over the planet the player was standing on, read off the round
	// fourteen sysmap snapshot where the two happened to coincide; the running
	// original disagrees — in the tenth round's capture the ship sits at (179,40)
	// and the planet at (145,67). Overlapping them also fused character 9 with
	// character 6 into one 36-pixel blob where the original has 28
	// (docs/atari-st/ux-scene-ledger.md, tenth round).
	if systemID == a.state.SystemID && a.frames/15%2 == 0 {
		sx, sy := systemMapShipOffset(a.state)
		a.drawSTChar(screen, stCharShip, cx+sx, cy+sy, white)
	}
}

// systemMapShipOffset is where the ship goes on the system map, measured from
// the star at the centre. It is the ship's own polar position — flight.go
// advances FlightAngle/FlightRadius each tick — and not the planet's, which is
// what the map used to use. The two coincide while the ship is parked, because
// state.go fills a zero radius in from the planet it is standing on; they part
// company in flight, and the original's own capture shows the ship out on the
// orbit while the planet sits elsewhere.
func systemMapShipOffset(state game.State) (int, int) {
	return game.PolarToRect(state.FlightAngle, state.FlightRadius)
}

// --- Shipboard: the corridor picture full screen, the stations as the window table has them ---

// stStation is one of the station rectangles XMOVEONS:0x0a get_location tests
// the walker against (windows 65–77 of the window table,
// docs/atari-st/window-menu-tables.json), and what XMOVEONS:0x0c check_enc does
// when the walker is in it: code 1 pilot_disp, 2 handle_pod, 4 handle_airlock,
// every other code SHIPLIB:0x05 do_repairs for that system.
//
// Twelve, not thirteen: get_location's loop runs 0x41..0x4d but skips 0x43
// outright (`L1 != 0x43`), so window 67 is not a station at all. The remake
// used to dispatch repairs from it.
type stStation struct {
	window         int
	x0, y0, x1, y1 int
	action         game.Action
	labelKey       string
}

var stStations = []stStation{
	{65, 60, 90, 64, 101, game.ActionOpenNavigation, "ui.pilotage"},
	{66, 21, 58, 30, 73, game.ActionOpenCargo, "ui.cargo_pod"},
	{68, 68, 108, 80, 116, game.ActionOpenGround, "ui.airlock"},
	{69, 103, 101, 111, 112, game.ActionOpenEngineering, ""},
	{70, 68, 68, 80, 76, game.ActionOpenStores, "ui.ship_stores"},
	{71, 32, 23, 44, 29, game.ActionOpenEngineering, ""},
	{72, 129, 29, 145, 37, game.ActionOpenEngineering, ""},
	{73, 146, 34, 154, 46, game.ActionOpenEngineering, ""},
	{74, 129, 46, 145, 51, game.ActionOpenEngineering, ""},
	{75, 129, 145, 145, 153, game.ActionOpenEngineering, ""},
	{76, 146, 150, 154, 162, game.ActionOpenEngineering, ""},
	{77, 129, 161, 145, 167, game.ActionOpenEngineering, ""},
}

const (
	stShipMessageX0 = 160
	stShipMessageY0 = 172
	stShipMessageW  = 156
	stShipMessageH  = 24
)

// usesSTShipboard: the full-screen corridor needs the picture from the disk;
// without it the shipboard keeps the framed layout.
func (a *app) usesSTShipboard() bool {
	if !a.usesSTLayout() || a.state.Screen != game.ScreenShipboard {
		return false
	}
	_, _, ok := a.shipInteriorImages()
	return ok
}

// stStationAt is the station a layout pixel is in.
func stStationAt(x, y int) (stStation, bool) {
	for _, station := range stStations {
		if x >= station.x0 && x <= station.x1 && y >= station.y0 && y <= station.y1 {
			return station, true
		}
	}
	return stStation{}, false
}

// drawSTShipboard paints the corridor picture over the whole frame with its
// two light animations (palette blink and bay sweep, as drawShipInterior),
// then the walker, and then the two overlays the remake keeps: the name of the
// station the walker is standing in, and a message window that fades. The
// original has neither — it has no text on this screen at all — so both are
// held to the moment they say something (stwalker.go).
func (a *app) drawSTShipboard(screen *ebiten.Image) {
	if a.usesHD() {
		a.drawHDCabin(screen)
		return
	}

	off, on, _ := a.shipInteriorImages()
	const beatFrames = 8
	beat := int(a.frames) / beatFrames
	backdrop := off
	if beat%5 == 1 {
		backdrop = on
	}
	screen.DrawImage(backdrop, &ebiten.DrawImageOptions{})
	sweep := beat % 10
	white := stPen(1)
	if left := 0x84 - 6*sweep; left >= 0 {
		uiFillRect(screen, float32(left), 0x7e, 1, 2, white, false)
	}
	if right := 0xda + 6*sweep; right <= 319 {
		uiFillRect(screen, float32(right), 0x7e, 1, 2, white, false)
	}
	a.drawSTShipWalker(screen)
	a.drawSTStationLabel(screen)
	if !a.usesSTShipboard() || !a.stSceneMessageFresh() {
		return
	}
	uiFillRect(screen, stShipMessageX0, stShipMessageY0, stShipMessageW, stShipMessageH, color.RGBA{0, 0, 0, 255}, false)
	uiFillRect(screen, stShipMessageX0, stShipMessageY0, stShipMessageW, 1, stPen(1), false)
	uiFillRect(screen, stShipMessageX0, stShipMessageY0+stShipMessageH-1, stShipMessageW, 1, stPen(1), false)
	a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), stShipMessageX0+3, stShipMessageY0+3, stShipMessageW-6, 2, white)
}

// drawSTStationLabel names the station the walker is standing in. The original
// names nothing: the walker arrives and the station's own window opens, and
// its title bar carries the name. The remake keeps a name here because three
// of the thirteen stations lead somewhere with no window of its own, but only
// while the walker is actually in one — a name that is always on the screen
// covers the corridor art and was the thing that made the picture unreadable.
func (a *app) drawSTStationLabel(screen *ebiten.Image) {
	station, ok := a.shipWalker.stationUnder()
	if !ok || station.labelKey == "" {
		return
	}
	label := a.lookup(station.labelKey)
	x := station.x1 + 3
	if x+a.textWidth(label) > logicalWidth-2 {
		x = station.x0 - 3 - a.textWidth(label)
	}
	y := (station.y0+station.y1)/2 - 4
	uiFillRect(screen, float32(x-1), float32(y), float32(a.textWidth(label)+2), 9, color.RGBA{0, 0, 0, 200}, false)
	a.drawText(screen, label, x, y, stPen(13))
}

// --- Ground: the city map full screen (XMOVEONG:0x0b wmap, ground-rules.md 〈地面怎麼畫〉) ---

const (
	stGroundPitch      = 40 // 32 + Lf, Lf = 8
	stGroundInset      = 4  // Lf / 2
	stGroundTile       = 32
	stGroundColumns    = logicalWidth / stGroundPitch  // 8
	stGroundRows       = logicalHeight / stGroundPitch // 5
	stGroundBackdrop   = 30                            // image 30, raw 0x1b166, 48×40; the top-left 40×40 is the ground pattern
	stGroundSolidCode  = 1
	stGroundWalkerX    = 13 // the 14×14 walker centred in the 40-pixel cell
	stGroundWalkerY    = 13
	stGroundMessageX0  = 4
	stGroundMessageY0  = 172
	stGroundMessageW   = 200
	stGroundMessageH   = 24
	stGroundOriginLeft = 4 // the walker sits four cells in and two down when the map allows
	stGroundOriginTop  = 2
)

// stGroundOrigin is the original's scroll: the walker is always the cell
// four in and two down, the map is not clamped to the screen, and cells off
// the map show the ground pattern. Two snapshots fix it: Drahew's origin
// (7,4) with the walker at (11,6) and Heavy's (16,10) with the walker on the
// landing square (20,12), both in the map's own 10-based columns and 6-based
// rows (hatari-live-run.md rounds four and eleven).
func stGroundOrigin(x, y int) (int, int) {
	return x - stGroundOriginLeft, y - stGroundOriginTop
}

// stGroundBackground is the 40×40 ground pattern, cut from image 30.
func (a *app) stGroundBackground() *ebiten.Image {
	if a.stGroundPattern != nil || a.stGroundPatternRead {
		return a.stGroundPattern
	}
	a.stGroundPatternRead = true
	if picture := a.stBackdrop(stGroundBackdrop); picture != nil {
		a.stGroundPattern = picture.SubImage(image.Rect(0, 0, stGroundPitch, stGroundPitch)).(*ebiten.Image)
	}
	return a.stGroundPattern
}

// usesSTGround: the full-screen city needs the disk's tiles.
func (a *app) usesSTGround() bool {
	return a.usesSTLayout() && a.state.Screen == game.ScreenGround && a.originalDisk != nil && a.stGroundBackground() != nil
}

// stOnGround names the screens that only open as windows over the city.  The
// character card is context-sensitive: reducer.State.StatusReturn records
// whether it was opened aboard ship or on the ground.
func stOnGround(screen game.Screen) bool {
	switch screen {
	case game.ScreenTrade, game.ScreenUniteller, game.ScreenTubeStation, game.ScreenParts, game.ScreenDepot, game.ScreenFuelPurchase, game.ScreenFTLBoost,
		game.ScreenDialog, game.ScreenInterior, game.ScreenSlots, game.ScreenArena, game.ScreenScore:
		return true
	}
	return false
}

// usesSTGroundBackdrop: an on-ground screen over the full-screen city.
func (a *app) usesSTGroundBackdrop() bool {
	onGround := stOnGround(a.state.Screen) ||
		(a.state.Screen == game.ScreenCharacterStatus && a.state.StatusReturn == game.ScreenGround)
	return a.usesSTLayout() && onGround && a.originalDisk != nil && a.stGroundBackground() != nil
}

// usesSTShipboardBackdrop is the other legal origin of the character card.
// Keeping the return screen as the discriminator prevents a shipboard status
// window from borrowing the city solely because both contexts share one
// ScreenCharacterStatus value.
func (a *app) usesSTShipboardBackdrop() bool {
	if !a.usesSTLayout() || a.state.Screen != game.ScreenCharacterStatus || a.state.StatusReturn != game.ScreenShipboard {
		return false
	}
	_, _, ok := a.shipInteriorImages()
	return ok
}

// drawSTGround paints the city the way wmap does: every cell first gets the
// ground pattern (32+Lf square, gap included), a solid cell (code 1) is
// painted black, every other code gets its 32×32 tile at the cell's inset.
// park_pods on code 5 and the original's scroll rule are not done (F4).
func (a *app) drawSTGround(screen *ebiten.Image, withMessage bool) bool {
	if a.usesHD() {
		return a.drawHDGround(screen, withMessage)
	}

	// The pixel walk draws the window it collides against, so it has its own
	// path: one blit of the composed scene, then the sprite over it
	// (stground.go). The cell walk below stays for the two presentations whose
	// tiles are the remake's own.
	if a.usesSTGroundWalker() || a.state.GroundView.Valid {
		if !a.groundWalker.ready || a.groundWalker.cityID != a.state.CityID || a.groundWalker.world != a.state.GroundWorld {
			a.resetSTGroundWalker()
		}
		if scene := a.stGroundSceneFor(a.state.CityID, a.groundWalker.scrollX, a.groundWalker.scrollY); scene != nil {
			screen.DrawImage(scene.picture, &ebiten.DrawImageOptions{})
			a.drawSTGroundMarks(screen, scene)
			a.drawSTGroundWalker(screen)
			if withMessage {
				a.drawSTGroundMessage(screen)
			}
			return true
		}
	}
	originX, originY := stGroundOrigin(a.state.Position.X, a.state.Position.Y)
	// The origin can be off the map; the walker's own cell names the city.
	_, cityNumber, ok := game.GroundTerrainCode(a.state, a.state.CityID, a.state.Position.X, a.state.Position.Y)
	if !ok {
		return false
	}
	cache := a.originalTiles[cityNumber]
	if cache == nil {
		set, err := a.originalDisk.LoadCityTiles(cityNumber)
		if err != nil {
			log.Printf("城市 %d 的原版圖塊讀不出來，改用重製版：%v", cityNumber, err)
			a.originalTiles[cityNumber] = &originalTileCache{}
			return false
		}
		cache = &originalTileCache{set: set, images: map[int]*ebiten.Image{}}
		a.originalTiles[cityNumber] = cache
	}
	if cache.set == nil {
		return false
	}
	background := a.stGroundBackground()
	// The six building slots draw the header's building tile over the cell.
	buildingCode := map[[2]int]int{}
	for _, building := range game.GroundCityBuildings(a.state, a.state.CityID) {
		buildingCode[[2]int{building.Row, building.Column}] = building.Code
	}
	screen.Fill(color.RGBA{0, 0, 0, 255})
	for row := 0; row < stGroundRows; row++ {
		for column := 0; column < stGroundColumns; column++ {
			cellX, cellY := originX+column, originY+row
			px, py := float64(column*stGroundPitch), float64(row*stGroundPitch)
			code, _, ok := game.GroundTerrainCode(a.state, a.state.CityID, cellX, cellY)
			if ok && code == stGroundSolidCode {
				continue
			}
			options := &ebiten.DrawImageOptions{}
			options.GeoM.Translate(px, py)
			screen.DrawImage(background, options)
			if !ok {
				// Off the map: the pattern alone, as the original shows it.
				continue
			}
			roll := (cellX*7 + cellY*13) & 3
			if building, ok := buildingCode[[2]int{cellX, cellY}]; ok {
				// A building's code names its tile outright; no die is rolled.
				code, roll = building, 0
			}
			if tile := cache.tile(code, roll); tile != nil {
				tileOptions := &ebiten.DrawImageOptions{}
				tileOptions.GeoM.Translate(px+stGroundInset, py+stGroundInset)
				screen.DrawImage(tile, tileOptions)
			}
			if code == stGroundPodCode {
				a.drawSTPods(screen, cellX, cellY, int(px), int(py))
			}
		}
	}
	// draw_map is wmap then put_ship then the sprites, so the parked ship goes
	// over the finished cells and under the walker.
	if column, row, ok := a.groundShipCell(originX, originY, stGroundColumns, stGroundRows); ok {
		a.drawGroundShip(screen, column*stGroundPitch, row*stGroundPitch, stGroundPitch)
	}
	playerX, playerY := a.state.Position.X-originX, a.state.Position.Y-originY
	if playerX >= 0 && playerX < stGroundColumns && playerY >= 0 && playerY < stGroundRows {
		a.drawGroundWalker(screen, playerX*stGroundPitch+stGroundWalkerX, playerY*stGroundPitch+stGroundWalkerY)
	}
	if !withMessage {
		return true
	}
	a.drawSTGroundMessage(screen)
	return true
}

// --- park_pods: the vehicles parked on a code 5 cell (XMOVEONG:0x0c) ---
//
// wmap calls park_pods after drawing a cell whose screen code is 0x21, the
// one the 4-bit code 5 becomes. It walks a table of parking places filled
// by init_movement:set_parking -- nine in an ordinary city, six in Enlie --
// and draws character 0 of the font strip at each with probability
// density/20, where density is the fourth 4-bit field of the star system's
// record (word +9, the one world-data.json calls priceLevel). The places are
// offsets from the cell's origin plus (7, 1), which the Drahew snapshot
// fixes: its six vehicles all sit on table entries (hatari-live-run.md,
// round thirteen). The original rolls the die on every redraw, so its
// vehicles flicker; the remake rolls once per place from the cell, which is
// listed under the deliberate differences (WORKLIST B). The cell park_pods
// tests against is where the ground pod is parked, not the landing square; the
// SunDog itself is put_ship's job, drawn over the finished map by
// drawGroundShip.
const (
	stGroundPodCode = 5
	stPodOriginX    = 7
	stPodOriginY    = 1
	stPodChar       = 0
	stPodDieSides   = 20
)

var stPodPlaces = [][2]int{{2, 7}, {2, 19}, {2, 31}, {10, 7}, {10, 19}, {10, 31}, {18, 31}, {26, 19}, {26, 31}}
var stPodPlacesEnlie = [][2]int{{3, 8}, {3, 28}, {10, 8}, {10, 28}, {18, 28}, {25, 28}}

// stPodParked is the remake's die for one place: fixed by the cell and the
// place so the street does not flicker.
func stPodParked(cellX, cellY, place, density int) bool {
	roll := (cellX*31 + cellY*17 + place*7 + 3) % stPodDieSides
	if roll < 0 {
		roll += stPodDieSides
	}
	return roll < density
}

func (a *app) drawSTPods(screen *ebiten.Image, cellX, cellY, px, py int) {
	system, ok := a.reducer.World.System(a.state.SystemID)
	if !ok {
		return
	}
	places := stPodPlaces
	if a.state.SystemID == game.EnlieSystemID {
		places = stPodPlacesEnlie
	}
	white := stPen(1)
	for place, offset := range places {
		if !stPodParked(cellX, cellY, place, system.PriceLevel) {
			continue
		}
		a.drawSTChar(screen, stPodChar, px+stPodOriginX+offset[0], py+stPodOriginY+offset[1], white)
	}
}

// drawSTGroundMessage is the small message window at the bottom left of a
// full-screen city. It only shows while the message is fresh: the original has
// no permanent window there, and a window that never goes away covers the
// bottom-left row of cells.
func (a *app) drawSTGroundMessage(screen *ebiten.Image) {
	if !a.stSceneMessageFresh() {
		return
	}
	white := stPen(1)
	uiFillRect(screen, stGroundMessageX0, stGroundMessageY0, stGroundMessageW, stGroundMessageH, color.RGBA{0, 0, 0, 255}, false)
	uiFillRect(screen, stGroundMessageX0, stGroundMessageY0, stGroundMessageW, 1, white, false)
	uiFillRect(screen, stGroundMessageX0, stGroundMessageY0+stGroundMessageH-1, stGroundMessageW, 1, white, false)
	a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), stGroundMessageX0+3, stGroundMessageY0+3, stGroundMessageW-6, 2, white)
}

// stGroundActionAt: a click on a cell walks toward it, going round what is in
// the way (groundStepToward).
func (a *app) stGroundActionAt(x, y int) game.Action {
	originX, originY := stGroundOrigin(a.state.Position.X, a.state.Position.Y)
	target := game.Position{X: originX + x/stGroundPitch, Y: originY + y/stGroundPitch}
	return a.groundStepToward(target)
}

// --- Zoom windows over the city: the shops, the bank, the talk, the card ---
//
// The original opens each of these with WINDOWLI:0x15 make_zoom(style, gx,
// gy): the window's inside starts at (gx×8+1, gy×8+1) and is 158×98 for
// every style under 12 (80×50 for the little message windows), with the
// yellow edge one pixel outside; the choices are the bands of menu 0 along
// the bottom, one every 13 rows. Where a window opens is the walker's
// screen position ÷ 8, clamped per screen — and the walker always sits on
// the screen's cell (4,2), so every window has one place (screen-geometry.md,
// the zoom.* rules):
//
//	trade         (11, gx≤19, gy≤10)  → inside (153, 81)
//	character card (0, gx 11–19, gy≤9) → (153, 73)
//	arena         (11, gx 3–18, gy 3–9) → (145, 73)
//	interior      (5–8, door cell ÷ 8, gy+1) → (161, 89)
//	depot / bank / tube  (11, rand 8–12, rand 3–9): random; the Drahew
//	depot was measured at (9, 3) → (73, 25), which the remake fixes on.
//
// The depot snapshot (hatari-live-run.md, round thirteen) fixes the rest:
// the title bar at rows 1–13 (red outline, white, blue text), the fill
// below it, three bands at y0+56, +69, +82 whose text sits four rows in,
// the bottom edge at y0+98. The remake anchors the bands to the bottom
// and lets the window grow only when its rows would not fit in 100.
const (
	stZoomW           = 160 // edge to edge
	stZoomH           = 100
	stZoomTitleH      = 13 // rows y0+1 .. y0+13
	stZoomContentY    = 14 // the fill starts at y0+14
	stZoomInner       = 2  // the fill is two in from the edge (the edge and a black column)
	stZoomBandPitch   = 13
	stZoomBandH       = 15
	stZoomBandTextY   = 4
	stZoomBandFrame   = 2
	stZoomBottom      = 16 // from the last band's top to the bottom edge
	stZoomTitleTextY  = 4
	stZoomMessageX    = 5 // the shop's answer, from the edge (77 on the depot)
	stZoomMessageY    = 17
	stZoomBandBlue    = 9
	stZoomMessageLine = 4
)

// The hardware palette entries the zoom windows are painted from, each one
// read off an original capture (see stZoomStyle).
const (
	stZoomBlack   = 0
	stZoomBlue    = 1
	stZoomGreen   = 2
	stZoomGrey    = 7
	stZoomInkBlue = 9
	stZoomRed     = 12
	stZoomWhite   = 15
)

// stZoomStyle is one window's colours: the rule that closes the title bar
// above and below, the title bar's own fill and the ink on it, the content's
// fill and the ink the content's text takes on that fill.
//
// make_zoom is not what decides them. The depot, the bank and the tube
// station all open through XDOUNITE:0x06 set_marquee with style 11, and the
// exchange opens style 11 as well (XDOTRADI:0x22 init_trade) — one style,
// four different looks — so the colours come from what each caller paints
// over the window afterwards, not from the window routine. Reading that out
// of the p-code would mean decompiling four callers and the GEM binding
// under them; the captures answer the same question exactly, so these are
// proven-by-oracle rather than proven.
type stZoomStyle struct {
	rule  int // the line at the top and the bottom of the title bar
	title int // the title bar's fill
	ink   int // the title's text
	fill  int // the content
	pen   int // the content's own text
}

// The four measured sets. Each capture was measured row by row across the
// full width of the window, so the numbers are the palette entries the
// original writes, not an eyeballed match.
var (
	// Drahew Depot (bldg2/shots/buy1.png): red rule, white bar, blue title,
	// green fill, black text. Also the parts and equipment shops.
	stZoomStyleDepot = stZoomStyle{rule: stZoomRed, title: stZoomWhite, ink: stZoomInkBlue, fill: stZoomGreen, pen: stZoomBlack}
	// Jondd Bank (teller/shots/building.png): red rule, but the bar and the
	// fill are both black and everything written on them is white.
	stZoomStyleBank = stZoomStyle{rule: stZoomRed, title: stZoomBlack, ink: stZoomWhite, fill: stZoomBlack, pen: stZoomWhite}
	// Tube Station (tube3/shots/building.png): red rule, white bar, black
	// title, blue fill, white text.
	stZoomStyleTube = stZoomStyle{rule: stZoomRed, title: stZoomWhite, ink: stZoomBlack, fill: stZoomBlue, pen: stZoomWhite}
	// Drahew Exch. (exch/shots/door2.png): the rule is grey here, not red;
	// white bar, black title, black fill, white text.
	stZoomStyleExchange = stZoomStyle{rule: stZoomGrey, title: stZoomWhite, ink: stZoomBlack, fill: stZoomBlack, pen: stZoomWhite}
)

// stZoomStyleFor picks the set for a screen. Screens with no capture of
// their own take the depot's; that inheritance is the remake's, and saying
// so here is what keeps it from looking measured.
func stZoomStyleFor(screen game.Screen) stZoomStyle {
	switch screen {
	case game.ScreenUniteller:
		return stZoomStyleBank
	case game.ScreenTubeStation:
		return stZoomStyleTube
	case game.ScreenTrade:
		return stZoomStyleExchange
	default:
		return stZoomStyleDepot
	}
}

// stZoomOrigin is the window's edge corner for a screen: the make_zoom
// inside corner minus one.
func stZoomOrigin(screen game.Screen) (int, int) {
	switch screen {
	case game.ScreenTrade:
		return 152, 80
	case game.ScreenCharacterStatus, game.ScreenDialog:
		// The talk opens the card style (make_man 1) at the walker too.
		return 152, 72
	case game.ScreenArena:
		return 144, 72
	case game.ScreenInterior, game.ScreenParts, game.ScreenSlots:
		return 160, 88
	case game.ScreenEngineering:
		// XDOREPAI:0x02 init_repairs opens one of five windows by station:
		// (3,9,8) for station 5, (3,10,0) for 6, (4,12,9) for 7,
		// (2,15,4) for 8–10 and (2,15,10) for 11–13. The remake has one
		// repair screen, so it takes station 5's.
		return 72, 64
	case game.ScreenStores:
		// XDOREPAI:02 selector6, native XJP -> make_zoom(3,10,0).
		return 80, 0
	default:
		// The marquee windows: depot, bank, tube station, and the score card.
		return 72, 24
	}
}

// stZoomWindow is the window for one on-ground screen.
type stZoomWindow struct {
	x0, y0, x1, y1 int
	contentH       int
	bands          int
}

func stZoomFor(screen game.Screen, readouts, bands, extra int) stZoomWindow {
	x0, y0 := stZoomOrigin(screen)
	window := stZoomWindow{x0: x0, y0: y0, x1: x0 + stZoomW - 1, bands: bands}
	needed := readouts*floatRowStep + 2*floatPadding + extra
	bandsH := 0
	if bands > 0 {
		bandsH = stZoomBandPitch*(bands-1) + stZoomBottom
	} else {
		bandsH = 3
	}
	window.contentH = stZoomH - 1 - stZoomContentY - bandsH
	if window.contentH < needed {
		// Remake-owned: the original's windows never grow; ours has more
		// rows than the depot's, so it takes the height it needs.
		window.contentH = needed
	}
	window.y1 = window.y0 + stZoomContentY + window.contentH + bandsH
	if window.y1 > logicalHeight-1 {
		window.y0 -= window.y1 - (logicalHeight - 1)
		window.y1 = logicalHeight - 1
		if window.y0 < 0 {
			window.y0 = 0
		}
	}
	return window
}

// content is the fill as a floating window, so the readout helpers draw
// into it unchanged.
func (w stZoomWindow) content() floatWindow {
	return floatWindow{x: w.x0 + stZoomInner, y: w.y0 + stZoomContentY, w: stZoomW - 2*stZoomInner, h: w.contentH}
}

// bandY is the top row of band i, counted from the bottom edge up.
func (w stZoomWindow) bandY(i int) int {
	return w.y1 - stZoomBottom - (w.bands-1-i)*stZoomBandPitch
}

// bandAt is which band the point is on.
func (w stZoomWindow) bandAt(x, y int) (int, bool) {
	if x < w.x0+stZoomInner || x > w.x1-stZoomInner {
		return 0, false
	}
	for i := 0; i < w.bands; i++ {
		if y >= w.bandY(i) && y < w.bandY(i)+stZoomBandPitch {
			return i, true
		}
	}
	return 0, false
}

// stZoomOf is the zoom window the current screen draws.
func (a *app) stZoomOf() (stZoomWindow, []string, int) {
	lines := a.screenPanelLines()
	readouts := clampInt(panelReadoutLines(a.state.Screen), 0, len(lines))
	extra := 0
	if a.state.Screen == game.ScreenCharacterStatus {
		extra = originalmedia.ItemIconSize + 4
	}
	return stZoomFor(a.state.Screen, readouts, len(lines)-readouts, extra), lines, readouts
}

// drawSTZoom paints the window: edge, title bar, fill, the readouts in the
// fill and one band per button.
func (a *app) drawSTZoom(screen *ebiten.Image, group theme.Group, title string) {
	if a.state.Screen == game.ScreenArena && a.usesSTArena() {
		// 原版的競技場視窗裡沒有選單，整塊內容區就是場地。
		a.drawSTArena(screen)
		return
	}
	window, lines, readouts := a.stZoomOf()
	black := color.RGBA{0, 0, 0, 255}
	innerX := float32(window.x0 + stZoomInner)
	innerW := float32(stZoomW - 2*stZoomInner)
	if a.state.Screen == game.ScreenInterior {
		if backdrop := a.interiorBackdropImage(); backdrop != nil {
			// A room is its picture: draw_zoom paints the 160×100 image over
			// the whole zoom window, edge included, with no title bar. The
			// remake's choices stay as bands along the bottom (its own).
			options := &ebiten.DrawImageOptions{}
			options.GeoM.Translate(float64(window.x0), float64(window.y0))
			if a.usesHD() {
				hdWorldBlit(screen, backdrop, a.originX+window.x0*a.viewScale(), a.originY+window.y0*a.viewScale())
			} else {
				screen.DrawImage(backdrop, options)
			}
			a.drawSTRoomPeople(screen, window)
			// The original has no menu in the room until the player reaches
			// the bartender (handle_bartender: within five of them); the
			// remake's bands come up at the counter, at the slots, or at the
			// door, and stay out of the picture while walking.
			if !(a.state.Interior.AtCounter() || a.state.Interior.AtSlots() || a.state.Interior.AtDoor() || a.state.Interior.BoothIndex() >= 0) {
				return
			}
			for index, line := range lines {
				if index < readouts {
					continue
				}
				band := index - readouts
				y := float32(window.bandY(band))
				uiFillRect(screen, innerX, y, innerW, stZoomBandH, stColour(stZoomBandBlue), false)
				uiFillRect(screen, innerX+stZoomBandFrame, y+stZoomBandFrame, innerW-2*stZoomBandFrame, stZoomBandH-2*stZoomBandFrame, black, false)
				a.drawTextCentered(screen, line, window.x0+stZoomInner+stZoomBandFrame, window.x1-stZoomInner-stZoomBandFrame+1, window.bandY(band)+stZoomBandTextY, stColour(stColourWhite))
			}
			return
		}
	}
	style := stZoomStyleFor(a.state.Screen)
	content := a.drawSTZoomFrameStyle(screen, window, title, style)
	if a.stZoomMessageFresh() {
		// A shop's answer: the fill goes black, the text starts five in
		// and seventeen down from the edge, the bands are gone until
		// delay_button times out or a click ends it (the depot's "Your pod
		// isn't here for us to load", round thirteen).
		uiFillRect(screen, innerX, float32(content.y), innerW, float32(window.y1-content.y), black, false)
		a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), window.x0+stZoomMessageX, window.y0+stZoomMessageY,
			window.x1-stZoomInner-(window.x0+stZoomMessageX), stZoomMessageLine, stColour(stColourWhite))
		return
	}
	// The fill.
	uiFillRect(screen, float32(content.x), float32(content.y), float32(content.w), float32(content.h), stColour(style.fill), false)
	if a.state.Screen == game.ScreenCharacterStatus {
		a.drawCompass(screen, group, content)
		a.drawCarriedIcons(screen, content)
	}
	for index, line := range lines {
		if index < readouts {
			a.drawTextWithin(screen, line, content.columnX(0, 0), content.rowY(index, floatRowStep), content.textWidth(), stColour(style.pen))
			continue
		}
		band := index - readouts
		y := float32(window.bandY(band))
		uiFillRect(screen, innerX, y, innerW, stZoomBandH, stColour(stZoomBandBlue), false)
		uiFillRect(screen, innerX+stZoomBandFrame, y+stZoomBandFrame, innerW-2*stZoomBandFrame, stZoomBandH-2*stZoomBandFrame, black, false)
		a.drawTextCentered(screen, line, window.x0+stZoomInner+stZoomBandFrame, window.x1-stZoomInner-stZoomBandFrame+1, window.bandY(band)+stZoomBandTextY, stColour(stColourWhite))
	}
}

// drawSTZoomFrame paints the edge, the black inside and the title bar in the
// screen's own colours, and hands back the fill's rectangle.
func (a *app) drawSTZoomFrame(screen *ebiten.Image, window stZoomWindow, title string) floatWindow {
	return a.drawSTZoomFrameStyle(screen, window, title, stZoomStyleFor(a.state.Screen))
}

// drawSTZoomFrameStyle is the same frame with a named colour set.
func (a *app) drawSTZoomFrameStyle(screen *ebiten.Image, window stZoomWindow, title string, style stZoomStyle) floatWindow {
	black := color.RGBA{0, 0, 0, 255}
	x0, y0 := float32(window.x0), float32(window.y0)
	w, h := float32(stZoomW), float32(window.y1-window.y0+1)
	innerX := float32(window.x0 + stZoomInner)
	innerW := float32(stZoomW - 2*stZoomInner)
	uiFillRect(screen, x0, y0, w, h, stColour(stColourBox), false)
	uiFillRect(screen, x0+1, y0+1, w-2, h-2, black, false)
	// The title bar: the rule, then the bar, then the title on it.
	uiFillRect(screen, x0+1, y0+1, w-2, stZoomTitleH, stColour(style.rule), false)
	uiFillRect(screen, innerX, y0+2, innerW, stZoomTitleH-2, stColour(style.title), false)
	a.drawTextCentered(screen, title, window.x0+stZoomInner, window.x1-stZoomInner+1, window.y0+stZoomTitleTextY, stColour(style.ink))
	return window.content()
}

// --- Repairs: XDOREPAI's window over the ship picture ---

// usesSTRepair: the repair screen is a zoom window over the ship's
// corridor picture, so it needs the disk's images.
func (a *app) usesSTRepair() bool {
	if !a.usesSTLayout() || (a.state.Screen != game.ScreenEngineering && a.state.Screen != game.ScreenStores) {
		return false
	}
	_, _, ok := a.shipInteriorImages()
	return ok
}

// drawSTRepair draws the ship picture and the repair window on it: the
// original's make_man slots and the system's part grid (strepair.go).
func (a *app) drawSTRepair(screen *ebiten.Image, group theme.Group) {
	off, _, _ := a.shipInteriorImages()
	screen.DrawImage(off, &ebiten.DrawImageOptions{})
	if a.state.Screen == game.ScreenStores {
		window := stZoomFor(game.ScreenStores, 0, 0, 0)
		a.drawSTZoomFrame(screen, window, a.lookup("ui.ship_stores"))
		a.drawSTStoresWindow(screen, window)
		return
	}
	window := stZoomFor(game.ScreenEngineering, 0, 0, 0)
	title := a.lookup(screenTitleKeys[game.ScreenEngineering])
	if selected, ok := a.state.SelectedShipSystem(); ok {
		title = a.lookup(game.ShipSystemLabelKey(selected.ID))
	}
	a.drawSTZoomFrame(screen, window, title)
	a.drawSTRepairWindow(screen, window)
	_ = group
}

// The message a shop answers with is shown in the window for a moment.
const stZoomMessageFrames = 120 // two seconds; the original's delay_button timeout is not measured

// stZoomMessageFresh: a message arrived on this screen within the last
// stZoomMessageFrames frames and has not been clicked away.
func (a *app) stZoomMessageFresh() bool {
	return a.stMessageFrame > 0 && a.frames-a.stMessageFrame < stZoomMessageFrames && a.state.MessageKey != ""
}

// The full-screen city and shipboard message windows fade on their own clock.
const stSceneMessageFrames = 240 // four seconds; remake-owned, the original has no such window

// stSceneMessageFresh: the message the full-screen scenes show. Unlike the zoom
// window this one counts a message that arrived with a screen change, because
// "you landed" and "you boarded" are exactly the messages worth reading.
func (a *app) stSceneMessageFresh() bool {
	return a.stSceneMessageFrame > 0 &&
		a.frames-a.stSceneMessageFrame < stSceneMessageFrames &&
		a.state.MessageKey != ""
}

// stZoomActionAt maps a click on a band to the action the panel row has: the
// bands are the panel's button rows, in order, so the row's click band
// (built for the left box) names the action and the zoom window names where
// it is.
func (a *app) stZoomActionAt(x, y int) (game.Action, bool) {
	window, _, readouts := a.stZoomOf()
	band, ok := window.bandAt(x, y)
	if !ok {
		return "", false
	}
	middle := window.x0 + stZoomW/2
	for _, spot := range panelMouseHotspots() {
		if spot.screen != a.state.Screen {
			continue
		}
		if rowY, _ := panelRowYFor(a.state.Screen, readouts+band); spot.minY != rowY {
			continue
		}
		// A split row keeps its halves.
		if spot.minX != panelX && x < middle {
			continue
		}
		if spot.maxX != panelX+panelW && x >= middle {
			continue
		}
		return spot.action, true
	}
	return "", false
}
