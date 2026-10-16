package main

// The city ground at the original's resolution.
//
// Until 2026-09-06 the remake walked the city a cell at a time and kept the
// player pinned to screen cell (4, 2). The original does none of that: the
// walker is a 7×7 sprite that moves toward the cursor a pixel at a time, what
// stops it is the *colour of the pixels under it*, and the map scrolls only
// when the sprite reaches a margin. All three were listed as deliberate
// differences in WORKLIST B because the remake's own tiles could never match
// the original pixel for pixel — but under the Atari ST presentation the
// ground *is* the original's pixels, decoded from the player's own disk, so
// there the whole model can be run as the original runs it.
//
// The three rules, each with a byte signature in ground-rules.json:
//
//   - **通行**（`passability.colour-flags` ＋ `passability.city-street`）：
//     `GEMBIND:0x28 collide(pattern, x, y)` 取樣 `(x−4+dx, y−4+dy)`，任何一點的
//     顏色在 `GEMBIND_G230` 的旗標為 1 就是不能站。城市的字串是
//     `0111110001011111`，可站的是色 0、6、7、8、10。
//   - **捲動**（`scroll.margin-chain` 等五條）：sprite 座標除以 8 之後 x < 5／
//     x > 34／y < 5／y > 19 各捲一格，x 先於 y；捲完原點 mod 40／mod 24，
//     走路人在畫面上往回退一格。
//   - **圖塊的骰子**（`tile.look-roll`）：`draw_city_map` 每次重畫都擲。原版只在
//     捲動時重畫，所以這裡把畫面存進一張持久的緩衝，捲動才重擲——同一格前後兩次
//     可以長得不一樣，而不是每幀閃。
//
// What stays the remake's: the walking speed (`do_sprite` 的速度沒有量過) and
// the keyboard nudge, which the original has no equivalent of.

import (
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/world"
)

const (
	// XMOVEONG get_marks raw 0x40cfe：步行地面使用固定圖樣 4。
	// 來源及 READY 審查見 docs/atari-st/native-collision-entry-20260908.md。
	stGroundFootPattern = 4
	// stGroundPassableFlags is the string XMOVEONG:0x23 init_city hands to
	// set_coll_color: a '1' means that colour cannot be stood on.
	stGroundPassableFlags = "0111110001011111"
	// stGroundWalkerSize is the walker sprite's box. The original's city
	// walker is the same 7×7 as the cabin's; the remake used to draw it at
	// twice that to suit a 40-pixel cell, which put the picture and the
	// collision box out of step.
	stGroundWalkerSize = 7
	// stGroundWalkerSpeed is how many pixels a frame the walker covers on each
	// axis. do_sprite's own speed is not measured, so this is the remake's,
	// the same number the cabin walker uses.
	stGroundWalkerSpeed = stWalkerSpeed
	// The margin chain, in the units update_position compares in: the sprite's
	// coordinates divided by eight.
	stGroundScrollUnit  = 8
	stGroundMarginLow   = 5
	stGroundMarginRight = 34
	stGroundMarginDown  = 19
)

// stGroundPassable is the flag string read as "can this colour be stood on".
var stGroundPassable = func() [16]bool {
	var out [16]bool
	for i := 0; i < 16; i++ {
		out[i] = stGroundPassableFlags[i] == '0'
	}
	return out
}()

// stGroundScene is one drawn window of the city: the picture the player sees
// and the palette indices behind it, which is what the collision reads. It is
// rebuilt when the window moves, and the tile dice are rolled then — which is
// the original's own timing, since redraw_map is what rolls them.
type stGroundScene struct {
	world            bool
	pod              bool // 載具在基底遮罩上另阻擋色彩7、8。
	blocked          uint16
	shipCity         string
	shipPosition     game.Position
	cityID           string
	cityNumber       int // source record resolved from the bounded camera anchor
	scrollX, scrollY int
	rebuilds         int
	colonyPhase      int
	colonySite       game.ColonySite
	picture          *ebiten.Image
	indices          []byte
	// marks 是城市背景上的停放載具與陽犬號，已以 drawSTChar 的相同
	// 字元／錨點／VDI pen 1（硬體索引 15）合成到 picture 與 indices。
	marks        []stGroundMark
	surfaceCodes [24][40]int
}

// stGroundMark is one font character drawn over the finished window.
type stGroundMark struct {
	ch   int
	x, y int
}

