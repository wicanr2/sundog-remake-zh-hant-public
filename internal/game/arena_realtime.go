package game

// 競技場的即時制。
//
// 原版的地面戰鬥沒有回合：`XDOCOMBA:0x01 xdo_combat` 的主迴圈每一圈跑一次
// `handle_player`（`0x10`，`0x07ae`）和一次 `handle_thugs`（`0x11`，`0x0958`），
// 兩支都只看當下的滑鼠與 sprite 位置。重製版原本把一場架收斂成一連串交換，
// 那是刻意的差異；這一份把原版的模型接回來。
//
// 場上每個人在原版是 `XDOCOMBA` 一筆五個 word 的記錄（外層 local `0x21` 起，
// 步距 5），記錄 1 是玩家、2 起是對手：
//
//	word 0  sprite 編號（`init_people` 直接寫成記錄索引）
//	word 1  開火速度——玩家 `DX div 5 + 3`、對手 `random() mod 8 + 6`
//	word 2  血量，`<= 32` 就是倒下
//	word 3  節拍計數器，每一圈減一
//	word 4  還在場上
//
// **word 1 不是走路的速度。** 走路的速度是 `create_sprite` 的第三個參數：
// 玩家固定 6、對手 `6 + random() mod 4`。word 1 只決定開火的節奏——
// 冷卻長度 `14 − word1`，以及開火機率的分子 `word1 − word3`。
//
// 對手的一圈（`handle_thugs`）：
//
//  1. 從玩家的七筆位置歷史裡擲 `random() mod 7` 挑一筆當目標。歷史由
//     `handle_player` 每一圈寫一筆，環狀覆蓋（`0x090c`）——所以對手瞄的是玩家
//     **前幾圈**的位置，這是原版射不準的來源。
//  2. `word 3` 減一。倒下的（`word 2 <= 32`）只做這件事，數到 `<= 0` 才撤走。
//  3. 依 `|dx| + |dy|` 與模式選一種走法：逃跑、遠距離的骰子、近距離的後退。
//  4. 走出內縮框就撤走。
//  5. 冷卻到期且模式允許時，`(word1 − word3) > random() mod 150` 才開火——
//     `word 3` 一直往下減，所以等得越久越容易開火，最遠 150 圈保底。
//
// 玩家的一圈（`handle_player`）：左鍵按住就朝滑鼠走、放開就停；右鍵開火，
// 而且要放開再按才有下一發（`L1` 那個閂）。走出內縮框就是離開戰鬥。
//
// 什麼是重製版自己的：**sprite 每一圈實際走多少像素**。原版的 `do_sprite`
// 在背景中斷裡跑，速度單位沒有量過，所以這裡讓速度直接當成每圈的像素數。
// 還有**射線碰到什麼**——原版的 `hit_scan` 用 `collide(圖樣 4, x, y)` 去讀畫面上
// 的顏色，撞到了再回查是誰；重製版的競技場沒有原版那張地板，所以這裡改成直接
// 量每個人的框。牆的部分因此只剩場地邊界。

import "fmt"

// 場地。`XDOCOMBA:0x09 init_arena`（`0x0334`）把視窗 3 開成 `make_zoom` 樣式 11、
// gx 夾 3..18、gy 夾 3..9，走路人固定在畫面第 (4,2) 格所以兩個都夾到上限，
// 內容區左上角落在 (145, 73)；樣式 11 < 12，內容區 158×98。
const (
	arenaWindowX0 = 145
	arenaWindowY0 = 73
	arenaWindowX1 = arenaWindowX0 + 158 - 1
	arenaWindowY1 = arenaWindowY0 + 98 - 1
	// init_arena 把視窗的四個界各往內縮 7 才是行動範圍（`0x035a`–`0x0373`）。
	arenaInset = 7
	// 行動範圍。走出去的：玩家算逃離，對手算撤走。
	arenaFieldLeft   = arenaWindowX0 + arenaInset
	arenaFieldRight  = arenaWindowX1 - arenaInset
	arenaFieldTop    = arenaWindowY0 + arenaInset
	arenaFieldBottom = arenaWindowY1 - arenaInset
	// 視窗的中心，`init_arena` 最後那一支 `get_centre`。玩家從這裡開始。
	arenaCentreX = (arenaWindowX0 + arenaWindowX1) / 2
	arenaCentreY = (arenaWindowY0 + arenaWindowY1) / 2
)

