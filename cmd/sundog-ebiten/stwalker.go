package main

import (
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// --- 船艙走路人：小人跟著游標走，走進站位矩形才到達 ---
//
// 原版的船艙裡有一個小人 sprite 一路走向十字游標；走進視窗表 65–77 的矩形時
// `XMOVEONS:0x0a get_location` 認出站位，`0x0c check_enc` 分派（stStations 就是
// 那張表）。畫面上沒有任何常駐文字——站名是視窗開起來之後才出現在標題列。
//
// 重製版先前沒有這個人：點矩形即到達，並在艙室圖上永遠壓三個站名與一個訊息窗。
// 這裡把小人接回來，站名改成只在小人站進該站時顯示。
//
// 互動模型照 `XMOVEONS:0x01 xmove_on_ship` 的主迴圈做：
//
//	if button(0,0) then get_mouse(&x,&y); move_sprite(0,x,y) else halt_sprite(0)
//	if button(0,0) <> 前一幀 then if 放開了 then check_enc
//
// 兩件事因此成立：**小人只在左鍵按住時走**，放開就停；**站位是在放開按鍵那一刻
// 才判**，走過去不算，所以可以從一個站位上面走過而不進去。`check_enc` 另外要求
// `get_location()` 與上一個站位不同（`L1 <> L4`）才分派，這就是從駕駛台回到船艙
// 不會立刻又進駕駛台的原因；`init_ship` 回來時用 `MAINLIB:0x3d center(視窗,&x,&y)`
// 把小人放在該站位視窗的中心，`L4` 也一併設成那個站位。
//
// **通行判定照做了，而且是 Hatari 實跑驗證過的**（2026-09-03，見
// `docs/atari-st/shipboard-walker.md`）。`ship_disp` 交給 `set_coll_color` 的字串是
// `1100000000000001`（色 0、1、15 不可站），碰撞用的是 `GEMBIND:0x28 collide` 那十組
// 取樣圖樣之一，取樣點在 `(x-4+dx, y-4+dy)`。關鍵是**錨點就是 sprite 方框的左上角
// 加 4**——取樣框與 sprite 框剛好重合。先前靜態分析漏掉這 4 個像素，才會把小人實際
// 站著的位置判成非法。
//
// 實跑對這個模型的三項驗證：
//
//   1. 模擬器那一幀與磁碟解出的影像 8 的「顏色→索引」對應一致度 99.83%
//      （不一致的 110 px 就是小人、游標與艙門燈），影像 9 是 21% 不一致——
//      所以底圖確實是影像 8，座標映射也對。
//   2. 把游標放在貨艙視窗上按住，小人走了 18 px 之後**停在模型算出來的那道牆前**：
//      停點合法，往游標方向的左、左上、左下三步全部不合法。
//   3. 再按住 18 秒，游標不動——小人**一個像素都沒有再動**。
//
// 第三項是「原版不繞路」的直接證據：撞到牆就停死，即使上方與右方都還走得動，
// 要繞開得由玩家自己挪游標。重製版的小人**會繞**（下面的 BFS 距離場），這是
// 刻意加的，列在 `WORKLIST.md` B 區。
//
// 視窗 66（貨艙）與 71 在這個模型下矩形內**一個合法錨點都沒有**，所以原版也走不
// 進去——貨艙是靠 `init_ship` 的 `center(0x42)` 放進去的，不是走進去的。重製版兩
// 者都另有鍵盤入口（船上 `C` 開貨艙、`E` 開工程），不會因此走不到。

const (
	// stWalkerSpeed 是每幀每軸走幾個版面像素。原版 `do_sprite` 的速度沒有量過，
	// 這個數字是重製版自訂的。
	stWalkerSpeed = 2
	// 艙內的小人是 7×7、1 倍——實跑量到的團塊就是 7×7。（地面那個畫兩倍是重製版
	// 為了 40 px 的格子做的選擇，兩邊不同。）
	stWalkerCells = 7
	stWalkerSize  = stWalkerCells
	// stWalkerAnchor 是 sprite 左上角到碰撞錨點的偏移，等於 collide 自己減掉的 4。
	stWalkerAnchor = originalmedia.CollideAnchorOffset
	// stWalkerStartWindow 是 init_ship 沒有別的線索時放小人的地方：
	// `G32.(4,4)` 為 0 那條分支是 `center(0x41, ...)`，也就是駕駛台的椅子。
	stWalkerStartWindow = 65
)

// stCabinPassable 是 XMOVEONS ship_disp 的 set_coll_color 字串
// "1100000000000001" 讀成「這個色號站得住嗎」：旗標 1 代表不可站。
var stCabinPassable = func() [16]bool {
	const flags = "1100000000000001"
	var out [16]bool
	for i := 0; i < 16; i++ {
		out[i] = flags[i] == '0'
	}
	return out
}()

// stShipWalker 是船艙小人的呈現狀態。它不進存檔：原版把位置留在 RAM 裡，
// 重製版的存檔 schema 也沒有對應欄位。
type stShipWalker struct {
	ready bool
	// x, y 是 sprite 方框的左上角，版面座標。
	x, y int
	// occupied 是小人目前站著的站位視窗編號，0 代表不在任何站位。走進去才分派一次，
	// 要先走出來才會再分派——這也是從站位畫面回到船艙時不會立刻又進去的原因。
	occupied int
	// pointerX/pointerY 是進船艙那一刻的游標位置。游標動過之前小人不跟，免得剛從
	// 氣閘回到船艙、游標還停在氣閘上，就又被拉出去一次。
	pointerX, pointerY int
	pointerMoved       bool
	// onShipboard 是上一幀在不在船艙，用來認出「剛進來」這一刻。
	onShipboard bool
}

// box 是小人目前的方框。
func (w stShipWalker) box() (x0, y0, x1, y1 int) {
	return w.x, w.y, w.x + stWalkerSize - 1, w.y + stWalkerSize - 1
}

// anchor 是碰撞與站位判定共用的那一點：sprite 左上角加 4。`get_location` 比的是
// **角色 sprite 的位置**對視窗矩形（hatari-live-run.md 第九輪），是單點不是方框——
// 這件事有實際差別：視窗 72（129–145, 29–37）與 73（146–154, 34–46）相鄰，用方框
// 判定會同時壓到兩個，先列出來的那個永遠贏，73 就再也選不到。
func (w stShipWalker) anchor() (int, int) {
	return w.x + stWalkerAnchor, w.y + stWalkerAnchor
}

// stationUnder 是小人現在站著的站位。
func (w stShipWalker) stationUnder() (stStation, bool) {
	return stStationAt(w.anchor())
}

// resetSTShipWalker 把小人放到駕駛台的椅子上，就是 init_ship 的
// `center(0x41, &x, &y)`；`occupied` 一起設成那個站位（原版的 `L4`），所以第一次
// 放開按鍵不會馬上又進駕駛台。
func (a *app) resetSTShipWalker() {
	a.shipWalker = stShipWalker{ready: true}
	a.placeSTShipWalkerAt(stWalkerStartWindow)
}

// placeSTShipWalkerAt 把小人放在某個站位視窗的中心，並記成目前佔用的站位——
// init_ship 用 MAINLIB:0x3d center() 做的就是這件事。
func (a *app) placeSTShipWalkerAt(window int) {
	for _, station := range stStations {
		if station.window != window {
			continue
		}
		a.shipWalker.x = (station.x0+station.x1)/2 - stWalkerAnchor
		a.shipWalker.y = (station.y0+station.y1)/2 - stWalkerAnchor
		a.shipWalker.occupied = station.window
		return
	}
}

// stepSTShipWalker 往目標挪一格。有牆：先問距離場該往哪走（繞得過去就繞），
// 沒有路才退回原版的走法——直接朝目標踏一步，踏不動就停在那裡。
func (a *app) stepSTShipWalker(targetX, targetY int) {
	if step, ok := a.cabinRouteStep(targetX, targetY); ok {
		a.shipWalker.x, a.shipWalker.y = step[0], step[1]
		return
	}
	nx := clampWalker(a.shipWalker.x+stepToward(a.shipWalker.x, targetX), logicalWidth)
	ny := clampWalker(a.shipWalker.y+stepToward(a.shipWalker.y, targetY), logicalHeight)
	if a.cabinLegalAt(nx+stWalkerAnchor, ny+stWalkerAnchor) {
		a.shipWalker.x, a.shipWalker.y = nx, ny
	}
	// 走不動就停在原地，這正是原版的行為（實跑：按住 18 秒沒動一個像素）。
}

// --- 艙內的通行與繞路 ---

// cabinPatternPoints 讀一次 collide 的走路人取樣圖樣。
func (a *app) cabinPatternPoints() []originalmedia.CollidePoint {
	if a.cabinPatternRead {
		return a.cabinPattern
	}
	a.cabinPatternRead = true
	if a.originalDisk == nil {
		return nil
	}
	patterns, err := a.originalDisk.CollidePatterns()
	if err != nil {
		log.Printf("原版 collide 取樣圖樣讀不出來，艙內小人改成不判牆：%v", err)
		return nil
	}
	a.cabinPattern = patterns[originalmedia.CollideWalkerPattern]
	return a.cabinPattern
}

// cabinLegalAt 是「錨點放這裡，collide 會不會說不行」。沒有底圖或沒有取樣圖樣時
// 一律當成可以站，這樣缺磁碟的情況只是退回不判牆，不會把小人鎖死。
func (a *app) cabinLegalAt(x, y int) bool {
	legal := a.cabinLegalMask()
	if legal == nil {
		return x >= 0 && y >= 0 && x < logicalWidth && y < logicalHeight
	}
	if x < 0 || y < 0 || x >= a.cabinWidth || y >= a.cabinHeight {
		return false
	}
	return legal[y*a.cabinWidth+x]
}

// cabinLegalMask 把「每個錨點合不合法」算一次存起來：320×200 個位置各取樣 22 點。
func (a *app) cabinLegalMask() []bool {
	if a.cabinLegal != nil {
		return a.cabinLegal
	}
	points := a.cabinPatternPoints()
	if points == nil || a.cabinFloor == nil {
		return nil
	}
	w, h := a.cabinWidth, a.cabinHeight
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ok := true
			for _, p := range points {
				sx, sy := x-stWalkerAnchor+p.X, y-stWalkerAnchor+p.Y
				if sx < 0 || sy < 0 || sx >= w || sy >= h || !a.cabinFloor[sy*w+sx] {
					ok = false
					break
				}
			}
			mask[y*w+x] = ok
		}
	}
	a.cabinLegal = mask
	return mask
}