// stGroundWalker is the sprite on the city ground.
type stGroundWalker struct {
	world            bool
	ready            bool
	cityID           string
	x, y             int // sprite box top-left, layout pixels
	scrollX, scrollY int // window origin, cells
	// direction 保存羅盤方向。步行繪製／碰撞固定為4；載具使用pattern。
	direction int
	pattern   int // 載具方向圖樣0–3，與羅盤北南西東編號分離。
}

// anchor is the collision anchor: the sprite box's top-left plus the four
// collide subtracts, so the sample box lands on the sprite box.
func (w stGroundWalker) anchor() (int, int) {
	return w.x + originalmedia.CollideAnchorOffset, w.y + originalmedia.CollideAnchorOffset
}

// cell is the map cell under the walker, by the original's own chain:
// column = (scrollX + x div 40) mod 40, row = (scrollY + y div 40) mod 24.
// The remake's city map is 20×12, so the wrap is that map's instead — the
// 40×24 buffer holds the city and the ground around it, which this shell does
// not model.
func (w stGroundWalker) cell() (int, int) {
	state := game.State{GroundWorld: w.world}
	x, y := w.anchor()
	pitch := game.GroundPitch(state)
	p := game.GroundWrap(state, game.Position{X: w.scrollX + x/pitch, Y: w.scrollY + y/pitch})
	return p.X, p.Y
}

// stGroundBackgroundIndices is image 30's top-left 40×40, the pattern the
// original tiles the gaps and the empty cells with, as palette indices.
func (a *app) stGroundBackgroundIndices() []byte {
	if a.stGroundPatternIndices != nil {
		return a.stGroundPatternIndices
	}
	if a.originalDisk == nil {
		return nil
	}
	picture, err := a.originalDisk.DecodeImage(stGroundBackdrop)
	if err != nil {
		log.Printf("地表圖樣讀不出來：%v", err)
		return nil
	}
	out := make([]byte, stGroundPitch*stGroundPitch)
	for y := 0; y < stGroundPitch; y++ {
		for x := 0; x < stGroundPitch; x++ {
			if y < picture.Height && x < picture.Width {
				out[y*stGroundPitch+x] = picture.Pixels[y*picture.Width+x] & 0x0f
			}
		}
	}
	a.stGroundPatternIndices = out
	return out
}

// stGroundSceneFor hands back the scene for a window, rebuilding it when the
// window has moved. Rebuilding is when the tile dice are rolled.
func (a *app) stGroundSceneFor(cityID string, scrollX, scrollY int) *stGroundScene {
	if a.stGroundScene != nil {
		// init_movement的模式旗標不改地圖外觀，不觸發重畫或外觀亂數。
		a.stGroundScene.pod = a.state.GroundMode == game.GroundInPod
	}
	if a.stGroundScene != nil && a.stGroundScene.cityID == cityID &&
		a.stGroundScene.scrollX == scrollX && a.stGroundScene.scrollY == scrollY &&
		a.stGroundScene.world == a.state.GroundWorld &&
		(!a.state.GroundWorld || a.stGroundScene.colonySite == a.state.ColonySite) &&
		(cityID != world.MissionDestinationID || a.stGroundScene.colonyPhase == a.state.Colony.Phase) && a.stGroundScene.shipCity == a.state.ShipCityID && a.stGroundScene.shipPosition == a.state.ShipPosition {
		return a.stGroundScene
	}
	rebuilds := 0
	if a.stGroundScene != nil {
		rebuilds = a.stGroundScene.rebuilds + 1
	}
	scene := a.buildSTGroundScene(cityID, scrollX, scrollY, rebuilds)
	a.stGroundScene = scene
	return scene
}

