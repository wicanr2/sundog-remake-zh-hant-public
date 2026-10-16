package game

import "testing"

// The pod is two records, not two counts. What that buys is the ability to say
// which hold: two lots of the same goods are no longer interchangeable.
func TestTheTwoHoldsAreToldApart(t *testing.T) {
	state := NewState()
	state.Holds[0] = Hold{Kind: HoldGoods, Goods: 5, Quality: 7, Price: 100, ItemKey: "atarist.cargo.food"}
	state.Holds[1] = Hold{Kind: HoldGoods, Goods: 5, Quality: 1, Price: 40, ItemKey: "atarist.cargo.food"}
	if got := CargoUnits(state); got != 2 {
		t.Fatalf("cargo units = %d, want 2", got)
	}
	if got := CargoLotsFree(state); got != 0 {
		t.Fatalf("free lots = %d, want 0", got)
	}
	runJettison(&state, JettisonHold1)
	if state.Holds[0].Occupied() {
		t.Fatalf("hold 1 survived the bolts: %+v", state.Holds[0])
	}
	if state.Holds[1].Quality != 1 {
		t.Fatalf("hold 2 changed: %+v", state.Holds[1])
	}
}

// Auxiliary fuel is hold code 1 in the original and takes a hold like anything
// else, but it is not cargo a pirate wants (PodDrawsEncounter) and it is not
// something the exchange will buy.
func TestAFuelTankIsAHoldButNotCargo(t *testing.T) {
	state := NewState()
	state.Holds[0] = Hold{Kind: HoldAuxFuel}
	if got := AuxFuelTanks(state); got != 1 {
		t.Fatalf("tanks = %d, want 1", got)
	}
	if got := CargoUnits(state); got != 0 {
		t.Fatalf("a fuel tank counted as cargo: %d", got)
	}
	if PodDrawsEncounter(state) {
		t.Fatal("a pod carrying only fuel drew an encounter")
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reducer.sellPodLot(&state, 0); ok {
		t.Fatal("the exchange bought a fuel tank")
	}
}

// A schema-29 save recorded counts, not slots. The migration lays them back
// into the holds in a fixed order and keeps everything that was aboard.
func TestASchema29PodMigratesIntoTheHolds(t *testing.T) {
	cases := []struct {
		name  string
		pod   LegacyPod
		gems  int
		kinds [CargoLots]HoldKind
	}{
		{
			name:  "一格貨物、一格燃料",
			pod:   LegacyPod{LegacyCargoUnits: 1, LegacyAuxFuelTanks: 1, LegacyPodLots: []Lot{{Goods: 5, Quality: 3}}},
			gems:  1,
			kinds: [CargoLots]HoldKind{HoldGoods, HoldAuxFuel},
		},
		{
			name:  "任務的兩格",
			pod:   LegacyPod{LegacyCargoUnits: 2, LegacySupplies: 1, LegacyCryogens: 1},
			kinds: [CargoLots]HoldKind{HoldSupplies, HoldCryogens},
		},
		{
			name:  "牽引撈到的一格",
			pod:   LegacyPod{LegacySalvageLots: 1},
			kinds: [CargoLots]HoldKind{HoldSalvage, HoldEmpty},
		},
		{
			// schema 7 has no counter at all: the gem is the lot.
			name:  "只有寶石計數的舊存檔",
			pod:   LegacyPod{LegacyCargoItemKey: "atarist.cargo.gems-crystals"},
			gems:  1,
			kinds: [CargoLots]HoldKind{HoldGoods, HoldEmpty},
		},
	}
	for _, test := range cases {
		holds := holdsFromLegacy(test.pod, test.gems)
		for i, want := range test.kinds {
			if holds[i].Kind != want {
				t.Fatalf("%s: hold %d is %q, want %q (%+v)", test.name, i+1, holds[i].Kind, want, holds)
			}
		}
	}
	// The lot's own record comes across, not just its kind.
	holds := holdsFromLegacy(LegacyPod{LegacyCargoUnits: 1, LegacyPodLots: []Lot{{Goods: 5, Quality: 3, Price: 90}}}, 1)
	if holds[0].Goods != 5 || holds[0].Quality != 3 || holds[0].Price != 90 {
		t.Fatalf("the goods record did not migrate: %+v", holds[0])
	}
}

// The tractor takes the last free hold, counting back from hold 2, and a
// purchase takes the first — do_tractor's own scan direction.
func TestTheTractorAndThePurchaseFillFromOppositeEnds(t *testing.T) {
	state := newGameWithFuel()
	state.Screen = ScreenTactical
	state.CombatActive = true
	state.CombatRange = 10
	if got := runTractor(&state); got != TractorLoaded {
		t.Fatalf("tractor = %q", got)
	}
	if state.Holds[CargoLots-1].Kind != HoldSalvage || state.Holds[0].Occupied() {
		t.Fatalf("the tractor did not take the last hold: %+v", state.Holds)
	}
	if !addPodLot(&state, Lot{Goods: 5, Quality: 3}, "atarist.cargo.food") {
		t.Fatal("the pod had no room for a lot")
	}
	if state.Holds[0].Kind != HoldGoods {
		t.Fatalf("the purchase did not take the first hold: %+v", state.Holds)
	}
}
