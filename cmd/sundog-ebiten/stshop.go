package main

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// --- The original's menu boxes and text grid inside a zoom window ---
//
// Every shop window's buttons come from one table: WINDOWLI:0x13 new_menu(style)
// copies the boxes of one style out of MAINLIB_G9e9 (MAINLIB_G9d5[style] is
// the first box and the count, packed low/high byte) and WINDOWLI:0x19
// put_box draws each as a 2-row frame around an inner rectangle, the label
// centred on it. The rectangles are relative to the window's inside corner
// (MAINLIB_G773 window 3: the zoom edge plus one). The table is data in RAM,
// read from six memory snapshots and identical in all of them; the depot's
// three bands (round thirteen: y0+56, +69, +82) are style 6's boxes 1–3
// under the content window, which pins the decoding.
//
// Text inside the fill goes through MAINLIB's v* routines on window 5, the
// fill itself: a cell is 7 wide and 8 high, column 0 starts three in from
// the fill's left edge and row 0 ten down from its top; the ink of a row
// sits seven above that baseline (the depot's answer at (77, 41)). In a
// string, '|' is a space that does not break and '\' is a new line.

// stMenuBox is one box of the table, relative to the window's inside corner.
type stMenuBox struct{ x0, x1, y0, y1 int }

// stMenuStyles is MAINLIB_G9e9 by style.
var stMenuStyles = map[int][]stMenuBox{
	0:  {{2, 156, 86, 96}, {2, 156, 73, 83}, {2, 156, 60, 70}, {2, 156, 47, 57}},
	1:  {{2, 94, 2, 12}, {2, 94, 15, 25}, {2, 94, 28, 38}, {2, 94, 41, 51}, {2, 94, 54, 64}, {2, 94, 67, 77}},
	2:  {{8, 74, 81, 91}, {84, 150, 81, 91}},
	3:  {{46, 112, 81, 91}},
	4:  {{3, 76, 2, 12}, {3, 76, 15, 25}, {3, 76, 28, 38}, {3, 76, 41, 51}, {3, 76, 54, 64}, {3, 76, 67, 77}, {3, 76, 80, 90}, {3, 76, 93, 103}},
	5:  {{3, 94, 71, 81}, {3, 94, 59, 69}, {3, 94, 47, 57}},
	6:  {{2, 153, 71, 81}, {2, 153, 58, 68}, {2, 153, 45, 55}, {2, 153, 32, 42}},
	7:  {{2, 51, 71, 81}, {53, 102, 71, 81}, {104, 153, 71, 81}, {2, 51, 58, 68}, {53, 102, 58, 68}, {104, 153, 58, 68}},
	8:  {{2, 77, 71, 81}, {79, 153, 71, 81}, {2, 153, 58, 68}},
	9:  {{2, 77, 71, 81}, {79, 153, 71, 81}, {2, 77, 58, 68}, {79, 153, 58, 68}, {2, 153, 45, 55}},
	10: {{2, 77, 71, 81}, {79, 153, 71, 81}, {2, 77, 58, 68}, {79, 153, 58, 68}, {2, 77, 45, 55}, {79, 153, 45, 55}, {2, 153, 32, 42}},
	11: {{3, 78, 18, 28}, {80, 154, 18, 28}},
	12: {{3, 154, 50, 60}, {3, 154, 63, 73}},
	13: {{3, 52, 14, 24}, {54, 103, 14, 24}, {105, 154, 14, 24}},
	14: {{3, 78, 4, 14}, {80, 154, 4, 14}},
	15: {{3, 154, 18, 28}},
	16: {{87, 125, 53, 65}},
}

const (
	stBoxFrame     = 2 // put_box: the frame is two outside the inner rectangle
	stBoxTextY     = 2 // the label's ink starts two rows into the inner rectangle
	stTextCellW    = 7
	stTextCellH    = 8
	stTextInkX     = 5  // fill edge + 3, from the zoom edge (+2 for the fill)
	stTextInkY     = 17 // fill top + 10 − 7, from the zoom edge (+14 for the fill)
	stMarqueeInkY  = 10 // update_quote: baseline two above the inside's bottom, ink seven above that
	stMarqueeInkX  = 3  // inside corner + 2
	stMarqueeChars = 22
	// stMarqueeFrames is how often the marquee steps one cell; the
	// original's set_timeout(6) is not measured in seconds.
	stMarqueeFrames = 20
)

// stMenuBoxes is the boxes of one style, first to last, or nil.
func stMenuBoxes(style int) []stMenuBox { return stMenuStyles[style] }

// stMenuBase is which window put_menu measures the boxes from: the
// exchange passes window 3, the zoom's inside (edge plus one); the depot
// passes the fill window (edge plus two across, plus fourteen down).
type stMenuBase int

const (
	stMenuBaseInside stMenuBase = iota
	stMenuBaseFill
)

func (b stMenuBase) origin(w stZoomWindow) (int, int) {
	if b == stMenuBaseFill {
		return w.x0 + stZoomInner, w.y0 + stZoomContentY
	}
	return w.x0 + 1, w.y0 + 1
}