// buildSTGroundScene composes the window as palette indices and turns those
// into the picture. Composing indices rather than drawing tiles is what makes
// the collision the original's: collide reads the bitplanes, so the remake has
// to have the same numbers in the same places.
func (a *app) buildSTGroundScene(cityID string, scrollX, scrollY, rebuilds int) *stGroundScene {
	if a.state.GroundWorld {
		return a.buildSTSurfaceScene(scrollX, scrollY, rebuilds)
	}
	background := a.stGroundBackgroundIndices()
	if background == nil {
		return nil
	}
	_, cityNumber, ok := game.GroundTerrainCode(a.state, cityID, clampInt(scrollX+stGroundOriginLeft, 0, world.MapWidth()-1),
		clampInt(scrollY+stGroundOriginTop, 0, world.MapHeight()-1))
	if !ok {
		return nil
	}
	cache := a.originalTiles[cityNumber]
	if cache == nil {
		set, err := a.originalDisk.LoadCityTiles(cityNumber)
		if err != nil {
			log.Printf("城市 %d 的原版圖塊讀不出來：%v", cityNumber, err)
			a.originalTiles[cityNumber] = &originalTileCache{}
			return nil
		}
		cache = &originalTileCache{set: set, images: map[int]*ebiten.Image{}}
		a.originalTiles[cityNumber] = cache
	}
	if cache.set == nil {
		return nil
	}
	buildingCode := map[[2]int]int{}
	for _, building := range game.GroundCityBuildings(a.state, cityID) {
		buildingCode[[2]int{building.Row, building.Column}] = building.Code
	}
	// The die: rolled once per drawn cell, as draw_city_map does, and only on
	// a rebuild — which is when the original redraws too.
	roll := newGroundRoller(cityID, scrollX, scrollY, rebuilds)
	indices := make([]byte, logicalWidth*logicalHeight)
	marks := []stGroundMark{}
	for row := 0; row < stGroundRows; row++ {
		for column := 0; column < stGroundColumns; column++ {
			cell := game.GroundWrap(a.state, game.Position{X: scrollX + column, Y: scrollY + row})
			cellX, cellY := cell.X, cell.Y
			px, py := column*stGroundPitch, row*stGroundPitch
			code, _, inMap := game.GroundTerrainCode(a.state, cityID, cellX, cellY)
			if inMap && code == stGroundSolidCode {
				// Code 1 is color_rect in the city's own colour: black.
				stGroundFill(indices, px, py, stGroundPitch, stGroundPitch, 0)
				continue
			}
			stGroundBlit(indices, background, px, py, stGroundPitch, stGroundPitch)
			if !inMap {
				continue
			}
			look := roll()
			if building, ok := buildingCode[[2]int{cellX, cellY}]; ok {
				code, look = building, 0
			}
			if tile, drawn := cache.set.TileIndices(code, look); drawn {
				stGroundBlit(indices, tile, px+stGroundInset, py+stGroundInset,
					originalmedia.TileSize, originalmedia.TileSize)
			}
			if code == stGroundPodCode {
				marks = a.rollSTGroundPods(marks, px, py, roll)
			}
		}
	}
	// draw_map is wmap then put_ship, so the parked ship goes over the cells.
	if column, row, ok := a.groundShipCell(scrollX, scrollY, stGroundColumns, stGroundRows); ok {
		marks = append(marks, stGroundShipMarks(column*stGroundPitch, row*stGroundPitch)...)
	}
	// 原版 collide 讀取 wmap／put_ship 後的螢幕位平面。飛船與停放物件
	// 必須同時存在於碰撞索引，不能只在最後畫到畫面上（Issue #2）。
	a.paintSTGroundMarkIndices(indices, marks)
	colours := originalmedia.PaletteRGBA()
	rgba := image.NewRGBA(image.Rect(0, 0, logicalWidth, logicalHeight))
	for i, index := range indices {
		rgba.SetRGBA(i%logicalWidth, i/logicalWidth, colours[index&0x0f])
	}
	return &stGroundScene{
		pod:    a.state.GroundMode == game.GroundInPod,
		cityID: cityID, cityNumber: cityNumber, scrollX: scrollX, scrollY: scrollY, rebuilds: rebuilds, colonyPhase: a.state.Colony.Phase,
		shipCity: a.state.ShipCityID, shipPosition: a.state.ShipPosition,
		picture: ebiten.NewImageFromImage(rgba), indices: indices, marks: marks,
	}
}

// paintSTGroundMarkIndices 與 drawSTChar 使用同一字元、錨點及 pen 1。
// 僅加入背景物件，人物／游標 sprite 不成為自身碰撞輸入。
func (a *app) paintSTGroundMarkIndices(indices []byte, marks []stGroundMark) {
	if len(marks) == 0 {
		return
	}
	font := a.stFont()
	whiteIndex := byte(vdiPen[1])
	for _, mark := range marks {
		if font == nil {
			stGroundFill(indices, mark.x-1, mark.y-1, 2, 2, whiteIndex)
			continue
		}
		glyph := font.Glyph(mark.ch)
		for gy := 0; gy < originalmedia.GlyphHeight; gy++ {
			for gx := 0; gx < glyph.Width; gx++ {
				if !glyph.Set(gx, gy) {
					continue
				}
				x, y := mark.x-3+gx, mark.y-4+gy
				if x >= 0 && x < logicalWidth && y >= 0 && y < logicalHeight {
					indices[y*logicalWidth+x] = whiteIndex
				}
			}
		}
	}
}

