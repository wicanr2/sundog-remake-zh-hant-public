package game

// Source locations, evidence, limits and save contract: ship-rules.md.
// Controls use the stable engineering identities, never original packed RAM.
type ShipControls struct {
	Warp      bool    `json:"warp"`
	Subc      bool    `json:"subc"`
	Guns      bool    `json:"guns"`
	Tactical  bool    `json:"tactical"`
	Pilotage  bool    `json:"pilotage"`
	Secondary [6]bool `json:"secondary"`
}

type GroundMode string

const (
	GroundOnFoot GroundMode = "foot"
	GroundInPod  GroundMode = "pod"
)

func shipSystemActive(s State, id ShipSystemID) bool {
	switch id {
	case ShipSystemWarpDrive:
		return s.ShipControls.Warp
	case ShipSystemSubcEngine:
		return s.ShipControls.Subc
	case ShipSystemShipGuns:
		return s.ShipControls.Guns
	case ShipSystemTacticalDisplay:
		return s.ShipControls.Tactical
	case ShipSystemForceShields:
		return s.ShieldsActive
	case ShipSystemPilotage:
		return s.ShipControls.Pilotage
	}
	return false
}

func setShipSystemActive(s *State, id ShipSystemID, v bool) {
	switch id {
	case ShipSystemWarpDrive:
		s.ShipControls.Warp = v
	case ShipSystemSubcEngine:
		s.ShipControls.Subc = v
	case ShipSystemShipGuns:
		s.ShipControls.Guns = v
	case ShipSystemTacticalDisplay:
		s.ShipControls.Tactical = v
	case ShipSystemForceShields:
		s.ShieldsActive = v
	case ShipSystemPilotage:
		s.ShipControls.Pilotage = v
	}
}

func closeShipSystemsAfterFuelExhaustion(s *State, oldFuel int) {
	if oldFuel <= 0 || s.Fuel != 0 {
		return
	}
	for _, id := range []ShipSystemID{ShipSystemSubcEngine, ShipSystemTacticalDisplay, ShipSystemForceShields} {
		setShipSystemActive(s, id, false)
		i, _ := shipSystemIndex(id)
		s.ShipControls.Secondary[i] = false
	}
	s.ShieldSetting = 0
	if s.CurrentSpeed < s.TargetSpeed {
		s.ShipControls.Warp = false
		s.ShipControls.Secondary[0] = false
		s.TargetSpeed = 0
	}
}

// checkShipFuel is the original signed check, including intermediate calls
// from energy payment. One call consumes at most one attached tank.
func checkShipFuel(s *State) bool {
	spent := false
	if s.Fuel <= 0 && s.CargoAttached {
		spent = emptyFirstHoldOf(s, HoldAuxFuel)
		if spent {
			s.Fuel += 255
		}
	}
	if s.Fuel < 0 {
		s.Fuel = 0
	}
	return spent
}

// updateShipEnergy executes the already-selected phase. Its caller advances
// the persistent phase before movement, matching SHIPLIB's parent order.
// draw supplies the original consumer's range, and is injectable for parity.
func updateShipEnergy(s *State, draw func(int) int) bool {
	return updateShipEnergyFromFuel(s, s.Fuel, draw)
}

func updateShipEnergyFromFuel(s *State, oldFuel int, draw func(int) int) bool {
	oldTanks := AuxFuelTanks(*s)
	if s.ShipPhase&1 == 0 {
		status := shipSystemStatusByID(*s, ShipSystemShipGuns)
		if s.Fuel > 0 && status > 0 && s.ShipControls.Guns {
			delta := min(status*4, 255-s.WeaponCharge)
			if delta > 0 {
				s.Fuel--
				checkShipFuel(s)
				s.WeaponCharge = clamp(s.WeaponCharge+delta, 0, 255)
			}
		} else if s.WeaponCharge > 0 {
			// The original pays zero even when charge decays. This can
			// consume a tank before later rescue or jettison replaces fuel.
			checkShipFuel(s)
			s.WeaponCharge = clamp(s.WeaponCharge-8, 0, 255)
		}
	}
	if s.ShipPhase == 0 {
		if s.ShieldLevel != s.ShieldSetting {
			status := shipSystemStatusByID(*s, ShipSystemForceShields)
			// CLP37 pays even a zero cost/delta before the shield write.
			checkShipFuel(s)
			s.ShieldLevel = clamp(min(s.ShieldLevel+status*8, s.ShieldSetting), 0, 255)
			if s.ShieldLevel < s.ShieldSetting {
				s.Fuel -= (s.ShieldSetting + 1) / 64
			}
		} else if s.ShieldLevel > 0 {
			s.Fuel--
		}
	}
	if s.TractorActive {
		s.Fuel -= s.CombatRange / 16
	}
	if s.ShipPhase == 1 && s.ShipControls.Secondary[4] {
		s.Fuel -= 6
	}
	if s.ShipPhase == 2 && s.CurrentSpeed != s.TargetSpeed {
		delta, cost := 0, 0
		if s.CurrentSpeed < s.TargetSpeed {
			if draw(16) < shipSystemStatusByID(*s, ShipSystemWarpDrive) {
				delta = min(8, s.TargetSpeed-s.CurrentSpeed)
			}
			cost = s.CurrentSpeed/64 + 1
			if s.CurrentSpeed > 223 {
				cost += 8
			}
		} else {
			delta = max(-4, s.TargetSpeed-s.CurrentSpeed)
		}
		s.Fuel -= cost
		checkShipFuel(s)
		s.CurrentSpeed = clamp(s.CurrentSpeed+delta, 0, 255)
	}
	if s.FuelRescuePending && s.TimeMinutes >= s.FuelRescueDue {
		s.Fuel = 255
		s.FuelRescuePending = false
		s.ShipRefresh = true
		odd := draw(2) != 0 // The original evaluates this even without battle.
		if s.CombatActive && odd {
			s.EnemyClosing, s.EnemyCourseMode = 128, 4
		}
	}
	if s.FuelJettisonPending {
		s.Fuel = 255
		s.FuelJettisonPending = false
	}
	checkShipFuel(s)
	closeShipSystemsAfterFuelExhaustion(s, oldFuel)
	return AuxFuelTanks(*s) < oldTanks
}

func advanceShipPhase(s *State) { s.ShipPhase = (s.ShipPhase + 1) % 3 }