// menuBoxRect is a box's inner rectangle on the screen.
func (w stZoomWindow) menuBoxRect(base stMenuBase, box stMenuBox) (x0, y0, x1, y1 int) {
	bx, by := base.origin(w)
	return bx + box.x0, by + box.y0, bx + box.x1, by + box.y1
}

// menuBoxHit reports which box of the style the point is on.
func (w stZoomWindow) menuBoxHit(base stMenuBase, style, x, y int) (int, bool) {
	for i, box := range stMenuBoxes(style) {
		x0, y0, x1, y1 := w.menuBoxRect(base, box)
		if x >= x0-stBoxFrame && x <= x1+stBoxFrame && y >= y0-stBoxFrame && y <= y1+stBoxFrame {
			return i, true
		}
	}
	return 0, false
}

// textCell is where the ink of a character cell of the fill's text grid
// starts.
func (w stZoomWindow) textCell(col, row int) (int, int) {
	return w.x0 + stTextInkX + col*stTextCellW, w.y0 + stTextInkY + row*stTextCellH
}

// drawSTMenuBox draws one box the way put_box does: the frame two rows out
// in the band colour, the inside black, the label centred.
func (a *app) drawSTMenuBox(screen *ebiten.Image, window stZoomWindow, base stMenuBase, box stMenuBox, label string) {
	x0, y0, x1, y1 := window.menuBoxRect(base, box)
	black := color.RGBA{0, 0, 0, 255}
	uiFillRect(screen, float32(x0-stBoxFrame), float32(y0-stBoxFrame), float32(x1-x0+1+2*stBoxFrame), float32(y1-y0+1+2*stBoxFrame), stColour(stZoomBandBlue), false)
	uiFillRect(screen, float32(x0), float32(y0), float32(x1-x0+1), float32(y1-y0+1), black, false)
	a.drawTextCentered(screen, label, x0, x1+1, y0+stBoxTextY, stColour(stColourWhite))
}

// drawSTMenuRow draws boxes first..first+n-1 of a style with the labels, as
// XDOTRADI:0x0b do_row does.
func (a *app) drawSTMenuRow(screen *ebiten.Image, window stZoomWindow, base stMenuBase, style, first int, labels ...string) {
	boxes := stMenuBoxes(style)
	for i, label := range labels {
		if first+i >= len(boxes) {
			return
		}
		a.drawSTMenuBox(screen, window, base, boxes[first+i], label)
	}
}

// The buy page keeps its text on window 3, the zoom's inside corner, not on
// the fill: do_buying blanks the whole inside — title bar included — and its
// row 0 lands where the title would have been. Measured on exch/shots/buy.png:
// the ink of column 0 is four in from the edge, row 0 four down, cells are the
// same 7x8, and each value is right-aligned to end on column 20.
const (
	stTradeInsideInkX   = 4
	stTradeInsideInkY   = 4
	stTradeInsideEndCol = 21 // 值的右緣：這一欄的起點
)

// insideTextCell is textCell measured from the inside corner instead of the fill.
func (w stZoomWindow) insideTextCell(col, row int) (int, int) {
	return w.x0 + stTradeInsideInkX + col*stTextCellW, w.y0 + stTradeInsideInkY + row*stTextCellH
}

// drawSTInsideRowCentred centres a row on the window's inside.
func (a *app) drawSTInsideRowCentred(screen *ebiten.Image, window stZoomWindow, row int, value string, c color.RGBA) {
	_, y := window.insideTextCell(0, row)
	a.drawTextCentered(screen, value, window.x0+1, window.x1, y, c)
}

// drawSTInsideLabelledRow puts a label at column 0 of the inside grid and its
// value right-aligned so it ends on the last column.
func (a *app) drawSTInsideLabelledRow(screen *ebiten.Image, window stZoomWindow, row int, label, value string, c color.RGBA) {
	x, y := window.insideTextCell(0, row)
	a.drawTextWithin(screen, label, x, y, window.x1-1-x, c)
	right, _ := window.insideTextCell(stTradeInsideEndCol, row)
	if left := right - a.textWidth(value); left > x+a.textWidth(label) {
		x = left
	} else {
		x += a.textWidth(label)
	}
	a.drawTextWithin(screen, value, x, y, window.x1-1-x, c)
}

// drawSTTextRow draws one row of the fill's text grid from a column.
func (a *app) drawSTTextRow(screen *ebiten.Image, window stZoomWindow, col, row int, value string, c color.RGBA) {
	x, y := window.textCell(col, row)
	a.drawTextWithin(screen, value, x, y, window.x1-stZoomInner-x, c)
}

// drawSTTextRowCentred is vcenter: the row centred on the fill.
func (a *app) drawSTTextRowCentred(screen *ebiten.Image, window stZoomWindow, row int, value string, c color.RGBA) {
	_, y := window.textCell(0, row)
	a.drawTextCentered(screen, value, window.x0+stZoomInner, window.x1-stZoomInner+1, y, c)
}