// newGroundRoller is the remake's stand-in for random(): a value per drawn
// cell, in the same 0..3 range draw_city_map's two dice cover. It is seeded by
// the window and the rebuild count, so a redraw of the same window rolls
// differently — which is the behaviour being reproduced — while one drawn
// frame stays put.
func newGroundRoller(cityID string, scrollX, scrollY, rebuilds int) func() int {
	state := uint32(2166136261)
	mix := func(v int) {
		state ^= uint32(v)
		state *= 16777619
	}
	for i := 0; i < len(cityID); i++ {
		mix(int(cityID[i]))
	}
	mix(scrollX)
	mix(scrollY)
	mix(rebuilds)
	return func() int {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		return int(state & 3)
	}
}

// rollSTGroundPods is park_pods: one die a place, rolled on this redraw. The
// original rolls on every redraw, which is why its vehicles come and go as the
// map scrolls; the remake used to roll from the cell so the street would not
// flicker, and with the window only redrawn on a scroll it does not have to
// any more.
func (a *app) rollSTGroundPods(marks []stGroundMark, px, py int, roll func() int) []stGroundMark {
	system, ok := a.reducer.World.System(a.state.SystemID)
	if !ok {
		return marks
	}
	places := stPodPlaces
	if a.state.SystemID == game.EnlieSystemID {
		places = stPodPlacesEnlie
	}
	for _, offset := range places {
		// random() mod 20 < density; the roller stands in for random().
		if roll()*stPodDieSides/4 >= system.PriceLevel {
			continue
		}
		marks = append(marks, stGroundMark{ch: stPodChar, x: px + stPodOriginX + offset[0], y: py + stPodOriginY + offset[1]})
	}
	return marks
}

// stGroundShipMarks is put_ship: characters 11 and 12 six pixels apart.
func stGroundShipMarks(left, top int) []stGroundMark {
	x := left + (stGroundPitch-stGroundShipWidth)/2 + 3
	y := top + (stGroundPitch-stGroundShipHeight)/2 + 4
	return []stGroundMark{
		{ch: stCharShipGroundNose, x: x, y: y},
		{ch: stCharShipGroundTail, x: x + stGroundShipGap, y: y},
	}
}

func stGroundFill(indices []byte, x, y, w, h int, value byte) {
	for row := 0; row < h; row++ {
		ty := y + row
		if ty < 0 || ty >= logicalHeight {
			continue
		}
		for column := 0; column < w; column++ {
			tx := x + column
			if tx < 0 || tx >= logicalWidth {
				continue
			}
			indices[ty*logicalWidth+tx] = value
		}
	}
}

func stGroundBlit(indices, source []byte, x, y, w, h int) {
	for row := 0; row < h; row++ {
		ty := y + row
		if ty < 0 || ty >= logicalHeight {
			continue
		}
		for column := 0; column < w; column++ {
			tx := x + column
			if tx < 0 || tx >= logicalWidth {
				continue
			}
			indices[ty*logicalWidth+tx] = source[row*w+column] & 0x0f
		}
	}
}

// canStand runs collide's sample pattern over the scene. A point outside the
// screen counts as blocked: the original's screen wraps, but the walker never
// reaches the edge because the margin chain scrolls first.
func (s *stGroundScene) canStand(patterns [originalmedia.CollidePatternCount][]originalmedia.CollidePoint, pattern, x, y int) bool {
	if s == nil {
		return true
	}
	if pattern < 0 || pattern >= len(patterns) {
		return true
	}
	for _, point := range patterns[pattern] {
		sx := x - originalmedia.CollideAnchorOffset + point.X
		sy := y - originalmedia.CollideAnchorOffset + point.Y
		if sx < 0 || sx >= logicalWidth || sy < 0 || sy >= logicalHeight {
			return false
		}
		index := s.indices[sy*logicalWidth+sx] & 0x0f
		if (s.pod && (index == 7 || index == 8)) || (!s.world && !stGroundPassable[index]) || (s.world && s.blocked&(1<<index) != 0) {
			return false
		}
	}
	return true
}

