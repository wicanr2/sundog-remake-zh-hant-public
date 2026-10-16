package game

// The cargo pod is two holds, and this file is what makes them two *records*
// rather than two counters.
//
// The original keeps the pod as `Gec`, an array of two records of two words
// each; word 0's bits 0–4 name what the hold carries — 0 is empty, 1 is an
// auxiliary fuel tank, 2–30 are goods (ship-rules.md「拖曳、拋艙、通訊」).
// `do_tractor` looks for the first empty hold counting back from hold 2, and
// `do_jettison`'s menu blows hold 1, hold 2 or both. Keeping the holds as
// records is what lets the player say *which* hold goes; counting by kind,
// which this shell did until 2026-09-06, cannot express that.
//
// The kinds below are wider than the original's word: this shell also parks the
// mission's supplies and cryogens in the pod, which the original does through
// the same goods codes. What matters for the rules is which holds are occupied
// and by what, and that is what the accessors here answer.

// HoldKind is what one hold carries.
type HoldKind string

const (
	HoldEmpty    HoldKind = ""
	HoldGoods    HoldKind = "goods"
	HoldAuxFuel  HoldKind = "fuel"
	HoldSalvage  HoldKind = "salvage"
	HoldSupplies HoldKind = "supplies"
	HoldCryogens HoldKind = "cryogens"
)

// Hold is one of the pod's holds. The lot fields carry the goods record for a
// HoldGoods hold and are zero for the rest.
type Hold struct {
	Kind    HoldKind `json:"kind,omitempty"`
	Goods   int      `json:"goods,omitempty"`
	Quality int      `json:"quality,omitempty"`
	Price   int      `json:"price,omitempty"`
	ItemKey string   `json:"itemKey,omitempty"`
	// SourceCode preserves the full source byte, including unknown upper bits.
	// A new depot fuel tank uses the source planet's one-based index.
	// Purchased goods use the original city's one-based index.
	SourceCode uint8 `json:"sourceCode"`
}

// Occupied reports whether the hold carries anything.
func (h Hold) Occupied() bool { return h.Kind != HoldEmpty }

// Lot is the goods record in the hold, for the code that trades in lots.
func (h Hold) Lot() Lot {
	return Lot{Goods: h.Goods, Quality: h.Quality, Price: h.Price, SourceCode: h.SourceCode}
}

// goodsHold wraps a lot as a hold.
func goodsHold(lot Lot, itemKey string) Hold {
	return Hold{Kind: HoldGoods, Goods: lot.Goods, Quality: lot.Quality, Price: lot.Price, ItemKey: itemKey, SourceCode: lot.SourceCode}
}

// countHolds is how many holds carry one of the kinds.
func countHolds(state State, kinds ...HoldKind) int {
	count := 0
	for _, hold := range state.Holds {
		for _, kind := range kinds {
			if hold.Kind == kind {
				count++
				break
			}
		}
	}
	return count
}

// CargoUnits is how many holds carry cargo the readouts count: goods and the
// two mission lots. Fuel tanks and salvage are listed apart, as the original's
// own screens list them.
func CargoUnits(state State) int {
	return countHolds(state, HoldGoods, HoldSupplies, HoldCryogens)
}

// AuxFuelTanks is how many holds carry a full auxiliary tank.
func AuxFuelTanks(state State) int { return countHolds(state, HoldAuxFuel) }

// SalvageLots is how many holds carry what the tractor brought in.
func SalvageLots(state State) int { return countHolds(state, HoldSalvage) }

// SuppliesAboard and CryogensAboard are the mission's two lots.
func SuppliesAboard(state State) int { return countHolds(state, HoldSupplies) }
func CryogensAboard(state State) int { return countHolds(state, HoldCryogens) }

// PodLots is the goods the trade can sell, in hold order.
func PodLots(state State) []Lot {
	lots := make([]Lot, 0, CargoLots)
	for _, hold := range state.Holds {
		if hold.Kind == HoldGoods {
			lots = append(lots, hold.Lot())
		}
	}
	return lots
}