const (
	// arenaHistory is how many of the player's past positions the thugs aim
	// at: handle_player writes one a pass into a ring of seven.
	arenaHistory = 7
	// arenaCloseRange is the |dx| + |dy| below which a thug backs off instead
	// of closing in (`0x09ea`, the constant is 0x23).
	arenaCloseRange = 35
	// arenaBackStep is the span of the retreat step: (sign x random()) mod 7.
	arenaBackStep = 7
	// arenaApproachJitter is the box a thug aims its approach into,
	// rand(-14, 14) on both axes — 7 x 2, written that way in the p-code.
	arenaApproachJitter = 14
	// arenaWanderFaces is the die a thug rolls at range: 0 and 1 close in,
	// 2 stops, 3..19 leave the sprite to carry on by itself.
	arenaWanderFaces = 20
	arenaWanderClose = 2
	arenaWanderHalt  = 2
	// arenaFireOdds is the denominator of the firing roll. The numerator is
	// how far past its cooldown the thug is, which grows without bound, so a
	// thug that has held fire for 150 passes fires for certain.
	arenaFireOdds = 150
	// arenaCooldownBase is what a shot costs: 14 - the shooter's speed, so
	// 1..8 passes.
	arenaCooldownBase = 14
	// The opening cooldown is rand(14-3, 14+5) minus the speed (`0x02bc`).
	arenaOpeningCooldownLow  = arenaCooldownBase - 3
	arenaOpeningCooldownHigh = arenaCooldownBase + 5
	// arenaPlayerMove and arenaThugMoveBase/Span are create_sprite's speeds:
	// the player is a flat 6, a thug is 6 + random() mod 4.
	arenaPlayerMove   = 6
	arenaThugMoveBase = 6
	arenaThugMoveSpan = 4
	// The scattergun spends a shot every fourth trigger pull (`0x08d1`), not
	// every one.
	arenaScattergunPullsPerShot = 4
	// A thug's first drop is anywhere in the field with a margin of 20 on x
	// and 10 on y (`XDOCOMBA:0x07`); the rest stand off the centre by these.
	arenaDropMarginX = 20
	arenaDropMarginY = 10
	arenaStandOffX   = 15
	arenaStandOffY   = 11
	// arenaBodySize is the box a shot has to cross to count as a hit. The
	// original reads the screen instead, so this figure is the remake's: it is
	// the walker sprite's own 7x7.
	arenaBodySize = 7
	// arenaRayStep is how far a shot advances an axis at a time (`0x05aa`).
	arenaRayStep = 2
	// arenaRayDeadZone is the gap below which a shot does not move an axis at
	// all (`0x0593`).
	arenaRayDeadZone = 2
)

// Arena modes, the original's XDOCOMBA local 0x0c.
const (
	// arenaModeFighting is the ordinary state: thugs close in and shoot.
	arenaModeFighting = 0
	// arenaModeHolding stops the shooting and leaves the thugs milling about
	// at range. handle_player drops back to it the moment the player fires.
	arenaModeHolding = 1
	// arenaModeScattering is what the player going down turns into: the thugs
	// take everything the player is carrying and run for the nearest edge.
	arenaModeScattering = 2
)

// ArenaPoint is one recorded position.
type ArenaPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ArenaInput is what one pass of the fight sees of the player: where the
// pointer is and which buttons are down. It is the original's two calls to
// GEMBIND:0x1f (button) and GEMBIND:0x1e (position).
type ArenaInput struct {
	PointerX int
	PointerY int
	// Moving is the left button: hold it and the player walks to the pointer.
	Moving bool
	// Firing is the right button. It has to be released between shots.
	Firing bool
}

