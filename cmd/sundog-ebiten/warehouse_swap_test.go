package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"golang.org/x/image/font"
)

func TestWarehouseSwapKeyboardAndMouseUseSameSlots(t *testing.T) {
	a := &app{state: game.NewState()}
	a.state.Screen = game.ScreenTrade
	w := a.stTradeWindow()
	for _, tc := range []struct {
		page, style int
		actions     []game.Action
		keys        []ebiten.Key
	}{
		{game.TradePageWarehouse, 13, []game.Action{game.ActionTradeStorePage, game.ActionTradeFetchPage, game.ActionTradeSwapPage}, []ebiten.Key{ebiten.KeyS, ebiten.KeyF, ebiten.KeyX}},
		{game.TradePageSwapPod, 12, []game.Action{game.ActionTradePick1, game.ActionTradePick2}, []ebiten.Key{ebiten.Key1, ebiten.Key2}},
		{game.TradePageSwapWarehouse, 0, []game.Action{game.ActionTradePick4, game.ActionTradePick3, game.ActionTradePick2, game.ActionTradePick1}, []ebiten.Key{ebiten.Key4, ebiten.Key3, ebiten.Key2, ebiten.Key1}},
	} {
		a.state.TradePage = tc.page
		for i, b := range stMenuBoxes(tc.style) {
			x0, y0, x1, y1 := w.menuBoxRect(stMenuBaseInside, b)
			mouse, ok := a.stTradeActionAt((x0+x1)/2, (y0+y1)/2)
			keyboard, keyOK := keyAction(tc.keys[i], a.state)
			if !ok || !keyOK || mouse != tc.actions[i] || keyboard != mouse {
				t.Fatalf("page%d box%d mouse%s key%s", tc.page, i, mouse, keyboard)
			}
		}
	}
}

func TestWarehouseSwapFiveLanguageTextFits(t *testing.T) {
	a := hdProductionTestApp(t)
	w := a.stTradeWindow()
	box := stMenuBoxes(13)[2]
	x0, _, x1, _ := w.menuBoxRect(stMenuBaseInside, box)
	for _, locale := range []localization.Locale{localization.TraditionalChinese, localization.SimplifiedChinese, localization.Japanese, localization.Korean, localization.English} {
		a.applyLocale(locale)
		for _, scale := range []int{1, 4} {
			face := a.newFace(8 * scale)
			for _, key := range []string{"ui.trade.swap", "ui.trade.swap_pod", "ui.trade.swap_warehouse"} {
				value := a.lookup(key)
				limit := w.x1 - w.x0 - stTextInkX - 4
				if key == "ui.trade.swap" {
					limit = x1 - x0 - 2
				}
				if strings.HasPrefix(value, "ui.") || font.MeasureString(face, value).Ceil() > limit*scale {
					t.Fatalf("%s/%s scale%d exceeds %d", locale, key, scale, limit)
				}
			}
		}
	}
}