// --- The exchange: XDOTRADI:0x1e do_exchange's three layouts ---
//
// The window (make_zoom style 11) opens with the city's name and " Exch." on
// the title bar, Buy and Sell as style 11's two boxes across the top,
// "Lots now selling:" on row 5 two columns in (xgotoxy(0, 8) and the string's
// leading new line), and the marquee of quotes along the bottom row:
// update_quote draws 22 cells from the inside corner plus two, two above the
// bottom, and scrolls the ring of quotes one cell at a time. Buy puts up
// do_buying's page: "PRICE ON GOODS" centred, the goods centred, then
// Quality, Asking and Your limit, with Buy and Pass as style 2's boxes.
// Sell lists the pod's lots as style 12's two boxes under "Pod contents:"
// (pick_item), and picking one sells it.
//
// The marquee uses the typed eight-quote ring; the source picker addresses
// four warehouse slots and two stable pod holds.

// stTradeWindow is the exchange's window: style 11 never grows.
func (a *app) stTradeWindow() stZoomWindow {
	return stZoomFor(game.ScreenTrade, 0, 0, 0)
}

// stTradeTitle is light_box's title: the city and " Exch.".
func (a *app) stTradeTitle() string {
	city := a.state.Location
	if parts := strings.Split(city, "／"); len(parts) > 0 {
		city = parts[len(parts)-1]
	}
	return fmt.Sprintf(a.lookup("ui.trade.title"), city)
}

// stTradeMarquee is the 22-cell window onto the ring at this frame: the
// eight quotes' texts (add_quote's name-price cells) end to end.
func (a *app) stTradeMarquee() string {
	ring := []rune{}
	for _, quote := range a.state.Quotes {
		ring = append(ring, []rune(a.reducer.MarqueeText(quote))...)
	}
	if len(ring) == 0 {
		ring = []rune(a.lookup("ui.market_unknown") + "   ")
	}
	offset := (a.frames / stMarqueeFrames) % len(ring)
	out := make([]rune, 0, stMarqueeChars)
	for i := 0; i < stMarqueeChars; i++ {
		out = append(out, ring[(offset+i)%len(ring)])
	}
	return string(out)
}

// stLotName is a lot as show_warehouse prints it: the name, then " / " and
// the quality letter.
func (a *app) stLotName(lot game.Lot) string {
	if lot.Goods == 0 {
		return a.lookup("ui.trade.nothing")
	}
	name := ""
	if kind, ok := a.reducer.World.CargoKindByCode(lot.Goods); ok {
		name = a.reducer.CargoNameForKey(kind.Key)
	}
	return fmt.Sprintf("%s / %s", name, game.QualityLetter(lot.Quality))
}

// stTradeLots is the pod's two holds for pick_item, in slot order. The pod is
// two records now (game/hold.go), so box 1 is hold 1 and box 2 is hold 2 —
// they no longer shuffle up as things are taken out.
func (a *app) stTradeLots() []string {
	lots := make([]string, 0, game.CargoLots)
	for i := 0; i < game.CargoLots; i++ {
		lots = append(lots, a.stHoldName(game.HoldAt(a.state, i)))
	}
	return lots
}

// stHoldName is what one hold reads as in a list.
func (a *app) stHoldName(hold game.Hold) string {
	switch hold.Kind {
	case game.HoldGoods:
		return a.stLotName(hold.Lot())
	case game.HoldAuxFuel:
		return a.lookup("ui.trade.aux_tank")
	case game.HoldSalvage:
		return a.lookup("ui.trade.salvage")
	case game.HoldSupplies:
		return a.lookup("ui.supplies")
	case game.HoldCryogens:
		return a.lookup("ui.cryogens")
	}
	return a.lookup("ui.trade.nothing")
}

