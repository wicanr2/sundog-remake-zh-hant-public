package game

import (
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/world"
)

type FuelPurchaseKind string

const (
	FuelPurchaseMain   FuelPurchaseKind = "main"
	FuelPurchaseAux    FuelPurchaseKind = "aux"
	FuelPurchaseHull   FuelPurchaseKind = "hull"
	ScreenDepot        Screen           = "depot"
	ActionOpenDepot    Action           = "open-depot"
	ActionRepairHull   Action           = "repair-hull"
	ScreenFuelPurchase Screen           = "fuel-purchase"
	ActionFuelBuyFull  Action           = "fuel-buy-full"
	ActionFuelBuyOne   Action           = "fuel-buy-one"
	ActionFuelCancel   Action           = "fuel-cancel"
	ActionExitCargoPod Action           = "exit-cargo-pod"
)

// The dialog is persistent: buying one returns to its menu, and closing runs
// the original caller's writeback, including the residue-15 cancel edge.
type FuelPurchase struct {
	Kind            FuelPurchaseKind `json:"kind"`
	BaseFuel        int              `json:"baseFuel"`
	InitialQuantity int              `json:"initialQuantity"`
	Quantity        int              `json:"quantity"`
	UnitPrice       int              `json:"unitPrice"`
	Funds           int              `json:"funds"`
	InitialBank     int              `json:"initialBank"`
	InitialCash     int              `json:"initialCash"`
	SourceCode      uint8            `json:"sourceCode"`
}

func (r Reducer) openDepot(s *State) error {
	if s.Screen != ScreenGround || s.GroundWorld {
		return fmt.Errorf("depot requires city ground")
	}
	if !r.GroundHasFacility(*s, world.FacilityDepot) {
		s.MessageKey = "ui.message.need_depot"
		return nil
	}
	s.Screen, s.GroundReturn, s.MessageKey = ScreenDepot, ScreenGround, "ui.depot_title"
	return nil
}

func (r Reducer) openFuelPurchase(s *State, kind FuelPurchaseKind) error {
	if (s.Screen != ScreenGround && s.Screen != ScreenDepot) || s.GroundWorld {
		return fmt.Errorf("fuel purchase requires a city ground screen")
	}
	if !r.GroundHasFacility(*s, world.FacilityDepot) {
		s.MessageKey = "ui.message.need_depot"
		return nil
	}
	system, ok := r.World.System(s.SystemID)
	if !ok {
		return fmt.Errorf("fuel purchase has no source system %q", s.SystemID)
	}
	factor := system.EncounterRating + system.PriceLevel + 16 - system.StreetDealRating
	planet, ok := r.World.Planet(s.PlanetID)
	if !ok {
		return fmt.Errorf("fuel purchase has no source planet %q", s.PlanetID)
	}
	p := FuelPurchase{Kind: kind, BaseFuel: s.Fuel, Funds: max(0, s.Bank) + s.Cash,
		InitialBank: s.Bank, InitialCash: s.Cash, SourceCode: uint8(planet.Index + 1)}
	if kind == FuelPurchaseMain {
		p.Quantity, p.UnitPrice = (s.Fuel+1)/16, 100*factor/14
		if s.CityID != s.ShipCityID {
			p.UnitPrice = -1
		}
	} else if kind == FuelPurchaseAux {
		p.Quantity, p.UnitPrice = 16-CargoLotsFree(*s), 100*factor
		if s.GroundMode == GroundOnFoot {
			p.UnitPrice = -1
		}
	} else if kind == FuelPurchaseHull {
		p.Quantity = s.HullCondition
		p.UnitPrice = priceFactor(priceFactor(250*((16-s.HullCondition)/4+1), 12, system.StreetDealRating), system.PriceLevel, 11)
		if s.CityID != s.ShipCityID {
			p.UnitPrice = -1
		}
	} else {
		return fmt.Errorf("unknown fuel purchase kind %q", kind)
	}
	p.InitialQuantity = p.Quantity
	s.GroundReturn = s.Screen
	s.FuelPurchase, s.Screen = p, ScreenFuelPurchase
	s.MessageKey = "ui.message.fuel_purchase"
	return nil
}

func buyFuelQuantity(s *State, full bool) {
	p := &s.FuelPurchase
	if p.UnitPrice < 0 || p.Quantity >= 16 {
		return
	}
	n := 16 - p.Quantity
	if !full {
		n = 1
	}
	if p.UnitPrice > 0 {
		n = min(n, p.Funds/p.UnitPrice)
	}
	if n <= 0 {
		s.MessageKey = "ui.message.insufficient_funds"
		return
	}
	p.Quantity += n
	p.Funds -= n * p.UnitPrice
	s.MessageKey = "ui.message.fuel_purchase"
}

func finishFuelPurchase(s *State) {
	p := s.FuelPurchase
	if p.Kind == FuelPurchaseMain {
		s.Fuel = min(255, p.BaseFuel%16+16*p.Quantity)
	} else if p.Kind == FuelPurchaseAux {
		remaining := p.Quantity - p.InitialQuantity
		for i := range s.Holds {
			if remaining > 0 && !s.Holds[i].Occupied() {
				s.Holds[i].Kind, s.Holds[i].Goods = HoldAuxFuel, 0
				s.Holds[i].Quality, s.Holds[i].SourceCode = 7, p.SourceCode
				s.Holds[i].ItemKey = "atarist.cargo.fuel"
				remaining--
			}
		}
	} else if p.Kind == FuelPurchaseHull {
		s.HullCondition = p.Quantity
	}
	spent := max(0, p.InitialBank) + p.InitialCash - p.Funds
	bankSpend := min(max(0, p.InitialBank), spent)
	s.Bank = p.InitialBank - bankSpend
	s.Cash = p.InitialCash - (spent - bankSpend)
	storeCurrentBank(s)
	s.FuelPurchase = FuelPurchase{}
	s.Screen, s.MessageKey = ScreenGround, "ui.message.refueled"
	if p.Kind == FuelPurchaseHull {
		s.MessageKey = "ui.message.hull_repaired"
	}
	if s.GroundReturn == ScreenDepot {
		s.Screen = ScreenDepot
	}
	s.GroundReturn = ScreenGround
}
