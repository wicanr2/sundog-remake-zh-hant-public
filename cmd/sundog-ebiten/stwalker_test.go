package main

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// TestShipWalkerStartsInThePilotChair: init_ship 的 `G32.(4,4) == 0` 那條分支是
// `center(0x41, &x, &y)`，也就是把小人放在駕駛台視窗的中心，並把 L4 設成那個站位。
func TestShipWalkerStartsInThePilotChair(t *testing.T) {
	a := newOptionsTestApp(t)
	a.resetSTShipWalker()
	station, ok := a.shipWalker.stationUnder()
	if !ok || station.window != stWalkerStartWindow {
		t.Fatalf("起點在 %v/%d，應該是視窗 %d", ok, station.window, stWalkerStartWindow)
	}
	if a.shipWalker.occupied != stWalkerStartWindow {
		t.Fatalf("起點的 occupied 是 %d", a.shipWalker.occupied)
	}
	x0, y0, x1, y1 := a.shipWalker.box()
	if x0 < 0 || y0 < 0 || x1 >= logicalWidth || y1 >= logicalHeight {
		t.Fatalf("起點方框 (%d,%d)-(%d,%d) 不在畫面內", x0, y0, x1, y1)
	}
}

// TestWindow67IsNotAStation: get_location 的迴圈跑 0x41..0x4d，但明寫 `L1 <> 0x43`
// 跳過 67，所以那一格不是站位。
func TestWindow67IsNotAStation(t *testing.T) {
	for _, station := range stStations {
		if station.window == 67 {
			t.Fatal("視窗 67 還在站位表裡；get_location 跳過它")
		}
	}
	if len(stStations) != 12 {
		t.Fatalf("站位有 %d 個，get_location 認得的是 12 個", len(stStations))
	}
}

// TestShipWalkerStepsTowardTheCursor: 每一步兩軸各走一個像素，到了就停。
func TestShipWalkerStepsTowardTheCursor(t *testing.T) {
	a := newOptionsTestApp(t)
	a.shipWalker = stShipWalker{ready: true, x: 100, y: 100}
	a.stepSTShipWalker(120, 90)
	if a.shipWalker.x != 101 || a.shipWalker.y != 99 {
		t.Fatalf("往右上走一步到 (%d,%d)，應該是 (101,99)", a.shipWalker.x, a.shipWalker.y)
	}
	a.shipWalker.x, a.shipWalker.y = 50, 50
	a.stepSTShipWalker(50, 50)
	if a.shipWalker.x != 50 || a.shipWalker.y != 50 {
		t.Fatalf("已經在目標上還移動到 (%d,%d)", a.shipWalker.x, a.shipWalker.y)
	}
}

// TestShipWalkerReachesEveryStationByWalking: 十二個站位都要從駕駛台走得到。
// 這是不照抄原版通行色表的理由（見 stwalker.go 開頭）。
func TestShipWalkerReachesEveryStationByWalking(t *testing.T) {
	a := newOptionsTestApp(t)
	for _, station := range stStations {
		a.resetSTShipWalker()
		targetX := (station.x0+station.x1)/2 - stWalkerAnchor
		targetY := (station.y0+station.y1)/2 - stWalkerAnchor
		arrived := false
		for step := 0; step < 4*logicalWidth; step++ {
			a.stepSTShipWalker(targetX, targetY)
			if got, ok := a.shipWalker.stationUnder(); ok && got.window == station.window {
				arrived = true
				break
			}
		}
		if !arrived {
			t.Fatalf("走不到視窗 %d 的站位", station.window)
		}
	}
}

// TestEveryStationCentreHitsItself: 每個矩形的中心點各自命中自己。方框判定在相鄰的
// 72／73 上會互相吃掉，而 get_location 比的本來就是 sprite 的位置這一個點。
func TestEveryStationCentreHitsItself(t *testing.T) {
	for _, station := range stStations {
		x, y := (station.x0+station.x1)/2, (station.y0+station.y1)/2
		got, ok := stStationAt(x, y)
		if !ok || got.window != station.window {
			t.Fatalf("視窗 %d 的中心 (%d,%d) 命中 %v/%d", station.window, x, y, ok, got.window)
		}
	}
}

