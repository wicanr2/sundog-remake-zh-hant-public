package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// 受控更新序列由 Talos 原版指令比較；不把本機輸出當成自身期望值。
func TestExportTalosWalkerSequences(t *testing.T) {
	output, diskPath := os.Getenv("SUNDOG_PARITY_OUTPUT"), os.Getenv("SUNDOG_PARITY_DISK")
	if output == "" && diskPath == "" {
		t.Skip("請由 tools/verify_talos_walker_slices.py 指定輸入")
	}
	if output == "" || diskPath == "" {
		t.Fatal("輸入與輸出必須同時指定")
	}
	disk, err := originalmedia.Open(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	patterns, err := disk.CollidePatterns()
	if err != nil {
		t.Fatal(err)
	}
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	type frame struct {
		Held   bool
		Anchor game.Position
	}
	type sequence struct {
		Current, Target game.Position
		Frames          []frame
	}
	report := struct {
		Schema     int
		Randomness string
		Sequences  []sequence
	}{Schema: 1, Randomness: "明示 typed 狀態、固定空白地表；GroundWorld 避開城市遭遇；移動碰撞不消耗 RNG"}
	for _, dx := range []int{-100, -3, -2, -1, 0, 1, 2, 3, 100} {
		for _, dy := range []int{-100, -3, -2, -1, 0, 1, 2, 3, 100} {
			state := game.State{Schema: game.CurrentSchema, Screen: game.ScreenGround, CityID: world.StartDestinationID, GroundWorld: true}
			scene := &stGroundScene{cityID: state.CityID, world: true, indices: make([]byte, 320*200)}
			a := &app{state: state, reducer: r, stGroundScene: scene, collideRead: true, collideOK: true, collidePatternData: patterns}
			a.groundWalker = stGroundWalker{ready: true, cityID: state.CityID, world: true, x: 146, y: 96}
			s := sequence{Current: game.Position{X: 150, Y: 100}, Target: game.Position{X: 150 + dx, Y: 100 + dy}}
			for _, held := range []bool{true, true, true, false, false, true, true, true, false, false} {
				if held {
					a.stepSTGroundWalker(s.Target.X, s.Target.Y)
				}
				x, y := a.groundWalker.anchor()
				s.Frames = append(s.Frames, frame{held, game.Position{X: x, Y: y}})
			}
			report.Sequences = append(report.Sequences, s)
		}
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "walker-sequences.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("匯出 %d 組固定錨點／目標，各十次受控更新與放開／再按住", len(report.Sequences))
}
