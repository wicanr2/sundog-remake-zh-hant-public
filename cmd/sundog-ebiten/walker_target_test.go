package main

import (
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"testing"
)

// 依 Talos 執行原版 68000 步長切片的結果，測正式移動路徑接近游標時的收斂。
func TestSTGroundWalkerSettlesAtOddPixelTargets(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, delta := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {3, 3}, {-3, -3}} {
		state := game.NewState()
		state.Screen = game.ScreenGround
		scene := &stGroundScene{cityID: state.CityID, shipCity: state.ShipCityID, shipPosition: state.ShipPosition, indices: make([]byte, 320*200)}
		a := &app{state: state, reducer: r, stGroundScene: scene, collideRead: true, collideOK: true}
		a.groundWalker = stGroundWalker{ready: true, cityID: state.CityID, x: 100, y: 100}
		targetX, targetY := 100+originalmedia.CollideAnchorOffset+delta[0], 100+originalmedia.CollideAnchorOffset+delta[1]
		for frame := 0; frame < 8; frame++ {
			a.stepSTGroundWalker(targetX, targetY)
			if frame >= 1 && (a.groundWalker.x != 100+delta[0] || a.groundWalker.y != 100+delta[1]) {
				t.Fatalf("目標差 %v，更新 %d：位置 (%d,%d) 未收斂或越過目標", delta, frame, a.groundWalker.x, a.groundWalker.y)
			}
		}
	}
}

func TestSTGroundWalkerUsesNativePatternForCollision(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range [][3]int{{0, -2, 0}, {0, 2, 0}, {-2, 0, 2}, {2, 0, 2}, {-2, -2, 3}, {2, 2, 3}, {-2, 2, 1}, {2, -2, 1}} {
		state := game.NewState()
		state.Screen = game.ScreenGround
		scene := &stGroundScene{cityID: state.CityID, shipCity: state.ShipCityID, shipPosition: state.ShipPosition, indices: make([]byte, 320*200)}
		for i := range scene.indices {
			scene.indices[i] = 1
		}
		a := &app{state: state, reducer: r, stGroundScene: scene, collideRead: true, collideOK: true}
		a.collidePatternData[stGroundFootPattern] = []originalmedia.CollidePoint{{X: 0, Y: 0}}
		// 只有地面圖樣 4 可通過；船艙家族各有額外受阻取樣，避免重新誤用。
		for shape := 0; shape < 4; shape++ {
			a.collidePatternData[originalmedia.CollideWalkerPattern+shape] = []originalmedia.CollidePoint{{X: 0, Y: 10}}
		}
		scene.indices[(100+c[1])*320+100+c[0]] = 0
		a.groundWalker = stGroundWalker{ready: true, cityID: state.CityID, x: 100, y: 100}
		a.stepSTGroundWalker(100+originalmedia.CollideAnchorOffset+c[0], 100+originalmedia.CollideAnchorOffset+c[1])
		if a.groundWalker.x != 100+c[0] || a.groundWalker.y != 100+c[1] || a.groundWalker.pattern != c[2] {
			t.Fatalf("方向 %v：位置 (%d,%d)、圖樣 %d", c, a.groundWalker.x, a.groundWalker.y, a.groundWalker.pattern)
		}
	}
}

func TestSTGroundWalkerChoosesOriginalCollisionCandidate(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		open    [][2]int
		want    [2]int
		pattern int
	}{
		{"斜向可走，水平中間點受阻", [][2]int{{2, 2}}, [2]int{2, 2}, 3},
		{"斜向受阻，優先垂直", [][2]int{{0, 2}, {2, 0}}, [2]int{0, 2}, 0},
		{"只剩水平整步", [][2]int{{2, 0}}, [2]int{2, 0}, 2},
		{"整步全阻，斜向短步", [][2]int{{1, 1}}, [2]int{1, 1}, 3},
		{"整步全阻，垂直短步優先", [][2]int{{0, 1}, {1, 0}}, [2]int{0, 1}, 0},
		{"只剩水平短步", [][2]int{{1, 0}}, [2]int{1, 0}, 2},
		{"全部受阻，圖樣不變", nil, [2]int{0, 0}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := game.NewState()
			state.Screen = game.ScreenGround
			scene := &stGroundScene{cityID: state.CityID, shipCity: state.ShipCityID, shipPosition: state.ShipPosition, indices: make([]byte, 320*200)}
			for i := range scene.indices {
				scene.indices[i] = 1
			}
			for _, p := range c.open {
				scene.indices[(100+p[1])*320+100+p[0]] = 0
			}
			a := &app{state: state, reducer: r, stGroundScene: scene, collideRead: true, collideOK: true}
			a.collidePatternData[stGroundFootPattern] = []originalmedia.CollidePoint{{X: 0, Y: 0}}
			a.groundWalker = stGroundWalker{ready: true, cityID: state.CityID, x: 100, y: 100, pattern: 1}
			a.stepSTGroundWalker(100+originalmedia.CollideAnchorOffset+2, 100+originalmedia.CollideAnchorOffset+2)
			if a.groundWalker.x != 100+c.want[0] || a.groundWalker.y != 100+c.want[1] || a.groundWalker.pattern != c.pattern {
				t.Fatalf("位置 (%d,%d) 圖樣 %d，預期差 %v 圖樣 %d", a.groundWalker.x, a.groundWalker.y, a.groundWalker.pattern, c.want, c.pattern)
			}
		})
	}
}