// usesSTGroundWalker: the pixel walker only runs where the ground on screen is
// the original's own pixels. Under the other two presentations the tiles are
// the remake's, so the colours under the walker mean nothing and the cell walk
// stays.
func (a *app) usesSTGroundWalker() bool {
	return a.usesSTGround() && a.originalDisk != nil && a.stGroundBackgroundIndices() != nil
}

// resetSTGroundWalker puts the walker where the state's cell says, with the
// window centred on it the way the remake's cell walk used to keep it. That is
// the entry point every arrival uses: the original's own arrival positions are
// place_person's, which this shell does not model.
func (a *app) resetSTGroundWalker() {
	explicit := a.state.GroundView.Valid
	if !explicit {
		game.ResetGroundView(&a.state, game.Position{X: 20, Y: 20})
	}
	view := a.state.GroundView
	a.groundWalker = stGroundWalker{ready: true, cityID: a.state.CityID, world: a.state.GroundWorld,
		pattern: 2, // get_marks的載具初始圖樣；步行另固定4。
		scrollX: view.Origin.X, scrollY: view.Origin.Y,
		x: view.Anchor.X - originalmedia.CollideAnchorOffset, y: view.Anchor.Y - originalmedia.CollideAnchorOffset}
	if !explicit {
		a.seatSTGroundWalker()
	}
	a.syncSTGroundCell()
}

// seatSTGroundWalker nudges the walker to somewhere it can actually stand.
// The middle of the landing cell is where the SunDog itself is parked, and
// put_ship draws it in colour 15 — a wall — so the walker would arrive stuck.
// The original has `place_person` for this; what it picks is not decoded, so
// this takes the nearest legal spot in a square around the cell's middle,
// which is the remake's.
func (a *app) seatSTGroundWalker() {
	scene := a.stGroundSceneFor(a.state.CityID, a.groundWalker.scrollX, a.groundWalker.scrollY)
	patterns, ok := a.collidePatterns()
	if scene == nil || !ok {
		return
	}
	pattern := a.stGroundModePattern()
	startX, startY := a.groundWalker.x, a.groundWalker.y
	if scene.canStand(patterns, pattern, startX+originalmedia.CollideAnchorOffset, startY+originalmedia.CollideAnchorOffset) {
		return
	}
	// A legal spot is not enough: the corner between the ship and a wall is
	// legal and has nowhere to go from, so the search takes the nearest spot
	// that can still be walked out of in every direction, and only settles for
	// less when there is none.
	best, bestScore, found := [2]int{}, -1, false
	for radius := 1; radius <= stGroundSeatSearch; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if abs(dx) != radius && abs(dy) != radius {
					continue
				}
				x, y := startX+dx, startY+dy
				if !scene.canStand(patterns, pattern, x+originalmedia.CollideAnchorOffset, y+originalmedia.CollideAnchorOffset) {
					continue
				}
				score := 0
				for _, step := range [][2]int{{stGroundWalkerSpeed, 0}, {-stGroundWalkerSpeed, 0}, {0, stGroundWalkerSpeed}, {0, -stGroundWalkerSpeed}} {
					if scene.canStand(patterns, pattern,
						x+step[0]+originalmedia.CollideAnchorOffset, y+step[1]+originalmedia.CollideAnchorOffset) {
						score++
					}
				}
				if score > bestScore {
					best, bestScore, found = [2]int{x, y}, score, true
				}
				if score == 4 {
					a.groundWalker.x, a.groundWalker.y = x, y
					return
				}
			}
		}
		if found && bestScore == 4 {
			break
		}
	}
	if found {
		a.groundWalker.x, a.groundWalker.y = best[0], best[1]
	}
}

// stGroundSeatSearch is how far seatSTGroundWalker looks. Half a cell each
// way covers the ship and a parked vehicle without ever putting the walker in
// a different cell than the state says it is in.
const stGroundSeatSearch = stGroundPitch / 2