// drawSTTrade draws the exchange window for the page that is up.
func (a *app) drawSTTrade(screen *ebiten.Image) {
	window := a.stTradeWindow()
	style := stZoomStyleExchange
	title := a.stTradeTitle()
	if a.state.TradePage == game.TradePageSellSource || a.state.TradePage == game.TradePageSellConfirm || a.state.TradePage == game.TradePageBuy || a.state.TradePage == game.TradePageBuyDestination {
		// The clear rectangle cannot erase text already queued on textLayer.
		title = ""
	}
	content := a.drawSTZoomFrameStyle(screen, window, title, style)
	black := color.RGBA{0, 0, 0, 255}
	white := stColour(stColourWhite)
	// The exchange's own text colour: white on the black fill (door2.png).
	pen := stColour(style.pen)
	uiFillRect(screen, float32(content.x), float32(content.y), float32(content.w), float32(content.h), stColour(style.fill), false)
	if a.stZoomMessageFresh() && a.state.TradePage == game.TradePageMain {
		innerX := float32(window.x0 + stZoomInner)
		innerW := float32(stZoomW - 2*stZoomInner)
		uiFillRect(screen, innerX, float32(content.y), innerW, float32(window.y1-content.y), black, false)
		a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), window.x0+stZoomMessageX, window.y0+stZoomMessageY,
			window.x1-stZoomInner-(window.x0+stZoomMessageX), stZoomMessageLine, white)
		return
	}
	switch a.state.TradePage {
	case game.TradePageSellConfirm:
		uiFillRect(screen, float32(window.x0+1), float32(window.y0+1), float32(stZoomW-2), float32(window.y1-window.y0-1), black, false)
		lot, _ := game.PendingTradeLot(a.state)
		name := a.stLotName(lot)
		heading := "ui.trade.price_on_goods"
		if a.state.CityID == world.MissionDestinationID {
			heading = "ui.trade.to_be_delivered"
		}
		a.drawSTInsideRowCentred(screen, window, 0, a.lookup(heading), pen)
		a.drawSTInsideRowCentred(screen, window, 1, name, pen)
		if a.state.CityID == world.MissionDestinationID {
			a.drawSTInsideLabelledRow(screen, window, 4, a.lookup("ui.trade.demand"), fmt.Sprint(a.state.Colony.Remaining[lot.Goods]), pen)
		} else {
			a.drawSTInsideLabelledRow(screen, window, 4, a.lookup("ui.trade.offer"), game.FormatCredits(game.Credits(a.state.TradeSalePrice)), pen)
		}
		if !game.TradeSaleAvailable(a.state) {
			a.drawSTMenuRow(screen, window, stMenuBaseInside, 0, 1, a.lookup("ui.trade.exit"))
		} else {
			first, second := "ui.trade.accept", "ui.trade.withdraw"
			if a.state.CityID == world.MissionDestinationID {
				first, second = "ui.trade.deliver", "ui.trade.withhold"
			}
			a.drawSTMenuRow(screen, window, stMenuBaseInside, 2, 0, a.lookup(first), a.lookup(second))
		}
	case game.TradePageBuy:
		quote, ok := game.CurrentQuote(a.state)
		name := a.lookup("ui.trade.nothing")
		if ok {
			if kind, found := a.reducer.World.CargoKindByCode(quote.Goods); found {
				name = a.reducer.CargoNameForKey(kind.Key)
			}
		}
		// do_buying blanks the whole inside, the title bar with it, and writes
		// on rows 0, 1, 3, 4 and 5 of the inside grid — row 2 stays empty.
		uiFillRect(screen, float32(window.x0+1), float32(window.y0+1), float32(stZoomW-2), float32(window.y1-window.y0-1), black, false)
		a.drawSTInsideRowCentred(screen, window, 0, a.lookup("ui.trade.price_on_goods"), pen)
		a.drawSTInsideRowCentred(screen, window, 1, name, pen)
		a.drawSTInsideLabelledRow(screen, window, 3, a.lookup("ui.trade.quality"), game.QualityLetter(quote.Quality), pen)
		a.drawSTInsideLabelledRow(screen, window, 4, a.lookup("ui.trade.asking"), game.FormatCredits(game.Credits(quote.Price)), pen)
		a.drawSTInsideLabelledRow(screen, window, 5, a.lookup("ui.trade.your_limit"), game.FormatCredits(game.TradeFunds(a.state)), pen)
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 2, 0, a.lookup("ui.trade.buy"), a.lookup("ui.trade.pass"))
	case game.TradePageSellSource, game.TradePageBuyDestination:
		// Menu14 occupies the former title band, as the original picker does.
		uiFillRect(screen, float32(window.x0+1), float32(window.y0+1), float32(stZoomW-2), float32(window.y1-window.y0-1), black, false)
		heading := "ui.trade.sell_from"
		if a.state.TradePage == game.TradePageBuyDestination {
			heading = "ui.trade.store_to"
		}
		a.drawSTTextRow(screen, window, 0, 4, a.lookup(heading), pen)
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 14, 0, a.lookup("ui.trade.warehouse"), a.lookup("ui.trade.pod"))
	case game.TradePageSell, game.TradePageStore:
		heading := "ui.trade.pod_contents"
		if a.state.TradePage == game.TradePageStore {
			heading = "ui.trade.store_which"
		}
		a.drawSTTextRow(screen, window, 0, 0, a.lookup(heading), pen)
		lots := a.stTradeLots()
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 12, 0, lots[0], lots[1])
	case game.TradePageFetch, game.TradePageSellWarehouse:
		heading := "ui.trade.fetch_which"
		if a.state.TradePage == game.TradePageSellWarehouse {
			heading = "ui.trade.contents"
		}
		a.drawSTTextRow(screen, window, 0, 0, a.lookup(heading), pen)
		names := []string{}
		for _, lot := range game.WarehouseLotsOf(a.state, a.state.CityID) {
			names = append(names, a.stLotName(lot))
		}
		// pick_item fills menu 0 from the last slot up: slot 1 is the top box.
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 0, 0, names[3], names[2], names[1], names[0])
	case game.TradePageWarehouse:
		// do_warehouse: "Contents:" then the four slots, one a row, and
		// Store / Fetch / Swap across the top (menu 13; Swap is Store then
		// Fetch here).
		a.drawSTTextRow(screen, window, 0, 4, a.lookup("ui.trade.contents"), pen)
		for i, lot := range game.WarehouseLotsOf(a.state, a.state.CityID) {
			a.drawSTTextRow(screen, window, 2, 5+i, a.stLotName(lot), pen)
		}
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 13, 0, a.lookup("ui.trade.store"), a.lookup("ui.trade.fetch"))
	case game.TradePageEntrance:
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 11, 0, a.lookup("ui.trade.exchange"), a.lookup("ui.trade.warehouse"))
		a.drawSTTextRow(screen, window, 2, 5, a.lookup("ui.trade.lots_now_selling"), pen)
		a.drawTextWithin(screen, a.stTradeMarquee(), window.x0+stMarqueeInkX, window.y1-stMarqueeInkY, stMarqueeChars*stTextCellW, pen)
	default:
		a.drawSTMenuRow(screen, window, stMenuBaseInside, 11, 0, a.lookup("ui.trade.buy"), a.lookup("ui.trade.sell"))
		a.drawSTTextRow(screen, window, 2, 5, a.lookup("ui.trade.lots_now_selling"), pen)
		a.drawTextWithin(screen, a.stTradeMarquee(), window.x0+stMarqueeInkX, window.y1-stMarqueeInkY, stMarqueeChars*stTextCellW, pen)
	}
}