// GEMBIND原生方向家族與成功才寫回契約見ship-rules.md的地面入口勘誤。
func TestSTGroundPodUsesEachCandidateShape(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name    string
		open    [][2]int
		want    [2]int
		pattern int
	}{
		{"斜向圖樣3通過", [][2]int{{162, 102}}, [2]int{102, 102}, 3},
		{"斜向受阻改垂直圖樣0", [][2]int{{100, 102}}, [2]int{100, 102}, 0},
		{"全部受阻保留水平圖樣2", nil, [2]int{100, 100}, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			state := game.NewState()
			state.Screen, state.GroundMode = game.ScreenGround, game.GroundInPod
			scene := &stGroundScene{cityID: state.CityID, shipCity: state.ShipCityID, shipPosition: state.ShipPosition, indices: make([]byte, 320*200)}
			for i := range scene.indices {
				scene.indices[i] = 1
			}
			for _, p := range c.open {
				scene.indices[p[1]*320+p[0]] = 0
			}
			a := &app{state: state, reducer: r, stGroundScene: scene, collideRead: true, collideOK: true}
			// 每個圖樣取不同點，誤用先前圖樣或步行圖樣便會受阻。
			for pattern := 0; pattern <= stGroundFootPattern; pattern++ {
				a.collidePatternData[pattern] = []originalmedia.CollidePoint{{X: pattern * 20, Y: 0}}
			}
			a.groundWalker = stGroundWalker{ready: true, cityID: state.CityID, x: 100, y: 100, pattern: 2}
			a.stepSTGroundWalker(106, 106)
			if a.groundWalker.x != c.want[0] || a.groundWalker.y != c.want[1] || a.groundWalker.pattern != c.pattern {
				t.Fatalf("位置(%d,%d)圖樣%d，預期%v圖樣%d", a.groundWalker.x, a.groundWalker.y, a.groundWalker.pattern, c.want, c.pattern)
			}
		})
	}
}

func TestSTGroundModeUpdatesCollisionWithoutRedrawing(t *testing.T) {
	for _, world := range []bool{false, true} {
		state := game.NewState()
		state.Screen, state.GroundWorld = game.ScreenGround, world
		scene := &stGroundScene{world: world, cityID: state.CityID, colonySite: state.ColonySite, shipCity: state.ShipCityID, shipPosition: state.ShipPosition, indices: make([]byte, 320*200), rebuilds: 7}
		for i := range scene.indices {
			scene.indices[i] = 8
		}
		a := &app{state: state, stGroundScene: scene}
		var patterns [originalmedia.CollidePatternCount][]originalmedia.CollidePoint
		patterns[2] = []originalmedia.CollidePoint{{X: 0, Y: 0}}
		for _, mode := range []game.GroundMode{game.GroundOnFoot, game.GroundInPod, game.GroundOnFoot} {
			a.state.GroundMode = mode
			got := a.stGroundSceneFor(state.CityID, 0, 0)
			if got != scene || got.rebuilds != 7 || got.indices[100*320+100] != 8 {
				t.Fatal("mode change redrew scene or changed pixels")
			}
			if got.canStand(patterns, 2, 104, 104) != (mode == game.GroundOnFoot) {
				t.Fatalf("world%v mode%s did not update collision", world, mode)
			}
		}
	}
}
