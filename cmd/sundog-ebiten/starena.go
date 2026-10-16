package main

// 競技場在 Atari ST 呈現下的樣子。
//
// 原版的地面戰鬥（`XDOCOMBA`）是即時的：`make_zoom` 樣式 11 開一個視窗，玩家與
// 對手都是 sprite，左鍵按住就朝滑鼠走、右鍵開火，而且要放開再按才有下一發。
// 規則整套在 `internal/game/arena_realtime.go`，這一支只負責畫和收輸入。
//
// 什麼是重製版自己的：**人長什麼樣**。原版的對手用 `create_sprite` 從
// `GEMBIND_G12c` 的形狀表取圖（玩家取第 1 張、對手取第 14 張），那張表沒有解出來，
// 所以這裡沿用城市地面那組 7×7 的走路人圖樣。還有讀數那一行——原版的競技場視窗
// 裡沒有數字，血量與彈藥都只從畫面上的人看得出來。

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/wicanr2/sundog-remake/internal/game"
)

const (
	// stArenaEdge is the yellow ring make_zoom draws round every style-11
	// window.
	stArenaEdge = stColourYellow
	// stArenaThugInk and stArenaPlayerInk keep the two sides apart. The
	// original tells them apart by the shape, not the colour.
	stArenaPlayerInk = stColourWhite
	stArenaThugInk   = stColourRed
	stArenaDownInk   = 8
	// stArenaReadoutY is where the one line of numbers sits inside the field,
	// and stArenaReadoutWidth is how much of the floor it may use.
	stArenaReadoutY     = 2
	stArenaReadoutWidth = 154
)

// usesSTArena reports whether the pointer drives the fight. Unlike the ground
// walk it does not need the player's disk: the arena floor is a plain rectangle
// and the figures fall back to blocks when the original sprites are not there.
func (a *app) usesSTArena() bool {
	return a.usesSTLayout()
}

// updateSTArena is one pass of xdo_combat's loop. handle_player runs every
// frame; the crowd is on its own timer inside StepArena.
func (a *app) updateSTArena() {
	if !a.usesSTArena() || a.state.Screen != game.ScreenArena || !a.state.Arena.Active {
		return
	}
	displayX, displayY := ebiten.CursorPosition()
	pointerX, pointerY := a.toLayout(displayX, displayY)
	input := game.ArenaInput{PointerX: pointerX, PointerY: pointerY}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		input.Moving = true
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		input.Firing = true
	}
	before := a.state
	next, err := a.reducer.StepArena(a.state, input)
	if err != nil {
		return
	}
	a.state = next
	a.sounds.play(soundCue(before, a.state))
	a.sounds.play(soundFollowUp(before, a.state))
	if a.state.MessageKey != before.MessageKey || a.state.Screen != before.Screen {
		a.stMessageFrame = a.frames
		a.stSceneMessageFrame = a.frames
	}
}

// drawSTArena paints the arena window: the edge, the floor, everyone on it and
// whatever was fired this pass.
func (a *app) drawSTArena(screen *ebiten.Image) {
	x0, y0, x1, y1 := game.ArenaField()
	black := color.RGBA{0, 0, 0, 255}
	// The edge is one ring outside the content area.
	uiFillRect(screen, float32(x0-1), float32(y0-1), float32(x1-x0+3), float32(y1-y0+3), stColour(stArenaEdge), false)
	uiFillRect(screen, float32(x0), float32(y0), float32(x1-x0+1), float32(y1-y0+1), black, false)

	arena := a.state.Arena
	if arena.Shot.Live {
		a.drawSTArenaShot(screen, arena.Shot)
	}
	for _, thug := range arena.Thugs {
		if thug.Gone {
			continue
		}
		ink := stArenaThugInk
		if thug.Down {
			ink = stArenaDownInk
		}
		a.drawSTArenaFigure(screen, thug.X, thug.Y, ink)
	}
	if arena.HitPoints > 0 {
		a.drawSTArenaFigure(screen, arena.PlayerX, arena.PlayerY, stArenaPlayerInk)
	}
	a.drawSTArenaReadout(screen, x0, y0)
}

// drawSTArenaFigure draws one fighter centred on its sprite position.
func (a *app) drawSTArenaFigure(screen *ebiten.Image, x, y, ink int) {
	half := game.ArenaBodySize / 2
	if a.ensureGroundSprites() {
		a.drawSTSpriteAt(screen, stPersonPatterns, x-half, y-half, 1, stColour(ink))
		return
	}
	uiFillRect(screen, float32(x-half), float32(y-half),
		float32(game.ArenaBodySize), float32(game.ArenaBodySize), stColour(ink), false)
}

// drawSTArenaShot draws the ray. The original traces it a point at a time as
// the hit scan walks it, in colour 12 for the scattergun and 13 for everything
// else; this draws the whole line at once, which is what a frame of that looks
// like.
func (a *app) drawSTArenaShot(screen *ebiten.Image, shot game.ArenaShot) {
	uiStrokeLine(screen, float32(shot.FromX), float32(shot.FromY),
		float32(shot.ToX), float32(shot.ToY), 1, stColour(shot.Colour), false)
}

// drawSTArenaReadout is the remake's own line of numbers along the top of the
// floor: what the player is holding, what is left of the shield, and how many
// opponents are still up.
func (a *app) drawSTArenaReadout(screen *ebiten.Image, x0, y0 int) {
	arena := a.state.Arena
	weapon := a.lookup("ui.arena.barehanded")
	if name, ok := game.ItemName(arena.Gun); ok {
		weapon = a.lookup(name)
	}
	line := fmt.Sprintf("%s  %s:%d  %s:%d  %s:%d", weapon,
		a.lookup("ui.arena.hitpoints"), arena.HitPoints,
		a.lookup("ui.arena.shield"), arena.ShieldCharges,
		a.lookup("ui.arena.opponents"), game.ArenaStanding(arena))
	a.drawTextWithin(screen, line, x0+2, y0+stArenaReadoutY, stArenaReadoutWidth, stColour(stColourWhite))
}
