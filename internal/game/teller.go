package game

import "fmt"

// XDOUNITE:0x0b do_teller is four boxes (menu 6): DEPOSIT CASH when there is
// cash, WITHDRAW CASH when there is a balance, TRANSFER unless the bank is
// closed, and PAY FINE in place of the last two when a fine is owed.
// Deposit, withdraw and transfer all go through move_cash (0x0c): a keypad of
// menu 7 -- ALL, 100, 1000, 10000, 50000, CLEAR -- composes an amount under
// "AVAILABLE:" and "AMOUNT:", capped at what is available, and the right
// button ends the keypad with the amount moved. The remake keeps the same
// pages and keys; ActionDepositCash / ActionWithdrawCash / ActionTransferBank
// stay as the one-step "all of it" the remake layout uses.

const (
	TellerPageMain     = 0
	TellerPageDeposit  = 1
	TellerPageWithdraw = 2
	TellerPageTransfer = 3
	// TellerPageTransferPick is XDOUNITE:0x05 pick_location before the
	// transfer's keypad: the systems to send money to.
	TellerPageTransferPick = 4

	// tellerPickPerPage is pick_location's first page: six boxes and OTHERS
	// when more than seven candidates remain; the second page holds the rest.
	tellerPickPerPage = 6
	// tellerPickNoBank is the third system record, Enlie: pick_location skips
	// it for transfers (kind 3), so no money can be sent there.
	tellerPickNoBank = 2
)

var tellerPickActions = []Action{ActionTellerPick1, ActionTellerPick2, ActionTellerPick3, ActionTellerPick4, ActionTellerPick5, ActionTellerPick6}

// TellerPick is one box of the transfer list.
type TellerPick struct {
	SystemID string
	Name     string
}

// TellerPickList is pick_location(kind 3) for the page that is up: every
// system but this one and Enlie, in record order; the first page is the
// first six, the second page the rest, and others says whether OTHERS is
// shown (both pages have it when the list needs two).
func (r Reducer) TellerPickList(state State) (boxes []TellerPick, others bool) {
	all := []TellerPick{}
	for i, system := range r.World.Systems() {
		if i == tellerPickNoBank || system.ID == state.SystemID {
			continue
		}
		all = append(all, TellerPick{SystemID: system.ID, Name: system.Name})
	}
	others = len(all) > tellerPickPerPage+1
	if !others {
		return all, false
	}
	if state.TellerPickPage == 0 {
		return all[:tellerPickPerPage], true
	}
	return all[tellerPickPerPage:], true
}

// tellerPick takes box n of the page: the keypad comes up for that system.
func (r Reducer) tellerPick(state *State, n int) bool {
	boxes, _ := r.TellerPickList(*state)
	if n < 0 || n >= len(boxes) {
		return false
	}
	state.TellerTransferID = boxes[n].SystemID
	state.TellerPage, state.TellerAmount = TellerPageTransfer, 0
	state.MessageKey = "ui.message.teller_amount"
	return true
}

var tellerKeypad = map[Action]int{
	ActionTellerAmount100:   100,
	ActionTellerAmount1000:  1000,
	ActionTellerAmount10000: 10000,
	ActionTellerAmount50000: 50000,
}

// TellerAvailable is move_cash's "AVAILABLE:" for the page: the cash for a
// deposit, the balance for a withdrawal or a transfer.
func TellerAvailable(state State) int {
	if state.TellerPage == TellerPageDeposit {
		return state.Cash
	}
	return state.Bank
}

// tellerKey is one press of the keypad: ALL takes everything available,
// CLEAR starts over, a number adds itself, and the sum never passes what is
// available (move_cash: more(sum, available) -> sum := available).
func tellerKey(state *State, action Action) {
	available := TellerAvailable(*state)
	switch action {
	case ActionTellerAmountAll:
		state.TellerAmount = available
	case ActionTellerAmountClear:
		state.TellerAmount = 0
	default:
		state.TellerAmount += tellerKeypad[action]
	}
	if state.TellerAmount > available {
		state.TellerAmount = available
	}
}

// tellerConfirm ends the keypad: the amount composed moves, an amount of
// zero moves nothing.
func (r Reducer) tellerConfirm(state *State) error {
	amount := clamp(state.TellerAmount, 0, TellerAvailable(*state))
	page := state.TellerPage
	state.TellerPage, state.TellerAmount = TellerPageMain, 0
	if amount == 0 {
		state.MessageKey = "ui.message.teller_nothing"
		return nil
	}
	switch page {
	case TellerPageDeposit:
		state.Cash -= amount
		state.Bank += amount
		storeCurrentBank(state)
		state.MessageKey = "ui.message.deposited"
	case TellerPageWithdraw:
		state.Bank -= amount
		state.Cash += amount
		storeCurrentBank(state)
		state.MessageKey = "ui.message.withdrawn"
	case TellerPageTransfer:
		targetSystem := state.TellerTransferID
		if targetSystem == "" {
			// The remake layout has no list: the money goes where the
			// navigation target is.
			destination, ok := r.World.Destination(state.NavigationTargetID)
			if !ok {
				return fmt.Errorf("unknown transfer destination %q", state.NavigationTargetID)
			}
			targetSystem = destination.SystemID
		}
		state.TellerTransferID = ""
		if targetSystem == state.SystemID {
			state.MessageKey = "ui.message.transfer_same_system"
			return nil
		}
		storeCurrentBank(state)
		state.Bank -= amount
		state.BankBalances[targetSystem] += amount
		storeCurrentBank(state)
		state.MessageKey = "ui.message.transferred"
	}
	return nil
}