// arenaRand is MAINLIB:0x0c rand(low, high), both ends included.
func arenaRand(arena *Arena, low, high int) int {
	if high < low {
		return low
	}
	return low + arenaRoll(arena, high-low+1)
}

// arenaSign is MAINLIB:0x08.
func arenaSign(value int) int {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	}
	return 0
}

// clampInt keeps a value inside [low, high], the way MAINLIB:0x0a condition
// does.
func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// arenaSeat drops a thug on the field. XDOCOMBA:0x07 rerolls the first one
// until it lands somewhere it can stand; without the original's floor there is
// nothing here to stand on, so the roll is taken once.
func arenaSeat(arena *Arena, index int) (int, int) {
	if index == 0 {
		return arenaRand(arena, arenaWindowX0+arenaDropMarginX, arenaWindowX1-arenaDropMarginX),
			arenaRand(arena, arenaWindowY0+arenaDropMarginY, arenaWindowY1-arenaDropMarginY)
	}
	x := arenaCentreX - arenaStandOffX
	if arenaRoll(arena, 2) == 1 {
		x = arenaCentreX + arenaStandOffX
	}
	y := arenaCentreY - arenaStandOffY
	if arenaRoll(arena, 2) == 1 {
		y = arenaCentreY + arenaStandOffY
	}
	return x, y
}

// arenaStepToward walks a sprite one pass toward its target at its own speed.
// The original leaves this to do_sprite in the background; how many pixels that
// covers a pass is not decoded, so the speed is taken as the pixel count.
func arenaStepToward(x, y, targetX, targetY, speed int) (int, int) {
	if speed <= 0 {
		speed = 1
	}
	step := func(from, to int) int {
		delta := to - from
		if delta > speed {
			delta = speed
		}
		if delta < -speed {
			delta = -speed
		}
		return from + delta
	}
	return step(x, targetX), step(y, targetY)
}

// arenaAim records where the player is now, so the thugs have something to aim
// at next pass. handle_player's own ring of seven (`0x090c`).
func arenaAim(arena *Arena) {
	arena.HistoryAt = (arena.HistoryAt + 1) % arenaHistory
	arena.History[arena.HistoryAt] = ArenaPoint{X: arena.PlayerX, Y: arena.PlayerY}
}

// arenaTargetPoint is what one thug aims at this pass: random() mod 7 of the
// player's recorded positions.
func arenaTargetPoint(arena *Arena) ArenaPoint {
	return arena.History[arenaRoll(arena, arenaHistory)]
}

// arenaInsideField reports whether a sprite is still on the field.
func arenaInsideField(x, y int) bool {
	return x >= arenaFieldLeft && x <= arenaFieldRight &&
		y >= arenaFieldTop && y <= arenaFieldBottom
}

// arenaMoveSprite is XDOCOMBA:0x12: clamp the destination into the window and
// hand it to move_sprite.
func arenaMoveSprite(x, y int) (int, int) {
	return clampInt(x, arenaWindowX0+1, arenaWindowX1-1),
		clampInt(y, arenaWindowY0+1, arenaWindowY1-1)
}

// arenaActorBox reports whether a point is inside a fighter's body box.
func arenaActorBox(x, y, pointX, pointY int) bool {
	half := arenaBodySize / 2
	return pointX >= x-half && pointX <= x+half &&
		pointY >= y-half && pointY <= y+half
}

// arenaShooterAt returns where a fighter is standing. Index 0 is the player,
// 1-based into Thugs for an opponent.
func arenaShooterAt(arena *Arena, index int) (int, int, bool) {
	if index == 0 {
		return arena.PlayerX, arena.PlayerY, true
	}
	if index-1 >= len(arena.Thugs) {
		return 0, 0, false
	}
	thug := arena.Thugs[index-1]
	if thug.Gone {
		return 0, 0, false
	}
	return thug.X, thug.Y, true
}

