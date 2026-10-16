package main

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// The city's flag string is XMOVEONG:0x23 init_city's own
// (passability.city-street): a '1' blocks that colour. Colours 0, 6, 7, 8 and
// 10 are the street; everything else is a wall.
func TestCityStreetColoursComeFromTheFlagString(t *testing.T) {
	want := map[int]bool{0: true, 6: true, 7: true, 8: true, 10: true}
	for colour, passable := range stGroundPassable {
		if passable != want[colour] {
			t.Fatalf("色 %d 可站＝%v，應該是 %v（旗標字串 %q）", colour, passable, want[colour], stGroundPassableFlags)
		}
	}
}

// update_position's margin chain, in the units it compares in: the sprite's
// coordinates divided by eight. x is tested before y, so one call scrolls one
// axis at most (scroll.margin-chain).
func TestTheMarginChainScrollsOneAxisXFirst(t *testing.T) {
	cases := []struct {
		name           string
		x, y           int
		wantDX, wantDY int
	}{
		{"中央不捲", 160, 100, 0, 0},
		{"左緣往左", 39, 100, -1, 0},
		{"左緣的界線是 40", 40, 100, 0, 0},
		{"右緣往右", 280, 100, 1, 0},
		{"右緣的界線是 279", 279, 100, 0, 0},
		{"上緣往上", 160, 39, 0, -1},
		{"下緣往下", 160, 160, 0, 1},
		{"下緣的界線是 159", 160, 159, 0, 0},
		{"x 先於 y", 39, 160, -1, 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			app := &app{}
			app.groundWalker = stGroundWalker{ready: true, x: test.x, y: test.y, scrollX: 5, scrollY: 3}
			app.scrollSTGround()
			gotDX := app.groundWalker.scrollX - 5
			gotDY := app.groundWalker.scrollY - 3
			if gotDX != test.wantDX || gotDY != test.wantDY {
				t.Fatalf("捲動 (%d,%d)，應該是 (%d,%d)", gotDX, gotDY, test.wantDX, test.wantDY)
			}
			// scroll.sprite-steps-back: the walker moves back one cell on
			// screen, which is what keeps it inside the margin.
			if gotDX != 0 && app.groundWalker.x != test.x-gotDX*stGroundPitch {
				t.Fatalf("走路人 x = %d，應該退到 %d", app.groundWalker.x, test.x-gotDX*stGroundPitch)
			}
			if gotDY != 0 && app.groundWalker.y != test.y-gotDY*stGroundPitch {
				t.Fatalf("走路人 y = %d，應該退到 %d", app.groundWalker.y, test.y-gotDY*stGroundPitch)
			}
		})
	}
}

// 城市外圍仍可捲動，超過原版城市範圍才切換地表。
func TestTheWindowContinuesIntoCityOutskirts(t *testing.T) {
	application := &app{state: game.NewState()}
	application.groundWalker = stGroundWalker{ready: true, x: 39, y: 100, scrollX: -4, scrollY: 3}
	application.scrollSTGround()
	if application.groundWalker.scrollX != -5 || application.state.GroundWorld {
		t.Fatal("城市外圍沒有繼續捲動")
	}
}

// check_exit's chain: the cell is the window origin plus the sprite's screen
// position divided by the 40-pixel pitch.
func TestTheCellUnderTheWalkerIsTheOriginalsChain(t *testing.T) {
	walker := stGroundWalker{scrollX: 7, scrollY: 4, x: 175, y: 120}
	column, row := walker.cell()
	if column != 7+175/40 || row != 4+120/40 {
		t.Fatalf("格座標 (%d,%d)，應該是 (%d,%d)", column, row, 7+175/40, 4+120/40)
	}
}

// The collision anchor is the sprite box's corner plus the four collide takes
// off both axes, so the sample box lands exactly on the sprite box.
func TestTheCollisionAnchorSitsOnTheSpriteBox(t *testing.T) {
	walker := stGroundWalker{x: 100, y: 50}
	x, y := walker.anchor()
	if x != 100+originalmedia.CollideAnchorOffset || y != 50+originalmedia.CollideAnchorOffset {
		t.Fatalf("錨點 (%d,%d)", x, y)
	}
}

// A wall stops the walker dead on that axis; the other axis still moves, which
// is what lets it slide along a wall it is walking into at an angle.
func TestAWallStopsTheWalkerOnThatAxisOnly(t *testing.T) {
	scene := &stGroundScene{indices: make([]byte, logicalWidth*logicalHeight)}
	// Colour 0 everywhere is street; a band of colour 1 down the middle is a
	// wall the flag string blocks.
	for y := 0; y < logicalHeight; y++ {
		for x := 120; x < 130; x++ {
			scene.indices[y*logicalWidth+x] = 1
		}
	}
	patterns := [originalmedia.CollidePatternCount][]originalmedia.CollidePoint{}
	patterns[originalmedia.CollideWalkerPattern] = []originalmedia.CollidePoint{{X: 0, Y: 0}, {X: 6, Y: 6}}
	if !scene.canStand(patterns, originalmedia.CollideWalkerPattern, 50+originalmedia.CollideAnchorOffset, 50+originalmedia.CollideAnchorOffset) {
		t.Fatal("空地上站不住")
	}
	if scene.canStand(patterns, originalmedia.CollideWalkerPattern, 122+originalmedia.CollideAnchorOffset, 50+originalmedia.CollideAnchorOffset) {
		t.Fatal("牆上站得住")
	}
	// Outside the screen counts as blocked: the margin chain scrolls before
	// the walker can ever get there.
	if scene.canStand(patterns, originalmedia.CollideWalkerPattern, -10, 50) {
		t.Fatal("畫面外站得住")
	}
}

// The four walker patterns are picked by the way the step goes, and each names
// one of collide's patterns 6–9.
func TestTheWalkerFacesTheWayItSteps(t *testing.T) {
	cases := []struct {
		dx, dy, want int
	}{{0, -1, 0}, {0, 1, 1}, {-1, 0, 2}, {1, 0, 3}, {-1, -1, 2}}
	for _, test := range cases {
		if got := stGroundFacing(test.dx, test.dy); got != test.want {
			t.Fatalf("(%d,%d) 面向 %d，應該是 %d", test.dx, test.dy, got, test.want)
		}
	}
	if originalmedia.CollideWalkerPattern+3 >= originalmedia.CollidePatternCount {
		t.Fatal("四個方向的取樣圖樣超出表的範圍")
	}
}
