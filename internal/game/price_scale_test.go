package game

import "testing"

// 測試注入入場金額後，明確重新建立研究session；正式路徑只在開交易時建立。
func resetTradeTestFunds(s *State) {
	s.TradeSession = TradeSession{}
	beginTradeSession(s)
}

// TestQuotePriceIsInHundredsOfCredits pins the original's price scale. The
// exchange stores a quote's price in hundreds: the marquee prints the word
// as-is ("gold-120") while the Asking: line multiplies it by 100 ("4,300" for a
// price word of 43). Cash is in plain credits — a new game shows "On hand: 202"
// (docs/atari-st/ux-scene-ledger.md, eighth round).
func TestQuotePriceIsInHundredsOfCredits(t *testing.T) {
	if got := Credits(43); got != 4300 {
		t.Fatalf("Credits(43) = %d, want 4300", got)
	}
}

// TestBuyingSpendsTheScaledPrice is the half that matters to the player: the
// purse has to lose the credits, not the price word.
func TestBuyingSpendsTheScaledPrice(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := gameStateAtTrade(t, reducer)
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatal("no quote to buy")
	}
	state.Bank = 0
	state.Cash = Credits(quote.Price) + 7
	resetTradeTestFunds(&state)
	after, err := reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	if TradeFunds(after) != 7 || after.Cash != state.Cash {
		t.Fatalf("funds after buying = %d, want7;cash must await settlement", TradeFunds(after))
	}
	finishTradeSession(&after)
	if after.Cash != 7 {
		t.Fatal("退出交易未結算7現金")
	}

	// One credit short is not enough, so the gate reads the scaled price too.
	state.Cash = Credits(quote.Price) - 1
	resetTradeTestFunds(&state)
	short, err := reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	if short.MessageKey != "ui.message.insufficient_cash" {
		t.Fatalf("message = %q, want ui.message.insufficient_cash", short.MessageKey)
	}
}

// TestANewGamePurseCannotAffordALot records the consequence of the scale: the
// original starts the player with 202 credits in hand against lots priced in
// the thousands, which is what the 36,306 in the bank is for. The remake banks
// nothing yet, so this pins the state as it stands rather than claiming it
// matches the original.
func TestANewGamePurseCannotAffordALot(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := gameStateAtTrade(t, reducer)
	state.Bank = 0
	state.Cash = NewState().Cash
	resetTradeTestFunds(&state)
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatal("no quote")
	}
	if state.Cash >= Credits(quote.Price) {
		t.Skipf("this quote happens to be affordable: cash %d, cost %d",
			state.Cash, Credits(quote.Price))
	}
	after, err := reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	if after.MessageKey != "ui.message.insufficient_cash" {
		t.Fatalf("a new game purse bought a lot: %q", after.MessageKey)
	}
}

// TestANewGameCanAffordALotThroughTheBank is the other half of the scale. The
// purse alone cannot buy anything (the test above), which is why the original
// opens with a bank balance — buyQuote spends the bank first, so a new game can
// trade after all. Without the balance the exchange would be unusable from the
// first minute.
func TestANewGameCanAffordALotThroughTheBank(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := gameStateAtTrade(t, reducer)
	fresh := NewState()
	state.Cash, state.Bank = fresh.Cash, fresh.Bank
	resetTradeTestFunds(&state)
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatal("no quote")
	}
	if state.Cash >= Credits(quote.Price) {
		t.Skipf("this quote is affordable from the purse alone: %d vs %d",
			state.Cash, Credits(quote.Price))
	}
	after, err := reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	if after.MessageKey != "ui.message.bought" {
		t.Fatalf("a new game could not buy through the bank: %q", after.MessageKey)
	}
	if after.Bank != 0 || TradeFunds(after) != fresh.Bank+fresh.Cash-Credits(quote.Price) {
		t.Errorf("bank = %d, want %d (spent %d first)",
			after.Bank, state.Bank-Credits(quote.Price), Credits(quote.Price))
	}
	if after.Cash != state.Cash {
		t.Errorf("cash = %d, want %d untouched while the bank covers it",
			after.Cash, state.Cash)
	}
	finishTradeSession(&after)
	if after.Bank != fresh.Bank-Credits(quote.Price) || after.Cash != fresh.Cash {
		t.Fatal("銀行優先扣款結算不同")
	}
}

// 原版的金額每三位一個逗號：`In bank: 36,306` 有、`On hand: 202` 沒有，
// 交易所的 `Asking: 4,300` 也有（第八輪與第七輪的擷取）。
func TestFormatCreditsPutsACommaEveryThreeDigits(t *testing.T) {
	for _, test := range []struct {
		in   int
		want string
	}{
		{0, "0"},
		{202, "202"},
		{999, "999"},
		{1000, "1,000"},
		{4300, "4,300"},
		{36306, "36,306"},
		{100000, "100,000"},
		{1234567, "1,234,567"},
		{-4300, "-4,300"},
	} {
		if got := FormatCredits(test.in); got != test.want {
			t.Fatalf("FormatCredits(%d) = %q，應該是 %q", test.in, got, test.want)
		}
	}
}

// 價格換算與格式化是兩件事，串起來要得到原版那一行。
func TestTheAskingLineMatchesTheOriginalsCapture(t *testing.T) {
	if got := FormatCredits(Credits(43)); got != "4,300" {
		t.Fatalf("word1 = 43 印成 %q，原版是 4,300", got)
	}
}