// stTradeActionAt maps a click in the exchange window to its action.
func (a *app) stTradeActionAt(x, y int) (game.Action, bool) {
	window := a.stTradeWindow()
	picks := []game.Action{game.ActionTradePick1, game.ActionTradePick2, game.ActionTradePick3, game.ActionTradePick4}
	switch a.state.TradePage {
	case game.TradePageSellConfirm:
		style := 2
		if !game.TradeSaleAvailable(a.state) {
			style = 0
		}
		if i, ok := window.menuBoxHit(stMenuBaseInside, style, x, y); ok {
			if style == 0 {
				if i == 1 {
					return game.ActionTradePick2, true
				}
				return "", false
			}
			return picks[i], true
		}
	case game.TradePageBuy:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 2, x, y); ok {
			if i == 0 {
				return game.ActionBuyGems, true
			}
			return game.ActionTradePass, true
		}
	case game.TradePageSellSource, game.TradePageBuyDestination:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 14, x, y); ok {
			return picks[i], true
		}
	case game.TradePageSell, game.TradePageStore:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 12, x, y); ok {
			return picks[i], true
		}
	case game.TradePageFetch, game.TradePageSellWarehouse:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 0, x, y); ok {
			return picks[3-i], true
		}
	case game.TradePageWarehouse:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 13, x, y); ok {
			if i == 0 {
				return game.ActionTradeStorePage, true
			}
			if i == 1 {
				return game.ActionTradeFetchPage, true
			}
		}
	case game.TradePageEntrance:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 11, x, y); ok {
			if i == 0 {
				return game.ActionTradeExchange, true
			}
			return game.ActionTradeWarehousePage, true
		}
	default:
		if i, ok := window.menuBoxHit(stMenuBaseInside, 11, x, y); ok {
			if i == 0 {
				return game.ActionTradeBuyPage, true
			}
			return game.ActionTradeSellPage, true
		}
	}
	return "", false
}

// --- The Uniteller: XDOUNITE:0x0b do_teller ---
//
// The title is the city and " Bank". The fill's top eleven rows go black
// with "UNITELLER >>>>>>>>>>" centred on row 0 in colour 13; row 2, column 1
// is "On hand:" with the cash and row 3 "In bank:" (or "Deficit:") with the
// balance. Menu 6 measured from the fill: box 1 DEPOSIT CASH (when there is
// cash), box 2 WITHDRAW CASH and box 3 TRANSFER (when there is a balance),
// box 4 PAY FINE instead of those two when a fine is owed.
//
// Only three of the four boxes are ever filled at once, and that is what
// keeps the header readable: box 4's frame starts at fill row 30, five rows
// into the "In bank:" ink. Anything the remake puts there covers the balance,
// which is why the tube ride is not there — it has the station's own window
// (drawSTTubeStation).
//
// move_cash keeps the header and shows "AVAILABLE:" on row 3 and "AMOUNT:"
// on row 4, column 1, with menu 7's six keys: ALL, 100, 1000 across the
// bottom row and 10000, 50000, CLEAR above; the right button ends it.

const (
	stTellerHeaderRows = 11
	// do_teller writes the header with vst_color(13), and 13 there is a GEM
	// VDI colour index, not a hardware pen: the VDI maps it through
	// {0,15,1,2,4,6,3,5,7,8,9,10,12,14,11,13}, so it comes out as hardware
	// entry 14 — the yellow the capture shows, not magenta.
	stTellerHeaderPen = 14
)

