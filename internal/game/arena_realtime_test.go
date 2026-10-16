package game

import "testing"

// arenaUnderway opens a fight with a fixed crowd, so a test can put the pieces
// where it wants them.
func arenaUnderway(t *testing.T, opponents int) State {
	t.Helper()
	state := NewState()
	state.Screen = ScreenGround
	beginArena(&state, opponents+1)
	if !state.Arena.Active || len(state.Arena.Thugs) != opponents {
		t.Fatalf("the fight did not open: active=%v thugs=%d", state.Arena.Active, len(state.Arena.Thugs))
	}
	return state
}

// XDOCOMBA:0x0a initialize's rand(2,6) counts records, and record 1 is the
// player. So a mugging is one to five opponents, not two to six.
func TestTheRecordCountIncludesThePlayer(t *testing.T) {
	for records := arenaMinRecords; records <= arenaMaxRecords; records++ {
		state := NewState()
		beginArena(&state, records)
		if got := len(state.Arena.Thugs); got != records-1 {
			t.Fatalf("%d 筆記錄開出 %d 個對手，應該是 %d", records, got, records-1)
		}
	}
	// Out of range clamps rather than panicking on a negative slice.
	state := NewState()
	beginArena(&state, 0)
	if len(state.Arena.Thugs) != arenaMinRecords-1 {
		t.Fatalf("記錄數 0 開出 %d 個對手", len(state.Arena.Thugs))
	}
}