// updateSTGroundWalker is xmove_on_ground's loop: while the left button is
// held, move toward the cursor, then let the margin chain scroll.
func (a *app) updateSTGroundWalker() {
	if !a.usesSTGroundWalker() || a.state.Screen != game.ScreenGround || isFailureTerminal(a.state) {
		a.groundWalker.ready = a.groundWalker.ready && a.state.Screen == game.ScreenGround
		return
	}
	if !a.groundWalker.ready || a.groundWalker.cityID != a.state.CityID || a.groundWalker.world != a.state.GroundWorld {
		a.resetSTGroundWalker()
	}
	// Something else moved the player — a keyboard step, a teleport, a tube
	// ride — so put the sprite back under the cell the rules now say it is on.
	if column, row := a.groundWalker.cell(); column != a.state.Position.X || row != a.state.Position.Y {
		a.resetSTGroundWalker()
	}
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		// halt_sprite: nothing moves with the button up. Letting go is also
		// when check_exit runs.
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			a.checkSTGroundExit()
		}
		return
	}
	displayX, displayY := ebiten.CursorPosition()
	// A press on one of the screen's own buttons is that button, not a walk.
	if action := a.mouseActionAtDisplay(displayX, displayY); action != "" && !isGroundMove(action) {
		return
	}
	cursorX, cursorY := a.toLayout(displayX, displayY)
	a.stepSTGroundWalker(cursorX, cursorY)
}

// stepSTGroundWalker is one frame of move_sprite plus the margin chain.
func (a *app) stepSTGroundWalker(targetX, targetY int) {
	scene := a.stGroundSceneFor(a.state.CityID, a.groundWalker.scrollX, a.groundWalker.scrollY)
	patterns, ok := a.collidePatterns()
	if scene == nil || !ok {
		return
	}
	// move_sprite's command flag remains set even when collision blocks it
	// or the target has been reached. Keep clock input apart from RND rolls.
	a.step(game.ActionGroundClockTick)
	if a.state.Screen != game.ScreenGround {
		return
	}
	// 原版 0xb80..0xbd8 以目前碰撞錨點計算目標差，不用圖框中心（Issue #4）。
	centreX, centreY := a.groundWalker.anchor()
	// GEMBIND native 0xbc0..0xbef：剩餘一像素時只走一步，避免越過游標後反覆折返。
	dx := max(-stGroundWalkerSpeed, min(stGroundWalkerSpeed, targetX-centreX))
	dy := max(-stGroundWalkerSpeed, min(stGroundWalkerSpeed, targetY-centreY))
	if dx == 0 && dy == 0 {
		return
	}
	a.groundWalker.direction = stGroundFacing(dx, dy)
	// GEMBIND native 0xc0c..0xc5b：同一起點先試完整步，再垂直、水平；
	// 仍受阻才縮至一像素重試，不能先移 X 再以新位置判 Y。
	for _, stride := range [][2]int{{dx, dy}, {signOf(dx), signOf(dy)}} {
		candidates := [][2]int{stride}
		if stride[1] != 0 {
			candidates = append(candidates, [2]int{0, stride[1]})
		}
		if stride[0] != 0 {
			candidates = append(candidates, [2]int{stride[0], 0})
		}
		moved := false
		for _, candidate := range candidates {
			shape := stGroundSpritePattern(candidate[0], candidate[1])
			collisionPattern := stGroundFootPattern
			if a.state.GroundMode == game.GroundInPod {
				collisionPattern = shape
			}
			x, y := a.groundWalker.x+candidate[0], a.groundWalker.y+candidate[1]
			if scene.canStand(patterns, collisionPattern,
				x+originalmedia.CollideAnchorOffset, y+originalmedia.CollideAnchorOffset) {
				a.groundWalker.x, a.groundWalker.y = x, y
				a.groundWalker.pattern = shape
				moved = true
				break
			}
		}
		if moved {
			break
		}
	}
	a.scrollSTGround()
	a.syncSTGroundCell()
	// check_enc: the original rolls for a street encounter on every pass of
	// the movement loop, not on every cell crossed.
	a.step(game.ActionGroundTick)
}

// scrollSTGround is update_position's margin chain: one direction a frame, x
// before y, and the walker steps back a cell on screen when the window moves.
func (a *app) scrollSTGround() {
	unitX := a.groundWalker.x / stGroundScrollUnit
	unitY := a.groundWalker.y / stGroundScrollUnit
	dx, dy := 0, 0
	switch {
	case unitX < stGroundMarginLow:
		dx = -1
	case unitX > stGroundMarginRight:
		dx = 1
	case unitY < stGroundMarginLow:
		dy = -1
	case unitY > stGroundMarginDown:
		dy = 1
	}
	if dx == 0 && dy == 0 {
		return
	}
	next := game.Position{X: a.groundWalker.scrollX + dx, Y: a.groundWalker.scrollY + dy}
	leftCity, err := game.LeaveGroundCityChecked(&a.state, next)
	if err != nil {
		log.Printf("出城世界圖準備失敗：%v", err)
		return
	}
	if leftCity {
		a.resetSTGroundWalker()
		return
	}
	next = game.GroundWrap(a.state, next)
	a.groundWalker.scrollX, a.groundWalker.scrollY = next.X, next.Y
	pitch := game.GroundPitch(a.state)
	a.groundWalker.x -= dx * pitch
	a.groundWalker.y -= dy * pitch
}