// TestPlacingAtAStationMarksItOccupied: check_enc 要 `L1 <> L4` 才分派，而 init_ship
// 把小人放到站位中心時一併設 L4。兩件事合起來就是「從駕駛台回到船艙不會又進駕駛台」。
func TestPlacingAtAStationMarksItOccupied(t *testing.T) {
	a := newOptionsTestApp(t)
	a.resetSTShipWalker()
	for _, station := range stStations {
		a.placeSTShipWalkerAt(station.window)
		got, ok := a.shipWalker.stationUnder()
		if !ok || got.window != station.window {
			t.Fatalf("放到視窗 %d 之後站在 %v/%d", station.window, ok, got.window)
		}
		if a.shipWalker.occupied != station.window {
			t.Fatalf("放到視窗 %d 之後 occupied 是 %d", station.window, a.shipWalker.occupied)
		}
	}
}

// TestShipWalkerStaysOnScreen: 游標貼著邊緣時小人不會整個走出畫面。
func TestShipWalkerStaysOnScreen(t *testing.T) {
	a := newOptionsTestApp(t)
	for _, target := range [][2]int{{-100, -100}, {logicalWidth + 100, logicalHeight + 100}} {
		a.resetSTShipWalker()
		for step := 0; step < 4*logicalWidth; step++ {
			a.stepSTShipWalker(target[0], target[1])
		}
		x0, y0, x1, y1 := a.shipWalker.box()
		if x0 < 0 || y0 < 0 || x1 >= logicalWidth || y1 >= logicalHeight {
			t.Fatalf("追 %v 之後方框跑到 (%d,%d)-(%d,%d)", target, x0, y0, x1, y1)
		}
	}
}

// TestGroundRepeatOnlyAfterTheDelay: 按住不放的第一次重複要在延遲之後，然後每
// stGroundRepeatFrames 幀一次。
func TestGroundRepeatOnlyAfterTheDelay(t *testing.T) {
	if stRepeatDue(1) || stRepeatDue(stGroundRepeatDelay) {
		t.Fatal("延遲還沒到就重複了")
	}
	if !stRepeatDue(stGroundRepeatDelay + stGroundRepeatFrames) {
		t.Fatal("延遲之後第一次重複沒發生")
	}
	fires := 0
	for held := 1; held <= stGroundRepeatDelay+10*stGroundRepeatFrames; held++ {
		if stRepeatDue(held) {
			fires++
		}
	}
	if fires != 10 {
		t.Fatalf("按住那段時間重複了 %d 次，應該是 10 次", fires)
	}
}

// TestOnlyMovementActionsRepeat: 按住只重複走路，交易、銀行、加油這些不重複。
func TestOnlyMovementActionsRepeat(t *testing.T) {
	for _, action := range []game.Action{
		game.ActionMoveNorth, game.ActionMoveSouth, game.ActionMoveWest, game.ActionMoveEast,
	} {
		if !isGroundMove(action) {
			t.Fatalf("%q 應該可以重複", action)
		}
	}
	for _, action := range []game.Action{
		game.ActionOpenTrade, game.ActionOpenUniteller, game.ActionRefuel,
		game.ActionTalk, game.ActionBack, game.ActionOpenStatus, "",
	} {
		if isGroundMove(action) {
			t.Fatalf("%q 不應該可以重複", action)
		}
	}
}