// arenaRayHit traces a shot the way XDOCOMBA:0x0d does: step ±2 on each axis,
// the two axes clocked against each other so the line keeps its slope, and stop
// at whoever it runs into. The original reads the screen for that; the remake
// has no arena floor to read, so it measures the fighters' own boxes and takes
// the field edge as the only wall.
//
// The dead zone at `0x0593` compares the *target's x against the shooter's y*
// before deciding whether x moves at all. That looks like a slip, but it is
// what the original does and it is visible in play — a shot lined up on that
// diagonal travels straight down instead of across — so it is copied here.
func arenaRayHit(arena *Arena, shooter, targetX, targetY, resolution int) (hit, endX, endY int) {
	x, y, ok := arenaShooterAt(arena, shooter)
	if !ok {
		return -1, targetX, targetY
	}
	stepX := 0
	if abs(targetX-y) >= arenaRayDeadZone {
		stepX = arenaRayStep
		if targetX <= x {
			stepX = -arenaRayStep
		}
	}
	stepY := 0
	if abs(targetY-y) >= arenaRayDeadZone {
		stepY = arenaRayStep
		if targetY <= y {
			stepY = -arenaRayStep
		}
	}
	if stepX == 0 && stepY == 0 {
		return -1, x, y
	}
	runX := abs(targetX - x)
	runY := abs(targetY - y)
	if resolution < 1 {
		resolution = 1
	}
	for runY > resolution || runX > resolution {
		runY /= 2
		runX /= 2
	}
	countX, countY := runY, runX
	spanX, spanY := runY, runX
	for {
		moved := false
		countX--
		if countX < 0 {
			x += stepX
			countX = spanX
			moved = true
		}
		countY--
		if countY < 0 {
			y += stepY
			countY = spanY
			moved = true
		}
		if !moved {
			continue
		}
		if victim := arenaVictimAt(arena, x, y, shooter); victim >= 0 {
			return victim, x, y
		}
		if !arenaInsideField(x, y) {
			return -1, x, y
		}
	}
}

// arenaVictimAt is XDOCOMBA:0x0c: who is standing on this point. The shooter
// itself does not count.
func arenaVictimAt(arena *Arena, x, y, shooter int) int {
	if shooter != 0 && arenaActorBox(arena.PlayerX, arena.PlayerY, x, y) {
		return 0
	}
	for index := range arena.Thugs {
		if arena.Thugs[index].Gone || index+1 == shooter {
			continue
		}
		if arenaActorBox(arena.Thugs[index].X, arena.Thugs[index].Y, x, y) {
			return index + 1
		}
	}
	return -1
}

// arenaFireResolution is the shooter's record word 1, which fire_gun's hit scan
// halves the run down to. It only sets how finely the line is stepped.
func arenaFireResolution(arena *Arena, shooter int) int {
	if shooter == 0 {
		return arena.Speed
	}
	if shooter-1 < len(arena.Thugs) {
		return arena.Thugs[shooter-1].Speed
	}
	return 1
}

// arenaShotColour is the ray's colour: the player's scattergun draws in 12, a
// stinger and every thug in 13.
func arenaShotColour(arena *Arena, shooter int) int {
	if shooter == 0 && arena.Gun == itemScattergun {
		return 12
	}
	return 13
}

// arenaFire is one shot: trace it, then hand whatever it landed on to
// fire_gun's own damage chain.
func arenaFire(state *State, shooter, targetX, targetY int) {
	arena := &state.Arena
	victim, endX, endY := arenaRayHit(arena, shooter, targetX, targetY, arenaFireResolution(arena, shooter))
	fromX, fromY, _ := arenaShooterAt(arena, shooter)
	arena.Shot = ArenaShot{
		Live: true, FromX: fromX, FromY: fromY,
		ToX: endX, ToY: endY, Colour: arenaShotColour(arena, shooter), Hit: victim,
	}
	if victim < 0 {
		return
	}
	// fire_gun picks the damage base off the *shooter*, not the target: the
	// player's own gun when the player fired, a flat three otherwise. So a
	// thug that shoots another thug hits for a thug's damage.
	base := arenaThugDamage
	if shooter == 0 {
		base = arenaPlayerDamageBase(arena.Gun)
	}
	if victim == 0 {
		arenaHitPlayer(state, base)
		return
	}
	arenaHitThug(state, victim-1, base)
}

