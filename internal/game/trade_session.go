package game

import (
	"fmt"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// XSTARTUP1703..172e. The random input is explicit and bounded to RND's
// nonnegative word; the new-game generator is remake-owned.
func tradeClassShift(counter, random int) (int, int, error) {
	if counter < 0 || counter > 255 || random < 0 || random > 32767 {
		return 0, 0, fmt.Errorf("invalid trade class startup input")
	}
	next := (counter + 5) % atarist.TradeClassRecords
	return next, (atarist.TradeClassRecords + next + random%3 - 1) % atarist.TradeClassRecords, nil
}

func newGameTradeClassShift(seed int64) int {
	data, err := atarist.EmbeddedTradeClassData()
	if err != nil {
		panic(err) // A bad shipped data pack must not silently choose a class.
	}
	_, shift, err := tradeClassShift(data.StartupCounter(), newSectorRoll(seed).next()%32768)
	if err != nil {
		panic(err)
	}
	return shift
}

// 入口：docs/atari-st/banville-restoration-spec.md。只保存typed交易狀態。
type TradeSession struct {
	Active      bool   `json:"active"`
	CityID      string `json:"cityId"`
	SystemID    string `json:"systemId"`
	InitialCash int    `json:"initialCash"`
	InitialBank int    `json:"initialBank"`
	Funds       int    `json:"funds"`
}

func beginTradeSession(s *State) {
	if s.TradeSession.Active {
		return
	}
	s.TradeSession = TradeSession{Active: true, CityID: s.CityID, SystemID: s.SystemID, InitialCash: s.Cash, InitialBank: s.Bank, Funds: s.Cash + max(0, s.Bank)}
	if s.Bank > 0 {
		s.Bank = 0
	}
	storeCurrentBank(s)
}

// TradeFunds is XDOTRADI's Your limit, not the still-unsettled purse.
func TradeFunds(s State) int {
	if s.TradeSession.Active {
		return s.TradeSession.Funds
	}
	return s.Cash + max(0, s.Bank)
}

func spendTradeFunds(s *State, cost int) bool {
	if !s.TradeSession.Active || cost < 0 || cost > s.TradeSession.Funds {
		return false
	}
	s.TradeSession.Funds -= cost
	return true
}

func creditTradeFunds(s *State, income int) { s.TradeSession.Funds += income }

func finishTradeSession(s *State) {
	if !s.TradeSession.Active {
		return
	}
	if s.Bank < 0 {
		s.Cash = s.TradeSession.Funds
	} else {
		s.Cash = min(s.TradeSession.InitialCash, s.TradeSession.Funds)
		s.Bank = s.TradeSession.Funds - s.Cash
	}
	s.TradeSession = TradeSession{}
	s.TradeSaleSlot, s.TradeSalePrice = 0, 0
	storeCurrentBank(s)
}

func tradeSessionContext(s State) bool {
	screen := s.Screen
	if screen == ScreenCharacterLibrary {
		screen = s.LibraryReturn
	}
	if screen == ScreenCharacterStatus {
		screen = s.StatusReturn
	}
	return screen == ScreenTrade
}

func ValidateTradeState(s State) error {
	if s.TradePurchaseLot != nil || tradeSessionContext(s) && s.TradePage == TradePageBuyDestination {
		lot := s.TradePurchaseLot
		if lot == nil || !s.TradeSession.Active || !tradeSessionContext(s) || s.TradePage != TradePageBuyDestination || s.GroundMode != GroundInPod || lot.Goods < 1 || lot.Goods > 31 || lot.Quality < 1 || lot.Quality > 7 || lot.Price < 1 || lot.Price > 32767 || !TradePurchaseFits(s) {
			return fmt.Errorf("invalid paid trade purchase destination")
		}
	}
	if err := ValidateColonySiteState(s); err != nil {
		return err
	}
	if err := ValidateColonyState(s); err != nil {
		return err
	}
	if s.TradePage == TradePageSwapWarehouse {
		if !s.TradeSession.Active || !tradeSessionContext(s) || s.TradeSaleSlot < 5 || s.TradeSaleSlot > 6 || s.TradeSalePrice != 0 || s.TradePurchaseLot != nil {
			return fmt.Errorf("invalid pending warehouse swap")
		}
		if _, ok := PendingTradeLot(s); !ok {
			return fmt.Errorf("missing pending swap cargo")
		}
	} else if s.TradeSaleSlot != 0 || s.TradeSalePrice != 0 || tradeSessionContext(s) && s.TradePage == TradePageSellConfirm {
		if !s.TradeSession.Active || !tradeSessionContext(s) || s.TradePage != TradePageSellConfirm || s.TradeSalePrice < 0 || s.TradeSalePrice > 32767 {
			return fmt.Errorf("invalid pending trade sale")
		}
		if _, ok := PendingTradeLot(s); !ok {
			return fmt.Errorf("missing pending trade lot")
		}
	}
	if s.TradeClassShift < 0 || s.TradeClassShift >= atarist.TradeClassRecords {
		return fmt.Errorf("invalid saved trade class shift")
	}
	p := s.TradeSession
	if !p.Active {
		if p != (TradeSession{}) || tradeSessionContext(s) {
			return fmt.Errorf("missing or misplaced trade session")
		}
		return nil
	}
	if !tradeSessionContext(s) || p.CityID != s.CityID || p.SystemID != s.SystemID || p.CityID == "" || p.SystemID == "" || p.InitialCash < 0 || p.Funds < 0 || s.Cash != p.InitialCash || s.Bank != min(0, p.InitialBank) || len(s.Quotes) != QuoteRing {
		return fmt.Errorf("invalid saved trade session")
	}
	return nil
}