// firstFreeHold is the hold a load goes into, or −1. The original's tractor
// counts back from the last hold; everything else takes the first one free,
// and with two holds the two orders only differ in which hold a lone lot sits
// in — nothing reads that.
func firstFreeHold(state State) int {
	for index, hold := range state.Holds {
		if !hold.Occupied() {
			return index
		}
	}
	return -1
}

// loadHold puts something in the first free hold and reports whether it fitted.
func loadHold(state *State, hold Hold) bool {
	index := firstFreeHold(*state)
	if index < 0 {
		return false
	}
	state.Holds[index] = hold
	return true
}

// emptyHold clears one hold and hands back what was in it.
func emptyHold(state *State, index int) Hold {
	if index < 0 || index >= len(state.Holds) {
		return Hold{}
	}
	was := state.Holds[index]
	state.Holds[index] = Hold{Quality: was.Quality, Price: was.Price, SourceCode: was.SourceCode}
	return was
}

// emptyFirstHoldOf clears the first hold carrying one of the kinds and reports
// whether there was one.
func emptyFirstHoldOf(state *State, kinds ...HoldKind) bool {
	for index, hold := range state.Holds {
		for _, kind := range kinds {
			if hold.Kind == kind {
				emptyHold(state, index)
				return true
			}
		}
	}
	return false
}

// HoldAt is the hold at a slot, or an empty one.
func HoldAt(state State, index int) Hold {
	if index < 0 || index >= len(state.Holds) {
		return Hold{}
	}
	return state.Holds[index]
}

// holdsFromLegacy lays a schema-29 pod back into the two holds. The order is
// fixed — goods, the mission's supplies and cryogens, an auxiliary tank, then
// salvage — because the old save recorded counts, not slots: which hold a lot
// sat in is information it never carried. Anything past the two holds is
// dropped, which can only happen to a save whose counters were already over
// capacity.
func holdsFromLegacy(legacy LegacyPod, gems int) [CargoLots]Hold {
	var holds [CargoLots]Hold
	next := 0
	put := func(hold Hold) {
		if next < len(holds) {
			holds[next] = hold
			next++
		}
	}
	for _, lot := range legacy.LegacyPodLots {
		if lot.Goods != 0 {
			put(goodsHold(lot, ""))
		}
	}
	// The counter also covered lots with no identity: the prototype cargo
	// screen's "load one unit" and the gem trade both bumped it. A save old
	// enough to predate the counter says the same thing with its gem count,
	// which is why that is the floor here — schema 7 carries a cargo identity
	// and a gem, and nothing else.
	plain := legacy.LegacyCargoUnits - legacy.LegacySupplies - legacy.LegacyCryogens
	if gems-len(legacy.LegacyPodLots) > plain {
		plain = gems - len(legacy.LegacyPodLots)
	} else {
		plain -= len(legacy.LegacyPodLots)
	}
	for i := 0; i < plain; i++ {
		put(Hold{Kind: HoldGoods, ItemKey: legacy.LegacyCargoItemKey})
	}
	for i := 0; i < legacy.LegacySupplies; i++ {
		put(Hold{Kind: HoldSupplies})
	}
	for i := 0; i < legacy.LegacyCryogens; i++ {
		put(Hold{Kind: HoldCryogens})
	}
	for i := 0; i < legacy.LegacyAuxFuelTanks; i++ {
		put(Hold{Kind: HoldAuxFuel})
	}
	for i := 0; i < legacy.LegacySalvageLots; i++ {
		put(Hold{Kind: HoldSalvage})
	}
	return holds
}

// firstGoodsHold is the slot of the first hold carrying goods, or −1.
func firstGoodsHold(state State) int {
	for index, hold := range state.Holds {
		if hold.Kind == HoldGoods && hold.Goods != 0 {
			return index
		}
	}
	return -1
}