// stTellerBox is one of do_teller's boxes: its label and what it does.
type stTellerBox struct {
	label  string
	action game.Action
}

func (a *app) stTellerWindow() stZoomWindow { return stZoomFor(game.ScreenUniteller, 0, 0, 0) }

func (a *app) stTellerTitle() string {
	city := a.state.Location
	if parts := strings.Split(city, "／"); len(parts) > 0 {
		city = parts[len(parts)-1]
	}
	return fmt.Sprintf(a.lookup("ui.teller.title"), city)
}

// stTellerBoxes is which menu-6 boxes do_teller shows, by box index.
func (a *app) stTellerBoxes() map[int]stTellerBox {
	boxes := map[int]stTellerBox{}
	if a.state.Cash > 0 {
		boxes[0] = stTellerBox{a.lookup("ui.teller.deposit"), game.ActionTellerDepositPage}
	}
	if a.reducer.ParkingFine(a.state) > 0 {
		boxes[3] = stTellerBox{a.lookup("ui.teller.pay_fine"), game.ActionPayFine}
		return boxes
	}
	if a.state.Bank > 0 {
		boxes[1] = stTellerBox{a.lookup("ui.teller.withdraw"), game.ActionTellerWithdrawPage}
		boxes[2] = stTellerBox{a.lookup("ui.teller.transfer"), game.ActionTellerTransferPage}
	}
	return boxes
}

var stTellerKeypad = []struct {
	key    string
	action game.Action
}{
	{"ui.teller.all", game.ActionTellerAmountAll},
	{"100", game.ActionTellerAmount100},
	{"1000", game.ActionTellerAmount1000},
	{"10000", game.ActionTellerAmount10000},
	{"50000", game.ActionTellerAmount50000},
	{"ui.teller.clear", game.ActionTellerAmountClear},
}

func (a *app) stTellerKeyLabel(key string) string {
	if strings.HasPrefix(key, "ui.") {
		return a.lookup(key)
	}
	return key
}

func (a *app) drawSTTeller(screen *ebiten.Image) {
	window := a.stTellerWindow()
	style := stZoomStyleBank
	content := a.drawSTZoomFrameStyle(screen, window, a.stTellerTitle(), style)
	black := color.RGBA{0, 0, 0, 255}
	white := stColour(stColourWhite)
	// The bank writes on black throughout, title bar included, and every
	// word on it is white (teller/shots/building.png).
	pen := stColour(style.pen)
	uiFillRect(screen, float32(content.x), float32(content.y), float32(content.w), float32(content.h), stColour(style.fill), false)
	if a.stZoomMessageFresh() && a.state.TellerPage == game.TellerPageMain {
		innerX := float32(window.x0 + stZoomInner)
		innerW := float32(stZoomW - 2*stZoomInner)
		uiFillRect(screen, innerX, float32(content.y), innerW, float32(window.y1-content.y), black, false)
		a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), window.x0+stZoomMessageX, window.y0+stZoomMessageY,
			window.x1-stZoomInner-(window.x0+stZoomMessageX), stZoomMessageLine, white)
		return
	}
	if a.state.TellerPage == game.TellerPageTransferPick {
		// pick_location: the boxes of the menu style its count picks
		// (6 / 8 / 9 / 10 for 1 / 2–3 / 4–5 / 6), OTHERS in the wide box
		// above them. color_win fills the window first, so no header.
		picks, others := a.reducer.TellerPickList(a.state)
		boxes := stMenuBoxes(stTellerPickStyle(len(picks)))
		for i, pick := range picks {
			a.drawSTMenuBox(screen, window, stMenuBaseFill, boxes[i], pick.Name)
		}
		if others {
			a.drawSTMenuBox(screen, window, stMenuBaseFill, boxes[len(boxes)-1], a.lookup("ui.teller.others"))
		}
		return
	}
	uiFillRect(screen, float32(content.x), float32(content.y), float32(content.w), stTellerHeaderRows+1, black, false)
	header := a.lookup("ui.teller.header")
	if a.state.TellerPage != game.TellerPageMain {
		header = a.lookup("ui.teller.commit_hint")
	}
	a.drawSTTextRowCentred(screen, window, 0, header, stColour(stTellerHeaderPen))
	if a.state.TellerPage == game.TellerPageMain {
		readoutRow := 2
		if a.usesHD() {
			// Modern glyphs extend below the original seven-row ink. Keep
			// both balances above menu 6's earliest button frame, including
			// the pay-fine branch, without moving its input rectangles.
			readoutRow = 1
		}
		a.drawSTTextRow(screen, window, 1, readoutRow, fmt.Sprintf("%s%s", a.lookup("ui.teller.on_hand"), game.FormatCredits(a.state.Cash)), pen)
		a.drawSTTextRow(screen, window, 1, readoutRow+1, fmt.Sprintf("%s%s", a.lookup("ui.teller.in_bank"), game.FormatCredits(a.state.Bank)), pen)
		boxes := stMenuBoxes(6)
		for i, box := range a.stTellerBoxes() {
			a.drawSTMenuBox(screen, window, stMenuBaseFill, boxes[i], box.label)
		}
		return
	}
	if a.state.TellerPage == game.TellerPageTransfer {
		a.drawSTTextRow(screen, window, 1, 2, fmt.Sprintf(a.lookup("ui.teller.transfer_to"), a.stTellerTransferName()), pen)
	}
	a.drawSTTextRow(screen, window, 1, 3, fmt.Sprintf("%s%s", a.lookup("ui.teller.available"), game.FormatCredits(game.TellerAvailable(a.state))), pen)
	a.drawSTTextRow(screen, window, 1, 4, fmt.Sprintf("%s%s", a.lookup("ui.teller.amount"), game.FormatCredits(a.state.TellerAmount)), pen)
	boxes := stMenuBoxes(7)
	for i, key := range stTellerKeypad {
		a.drawSTMenuBox(screen, window, stMenuBaseFill, boxes[i], a.stTellerKeyLabel(key.key))
	}
}