// stCabinSteps 是繞路時試的八個方向。
var stCabinSteps = [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// cabinRouteStep 回傳往目標的下一個位置（sprite 左上角），以及有沒有路。
//
// 做法是對目標算一次寬度優先的距離場，之後每一幀只要挑相鄰格裡距離最小的那個就
// 好；游標沒動就不用重算。目標本身不可站（例如游標指在牆上、或指在視窗 66 那種
// 沒有合法錨點的矩形裡）就沒有路，交回給原版的走法去撞牆。
func (a *app) cabinRouteStep(targetX, targetY int) ([2]int, bool) {
	legal := a.cabinLegalMask()
	if legal == nil {
		return [2]int{}, false
	}
	tx, ty := targetX+stWalkerAnchor, targetY+stWalkerAnchor
	if !a.cabinLegalAt(tx, ty) {
		// 游標指在站不住的地方（牆上，或視窗 66、71 那種整格沒有合法錨點的矩形）。
		// 原版在這裡是直接撞牆停死；重製版改成走到離目標最近的可站點，等於替玩家
		// 做了他本來要自己挪游標做的事。找不到就交回原版的走法。
		nx, ny, ok := a.cabinNearestReachable(tx, ty)
		if !ok {
			return [2]int{}, false
		}
		tx, ty = nx, ny
	}
	a.cabinBuildField(tx, ty)
	w, h := a.cabinWidth, a.cabinHeight
	ax, ay := a.shipWalker.anchor()
	if ax < 0 || ay < 0 || ax >= w || ay >= h {
		return [2]int{}, false
	}
	here := a.cabinField[ay*w+ax]
	if here < 0 {
		return [2]int{}, false
	}
	best, bestX, bestY := here, -1, -1
	for _, step := range stCabinSteps {
		nx, ny := ax+step[0], ay+step[1]
		if nx < 0 || ny < 0 || nx >= w || ny >= h || !legal[ny*w+nx] {
			continue
		}
		if d := a.cabinField[ny*w+nx]; d >= 0 && d < best {
			best, bestX, bestY = d, nx, ny
		}
	}
	if bestX < 0 {
		return [2]int{}, false
	}
	return [2]int{bestX - stWalkerAnchor, bestY - stWalkerAnchor}, true
}

// stCabinNearestRadius 是找最近可站點時往外搜的上限，夠涵蓋艙室裡任何一道牆的厚度。
const stCabinNearestRadius = 48

// cabinNearestReachable 是離 (x, y) 最近、而且**與小人在同一個連通區**的合法錨點。
// 連通區這個條件不能省：視窗 66 附近有幾塊站得住但走不到的小口袋，挑到那裡會算不出
// 路，小人就整個不動了。由內往外一圈一圈找，所以先找到的就是最近的。
func (a *app) cabinNearestReachable(x, y int) (int, int, bool) {
	legal := a.cabinLegalMask()
	component := a.cabinComponents()
	if legal == nil || component == nil {
		return 0, 0, false
	}
	w, h := a.cabinWidth, a.cabinHeight
	ax, ay := a.shipWalker.anchor()
	if ax < 0 || ay < 0 || ax >= w || ay >= h {
		return 0, 0, false
	}
	want := component[ay*w+ax]
	if want < 0 {
		return 0, 0, false
	}
	for radius := 1; radius <= stCabinNearestRadius; radius++ {
		bestX, bestY, bestDistance := 0, 0, 1<<30
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if max(abs(dx), abs(dy)) != radius {
					continue
				}
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx >= w || ny >= h || !legal[ny*w+nx] {
					continue
				}
				if component[ny*w+nx] != want {
					continue
				}
				if d := dx*dx + dy*dy; d < bestDistance {
					bestX, bestY, bestDistance = nx, ny, d
				}
			}
		}
		if bestDistance < 1<<30 {
			return bestX, bestY, true
		}
	}
	return 0, 0, false
}