// arenaHitPlayer is fire_gun's first branch: a shield eats the shot outright,
// and the shot only reaches the player once the shield is spent.
func arenaHitPlayer(state *State, base int) {
	arena := &state.Arena
	if arena.ShieldCharges > 0 {
		arena.ShieldCharges--
		if arena.ShieldCharges == 0 && arena.ShieldSlot >= 0 {
			slots := carrySlots(*state)
			slots[arena.ShieldSlot] = itemDeadPart
			writeCarrySlots(state, slots)
			arena.ShieldSlot, arena.ShieldCharges = arenaShieldSlot(slots)
		}
		return
	}
	if applyArenaDamage(&arena.HitPoints, arenaShotDamage(arena, base)) {
		arenaPlayerGoesDown(arena)
	}
}

// arenaHitThug takes the damage off an opponent and, if that puts it down,
// starts its own count off the field.
func arenaHitThug(state *State, index, base int) {
	arena := &state.Arena
	if index >= len(arena.Thugs) || arena.Thugs[index].Gone {
		return
	}
	thug := &arena.Thugs[index]
	if !applyArenaDamage(&thug.HitPoints, arenaShotDamage(arena, base)) {
		return
	}
	thug.Down = true
	// The sprite goes the moment the hit points reach zero; anything from 1 to
	// 32 leaves the body lying there until handle_thugs counts it away.
	if thug.HitPoints == 0 {
		thug.Gone = true
	}
}

// arenaPlayerGoesDown is XDOCOMBA:0x04: the player hits the floor and a count
// of nineteen passes starts before the crowd helps itself.
func arenaPlayerGoesDown(arena *Arena) {
	arena.Cooldown = 19
	arena.Mode = arenaModeHolding
	arena.DownPending = false
}

// arenaRobPlayer is what the count runs out into: everything in the carry slots
// is gone and the crowd scatters.
func arenaRobPlayer(state *State) {
	arena := &state.Arena
	var empty [CarrySlots]int
	writeCarrySlots(state, empty)
	arena.Gun = 0
	arena.Shots = 0
	arena.ShieldSlot, arena.ShieldCharges = -1, 0
	arena.Mode = arenaModeScattering
	arena.Robbed = true
}

// arenaHandlePlayer is XDOCOMBA:0x10. It runs every pass.
func arenaHandlePlayer(state *State, input ArenaInput) {
	arena := &state.Arena
	if arena.HitPoints <= arenaDownThreshold || arena.DownPending {
		// On the floor. The count runs down and then the crowd takes
		// everything; the original's way out of this (flag 0x30 plus a button)
		// needs a flag nothing in the segment ever sets, so there is none.
		if arena.Cooldown > 0 {
			arena.Cooldown--
		}
		switch {
		case arena.Cooldown == 0 && arena.Mode == arenaModeHolding:
			arenaRobPlayer(state)
		case arena.Cooldown <= 0 && arenaRecordsLeft(arena) == 1:
			arena.Escaped = true
		}
		arenaAim(arena)
		return
	}
	if input.Moving {
		arena.TargetX, arena.TargetY = input.PointerX, input.PointerY
		arena.Halted = false
	} else {
		arena.Halted = true
	}
	if !arena.Halted {
		arena.PlayerX, arena.PlayerY = arenaStepToward(
			arena.PlayerX, arena.PlayerY, arena.TargetX, arena.TargetY, arenaPlayerMove)
	}
	if !arenaInsideField(arena.PlayerX, arena.PlayerY) {
		arena.Escaped = true
	}
	switch {
	case input.Firing && arena.FireLatch && arena.Mode < arenaModeScattering:
		arenaFire(state, 0, input.PointerX, input.PointerY)
		if arena.Gun == itemScattergun {
			arena.Pulls = (arena.Pulls + 1) % arenaScattergunPullsPerShot
			if arena.Pulls == 0 {
				arena.Shots--
				if arena.Shots <= 0 {
					// The gun in the hand becomes a dead part and check_guns
					// runs again, so a second gun further down the carry slots
					// takes over.
					arena.Shots = 0
					slots := carrySlots(*state)
					slots[0] = itemDeadPart
					arena.Gun, arena.Shots = arenaDrawGun(&slots)
					writeCarrySlots(state, slots)
				}
			}
		}
		arena.Mode = arenaModeFighting
		arena.FireLatch = false
	case !input.Firing:
		arena.FireLatch = true
	}
	arenaAim(arena)
}