// TestGroundMoveKeysAreTheOnesKeyActionMoves: 會重複的按鍵清單要跟 keyAction 在地面
// 上真的當成移動的那些鍵一致——漏一個鍵就是那個方向按住沒反應。
func TestGroundMoveKeysAreTheOnesKeyActionMoves(t *testing.T) {
	state := game.NewState()
	state.Screen = game.ScreenGround
	listed := map[game.Action]bool{}
	for _, key := range stGroundMoveKeys {
		action, ok := keyAction(key, state)
		if !ok || !isGroundMove(action) {
			t.Fatalf("按鍵 %v 在地面上不是移動（%q, %v）", key, action, ok)
		}
		listed[action] = true
	}
	for _, action := range []game.Action{
		game.ActionMoveNorth, game.ActionMoveSouth, game.ActionMoveWest, game.ActionMoveEast,
	} {
		if !listed[action] {
			t.Fatalf("沒有任何一個會重複的按鍵走 %q", action)
		}
	}
}

// groundMoveTarget 把一個移動動作套到位置上，與 reducer 的 move 同義。
func groundMoveTarget(from game.Position, action game.Action) game.Position {
	switch action {
	case game.ActionMoveWest:
		return game.Position{X: from.X - 1, Y: from.Y}
	case game.ActionMoveEast:
		return game.Position{X: from.X + 1, Y: from.Y}
	case game.ActionMoveNorth:
		return game.Position{X: from.X, Y: from.Y - 1}
	case game.ActionMoveSouth:
		return game.Position{X: from.X, Y: from.Y + 1}
	}
	return from
}