// cabinComponents 給每個合法錨點一個連通區編號，不合法的是 -1。遮罩是靜態的，所以
// 只算一次。
func (a *app) cabinComponents() []int32 {
	if a.cabinComponent != nil {
		return a.cabinComponent
	}
	legal := a.cabinLegalMask()
	if legal == nil {
		return nil
	}
	w, h := a.cabinWidth, a.cabinHeight
	component := make([]int32, w*h)
	for i := range component {
		component[i] = -1
	}
	var next int32
	queue := make([]int32, 0, w*h)
	for start := 0; start < w*h; start++ {
		if !legal[start] || component[start] >= 0 {
			continue
		}
		component[start] = next
		queue = append(queue[:0], int32(start))
		for head := 0; head < len(queue); head++ {
			index := int(queue[head])
			x, y := index%w, index/w
			for _, step := range stCabinSteps {
				nx, ny := x+step[0], y+step[1]
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					continue
				}
				ni := ny*w + nx
				if !legal[ni] || component[ni] >= 0 {
					continue
				}
				component[ni] = next
				queue = append(queue, int32(ni))
			}
		}
		next++
	}
	a.cabinComponent = component
	return component
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// cabinBuildField 對目標算一次距離場，目標沒換就沿用上一次的。
func (a *app) cabinBuildField(tx, ty int) {
	if a.cabinFieldHasTarget && a.cabinFieldTargetX == tx && a.cabinFieldTargetY == ty {
		return
	}
	legal := a.cabinLegalMask()
	w, h := a.cabinWidth, a.cabinHeight
	field := make([]int32, w*h)
	for i := range field {
		field[i] = -1
	}
	field[ty*w+tx] = 0
	queue := make([]int32, 0, w*h)
	queue = append(queue, int32(ty*w+tx))
	for head := 0; head < len(queue); head++ {
		index := int(queue[head])
		x, y := index%w, index/w
		next := field[index] + 1
		for _, step := range stCabinSteps {
			nx, ny := x+step[0], y+step[1]
			if nx < 0 || ny < 0 || nx >= w || ny >= h {
				continue
			}
			ni := ny*w + nx
			if !legal[ni] || field[ni] >= 0 {
				continue
			}
			field[ni] = next
			queue = append(queue, int32(ni))
		}
	}
	a.cabinField = field
	a.cabinFieldTargetX, a.cabinFieldTargetY, a.cabinFieldHasTarget = tx, ty, true
}

