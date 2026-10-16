package game

import "testing"

// The four readings and their decay are the original's (ground rules
// condition.decay, condition.decay-tables, condition.vigor-collapse). These pin
// the parts that would silently drift: the band width, each table, the sleep
// exception, and the one reading whose floor has a consequence.

func TestANewGameFillsAllFourReadings(t *testing.T) {
	state := NewState()
	for name, got := range map[string]int{
		"vigor": state.Vigor, "rest": state.Rest,
		"health": state.Health, "nourishment": state.Nourishment,
	} {
		if got != MaxCondition {
			t.Fatalf("%s = %d, want the original's full byte %d", name, got, MaxCondition)
		}
	}
}

func TestDecayReadsTheTableBandByBand(t *testing.T) {
	// Nourish's table is 1123: the fuller it is, the faster it goes.
	for _, c := range []struct{ value, want int }{
		{0xff, 0xff - 3}, // top band
		{0xc0, 0xc0 - 3},
		{0xbf, 0xbf - 2}, // third band
		{0x80, 0x80 - 2},
		{0x7f, 0x7f - 1}, // second band
		{0x40, 0x40 - 1},
		{0x3f, 0x3f - 1}, // bottom band
		{0, 0},
	} {
		if got := decayReading(c.value, nourishedDecay); got != c.want {
			t.Fatalf("nourish decay from %d = %d, want %d", c.value, got, c.want)
		}
	}
}

func TestSleepingHoldsRestStill(t *testing.T) {
	awake := State{Vigor: MaxCondition, Rest: 200, Health: 100, Nourishment: 200}
	asleep := awake
	advanceConditions(&awake, false)
	advanceConditions(&asleep, true)
	if awake.Rest >= 200 {
		t.Fatalf("rest did not decay while awake: %d", awake.Rest)
	}
	if asleep.Rest != 200 {
		t.Fatalf("rest moved while asleep: %d", asleep.Rest)
	}
}

func TestHealthClimbsBackAboveTheHalfwayMark(t *testing.T) {
	// (0xff - 0x80) / 0x20 = 3, times four = 12 a tick at full vigour.
	high := State{Health: 0x90, Vigor: MaxCondition, Rest: MaxCondition, Nourishment: MaxCondition}
	advanceConditions(&high, false)
	if high.Health != 0x90+12 {
		t.Fatalf("health above 0x80 at full vigour = %d, want %d", high.Health, 0x90+12)
	}
	low := State{Health: 0x30, Vigor: MaxCondition, Rest: MaxCondition, Nourishment: MaxCondition}
	advanceConditions(&low, false)
	if low.Health != 0x2f {
		t.Fatalf("health in the bottom band = %d, want it to fall to 0x2f", low.Health)
	}
}

// A tired character does not mend. At half vigour the recovery step is zero, so
// the reading falls through to its decay table -- and that table is 1000, which
// is zero in this band. The result is a wound that simply stops closing.
func TestExhaustionStopsHealthClimbing(t *testing.T) {
	tired := State{Health: 0x90, Vigor: 0x80, Rest: MaxCondition, Nourishment: MaxCondition}
	advanceConditions(&tired, false)
	if tired.Health != 0x90 {
		t.Fatalf("health at half vigour = %d, want it to stall at 0x90", tired.Health)
	}
	// The bottom band is the one place health falls on its own.
	hurt := State{Health: 0x20, Vigor: MaxCondition, Rest: MaxCondition, Nourishment: MaxCondition}
	advanceConditions(&hurt, false)
	if hurt.Health != 0x1f {
		t.Fatalf("health in the bottom band = %d, want 0x1f", hurt.Health)
	}
}

func TestOnlyVigorReportsACollapse(t *testing.T) {
	almost := State{Vigor: 13, Rest: 1, Health: 1, Nourishment: 1}
	if advanceConditions(&almost, false) {
		t.Fatal("a collapse was reported before vigor reached the floor")
	}
	// Each depleted reading contributes -4, leaving 13-12 = 1 Vigor.
	if almost.Vigor != 1 {
		t.Fatalf("vigor = %d, want 1 after the original's low-reading penalty", almost.Vigor)
	}
	// Collapse follows Vigor reaching zero, not the other readings alone.
	empty := State{Vigor: 1, Rest: 0, Health: 0, Nourishment: 0}
	if !advanceConditions(&empty, false) {
		t.Fatal("vigor hit the floor without reporting the collapse")
	}
}

