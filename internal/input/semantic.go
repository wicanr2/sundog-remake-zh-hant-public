package input

import "github.com/wicanr2/sundog-remake/internal/game"

// RightClickAction keeps the original BACK UP semantic at the presentation boundary.
// The manual's rule is「若沒有可返回的選項，右鍵會顯示角色狀態與持有物品」
// (docs/manual/atari-st-manual-en.md), so the two root screens the player stands
// on -- the shipboard and the city ground -- answer with the character card.
// Backing out of the ground would board the ship, which in the original is done
// by walking onto the landing square, not by the BACK UP button.
func RightClickAction(screen game.Screen) game.Action {
	switch screen {
	case game.ScreenShipboard, game.ScreenGround:
		return game.ActionOpenStatus
	}
	return game.ActionBack
}
