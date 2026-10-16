package game

// The cargo pod holds two lots. The manual says a lot may carry auxiliary fuel
// instead of goods, and the original empties a full tank into the main one in
// two different places, on two different thresholds:
//
//   - SHIPLIB:0x24 update_levels:check_fuel (ship rule fuel.aux-tank-refill)
//     waits for the main tank to run dry.
//   - XMOVEONS:0x08 check_fuel (fuel.aux-pod-autoconsume) is called from
//     XMOVEONS:0x09 check_move at 0x044e and refills below five.
//     It runs on pod reentry; it is not the SHIPLIB flight consumer.
//
// 船艙入口在Fuel<5時設255；SHIPLIB末段在Fuel<=0且附件旗標7成立時加255。
// 原生320／1728案證據見docs/atari-st/talos-combat-audit-20260907.md。
// READY contract, source-derived purchase price and player paths:
// docs/atari-st/ship-rules.md and fuel_purchase.go.

// auxPodRefillThreshold is XMOVEONS:0x08 check_fuel's own number. The tank is
// spent below five, not at zero.
const auxPodRefillThreshold = 5

// cargoLotsUsed is how many of the pod's holds are spoken for.
func cargoLotsUsed(state State) int {
	used := 0
	for _, hold := range state.Holds {
		if hold.Occupied() {
			used++
		}
	}
	return used
}

// CargoLotsFree is what the pod has left. The trade and cargo screens both need
// it, and so does buying a tank.
func CargoLotsFree(state State) int {
	free := CargoLots - cargoLotsUsed(state)
	if free < 0 {
		return 0
	}
	return free
}

// drainAuxFuelTank is the dry-tank half of the pair, SHIPLIB:0x24's threshold:
// nothing left in the main tank at all. It reports whether a tank was spent, so
// the caller can tell the player the difference between "topped up" and
// "nothing left".
func drainAuxFuelTank(state *State) bool {
	return checkShipFuel(state)
}

// Pod reentry sets255 below5, independently of the attachment flag. SHIPLIB's
// consumer instead adds255 and requires attachment; do not merge them.
func drainAuxFuelTankBelow(state *State, threshold int) bool {
	if state.Fuel >= threshold || AuxFuelTanks(*state) <= 0 {
		return false
	}
	emptyFirstHoldOf(state, HoldAuxFuel)
	state.Fuel = 255
	return true
}
