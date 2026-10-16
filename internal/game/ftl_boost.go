package game

import (
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/world"
)

const (
	ScreenFTLBoost        Screen = "ftl-boost"
	ActionOpenFTLBoost    Action = "open-ftl-boost"
	ActionInstallFTLBoost Action = "install-ftl-boost"
)

// XDOUNITE:0x17. MAINLIB:set_money takes the computed value as thousands.
// Evidence and native execution: docs/atari-st/ftl-boost-spec.md.
func FTLBoostPrice(s State) int { return (290 - (s.Colony.Phase-5)*30) * 1000 }

func (r Reducer) openFTLBoost(s *State) error {
	if s.Screen != ScreenDepot || s.CityID != "banville" || s.GroundWorld ||
		!r.GroundHasFacility(*s, world.FacilityDepot) {
		return fmt.Errorf("FTL engineering requires the Banville depot")
	}
	s.GroundReturn = ScreenDepot
	s.Screen, s.MessageKey = ScreenFTLBoost, "ui.message.ftl_quote"
	return nil
}

func installFTLBoost(s *State) {
	// Preserve XDOUNITE:0x17's message priority as well as its write gates:
	// ship presence, available money, then whether a boost is already fitted.
	if s.ShipCityID != s.CityID {
		s.MessageKey = "ui.message.ftl_ship_absent"
		return
	}
	price := FTLBoostPrice(*s)
	if max(0, s.Bank)+s.Cash < price {
		s.MessageKey = "ui.message.ftl_insufficient"
		return
	}
	if s.JumpRange != 8 {
		s.MessageKey = "ui.message.ftl_installed"
		return
	}
	bankSpend := min(max(0, s.Bank), price)
	s.Bank -= bankSpend
	s.Cash -= price - bankSpend
	storeCurrentBank(s)
	s.JumpRange, s.MessageKey = 9, "ui.message.ftl_installed"
}
