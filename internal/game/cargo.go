package game

import (
	"fmt"
	"strings"
)

// CargoName gives the player-visible name of the cargo the player is carrying.
// An empty hold has no identity of its own, so the local market's offer names
// the column instead — that is what a purchase would put in there.
//
// The save keeps the source identity (`atarist.cargo.*`, which record it came
// from) and the catalog keys the display name (`ui.cargo.*`), so retranslating
// never costs a save its pointer back into the source table.
func (r Reducer) CargoName(state State) string {
	key := CargoItemKey(state)
	if key == "" {
		quote, ok := r.World.Market(state.CityID)
		if !ok {
			return ""
		}
		key = quote.ItemKey
	}
	return r.cargoName(key)
}

// cargoName gives the player-visible name of a market item. The source record's
// own English name is the fallback, so a missing translation shows the cargo
// rather than a key.
// CargoNameForKey names a cargo record for the shells.
func (r Reducer) CargoNameForKey(itemKey string) string { return r.cargoName(itemKey) }

func (r Reducer) cargoName(itemKey string) string {
	if kind, ok := r.World.CargoKind(itemKey); ok {
		return r.localize("ui.cargo."+kind.ID, kind.Name)
	}
	// A save written before the Atari ST cargo table carries an Apple II key,
	// which the source text catalog can still name.
	if r.AppleIIText != nil {
		if label, ok := r.AppleIIText.Localized(itemKey); ok {
			return label
		}
	}
	return itemKey
}

// appleIICargoRemap names the one cargo whose Apple II name differs from its
// Atari ST name. Every other Apple II cargo key becomes an Atari ST identity by
// the same underscore-to-dash rewrite, because the first sixteen records of the
// Atari ST cargo table are the Apple II list in the same order — a match that
// TestAtariSTCargoTableExtendsTheAppleIICargoList keeps honest.
var appleIICargoRemap = map[string]string{"ships_fuel": "fuel"}

// migrateAppleIICargoIdentity rewrites a schema 7 save's cargo identity onto the
// Atari ST cargo table. An identity with no tradable counterpart is cleared
// rather than guessed; the units stay in the hold and the next purchase names
// them.
func migrateAppleIICargoIdentity(state *State) {
	suffix, ok := strings.CutPrefix(state.LegacyCargoItemKey, "apple2.cargo.")
	if !ok {
		return
	}
	id := strings.ReplaceAll(suffix, "_", "-")
	if replacement, found := appleIICargoRemap[suffix]; found {
		id = replacement
	}
	key := "atarist.cargo." + id
	if geographyRegistryOK {
		if _, found := geographyRegistry.CargoKind(key); !found {
			state.LegacyCargoItemKey = ""
			return
		}
	}
	state.LegacyCargoItemKey = key
}

// CargoItemKey is which cargo record the pod is carrying: the first hold that
// names one. The pod can hold two different goods now, so this is what the
// one-line readouts show, not the whole manifest.
func CargoItemKey(state State) string {
	for _, hold := range state.Holds {
		if hold.Kind == HoldGoods && hold.ItemKey != "" {
			return hold.ItemKey
		}
	}
	return ""
}

// checkCargoKeyIsNamed is the guard a trade runs before it moves anything: the
// cargo the player is about to hold has to have a name somewhere, or the hold
// ends up carrying a key.
//
// Which table to ask depends on the key. An atarist.cargo.* key comes from the
// Atari ST cargo table and is named there; an apple2.* key comes from the Apple
// II source catalog. Asking the wrong one is not a stricter check, it is a
// broken one.
func (r Reducer) checkCargoKeyIsNamed(itemKey string) error {
	if _, ok := r.World.CargoKind(itemKey); ok {
		return nil
	}
	if r.AppleIIText != nil {
		if _, ok := r.AppleIIText.Entry(itemKey); ok {
			return nil
		}
	}
	return fmt.Errorf("no source table names the cargo %q", itemKey)
}

// The exchange pages preserve do_exchange and pick_item's source/slot stages.
const (
	TradePageMain           = 0  // Buy / Sell
	TradePageBuy            = 1  // the quote
	TradePageSell           = 2  // the pod's lots
	TradePageEntrance       = 3  // Exchange / Warehouse, the first thing the building shows
	TradePageWarehouse      = 4  // Store / Fetch / Swap over the warehouse's contents
	TradePageStore          = 5  // pick the pod lot to store
	TradePageFetch          = 6  // pick the warehouse lot to fetch
	TradePageSellSource     = 7  // Warehouse / Pod source menu
	TradePageSellWarehouse  = 8  // pick one of the four warehouse lots to sell
	TradePageSellConfirm    = 9  // approve or withdraw the selected lot
	TradePageBuyDestination = 10 // paid lot: Warehouse / Pod destination
	TradePageSwapPod        = 11 // first select an occupied pod slot
	TradePageSwapWarehouse  = 12 // then select an occupied warehouse slot
)