// TestGroundRoutingGoesRoundAWall: 目標在牆的另一邊時，第一步要走得開，而且那一步
// 本身要落在可通行的格子上——貪心方向會直接撞牆卡住，這正是繞路要解決的。
func TestGroundRoutingGoesRoundAWall(t *testing.T) {
	a := newOptionsTestApp(t)
	cityID := a.state.CityID
	blocked := func(p game.Position) bool {
		return !a.reducer.World.CanEnter(cityID, world.TilePosition{X: p.X, Y: p.Y})
	}
	checked := 0
	for y := 0; y < world.MapHeight(); y++ {
		for x := 0; x < world.MapWidth(); x++ {
			from := game.Position{X: x, Y: y}
			if blocked(from) {
				continue
			}
			a.state.Position = from
			for ty := 0; ty < world.MapHeight(); ty++ {
				for tx := 0; tx < world.MapWidth(); tx++ {
					target := game.Position{X: tx, Y: ty}
					if target == from || blocked(target) {
						continue
					}
					greedy := directionTo(from, target)
					if !blocked(groundMoveTarget(from, greedy)) {
						continue
					}
					step, ok := a.groundPathFirstStep(from, target)
					if !ok {
						continue
					}
					if next := groundMoveTarget(from, step); blocked(next) {
						t.Fatalf("從 %v 往 %v 的第一步 %q 走進牆裡 %v", from, target, step, next)
					}
					if step == greedy {
						t.Fatalf("繞路和貪心給了同一步 %q，但貪心那格是牆", step)
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Skip("這座城市裡找不到需要繞路的組合")
	}
	t.Logf("繞路生效的組合：%d 組", checked)
}

// TestGroundRoutingFallsBackWhenTheTargetIsAWall: 目標本身是牆時沒有路，退回貪心
// 方向，這樣往牆按仍然會撞牆並拿到 cannot_move，跟原版一樣。
func TestGroundRoutingFallsBackWhenTheTargetIsAWall(t *testing.T) {
	a := newOptionsTestApp(t)
	cityID := a.state.CityID
	for y := 0; y < world.MapHeight(); y++ {
		for x := 0; x < world.MapWidth(); x++ {
			target := game.Position{X: x, Y: y}
			if a.reducer.World.CanEnter(cityID, world.TilePosition{X: x, Y: y}) {
				continue
			}
			if _, ok := a.groundPathFirstStep(a.state.Position, target); ok {
				t.Fatalf("目標 %v 是牆卻找到路", target)
			}
			if got := a.groundStepToward(target); got != directionTo(a.state.Position, target) {
				t.Fatalf("目標是牆時沒有退回貪心方向：%q", got)
			}
			return
		}
	}
	t.Skip("這座城市沒有牆")
}

// buildTestCabin 給 app 一份小小的合成艙室：全部可站，只有中間一道直牆，牆上開一個
// 缺口。取樣圖樣用單點，這樣錨點與位置一對一，測試好讀。
func buildTestCabin(a *app, width, height, wallX, gapY int) {
	a.cabinWidth, a.cabinHeight = width, height
	a.cabinFloor = make([]bool, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			a.cabinFloor[y*width+x] = x != wallX || y == gapY
		}
	}
	a.cabinPattern = []originalmedia.CollidePoint{{X: stWalkerAnchor, Y: stWalkerAnchor}}
	a.cabinPatternRead = true
	a.cabinLegal = nil
	a.cabinFieldHasTarget = false
}

// TestCabinWallsComeFromTheColourString: set_coll_color 的
// "1100000000000001" 讀成色 0、1、15 不可站，其餘十三色可以。
func TestCabinWallsComeFromTheColourString(t *testing.T) {
	for colour, passable := range stCabinPassable {
		want := colour != 0 && colour != 1 && colour != 15
		if passable != want {
			t.Fatalf("色 %d 的可站性是 %v，字串說 %v", colour, passable, want)
		}
	}
}

// TestCabinWalkerStopsAtAWall: 實跑量到的行為——朝目標踏一步，那一步是牆就停在原地，
// 而且再按住多久都不會動（原版不沿牆滑、也不繞）。這裡把繞路關掉（目標在牆上）來測
// 那條退路。
func TestCabinWalkerStopsAtAWall(t *testing.T) {
	a := newOptionsTestApp(t)
	buildTestCabin(a, 40, 20, 20, -1) // 沒有缺口
	a.shipWalker = stShipWalker{ready: true, x: 10 - stWalkerAnchor, y: 10 - stWalkerAnchor}
	target := [2]int{25 - stWalkerAnchor, 10 - stWalkerAnchor}
	for step := 0; step < 200; step++ {
		a.stepSTShipWalker(target[0], target[1])
	}
	ax, _ := a.shipWalker.anchor()
	if ax != 19 {
		t.Fatalf("小人停在錨點 x=%d，牆在 x=20，應該停在 19", ax)
	}
	before := a.shipWalker
	for step := 0; step < 200; step++ {
		a.stepSTShipWalker(target[0], target[1])
	}
	if a.shipWalker.x != before.x || a.shipWalker.y != before.y {
		t.Fatal("撞牆之後又動了；原版按住 18 秒是一個像素都不動")
	}
}

// TestCabinWalkerRoutesThroughTheGap: 牆上有缺口而目標可站時，重製版會繞過去。
// 這一段是重製版自己加的（原版停在牆前不動），列在 WORKLIST B。
func TestCabinWalkerRoutesThroughTheGap(t *testing.T) {
	a := newOptionsTestApp(t)
	buildTestCabin(a, 40, 20, 20, 5)
	a.shipWalker = stShipWalker{ready: true, x: 10 - stWalkerAnchor, y: 10 - stWalkerAnchor}
	target := [2]int{25 - stWalkerAnchor, 10 - stWalkerAnchor}
	for step := 0; step < 400; step++ {
		a.stepSTShipWalker(target[0], target[1])
		if a.shipWalker.x == target[0] && a.shipWalker.y == target[1] {
			return
		}
	}
	ax, ay := a.shipWalker.anchor()
	t.Fatalf("四百步之後小人在錨點 (%d,%d)，沒有從缺口繞到 (25,10)", ax, ay)
}

// TestCabinWalkerNeedsNoDiskToMove: 沒有磁碟就沒有底圖也沒有取樣圖樣，這時不判牆
// ——退化成走直線，而不是把小人鎖死。
func TestCabinWalkerNeedsNoDiskToMove(t *testing.T) {
	a := newOptionsTestApp(t)
	a.shipWalker = stShipWalker{ready: true, x: 100, y: 100}
	a.stepSTShipWalker(120, 90)
	if a.shipWalker.x != 101 || a.shipWalker.y != 99 {
		t.Fatalf("沒有磁碟時走一步到 (%d,%d)，應該是 (101,99)", a.shipWalker.x, a.shipWalker.y)
	}
}
