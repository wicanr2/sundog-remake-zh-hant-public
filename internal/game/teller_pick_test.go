package game

import "testing"

// pick_location for a transfer: every system but this one and Enlie, six a
// page with OTHERS turning the page; picking one puts the keypad up for it.
func TestTellerTransferPickList(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		if state, err = reducer.Step(state, action); err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.Screen, state.Cash, state.Bank = ScreenUniteller, 0, 500
	// 新遊戲的每個星系都有開局餘額（XSTARTUP 的世界迴圈），這條測的是「轉過去的
	// 錢有沒有到」，所以先把帳面清空，免得讀到的是開局餘額加轉帳。
	state.BankBalances = map[string]int{state.SystemID: state.Bank}
	state, err = reducer.Step(state, ActionTellerTransferPage)
	if err != nil || state.TellerPage != TellerPageTransferPick {
		t.Fatalf("transfer page: %v page=%d", err, state.TellerPage)
	}
	first, others := reducer.TellerPickList(state)
	if len(first) != tellerPickPerPage || !others {
		t.Fatalf("first page %d boxes others=%v, want 6 and OTHERS", len(first), others)
	}
	state, _ = reducer.Step(state, ActionTellerOthers)
	second, _ := reducer.TellerPickList(state)
	if len(second) != 4 {
		t.Fatalf("second page %d boxes, want 4 (12 systems less this one and Enlie)", len(second))
	}
	enlie := reducer.World.Systems()[tellerPickNoBank].ID
	for _, pick := range append(first, second...) {
		if pick.SystemID == state.SystemID || pick.SystemID == enlie {
			t.Fatalf("list offers %s", pick.SystemID)
		}
	}
	state, _ = reducer.Step(state, ActionTellerPick2)
	if state.TellerPage != TellerPageTransfer || state.TellerTransferID != second[1].SystemID {
		t.Fatalf("pick 2: page=%d target=%q want %q", state.TellerPage, state.TellerTransferID, second[1].SystemID)
	}
	state, _ = reducer.Step(state, ActionTellerAmount100)
	state, err = reducer.Step(state, ActionBack)
	if err != nil || state.Bank != 400 || state.BankBalances[second[1].SystemID] != 100 {
		t.Fatalf("transfer: %v bank=%d there=%d", err, state.Bank, state.BankBalances[second[1].SystemID])
	}
	// The right button on the list itself moves nothing.
	state, _ = reducer.Step(state, ActionTellerTransferPage)
	state, _ = reducer.Step(state, ActionBack)
	if state.TellerPage != TellerPageMain || state.Bank != 400 {
		t.Fatalf("backing out of the list: page=%d bank=%d", state.TellerPage, state.Bank)
	}
}