// clampWalker 把 sprite 方框的一個角夾在 0..limit-stWalkerSize。
func clampWalker(value, limit int) int {
	if value < 0 {
		return 0
	}
	if high := limit - stWalkerSize; value > high {
		return high
	}
	return value
}

// stepToward 是往目標的一個像素，已經到了就 0。
func stepToward(from, to int) int {
	switch {
	case from < to:
		return 1
	case from > to:
		return -1
	default:
		return 0
	}
}

// updateSTShipWalker 每幀把小人往游標挪 stWalkerSpeed 步，走進站位就分派該站的
// 動作一次。不在 Atari ST 滿版船艙時什麼都不做，也不重設——這樣從駕駛台或工程艙
// 回來，小人還站在原來那個站位上，不會又被分派一次。
func (a *app) updateSTShipWalker() {
	if !a.usesSTShipboard() || isFailureTerminal(a.state) {
		a.shipWalker.onShipboard = false
		return
	}
	if !a.shipWalker.ready {
		a.resetSTShipWalker()
	}
	windowX, windowY := ebiten.CursorPosition()
	if !a.shipWalker.onShipboard {
		// 剛回到船艙：把游標基準重設，玩家真的動了滑鼠小人才開始跟。
		a.shipWalker.onShipboard = true
		a.shipWalker.pointerX, a.shipWalker.pointerY = windowX, windowY
		a.shipWalker.pointerMoved = false
	}
	if windowX != a.shipWalker.pointerX || windowY != a.shipWalker.pointerY {
		a.shipWalker.pointerMoved = true
	}
	// move_sprite 只在按著左鍵時被呼叫；放開就是 halt_sprite。
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) &&
		a.shipWalker.pointerMoved && a.pointerInView(windowX, windowY) {
		layoutX, layoutY := a.toLayout(windowX, windowY)
		// 游標指的是小人的中心，不是左上角。
		targetX, targetY := layoutX-stWalkerSize/2, layoutY-stWalkerSize/2
		for step := 0; step < stWalkerSpeed; step++ {
			a.stepSTShipWalker(targetX, targetY)
		}
	}
	// check_enc 只在放開的那一刻跑：走過站位不算，停在裡面放手才算。
	if !inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		return
	}
	station, inside := a.shipWalker.stationUnder()
	if !inside || station.window == a.shipWalker.occupied {
		return
	}
	// init_ship 回來時會把小人放在該站位的中心，所以進去之前先站好。
	a.placeSTShipWalkerAt(station.window)
	a.step(station.action)
}