func (a *app) stTellerActionAt(x, y int) (game.Action, bool) {
	window := a.stTellerWindow()
	if a.state.TellerPage == game.TellerPageMain {
		i, ok := window.menuBoxHit(stMenuBaseFill, 6, x, y)
		if !ok {
			return "", false
		}
		box, ok := a.stTellerBoxes()[i]
		if !ok {
			return "", false
		}
		return box.action, true
	}
	if a.state.TellerPage == game.TellerPageTransferPick {
		picks, others := a.reducer.TellerPickList(a.state)
		style := stTellerPickStyle(len(picks))
		i, ok := window.menuBoxHit(stMenuBaseFill, style, x, y)
		if !ok {
			return "", false
		}
		if i < len(picks) {
			return stTellerPickActions[i], true
		}
		if others && i == len(stMenuBoxes(style))-1 {
			return game.ActionTellerOthers, true
		}
		return "", false
	}
	if i, ok := window.menuBoxHit(stMenuBaseFill, 7, x, y); ok {
		return stTellerKeypad[i].action, true
	}
	return "", false
}

// --- The tube station: XDOUNITE:0x09 do_tube_station ---
//
// Measured off a run of the port with the walker put on a tube station door
// (tools/port_oracle_building_probe.sh with SUNDOG_BUILDING_FACILITY=tube-station):
// the window's edge is at (88, 72), the title bar reads "Tube Station" in
// black on white — not the blue the depot and the bank write theirs in — and
// the fill is palette entry 1, blue, where those two are green. "Select" and
// "destination city" are centred on fill rows 0 and 1, and the destinations
// sit in pick_location's boxes: four of them at style 9's narrow boxes, the
// upper row at fill row 58 and the lower at 71. A box carries the city name
// and nothing else; the fare is not on it.
//
// The remake's window opens where its own zoom opens; the original anchors it
// on the player's cell, which the remake does not copy for any of these
// windows. The fares go on the left box's rows, which is the remake's own
// panel, so the window itself stays as the original draws it.

var stTubePickActions = []game.Action{game.ActionTubePick1, game.ActionTubePick2, game.ActionTubePick3, game.ActionTubePick4, game.ActionTubePick5, game.ActionTubePick6}

func (a *app) stTubeWindow() stZoomWindow { return stZoomFor(game.ScreenTubeStation, 0, 0, 0) }

func (a *app) drawSTTubeStation(screen *ebiten.Image) {
	window := a.stTubeWindow()
	style := stZoomStyleTube
	content := a.drawSTZoomFrameStyle(screen, window, a.lookup("ui.tube.title"), style)
	uiFillRect(screen, float32(content.x), float32(content.y), float32(content.w), float32(content.h), stColour(style.fill), false)
	white := stColour(style.pen)
	if a.state.TubePage == game.TubePageConfirm {
		a.drawSTTubeConfirm(screen, window, white)
		return
	}
	if a.stZoomMessageFresh() {
		a.drawWrappedTextWithin(screen, a.lookup(a.state.MessageKey), window.x0+stZoomMessageX, window.y0+stZoomMessageY,
			window.x1-stZoomInner-(window.x0+stZoomMessageX), stZoomMessageLine, white)
		return
	}
	a.drawSTTextRowCentred(screen, window, 0, a.lookup("ui.tube.select_1"), white)
	a.drawSTTextRowCentred(screen, window, 1, a.lookup("ui.tube.select_2"), white)
	list := a.reducer.TubeDestinations(a.state)
	boxes := stMenuBoxes(stTellerPickStyle(len(list)))
	for i, destination := range list {
		if i >= len(boxes) {
			break
		}
		a.drawSTMenuBox(screen, window, stMenuBaseFill, boxes[i], destination.Name)
	}
}