// --- check_exit / check_building on the button release ---
//
// `xmove_on_ground` only dispatches when the button goes up, and only when the
// code under the walker is greater than 9 (or is 5). `check_building` then
// asks a second question the cell walk had no way to ask: the walker's
// position *inside* the cell has to be within ±4 of the door
// (`check_loc`'s `within(a, b, 4)`), so standing on the far corner of a
// depot's cell does not open it. Both are the original's; the door offsets are
// the ones read out of the jump table at XMOVEONG segment 0x2a80.
//
// 碼19的門接入原版depot父選單，見ftl-boost-spec.md。
var stGroundDoors = map[int][]stGroundDoor{
	16: {{13, 21, game.ActionOpenUniteller}},
	18: {{14, 26, game.ActionOpenTubeStation}},
	19: {{15, 16, game.ActionOpenDepot}, {19, 21, game.ActionOpenDepot}},
	20: {{24, 21, game.ActionOpenTrade}, {15, 16, game.ActionOpenTrade}},
}

type stGroundDoor struct {
	x, y   int
	action game.Action
}

// stGroundDoorTolerance is check_loc's within(...) radius, inclusive.
const stGroundDoorTolerance = 4

// stGroundEntryFloor is check_exit's gate: only a code above nine dispatches,
// with code 5 — the pod pad — joining them by its own branch.
const stGroundEntryFloor = 9

func (a *app) checkSTGroundExit() {
	ax, ay := a.groundWalker.anchor()
	pitch := game.GroundPitch(a.state)
	offset := game.Position{X: (ax%pitch + pitch) % pitch, Y: (ay%pitch + pitch) % pitch}
	if game.BoardGroundShip(&a.state, offset, a.stGroundModePattern() == 2) {
		a.groundWalker.ready = false
		return
	}
	if a.state.GroundWorld {
		if game.EnterGroundCity(&a.state, offset) {
			a.resetSTGroundWalker()
		}
		return
	}

	code, _, ok := game.GroundTerrainCode(a.state, a.state.CityID, a.state.Position.X, a.state.Position.Y)
	if !ok {
		return
	}
	// check_exit reads the drawn cell, including draw_city_map's building
	// overlay. Slots 34..39 are normalized by subtracting 16 before dispatch.
	// READY evidence: docs/atari-st/ship-rules.md, 建築入口補證。
	for _, building := range game.GroundCityBuildings(a.state, a.state.CityID) {
		if building.Row == a.state.Position.X && building.Column == a.state.Position.Y {
			code = building.Code - 16
		}
	}
	if code <= stGroundEntryFloor && code != stGroundPodCode {
		return
	}
	offsetX, offsetY := offset.X, offset.Y
	for _, door := range stGroundDoors[code] {
		if code == 19 && (door.x == 15) != (a.state.GroundMode == game.GroundOnFoot) {
			continue
		}
		if abs(offsetX-door.x) <= stGroundDoorTolerance && abs(offsetY-door.y) <= stGroundDoorTolerance {
			a.step(door.action)
			return
		}
	}
	// The rooms — tavern, inn, parts, equipment — take the cell rather than a
	// door position: their own offsets come from a constant table this shell
	// has not read out yet, so the whole cell opens them.
	if _, isRoom := game.InteriorKindForTerrain(code); isRoom {
		a.step(game.ActionEnterBuilding)
	}
}

// syncSTGroundCell puts the cell the walker is standing on into the game
// state. The rules downstream — buildings, facilities, street encounters —
// all speak in cells, so this is where the pixel walk rejoins them.
func (a *app) syncSTGroundCell() {
	column, row := a.groundWalker.cell()
	a.state.Position = game.Position{X: column, Y: row}
	game.SyncCargoGroundPosition(&a.state)
	x, y := a.groundWalker.anchor()
	a.state.GroundView = game.GroundView{Valid: true,
		Origin: game.Position{X: a.groundWalker.scrollX, Y: a.groundWalker.scrollY}, Anchor: game.Position{X: x, Y: y}}
	a.state.Heading = game.Heading(stGroundHeadings[a.groundWalker.direction])
}