// arenaRecordsLeft is the original's local 0x1a: the player plus whichever
// opponents still have a record.
func arenaRecordsLeft(arena *Arena) int {
	left := 1
	for _, thug := range arena.Thugs {
		if !thug.Gone {
			left++
		}
	}
	return left
}

// arenaHandleThugs is XDOCOMBA:0x11. The original runs it off a two-tick timer
// rather than every pass, so it moves at half the player's rate.
func arenaHandleThugs(state *State) {
	arena := &state.Arena
	for index := range arena.Thugs {
		thug := &arena.Thugs[index]
		if thug.Gone {
			continue
		}
		target := arenaTargetPoint(arena)
		thug.Cooldown--
		if thug.Down {
			if thug.Cooldown <= 0 {
				thug.Gone = true
			}
			continue
		}
		distance := abs(target.X-thug.X) + abs(target.Y-thug.Y)
		switch {
		case arena.Mode == arenaModeScattering || arena.Fleeing:
			// Run for the nearest edge. This one is not clamped: getting out is
			// the point.
			thug.TargetX = arenaWindowX1
			if thug.X <= arenaCentreX {
				thug.TargetX = arenaWindowX0
			}
			thug.TargetY = arenaWindowY1
			if thug.Y <= arenaCentreY {
				thug.TargetY = arenaWindowY0
			}
			thug.Halted = false
		case distance > arenaCloseRange || arena.Mode == arenaModeHolding:
			switch roll := arenaRoll(arena, arenaWanderFaces); {
			case roll < arenaWanderClose:
				thug.TargetX, thug.TargetY = arenaMoveSprite(
					target.X+arenaRand(arena, -arenaApproachJitter, arenaApproachJitter),
					target.Y+arenaRand(arena, -arenaApproachJitter, arenaApproachJitter))
				thug.Halted = false
			case roll == arenaWanderHalt:
				thug.Halted = true
			default:
				// Seventeen faces out of twenty leave the sprite alone: it
				// carries on toward wherever it was last sent.
			}
		default:
			// Close in: back off instead, which is what keeps the crowd at
			// shooting distance.
			thug.TargetX, thug.TargetY = arenaMoveSprite(
				thug.X+goMod(arenaSign(thug.X-target.X)*arenaRoll(arena, 1<<15), arenaBackStep),
				thug.Y+goMod(arenaSign(thug.Y-target.Y)*arenaRoll(arena, 1<<15), arenaBackStep))
			thug.Halted = false
		}
		if !thug.Halted {
			thug.X, thug.Y = arenaStepToward(thug.X, thug.Y, thug.TargetX, thug.TargetY, thug.Move)
		}
		if !arenaInsideField(thug.X, thug.Y) {
			thug.Gone = true
			continue
		}
		if thug.Cooldown <= 0 && arena.Mode <= arenaModeFighting && !arena.Fleeing &&
			thug.Speed-thug.Cooldown > arenaRoll(arena, arenaFireOdds) {
			arenaFire(state, index+1, target.X, target.Y)
			thug.Cooldown = arenaCooldownBase - thug.Speed
		}
	}
}

// goMod is Pascal's mod as the p-machine's MODI does it: the remainder keeps
// the dividend's sign, which is the whole point of the sign() in front of the
// retreat step. Go's % already works that way; this is here to say so.
func goMod(value, span int) int {
	if span == 0 {
		return 0
	}
	return value % span
}