// The confirmation page's own geometry, measured on the capture: `Destination`
// centred on row 0; `City:`／`Planet:`／`System:` in column 2 on rows 2–4 with
// their values right-aligned to the end of column 19; `Cost:` a row lower than
// the run — row 6, not 5 — and a column further left, its value ending one
// column further right. The gap and the two columns are the original's: the
// three names go through one helper (XDOUNITE proc 0x0a) and the cost line is
// assembled separately at (column 1, row 6).
const (
	stTubeLabelColumn = 2
	stTubeValueEnd    = 20 // 值的右緣：這一欄的起點
	stTubeCostColumn  = 1
	stTubeCostEnd     = 21
	stTubeCostRow     = 6
	stTubeRefusalRow  = 7
	stTubeRefusalInk  = 12 // 原版那一行是紅的
)

// drawSTTubeConfirm is do_tube_station's second page: the ride named on four
// rows, and menu 6's box 1 asking for it. When the fare is out of reach the
// box does not appear and row 7 says so instead.
func (a *app) drawSTTubeConfirm(screen *ebiten.Image, window stZoomWindow, ink color.RGBA) {
	selection, ok := a.reducer.TubeSelected(a.state)
	if !ok {
		a.drawSTTextRowCentred(screen, window, 0, a.lookup("ui.message.tube_no_destination"), ink)
		return
	}
	a.drawSTTextRowCentred(screen, window, 0, a.lookup("ui.tube.destination"), ink)
	for i, row := range []struct{ label, value string }{
		{a.lookup("ui.tube.city"), selection.CityName},
		{a.lookup("ui.tube.planet"), selection.PlanetName},
		{a.lookup("ui.tube.system"), selection.SystemName},
	} {
		a.drawSTLabelledRow(screen, window, stTubeLabelColumn, 2+i, stTubeValueEnd, row.label, row.value, ink)
	}
	a.drawSTLabelledRow(screen, window, stTubeCostColumn, stTubeCostRow, stTubeCostEnd,
		a.lookup("ui.tube.cost"), game.FormatCredits(selection.Destination.Fare)+a.lookup("ui.tube.credits"), ink)
	if !selection.Affordable {
		// 原版把這一行印成紅色（調色盤 12），列 7，欄與 Cost: 同一欄。
		a.drawSTTextRow(screen, window, stTubeCostColumn, stTubeRefusalRow,
			a.lookup("ui.tube.insufficient"), stColour(stTubeRefusalInk))
		return
	}
	a.drawSTMenuBox(screen, window, stMenuBaseFill, stMenuBoxes(6)[0], a.lookup("ui.tube.transport"))
}

// drawSTLabelledRow puts a label at a column and its value right-aligned so it
// ends where the given column starts.
func (a *app) drawSTLabelledRow(screen *ebiten.Image, window stZoomWindow, col, row, endCol int, label, value string, c color.RGBA) {
	a.drawSTTextRow(screen, window, col, row, label, c)
	x, y := window.textCell(col, row)
	right, _ := window.textCell(endCol, row)
	if left := right - a.textWidth(value); left > x+a.textWidth(label) {
		x = left
	} else {
		x += a.textWidth(label)
	}
	a.drawTextWithin(screen, value, x, y, window.x1-stZoomInner-x, c)
}

func (a *app) stTubeActionAt(x, y int) (game.Action, bool) {
	window := a.stTubeWindow()
	if a.state.TubePage == game.TubePageConfirm {
		selection, ok := a.reducer.TubeSelected(a.state)
		if !ok || !selection.Affordable {
			return "", false
		}
		if i, ok := window.menuBoxHit(stMenuBaseFill, 6, x, y); ok && i == 0 {
			return game.ActionRideTube, true
		}
		return "", false
	}
	list := a.reducer.TubeDestinations(a.state)
	if len(list) == 0 {
		return "", false
	}
	i, ok := window.menuBoxHit(stMenuBaseFill, stTellerPickStyle(len(list)), x, y)
	if !ok || i >= len(list) || i >= len(stTubePickActions) {
		return "", false
	}
	return stTubePickActions[i], true
}

var stTellerPickActions = []game.Action{game.ActionTellerPick1, game.ActionTellerPick2, game.ActionTellerPick3, game.ActionTellerPick4, game.ActionTellerPick5, game.ActionTellerPick6}

// stTellerPickStyle is pick_location's new_menu by box count: 1 → 6,
// 2–3 → 8, 4–5 → 9, 6 → 10. Each style's last box is the wide one OTHERS uses.
func stTellerPickStyle(count int) int {
	switch {
	case count <= 1:
		return 6
	case count <= 3:
		return 8
	case count <= 5:
		return 9
	}
	return 10
}

// stTellerTransferName is do_transfer's " Transfer to " line: the system the
// list picked, or the navigation target under the remake's one-step flow.
func (a *app) stTellerTransferName() string {
	if a.state.TellerTransferID != "" {
		for _, system := range a.reducer.World.Systems() {
			if system.ID == a.state.TellerTransferID {
				return system.Name
			}
		}
	}
	return a.state.NavigationTarget
}