// stGroundHeadings maps the collide pattern order onto the compass the
// character card shows.
var stGroundHeadings = [4]game.Heading{game.HeadingNorth, game.HeadingSouth, game.HeadingWest, game.HeadingEast}

// stGroundFacing 是羅盤朝向；不可當成 sprite 或碰撞圖樣索引。
func stGroundFacing(dx, dy int) int {
	switch {
	case dx < 0:
		return 2
	case dx > 0:
		return 3
	case dy < 0:
		return 0
	default:
		return 1
	}
}

// GEMBIND native 0xd2c..0xd53；Talos 八方向驗證，見 walker-timing-spec.md。
func stGroundSpritePattern(dx, dy int) int {
	if dx == 0 {
		return 0
	}
	if dy == 0 {
		return 2
	}
	if (dx < 0) != (dy < 0) {
		return 1
	}
	return 3
}

func signOf(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// 鍵盤保留一格的便利步幅，但每一步仍經原版顏色碰撞，不能越過水岸或牆。
func (a *app) nudgeSTGroundWalker(action game.Action) {
	startWorld, startPosition := a.state.GroundWorld, a.state.Position
	if !a.groundWalker.ready || a.groundWalker.cityID != a.state.CityID || a.groundWalker.world != a.state.GroundWorld {
		a.resetSTGroundWalker()
	}
	dx, dy := 0, 0
	switch action {
	case game.ActionMoveNorth:
		dy = -1
	case game.ActionMoveSouth:
		dy = 1
	case game.ActionMoveWest:
		dx = -1
	case game.ActionMoveEast:
		dx = 1
	}
	steps := game.GroundPitch(a.state) / stGroundWalkerSpeed
	progressed := false
	for i := 0; i < steps && a.state.Screen == game.ScreenGround; i++ {
		city, surface := a.state.CityID, a.state.GroundWorld
		x, y := a.groundWalker.x, a.groundWalker.y
		a.stepSTGroundWalker(x+originalmedia.CollideAnchorOffset+dx*100, y+originalmedia.CollideAnchorOffset+dy*100)
		if city != a.state.CityID || surface != a.state.GroundWorld {
			// 換圖本身結束本次鍵盤步幅；不可用同一次輸入立即又進回原城。
			return
		}
		if x == a.groundWalker.x && y == a.groundWalker.y {
			break
		}
		progressed = true
	}
	if startWorld && a.state.GroundWorld && a.state.Position == startPosition {
		// 鍵盤進城需跨入城市格；出城時仍站在同格，不能因貼住邊緣又進城。
		return
	}
	if progressed && a.state.Screen == game.ScreenGround {
		a.checkSTGroundExit()
	}
}

// collidePatterns reads the ten sample patterns once.
func (a *app) collidePatterns() ([originalmedia.CollidePatternCount][]originalmedia.CollidePoint, bool) {
	if !a.collideRead {
		a.collideRead = true
		if a.originalDisk != nil {
			patterns, err := a.originalDisk.CollidePatterns()
			if err != nil {
				log.Printf("碰撞取樣圖樣讀不出來：%v", err)
			} else {
				a.collidePatternData = patterns
				a.collideOK = true
			}
		}
	}
	return a.collidePatternData, a.collideOK
}

// drawSTGroundMarks paints the parked vehicles and the ship over the window.
func (a *app) drawSTGroundMarks(screen *ebiten.Image, scene *stGroundScene) {
	if scene == nil || len(scene.marks) == 0 {
		return
	}
	white := stPen(1)
	for _, mark := range scene.marks {
		a.drawSTChar(screen, mark.ch, mark.x, mark.y, white)
	}
}

// get_marks：步行圖樣4、載具初始2並依成功位移維持0–3方向家族。
// init_movement的外層local12=1是移動模式，不能當成圖樣。
func (a *app) stGroundModePattern() int {
	if a.state.GroundMode == game.GroundInPod {
		if !a.groundWalker.ready {
			return 2
		}
		return a.groundWalker.pattern
	}
	return stGroundFootPattern
}

func (a *app) drawSTGroundWalker(screen *ebiten.Image) bool {
	if !a.ensureGroundSprites() {
		return false
	}
	pattern := a.stGroundModePattern()
	a.drawSTSpriteAt(screen, pattern, a.groundWalker.x, a.groundWalker.y, 1,
		color.RGBA{R: 255, G: 255, B: 255, A: 255})
	return true
}
