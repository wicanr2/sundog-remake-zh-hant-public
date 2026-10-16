package game

// Leaving the ship parked on a city street costs money. XDOUNITE:0x0e
// do_teller:pay_fine works out how long it has been sitting there and charges
// for it at the Uniteller counter. Three rules pin it:
//
//   - fine.elapsed-minutes: the clock minus the timestamp taken when the player
//     left the pod, in minutes, with the hour difference times 60 and the day
//     difference times 1440.
//   - fine.amount: (minutes / 120 + 3) × (law level + price level), rounded up
//     to a multiple of ten. Those two nibbles are the same ones the remake calls
//     EncounterRating and PriceLevel, so a safe, expensive system fines hardest.
//   - fine.clamp-and-enlie: clamped to 10..1000, and doubled in Enlie.
//
// The one thing this shell adds is when the bill arrives: the original charges
// it at the counter, and so does this, but this shell has no separate act of
// parking, so the timestamp is taken when the player steps onto the ground.

const (
	fineMinutesPerUnit = 120
	fineBaseUnits      = 3
	fineRounding       = 10
	fineFloor          = 10
	fineCeiling        = 1000
	fineEnlieFactor    = 2
	// parkingMeterUnstarted marks a ship that has not been left on a street, so
	// no fine is running.
	parkingMeterUnstarted = -1
)

// ParkingFine is what the counter would charge right now.
func (r Reducer) ParkingFine(state State) int {
	if state.PodParkedAt < 0 || state.PodParkedAt > state.TimeMinutes {
		return 0
	}
	system, ok := r.World.System(state.SystemID)
	if !ok {
		return 0
	}
	elapsed := state.TimeMinutes - state.PodParkedAt
	fine := (elapsed/fineMinutesPerUnit + fineBaseUnits) * (system.EncounterRating + system.PriceLevel)
	// The original rounds up to the next ten rather than to the nearest, so a
	// fine of exactly 40 is charged as 50.
	fine = fine - fine%fineRounding + fineRounding
	fine = clamp(fine, fineFloor, fineCeiling)
	if state.SystemID == commJammedSystemID {
		fine *= fineEnlieFactor
	}
	return fine
}

// payParkingFine settles the bill and restarts the clock on it. It reports
// whether the money was there.
func payParkingFine(state *State, fine int) bool {
	if fine <= 0 {
		return false
	}
	if state.Cash+state.Bank < fine {
		return false
	}
	fromCash := fine
	if fromCash > state.Cash {
		fromCash = state.Cash
	}
	state.Cash -= fromCash
	if remainder := fine - fromCash; remainder > 0 {
		state.Bank -= remainder
		storeCurrentBank(state)
	}
	state.PodParkedAt = state.TimeMinutes
	return true
}