// pointerInView 回報視窗像素座標是不是落在那塊 320×200 的畫面上。
func (a *app) pointerInView(x, y int) bool {
	scale := a.viewScale()
	if scale <= 0 {
		return false
	}
	return x >= a.originX && x < a.originX+logicalWidth*scale &&
		y >= a.originY && y < a.originY+logicalHeight*scale
}

// drawSTShipWalker 把小人畫在艙室圖上，用的是地面那支 sprite 畫法。
func (a *app) drawSTShipWalker(screen *ebiten.Image) {
	if !a.shipWalker.ready {
		return
	}
	if !a.ensureGroundSprites() {
		// 沒有磁碟就沒有原版 sprite 可畫；船艙滿版本來也要磁碟才成立。
		return
	}
	a.drawSTSpriteAt(screen, stPersonPatterns, a.shipWalker.x, a.shipWalker.y, 1,
		color.RGBA{R: 255, G: 255, B: 255, A: 255})
}

// --- 地面：按住不放連續走 ---
//
// 原版的城市地面是即時走路，按住左鍵小人就一路走向游標。重製版是格子制，沒有
// 那個即時迴圈，所以改成「按住就每隔一段時間再走一格」：第一步由 just-pressed
// 那條路徑走掉，這裡接後續。兩個間隔是重製版自訂的——原版的走路速度沒有量過。
const (
	stGroundRepeatDelay  = 18
	stGroundRepeatFrames = 10
)

// stGroundMoveKeys 是地面上會走路的按鍵。只有這些鍵按住會重複；其他鍵（交易、
// 銀行、加油）重複觸發都是錯的。
var stGroundMoveKeys = []ebiten.Key{
	ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
	ebiten.KeyW, ebiten.KeyA, ebiten.KeyS,
}

// isGroundMove 認出四個移動動作。
func isGroundMove(action game.Action) bool {
	switch action {
	case game.ActionMoveNorth, game.ActionMoveSouth, game.ActionMoveWest, game.ActionMoveEast:
		return true
	}
	return false
}

