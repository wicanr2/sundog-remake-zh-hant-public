package input

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

// 船艙與地面都是玩家站著的根畫面，沒有可返回的上一層，手冊要求右鍵改開角色狀態。
func TestRightClickActionOpensCharacterStatusOnRootScreens(t *testing.T) {
	for _, screen := range []game.Screen{
		game.ScreenShipboard,
		game.ScreenGround,
	} {
		if got := RightClickAction(screen); got != game.ActionOpenStatus {
			t.Fatalf("%s right click = %q, want %q", screen, got, game.ActionOpenStatus)
		}
	}
}

func TestRightClickActionReturnsFromChildScreens(t *testing.T) {
	for _, screen := range []game.Screen{
		game.ScreenCharacterStatus,
		game.ScreenTrade,
		game.ScreenEngineering,
		game.ScreenEquipment,
	} {
		if got := RightClickAction(screen); got != game.ActionBack {
			t.Fatalf("%s right click = %q, want %q", screen, got, game.ActionBack)
		}
	}
}
