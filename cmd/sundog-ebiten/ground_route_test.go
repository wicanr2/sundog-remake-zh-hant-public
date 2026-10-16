package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"testing"
)

func TestKeyboardBlockedAtCityMarkerDoesNotReenter(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                      string
		allBlocked                bool
		wallOffset, moved, cellDY int
	}{
		{"fully-blocked", true, 0, 0, 0},
		{"blocked-before-boundary", false, 6, 2, 0},
		{"one-pixel-crosses-boundary", false, 7, 3, -1},
	} {
		state := game.NewState()
		state.Screen = game.ScreenGround
		state.GroundWorld = true
		city, ok := r.World.Destination(state.CityID)
		if !ok {
			t.Fatal("缺少起始城市")
		}
		state.Position = game.Position{X: city.MapColumn, Y: city.MapRow}
		game.ResetGroundView(&state, game.Position{X: 11, Y: 2})
		view := state.GroundView
		scene := &stGroundScene{world: true, cityID: state.CityID, scrollX: view.Origin.X, scrollY: view.Origin.Y,
			colonySite: state.ColonySite,
			shipCity:   state.ShipCityID, shipPosition: state.ShipPosition, blocked: 2, indices: make([]byte, 320*200)}
		for i := range scene.indices {
			// 碰撞取樣減去四像素錨點偏移。牆設於 anchor-6 時，
			// 允許兩像素整步但擋住跨界短步；anchor-7 則容許再走一像素。
			if tc.allBlocked || i/320 < view.Anchor.Y-tc.wallOffset {
				scene.indices[i] = 1
			}
		}
		a := &app{state: state, reducer: r, originalDisk: &originalmedia.Disk{},
			stGroundPattern: ebiten.NewImage(40, 40), stGroundPatternIndices: make([]byte, 40*40),
			stGroundScene: scene, collideRead: true, collideOK: true}
		a.collidePatternData[stGroundFootPattern] = []originalmedia.CollidePoint{{X: 0, Y: 0}}
		a.resetSTGroundWalker()
		a.nudgeSTGroundWalker(game.ActionMoveNorth)
		want := game.Position{X: state.Position.X, Y: state.Position.Y + tc.cellDY}
		if !a.state.GroundWorld || a.state.Position != want {
			t.Fatalf("%s：world=%v position=%+v want=%+v anchor=%+v start=%+v", tc.name, a.state.GroundWorld, a.state.Position, want, a.state.GroundView.Anchor, view.Anchor)
		}
		if a.state.GroundView.Anchor.Y != view.Anchor.Y-tc.moved {
			t.Fatalf("%s：位移=%d want=%d", tc.name, view.Anchor.Y-a.state.GroundView.Anchor.Y, tc.moved)
		}
	}
}
