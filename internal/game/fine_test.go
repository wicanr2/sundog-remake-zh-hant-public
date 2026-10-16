package game

import "testing"

// fine.amount: (minutes/120 + 3) × (law + price), rounded up to a ten.
func TestParkingFineFollowsTheOriginalsFormula(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	system, ok := reducer.World.System("jondd")
	if !ok {
		t.Fatal("jondd is missing from the world")
	}
	state := NewState()
	// A new game's meter has not started (the player is still aboard); these
	// tests are about what the meter does once it has.
	state.PodParkedAt = 0
	state.SystemID = "jondd"
	state.PodParkedAt = 0
	state.TimeMinutes = 5 * fineMinutesPerUnit
	raw := (5 + fineBaseUnits) * (system.EncounterRating + system.PriceLevel)
	want := clamp(raw-raw%fineRounding+fineRounding, fineFloor, fineCeiling)
	if got := reducer.ParkingFine(state); got != want {
		t.Fatalf("fine = %d, want %d", got, want)
	}
}

// The fine grows with how long the ship sat there.
func TestParkingFineGrowsWithTime(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game's meter has not started (the player is still aboard); these
	// tests are about what the meter does once it has.
	state.PodParkedAt = 0
	state.SystemID = "jondd"
	short := reducer.ParkingFine(state)
	state.TimeMinutes = 40 * fineMinutesPerUnit
	long := reducer.ParkingFine(state)
	if !(long > short) {
		t.Fatalf("a longer stay cost %d against %d", long, short)
	}
}

// fine.clamp-and-enlie: never below ten, never above a thousand, doubled in
// Enlie.
func TestParkingFineIsClampedAndDoubledInEnlie(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game's meter has not started (the player is still aboard); these
	// tests are about what the meter does once it has.
	state.PodParkedAt = 0
	state.SystemID = "jondd"
	state.TimeMinutes = 10_000 * fineMinutesPerUnit
	if got := reducer.ParkingFine(state); got != fineCeiling {
		t.Fatalf("an endless stay cost %d, want the %d ceiling", got, fineCeiling)
	}

	// Enlie doubles its own bill. Comparing it against another system would
	// prove nothing: Enlie's law and price levels are the lowest in the game, so
	// twice its fine is still under most systems' single one.
	enlieSystem, ok := reducer.World.System(commJammedSystemID)
	if !ok {
		t.Fatalf("%s is missing from the world", commJammedSystemID)
	}
	state.SystemID = commJammedSystemID
	state.TimeMinutes = 0
	state.PodParkedAt = 0
	raw := fineBaseUnits * (enlieSystem.EncounterRating + enlieSystem.PriceLevel)
	single := clamp(raw-raw%fineRounding+fineRounding, fineFloor, fineCeiling)
	if got := reducer.ParkingFine(state); got != single*fineEnlieFactor {
		t.Fatalf("Enlie charged %d, want %d — twice its own %d", got, single*fineEnlieFactor, single)
	}
}

// Paying restarts the meter and takes the money from cash first.
func TestPayingTheFineRestartsTheMeter(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game's meter has not started (the player is still aboard); these
	// tests are about what the meter does once it has.
	state.PodParkedAt = 0
	state.Screen = ScreenUniteller
	state.SystemID = "jondd"
	state.TimeMinutes = 20 * fineMinutesPerUnit
	state.Cash = 5000
	fine := reducer.ParkingFine(state)
	next, err := reducer.Step(state, ActionPayFine)
	if err != nil {
		t.Fatal(err)
	}
	if next.Cash != state.Cash-fine {
		t.Fatalf("cash = %d, want %d", next.Cash, state.Cash-fine)
	}
	if next.PodParkedAt != next.TimeMinutes {
		t.Fatalf("the meter was not restarted: parked %d, clock %d", next.PodParkedAt, next.TimeMinutes)
	}
	// A freshly paid meter is not free: the formula's base is three units even
	// at zero minutes, so the counter always wants something. What matters is
	// that it dropped back to that base.
	base := reducer.ParkingFine(next)
	if base >= fine {
		t.Fatalf("paying left the fine at %d, no lower than the %d just settled", base, fine)
	}
}

func TestPayingTheFineNeedsTheMoney(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game's meter has not started (the player is still aboard); these
	// tests are about what the meter does once it has.
	state.PodParkedAt = 0
	state.Screen = ScreenUniteller
	state.SystemID = "jondd"
	state.TimeMinutes = 20 * fineMinutesPerUnit
	state.Cash = 0
	state.Bank = 0
	next, err := reducer.Step(state, ActionPayFine)
	if err != nil {
		t.Fatal(err)
	}
	if next.MessageKey != "ui.message.no_funds" || next.PodParkedAt != state.PodParkedAt {
		t.Fatalf("a broke player still settled the fine: %+v", next)
	}
}