func TestAdvanceTimeUpdatesConditionsAtEachCrossedHour(t *testing.T) {
	// MAINLIB:35 loops at 0bff after each hourly update. These reading vectors
	// are the original's first two full-condition hourly outputs.
	for _, c := range []struct {
		start, minutes int
		want           [4]int
	}{
		{0, 1, [4]int{255, 255, 255, 255}},
		{0, 59, [4]int{255, 255, 255, 255}},
		{59, 1, [4]int{253, 251, 255, 252}},
		{0, 60, [4]int{253, 251, 255, 252}},
		{0, 61, [4]int{253, 251, 255, 252}},
		{0, 120, [4]int{251, 247, 255, 249}},
	} {
		state := State{TimeMinutes: c.start, Vigor: 255, Rest: 255, Health: 255, Nourishment: 255}
		advanceTime(&state, c.minutes)
		got := [4]int{state.Vigor, state.Rest, state.Health, state.Nourishment}
		if got != c.want || state.TimeMinutes != c.start+c.minutes {
			t.Fatalf("start=%d minutes=%d: readings=%v time=%d, want %v time=%d",
				c.start, c.minutes, got, state.TimeMinutes, c.want, c.start+c.minutes)
		}
	}
	// An empty Nourish reading does not add per-action Health damage.
	hungry := State{Health: 1, Vigor: 13}
	advanceTime(&hungry, 59)
	if hungry.Health != 1 || hungry.Vigor != 13 {
		t.Fatalf("condition changed before the hour: health=%d vigor=%d", hungry.Health, hungry.Vigor)
	}
	advanceTime(&hungry, 1)
	if hungry.Health != 0 || hungry.Vigor != 1 {
		t.Fatalf("hourly depleted readings: health=%d vigor=%d, want 0/1", hungry.Health, hungry.Vigor)
	}
}

// TestFullReadingsKeepVigorOffTheFloor pins the consequence of the decoded
// arithmetic: with the other three full, Vigor climbs instead of collapsing.
// The table-based approximation this replaced would have driven it to zero.
func TestFullReadingsKeepVigorOffTheFloor(t *testing.T) {
	rested := State{Vigor: 1, Rest: MaxCondition, Health: MaxCondition, Nourishment: MaxCondition}
	if advanceConditions(&rested, false) {
		t.Fatal("a rested, healthy, fed character collapsed")
	}
	if rested.Vigor <= 1 {
		t.Fatalf("vigor = %d, want it climbing towards the average", rested.Vigor)
	}
}

// TestAdvanceVigorChasesTheAverage 釘住原版的 Vigor 算式。
//
// 原版不查表：Vigor 朝其他三項的平均值移動，一次兩點。數值取自在原版
// MAINLIB:0x35 的 0x0c7d／0x0c8a 下中斷點讀到的五組樣本。
func TestAdvanceVigorChasesTheAverage(t *testing.T) {
	cases := []struct {
		name                             string
		vigor, rest, health, nourishment int
		want                             int
	}{
		// 實測第一組：三項和 758，平均 252，Vigor 255 高於它。
		{"高於平均就往下兩點", 255, 251, 255, 252, 253},
		// 實測第二組：三項和 751，平均 250。
		{"持續下降", 253, 247, 255, 249, 251},
		// 實測第三組：三項和 744，平均 248。
		{"再一次", 251, 245, 255, 244, 249},
		{"低於平均就往上兩點", 100, 200, 200, 200, 102},
		{"正好等於平均就不動", 200, 200, 200, 200, 200},
		{"上限夾在 255", 255, 255, 255, 255, 255},
		{"下限夾在 0", 1, 0, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := advanceVigor(tc.vigor, tc.rest, tc.health, tc.nourishment); got != tc.want {
				t.Fatalf("advanceVigor(%d, %d, %d, %d) = %d, want %d",
					tc.vigor, tc.rest, tc.health, tc.nourishment, got, tc.want)
			}
		})
	}
}

// TestAdvanceVigorIsNotATable 確認新算式與舊的近似（沿用 Rest 的表）行為不同。
//
// 這一條存在的理由是防止回歸：如果有人把它改回查表，曲線形狀會變，而單看某一個
// 時間點的數值不一定看得出來。
func TestAdvanceVigorIsNotATable(t *testing.T) {
	// 其他三項掉得快時，原版的 Vigor 會跟著加速下降；查表版只看自己的值。
	const vigor = 200
	fast := advanceVigor(vigor, 10, 10, 10)    // 平均 10，遠低於 Vigor
	slow := advanceVigor(vigor, 255, 255, 255) // 平均 255，高於 Vigor
	if fast >= vigor {
		t.Fatalf("三項偏低時 Vigor 應下降，得到 %d", fast)
	}
	if slow <= vigor {
		t.Fatalf("三項偏高時 Vigor 應上升，得到 %d", slow)
	}
	if table := decayReading(vigor, restDecay); fast == table && slow == table {
		t.Fatal("兩種方向都等於查表結果——算式退化成表了")
	}
}