// stRepeatDue 回報按住 held 幀之後這一幀該不該再走一格。
func stRepeatDue(held int) bool {
	return held > stGroundRepeatDelay && (held-stGroundRepeatDelay)%stGroundRepeatFrames == 0
}

// updateGroundWalkRepeat 讓地面上按住左鍵或方向鍵可以連續走。
func (a *app) updateGroundWalkRepeat() {
	if a.state.Screen != game.ScreenGround || isFailureTerminal(a.state) {
		return
	}
	// Under the Atari ST presentation the walk is per pixel (stground.go), so
	// the cell step on a held button would fight it; the keyboard nudge stays
	// either way, since the original has no equivalent of it at all.
	if !a.usesSTGroundWalker() && stRepeatDue(inpututil.MouseButtonPressDuration(ebiten.MouseButtonLeft)) {
		x, y := ebiten.CursorPosition()
		if action := a.mouseActionAtDisplay(x, y); isGroundMove(action) {
			a.step(action)
		}
	}
	for _, key := range stGroundMoveKeys {
		if !stRepeatDue(inpututil.KeyPressDuration(key)) {
			continue
		}
		if action, ok := keyAction(key, a.state); ok && isGroundMove(action) {
			a.step(action)
		}
	}
}

// --- 地面的繞路 ---
//
// 原版的城市地面是逐像素走的：小人朝游標走，撞到不可站的顏色就停在那裡，玩家自己
// 把游標挪開繞過去。重製版是格子制、一次一格，同一條規則會變成「按住不放卡在牆上
// 完全不動」——貪心地往目標的方向走，第一格是牆就永遠走不了。
//
// 所以這裡改成先在可通行的格子上找一條最短路，走那條路的第一步。**原版沒有尋路**，
// 這一段是重製版自己的（列在 WORKLIST B）；理由是格子制把原版那種「貼著牆蹭過去」
// 的手感變成了卡死，而不是為了讓走路更聰明。
//
// 找不到路（目標是牆、或被完全隔開）才退回原本的貪心方向，這樣「往牆的方向按」
// 仍然會撞牆並吃到 cannot_move 的訊息，與原版一致。
func (a *app) groundStepToward(target game.Position) game.Action {
	if step, ok := a.groundPathFirstStep(a.state.Position, target); ok {
		return step
	}
	return directionTo(a.state.Position, target)
}

// stGroundNeighbours 是展開的順序。四個方向都會被走到，順序只決定同長度的路挑哪
// 一條；這裡跟 directionTo 一樣先左右再上下，所以斜著走的路看起來會先橫向移動。
var stGroundNeighbours = []struct {
	dx, dy int
	action game.Action
}{
	{-1, 0, game.ActionMoveWest},
	{1, 0, game.ActionMoveEast},
	{0, -1, game.ActionMoveNorth},
	{0, 1, game.ActionMoveSouth},
}

// groundPathFirstStep 是從 from 到 target 的最短路的第一步，以及有沒有路。
// 城市地圖只有 40×24 格，每次點擊或每次重複走一格各跑一次寬度優先就夠了。
func (a *app) groundPathFirstStep(from, target game.Position) (game.Action, bool) {
	if from == target {
		return "", false
	}
	width, height := world.MapWidth(), world.MapHeight()
	if target.X < 0 || target.X >= width || target.Y < 0 || target.Y >= height {
		return "", false
	}
	if !a.reducer.World.CanEnter(a.state.CityID, world.TilePosition{X: target.X, Y: target.Y}) {
		return "", false
	}
	// first[y*width+x] 是走到那一格的第一步；空字串代表還沒走到。
	first := make([]game.Action, width*height)
	queue := []game.Position{from}
	for len(queue) > 0 {
		here := queue[0]
		queue = queue[1:]
		for _, step := range stGroundNeighbours {
			next := game.Position{X: here.X + step.dx, Y: here.Y + step.dy}
			if next.X < 0 || next.X >= width || next.Y < 0 || next.Y >= height {
				continue
			}
			index := next.Y*width + next.X
			if first[index] != "" || next == from {
				continue
			}
			if !a.reducer.World.CanEnter(a.state.CityID, world.TilePosition{X: next.X, Y: next.Y}) {
				continue
			}
			if here == from {
				first[index] = step.action
			} else {
				first[index] = first[here.Y*width+here.X]
			}
			if next == target {
				return first[index], true
			}
			queue = append(queue, next)
		}
	}
	return "", false
}