// handle_player writes the player's position into a ring of seven and
// handle_thugs aims at one of those, drawn fresh every pass. What that buys is
// a crowd that shoots at where the player was, not where the player is.
func TestTheCrowdAimsAtTheTrail(t *testing.T) {
	state := arenaUnderway(t, 1)
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state.Screen = ScreenArena
	// Walk east for exactly as many passes as the ring is long, so it holds
	// seven different positions and the player has not reached the edge.
	for pass := 0; pass < arenaHistory; pass++ {
		state, err = reducer.StepArena(state, ArenaInput{
			PointerX: arenaFieldRight, PointerY: arenaCentreY, Moving: true,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[ArenaPoint]bool{}
	for _, point := range state.Arena.History {
		seen[point] = true
	}
	if len(seen) != arenaHistory {
		t.Fatalf("七筆歷史只有 %d 個不同的點：%+v", len(seen), state.Arena.History)
	}
	// Every recorded position is behind the player, never ahead of it.
	for _, point := range state.Arena.History {
		if point.X > state.Arena.PlayerX {
			t.Fatalf("歷史裡有走在玩家前面的點 %+v（玩家在 %d）", point, state.Arena.PlayerX)
		}
	}
}

// The close-range branch backs away instead of closing in, which is what keeps
// the crowd at shooting distance. sign() points the step away from the target,
// and Go's % keeps the dividend's sign the way MODI does.
func TestAThugBacksOffInsideThirtyFive(t *testing.T) {
	state := arenaUnderway(t, 1)
	arena := &state.Arena
	arena.Mode = arenaModeFighting
	arena.PlayerX, arena.PlayerY = arenaCentreX, arenaCentreY
	for index := range arena.History {
		arena.History[index] = ArenaPoint{X: arenaCentreX, Y: arenaCentreY}
	}
	// Stand the thug ten to the right of the player: |dx| + |dy| = 10 < 35.
	arena.Thugs[0].X, arena.Thugs[0].Y = arenaCentreX+10, arenaCentreY
	arena.Thugs[0].TargetX, arena.Thugs[0].TargetY = arena.Thugs[0].X, arena.Thugs[0].Y
	arena.Thugs[0].Cooldown = 100 // keep it from firing and muddying the test
	arenaHandleThugs(&state)
	if arena.Thugs[0].TargetX < arenaCentreX+10 {
		t.Fatalf("近距離的暴徒往玩家靠了：目標 x = %d，起點 %d", arena.Thugs[0].TargetX, arenaCentreX+10)
	}
}

// Seventeen of the twenty faces leave the sprite alone, so a thug at range
// mostly keeps drifting toward wherever it was last sent rather than tracking
// the player every pass.
func TestAThugAtRangeMostlyKeepsItsCourse(t *testing.T) {
	state := arenaUnderway(t, 1)
	arena := &state.Arena
	arena.PlayerX, arena.PlayerY = arenaFieldLeft, arenaFieldTop
	for index := range arena.History {
		arena.History[index] = ArenaPoint{X: arena.PlayerX, Y: arena.PlayerY}
	}
	arena.Thugs[0].X, arena.Thugs[0].Y = arenaFieldRight, arenaFieldBottom
	arena.Thugs[0].Cooldown = 1000
	retargets := 0
	for pass := 0; pass < 200; pass++ {
		before := ArenaPoint{X: arena.Thugs[0].TargetX, Y: arena.Thugs[0].TargetY}
		arena.Thugs[0].X, arena.Thugs[0].Y = arenaFieldRight, arenaFieldBottom
		arenaHandleThugs(&state)
		if arena.Thugs[0].TargetX != before.X || arena.Thugs[0].TargetY != before.Y {
			retargets++
		}
	}
	if retargets == 0 || retargets > 60 {
		t.Fatalf("200 圈裡重新瞄準了 %d 次，應該在十分之一上下", retargets)
	}
}

// The firing roll's numerator is how far past its cooldown the thug is, and
// that grows without bound: a thug that has held fire long enough fires for
// certain, which is what stops a fight stalling.
func TestHoldingFireLongEnoughGuaranteesAShot(t *testing.T) {
	state := arenaUnderway(t, 1)
	arena := &state.Arena
	arena.PlayerX, arena.PlayerY = arenaCentreX, arenaCentreY
	for index := range arena.History {
		arena.History[index] = ArenaPoint{X: arena.PlayerX, Y: arena.PlayerY}
	}
	arena.Thugs[0].X, arena.Thugs[0].Y = arenaCentreX+60, arenaCentreY
	arena.Thugs[0].Cooldown = -arenaFireOdds
	arena.ShieldCharges = 0
	before := arena.HitPoints
	arenaHandleThugs(&state)
	if arena.Thugs[0].Cooldown != arenaCooldownBase-arena.Thugs[0].Speed {
		t.Fatalf("冷卻沒有重設：%d", arena.Thugs[0].Cooldown)
	}
	if !arena.Shot.Live {
		t.Fatal("等了 150 圈還是沒開火")
	}
	if arena.Shot.Hit == 0 && arena.HitPoints >= before {
		t.Fatalf("打中了卻沒扣血：%d → %d", before, arena.HitPoints)
	}
}

// Walking off the field is how a fight ends without anyone winning — the same
// bounds check on both sides, opponents get taken off it and the player leaves.
func TestWalkingOffTheFieldEndsIt(t *testing.T) {
	state := arenaUnderway(t, 1)
	state.Screen = ScreenArena
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state.Arena.PlayerX, state.Arena.PlayerY = arenaFieldLeft, arenaCentreY
	next, err := reducer.StepArena(state, ArenaInput{
		PointerX: arenaWindowX0 - 40, PointerY: arenaCentreY, Moving: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !next.Arena.Escaped || next.Arena.Active || next.Screen != ScreenGround {
		t.Fatalf("走出場地沒有結束戰鬥：%+v screen=%q", next.Arena, next.Screen)
	}
}

// The player going down costs what they were carrying, not the game: the
// original's arena has no death in it, and it never writes its hit points back
// to the character.
func TestGoingDownCostsTheCarrySlotsNotTheGame(t *testing.T) {
	state := arenaUnderway(t, 1)
	state.Screen = ScreenArena
	state.HandItem = itemStinger
	state.Parts = []int{itemShield + 32*3, 0, 0}
	arena := &state.Arena
	slots := carrySlots(state)
	arena.Gun, arena.Shots = arenaDrawGun(&slots)
	arena.ShieldSlot, arena.ShieldCharges = arenaShieldSlot(slots)
	writeCarrySlots(&state, slots)
	arena.HitPoints = arenaDownThreshold
	arenaPlayerGoesDown(arena)
	if arena.Cooldown != 19 || arena.Mode != arenaModeHolding {
		t.Fatalf("倒下沒有起算：cooldown=%d mode=%d", arena.Cooldown, arena.Mode)
	}
	deaths := state.Deaths
	for pass := 0; pass < 25 && !arena.Robbed; pass++ {
		arenaHandlePlayer(&state, ArenaInput{})
	}
	if !arena.Robbed {
		t.Fatal("倒地二十五圈還沒有被搶")
	}
	for _, slot := range carrySlots(state) {
		if slot != 0 {
			t.Fatalf("被搶之後身上還有東西：%v", carrySlots(state))
		}
	}
	if state.Deaths != deaths {
		t.Fatalf("競技場記了一次死亡：%d → %d", deaths, state.Deaths)
	}
	// The crowd scatters; the fight only closes once they are all off the field.
	if arena.Mode != arenaModeScattering {
		t.Fatalf("被搶之後模式是 %d", arena.Mode)
	}
	if !arena.Active {
		t.Fatal("被搶的那一圈就把戰鬥關掉了，原版還要等暴徒跑光")
	}
}

// The right button has to be released between shots (local 0x01), so holding
// it down puts one shot out, not one a frame.
func TestTheTriggerHasToBeReleased(t *testing.T) {
	state := arenaUnderway(t, 1)
	state.Screen = ScreenArena
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	shots := 0
	for pass := 0; pass < 8; pass++ {
		state, err = reducer.StepArena(state, ArenaInput{
			PointerX: arenaFieldRight, PointerY: arenaCentreY, Firing: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if state.Arena.Shot.Live {
			shots++
		}
	}
	if shots != 1 {
		t.Fatalf("按住右鍵八圈開了 %d 槍，應該只有一槍", shots)
	}
	// Let go and it arms again.
	state, err = reducer.StepArena(state, ArenaInput{PointerX: arenaFieldRight, PointerY: arenaCentreY})
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.StepArena(state, ArenaInput{
		PointerX: arenaFieldRight, PointerY: arenaCentreY, Firing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Arena.Shot.Live {
		t.Fatal("放開再按沒有開火")
	}
}

// The scattergun spends a shot every fourth pull, not every one (`0x08d1`).
func TestTheScattergunSpendsAShotEveryFourthPull(t *testing.T) {
	state := arenaUnderway(t, 1)
	state.Screen = ScreenArena
	arena := &state.Arena
	arena.Gun, arena.Shots = itemScattergun, 3
	arena.Pulls = 0
	for pull := 0; pull < 4; pull++ {
		arena.FireLatch = true
		arenaHandlePlayer(&state, ArenaInput{PointerX: arenaFieldRight, PointerY: arenaCentreY, Firing: true})
		want := 3
		if pull == 3 {
			want = 2
		}
		if arena.Shots != want {
			t.Fatalf("第 %d 次扣扳機之後剩 %d 發，應該是 %d", pull+1, arena.Shots, want)
		}
	}
}

// The hit scan walks a line and stops at the first body it crosses. A thug
// standing between the player and the target eats the shot.
func TestAShotStopsAtTheFirstBodyOnTheLine(t *testing.T) {
	state := arenaUnderway(t, 2)
	arena := &state.Arena
	arena.PlayerX, arena.PlayerY = arenaFieldLeft+10, arenaCentreY
	arena.Thugs[0].X, arena.Thugs[0].Y = arenaFieldLeft+40, arenaCentreY
	arena.Thugs[1].X, arena.Thugs[1].Y = arenaFieldLeft+80, arenaCentreY
	before := arena.Thugs[1].HitPoints
	arenaFire(&state, 0, arena.Thugs[1].X, arena.Thugs[1].Y)
	if arena.Shot.Hit != 1 {
		t.Fatalf("射線打到 %d，應該是擋在中間的第一個對手", arena.Shot.Hit)
	}
	if arena.Thugs[1].HitPoints != before {
		t.Fatalf("後面那個也被扣血了：%d → %d", before, arena.Thugs[1].HitPoints)
	}
}

// A shot that crosses nobody runs to the field edge and comes back with no
// victim.
func TestAMissRunsOffTheField(t *testing.T) {
	state := arenaUnderway(t, 1)
	arena := &state.Arena
	arena.PlayerX, arena.PlayerY = arenaCentreX, arenaCentreY
	arena.Thugs[0].X, arena.Thugs[0].Y = arenaFieldLeft, arenaFieldTop
	arenaFire(&state, 0, arenaFieldRight, arenaCentreY)
	if arena.Shot.Hit != -1 {
		t.Fatalf("空槍打中了 %d", arena.Shot.Hit)
	}
	if arenaInsideField(arena.Shot.ToX, arena.Shot.ToY) {
		t.Fatalf("射線停在場內 (%d,%d)", arena.Shot.ToX, arena.Shot.ToY)
	}
}

// A downed thug stays on the floor until its own counter runs out, and only
// then is the sprite taken off — which is what makes the bodies linger.
func TestADownedThugLingersUntilItsCounterRunsOut(t *testing.T) {
	state := arenaUnderway(t, 1)
	arena := &state.Arena
	arena.Thugs[0].Down = true
	arena.Thugs[0].HitPoints = 10
	arena.Thugs[0].Cooldown = 3
	for pass := 0; pass < 3; pass++ {
		arenaHandleThugs(&state)
		if arena.Thugs[0].Gone && pass < 2 {
			t.Fatalf("第 %d 圈就撤走了", pass+1)
		}
	}
	if !arena.Thugs[0].Gone {
		t.Fatalf("倒數走完還在場上：%+v", arena.Thugs[0])
	}
}

// Taking a thug's hit points to exactly zero removes it there and then; leaving
// it between one and thirty-two only puts it down.
func TestZeroHitPointsRemovesAThugOutright(t *testing.T) {
	state := arenaUnderway(t, 1)
	arena := &state.Arena
	arena.Thugs[0].HitPoints = 1
	arenaHitThug(&state, 0, arenaScattergunDamage)
	if !arena.Thugs[0].Down || !arena.Thugs[0].Gone {
		t.Fatalf("打到歸零沒有立刻撤走：%+v", arena.Thugs[0])
	}
	state = arenaUnderway(t, 1)
	arena = &state.Arena
	arena.Thugs[0].HitPoints = arenaDownThreshold + 1
	arenaHitThug(&state, 0, arenaThugDamage)
	if !arena.Thugs[0].Down {
		t.Fatalf("打到門檻以下沒有倒地：%+v", arena.Thugs[0])
	}
	if arena.Thugs[0].HitPoints > 0 && arena.Thugs[0].Gone {
		t.Fatalf("血量還有 %d 就被撤走了", arena.Thugs[0].HitPoints)
	}
}

// The fight is fast and it is meant to be: three opponents at thirty passes a
// second put a bare-handed character on the floor inside two seconds. What
// makes it survivable is that walking out is a fight-ending move and the player
// moves faster than the crowd — so heading for the edge on the first pass gets
// out with most of the hit points intact.
func TestRunningForTheEdgeGetsOutAlive(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for opponents := 1; opponents <= arenaMaxRecords-1; opponents++ {
		state := arenaUnderway(t, opponents)
		state.Screen = ScreenArena
		opening := state.Arena.HitPoints
		passes := 0
		for state.Arena.Active && passes < 600 {
			state, err = reducer.StepArena(state, ArenaInput{
				PointerX: arenaWindowX0 - 60, PointerY: arenaCentreY, Moving: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			passes++
		}
		if !state.Arena.Escaped {
			t.Fatalf("%d 個對手：往外跑了 %d 圈還沒出去（%+v）", opponents, passes, state.Arena)
		}
		if passes > 30 {
			t.Fatalf("%d 個對手：跑出場地花了 %d 圈，一秒都不到的距離不該這麼久", opponents, passes)
		}
		if state.Arena.HitPoints <= arenaDownThreshold {
			t.Fatalf("%d 個對手：跑出去時已經倒了（%d ← %d）", opponents, state.Arena.HitPoints, opening)
		}
	}
}

// Standing still is the other half of that: a bare-handed character who does
// nothing is on the floor in well under a hundred passes. This is not a
// difficulty knob, it is what the original's numbers add up to — three
// opponents firing on a 1/150 roll that climbs every pass, at 27 to 36 a hit,
// against health x ST / 90.
func TestStandingStillLosesTheFight(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := arenaUnderway(t, 3)
	state.Screen = ScreenArena
	passes := 0
	for state.Arena.HitPoints > arenaDownThreshold && passes < 600 {
		state, err = reducer.StepArena(state, ArenaInput{
			PointerX: state.Arena.PlayerX, PointerY: state.Arena.PlayerY,
		})
		if err != nil {
			t.Fatal(err)
		}
		passes++
	}
	if passes >= 600 {
		t.Fatal("站著不動六百圈還沒倒，開火閘門沒有在跑")
	}
	t.Logf("站著不動 %d 圈就倒了（每圈一幀，約 %.1f 秒）", passes, float64(passes)/60)
}
