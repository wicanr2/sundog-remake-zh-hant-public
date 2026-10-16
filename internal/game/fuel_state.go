package game

import "fmt"

// ValidateFuelState rejects missing or invalid typed state instead of guessing
// a phase, attachment or percentage migration. JSON field presence is checked
// separately by storage before it decodes the current save.
func ValidateFuelState(s State) error {
	if s.Schema != CurrentSchema {
		return fmt.Errorf("不支援舊存檔，請開始新遊戲：schema %d", s.Schema)
	}
	if s.ShipPhase < 0 || s.ShipPhase > 2 {
		return fmt.Errorf("invalid ship phase %d", s.ShipPhase)
	}
	for name, v := range map[string]int{"fuel": s.Fuel, "weapon": s.WeaponCharge,
		"shield": s.ShieldLevel, "setting": s.ShieldSetting,
		"current speed": s.CurrentSpeed, "target speed": s.TargetSpeed} {
		if v < 0 || v > 255 {
			return fmt.Errorf("invalid %s %d", name, v)
		}
	}
	if s.GroundMode != GroundOnFoot && s.GroundMode != GroundInPod {
		return fmt.Errorf("invalid ground mode %q", s.GroundMode)
	}
	if s.CargoCityID == "" || s.FuelRescueDue < 0 {
		return fmt.Errorf("missing cargo location or invalid rescue time")
	}
	p := s.FuelPurchase
	if s.Screen == ScreenFuelPurchase || (s.Screen == ScreenCharacterLibrary && s.LibraryReturn == ScreenFuelPurchase) {
		if (p.Kind != FuelPurchaseMain && p.Kind != FuelPurchaseAux && p.Kind != FuelPurchaseHull) ||
			p.BaseFuel < 0 || p.BaseFuel > 255 || p.InitialQuantity < 0 ||
			p.InitialQuantity > p.Quantity || p.Quantity > 16 ||
			p.UnitPrice < -1 || p.Funds < 0 || p.Funds > max(0, p.InitialBank)+p.InitialCash {
			return fmt.Errorf("invalid saved fuel purchase")
		}
	} else if p != (FuelPurchase{}) {
		return fmt.Errorf("fuel purchase state outside its dialog")
	}
	return nil
}