// StepArena runs one pass of the fight. It is a step the way Reducer.Step is,
// but it carries the pointer and the two buttons, which an Action cannot.
//
// The original runs handle_player every time round its loop and handle_thugs
// only when a two-tick timer comes due (`XDOCOMBA:0x01`, `0x0af6`), so the
// crowd moves at half the player's rate. What one tick is worth in wall clock
// is not decoded; the remake takes a pass to be a frame.
func (r Reducer) StepArena(state State, input ArenaInput) (State, error) {
	if state.Screen != ScreenArena {
		return state, fmt.Errorf("the arena only runs on the arena screen")
	}
	next := state
	next.Arena.Thugs = append([]ArenaThug(nil), state.Arena.Thugs...)
	next.MessageKey = arenaPass(&next, input)
	return next, nil
}

// arenaPass is the loop body both the pointer and the keyboard go through.
func arenaPass(state *State, input ArenaInput) string {
	arena := &state.Arena
	arena.Shot = ArenaShot{}
	if !arena.Active {
		return state.MessageKey
	}
	hitPoints, charges, gun := arena.HitPoints, arena.ShieldCharges, arena.Gun
	message := state.MessageKey
	arenaHandlePlayer(state, input)
	switch {
	case gun == itemScattergun && arena.Gun != gun:
		message = "ui.message.arena_ammo_out"
	case arena.Shot.Live:
		message = "ui.message.arena_exchange"
	}
	arena.Ticks++
	if arena.Ticks%2 == 0 {
		arenaHandleThugs(state)
	}
	// Anything that reached the player outranks what the player did.
	if arena.HitPoints < hitPoints || arena.ShieldCharges < charges {
		message = "ui.message.arena_hit"
	}
	state.MessageKey = message
	return arenaSettle(state)
}

// arenaNearestThug is what the keyboard aims at, since it has no pointer: the
// closest opponent still on its feet.
func arenaNearestThug(arena *Arena) (int, int, bool) {
	best, bestX, bestY := -1, 0, 0
	for _, thug := range arena.Thugs {
		if thug.Gone || thug.Down {
			continue
		}
		distance := abs(thug.X-arena.PlayerX) + abs(thug.Y-arena.PlayerY)
		if best < 0 || distance < best {
			best, bestX, bestY = distance, thug.X, thug.Y
		}
	}
	return bestX, bestY, best >= 0
}

// arenaSettle is what closes a fight: everyone on the field has been taken off
// it, or the player walked out. Being robbed is not itself the end — the crowd
// has to scatter off the field first, which is the original's own condition
// (`0x0855`, records left back down to one).
func arenaSettle(state *State) string {
	arena := &state.Arena
	standing := 0
	for _, thug := range arena.Thugs {
		if !thug.Gone {
			standing++
		}
	}
	switch {
	case arena.Escaped:
		arena.Active = false
		state.Screen = ScreenGround
		return "ui.message.arena_fled"
	case standing > 0:
		if arena.Robbed {
			return "ui.message.arena_lost"
		}
		return state.MessageKey
	case arena.Robbed:
		// Being beaten in the arena costs what the player was carrying, not the
		// game: the original's fight has no death in it. The hit points it
		// works in are its own -- health x ST / 90 -- and it never writes them
		// back to the character.
		arena.Active = false
		arena.Lost = true
		state.Screen = ScreenGround
		return "ui.message.arena_lost"
	}
	// The original hands the win to a type-15 conversation with a purse in it
	// (fire_gun writes MAINLIB_Gdb := 15 as the last opponent goes down). That
	// conversation is not in the remake, so nothing is paid out here.
	arena.Active = false
	arena.Won = true
	state.Screen = ScreenGround
	return "ui.message.arena_won"
}

// ArenaField is the rectangle the fight happens in: make_zoom's content area
// for the arena window. The shell draws the window from these, so the picture
// and the rules cannot drift apart.
func ArenaField() (x0, y0, x1, y1 int) {
	return arenaWindowX0, arenaWindowY0, arenaWindowX1, arenaWindowY1
}

// ArenaBodySize is how wide a fighter is, for drawing and for the hit scan.
const ArenaBodySize = arenaBodySize

// ArenaStanding is how many opponents are still on their feet.
func ArenaStanding(arena Arena) int {
	standing := 0
	for _, thug := range arena.Thugs {
		if !thug.Gone && !thug.Down {
			standing++
		}
	}
	return standing
}
