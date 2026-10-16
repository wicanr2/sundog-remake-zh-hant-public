package game

import (
	"sort"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// The ship's locker and stores are what a new game puts aboard. What goes in
// them is the original's own — XSTARTUP:0x32 set_up_lockers, decoded into
// docs/atari-st/new-game-loadout.json — so no count or item code is written
// here. What the remake does with them afterwards is marked where it is
// remake-owned.
const (
	// storeSlots is the size of the stores block the secondary notes record.
	// The initializer fills fewer than that, and the rest stay empty.
	storeSlots = 20
	// itemCodeMask is the five bits an item code occupies. The original packs
	// flags into the high bits, so a stored byte is not the code by itself.
	itemCodeMask = 0x1F
)

var (
	loadoutSource   atarist.LoadoutData
	loadoutSourceOK bool
)

func init() {
	data, err := atarist.EmbeddedLoadoutData()
	if err != nil {
		return
	}
	loadoutSource = data
	loadoutSourceOK = true
}

// newGameShunts is how many shunts a new game carries. The number comes from
// the initializer rather than from a choice made here.
func newGameShunts() int {
	if !loadoutSourceOK {
		return 0
	}
	return loadoutSource.ShuntCount()
}

// defaultShipStores is the stores block a new game starts with, laid out by the
// source's own writes.
func defaultShipStores() []int {
	if !loadoutSourceOK {
		return make([]int, storeSlots)
	}
	return loadoutSource.Stores(storeSlots)
}

// PartsForSale lists the ship parts a warehouse offers, in item-table order.
// The original's stock is a contiguous run of item ids carried in the dialogue
// frame's postcode opcode 3, so it lives per shop rather than per warehouse
// (rule price.stock-range). The remake has no frame VM to read it from, so it
// offers every part the six systems are built from, plus the shunt that stands
// in for one.
func PartsForSale() []int {
	if !shipSystemSourceOK {
		return nil
	}
	seen := map[int]struct{}{atarist.ShuntItemIndex(): {}}
	for _, system := range shipSystemSource.Systems() {
		for _, column := range system.Columns {
			seen[column.ItemIndex] = struct{}{}
		}
		if system.Alternate != nil {
			seen[system.Alternate.ItemIndex] = struct{}{}
		}
	}
	parts := make([]int, 0, len(seen))
	for item := range seen {
		parts = append(parts, item)
	}
	sort.Ints(parts)
	return parts
}

// ItemCode strips the flags the original packs into the high bits of an item
// byte, leaving the five bits that index the item table. Anything that looks a
// stored item up — its name, its price, its icon — has to go through this
// rather than use the byte as it stands.
func ItemCode(item int) int { return item & itemCodeMask }

// ItemName returns a part or store item's source name.
func ItemName(item int) (string, bool) {
	items, err := atarist.EmbeddedItemData()
	if err != nil {
		return "", false
	}
	table := items.Items()
	code := item & itemCodeMask
	if code < 0 || code >= len(table) {
		return "", false
	}
	return table[code].Name, true
}

// The price rules are XDOINTER:0x02 set_up_price's, and every constant below
// comes from it.
const (
	// tradeInItems are the four optional parts the price rule treats
	// differently: the ones a ship can be fitted with but does not have to be.
	// The original holds them as a set constant in its own code.
	tradeInFirstItem = 19
	tradeInLastItem  = 22
	// An optional part's price starts from its value plus a flat hundred, then
	// costs five times that if the ship already carries one and eight if it
	// does not. Anything else doubles when the ship does not carry one.
	optionalPriceBonus  = 100
	optionalOwnedFactor = 5
	optionalNewFactor   = 8
	plainNewFactor      = 2
	// The multiplier every shop applies: the system's own price level over a
	// per-city divisor, times a flat thirty, clamped.
	priceLevelScale      = 30
	priceCityDivisorBase = 26
	priceCityDivisorSpan = 9
	priceScaleMinimum    = 1
	priceScaleMaximum    = 32
)

// ItemPrice returns a part's base price: the value byte its own record carries,
// adjusted by whether the ship already has one. What a particular shop charges
// is that number times the multiplier its star system sets; see ShopItemPrice.
func ItemPrice(item int) (int, bool) {
	return itemPrice(item, false)
}

func itemPrice(item int, owned bool) (int, bool) {
	items, err := atarist.EmbeddedItemData()
	if err != nil {
		return 0, false
	}
	table := items.Items()
	code := item & itemCodeMask
	if code < 0 || code >= len(table) {
		return 0, false
	}
	entry := table[code]
	if !entry.HasValue || entry.Value <= 0 {
		return 0, false
	}
	if code >= tradeInFirstItem && code <= tradeInLastItem {
		factor := optionalNewFactor
		if owned {
			factor = optionalOwnedFactor
		}
		return (entry.Value + optionalPriceBonus) * factor, true
	}
	if !owned {
		return entry.Value * plainNewFactor, true
	}
	return entry.Value, true
}

// PriceScale is the multiplier a city's shops apply. The star system sets the
// level; the divisor varies a little from city to city, which is what makes two
// cities of one system quote differently.
//
// The original's divisor is 26 + ((sum of two city fields) mod 9), so 26..34
// (rule price.city-divisor). Which two fields those are has not been named, so
// the remake takes the divisor from the city's own identity instead — the same
// city always quotes the same way, and the range is the source's, but which
// city sits where in that range is remake-owned.
func (r Reducer) PriceScale(state State) int {
	system, ok := r.World.System(state.SystemID)
	if !ok || system.PriceLevel <= 0 {
		return priceScaleMinimum
	}
	divisor := priceCityDivisorBase + cityPriceOffset(state.CityID)
	scale := priceLevelScale * system.PriceLevel / divisor
	return clamp(scale, priceScaleMinimum, priceScaleMaximum)
}

// ShopItemPrice is what the shop in front of the player charges for a part.
func (r Reducer) ShopItemPrice(state State, item int) (int, bool) {
	base, ok := itemPrice(item, CarriedPartCount(state, item) > 0)
	if !ok {
		return 0, false
	}
	return base * r.PriceScale(state), true
}

// cityPriceOffset spreads the cities across the divisor's range the way the
// original spreads them across its own: deterministically, and by no more than
// the span the source allows.
func cityPriceOffset(cityID string) int {
	sum := 0
	for _, char := range cityID {
		sum += int(char)
	}
	return sum % priceCityDivisorSpan
}

// itemKeySuffix turns a source name into its catalog key suffix, the same way
// the catalog was built from the evidence document.
func itemKeySuffix(name string) string {
	out := make([]rune, 0, len(name))
	dash := true
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			dash = false
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
			dash = false
		default:
			if !dash {
				out = append(out, '-')
				dash = true
			}
		}
	}
	trimmed := string(out)
	for len(trimmed) > 0 && trimmed[len(trimmed)-1] == '-' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}

// SelectedPart returns the part the warehouse counter is showing.
func (state State) SelectedPart() (int, bool) {
	parts := PartsForSale()
	if len(parts) == 0 || state.SelectedPartIndex < 0 || state.SelectedPartIndex >= len(parts) {
		return 0, false
	}
	return parts[state.SelectedPartIndex], true
}

// CarriedPartCount reports how many of a part are aboard. Shunts are counted
// separately because three older repair paths still spend them as a bare
// number.
func CarriedPartCount(state State, item int) int {
	if item == atarist.ShuntItemIndex() {
		return state.Shunts
	}
	count := 0
	for _, carried := range state.Parts {
		if carried == item {
			count++
		}
	}
	return count
}

func cyclePart(state *State, delta int) {
	parts := PartsForSale()
	if len(parts) == 0 {
		return
	}
	index := (state.SelectedPartIndex + delta) % len(parts)
	if index < 0 {
		index += len(parts)
	}
	state.SelectedPartIndex = index
}

// takeCarriedPart removes one of a part from what is aboard.
func takeCarriedPart(state *State, item int) bool {
	if item == atarist.ShuntItemIndex() {
		if state.Shunts <= 0 {
			return false
		}
		state.Shunts--
		return true
	}
	for index, carried := range state.Parts {
		if carried != item {
			continue
		}
		state.Parts = bodySlots(*state)
		state.Parts[index] = 0
		return true
	}
	return false
}

// addCarriedPart puts a part aboard.
func addCarriedPart(state *State, item int) {
	if item == atarist.ShuntItemIndex() {
		state.Shunts++
		return
	}
	slots := bodySlots(*state)
	for index, held := range slots {
		if held == 0 {
			slots[index] = item
			state.Parts = slots
			return
		}
	}
}

// installablePart finds a slot of a system that a carried part would fix, and
// the part to spend on it. It prefers the part that belongs in the slot over a
// shunt, because the shunt costs the row a point and the real part does not.
func installablePart(state State, system ShipSystem) (row, column, item int, ok bool) {
	source, found := sourceShipSystem(system.ID)
	if !found {
		return 0, 0, 0, false
	}
	shunt := atarist.ShuntItemIndex()
	fallbackFound := false
	var fallbackRow, fallbackColumn int
	for rowIndex, slots := range system.Rows {
		for columnIndex, installed := range slots {
			if _, accepted := source.Accepts(columnIndex, installed); accepted && installed != shunt {
				continue
			}
			wanted := source.Columns[columnIndex].ItemIndex
			if CarriedPartCount(state, wanted) > 0 {
				return rowIndex, columnIndex, wanted, true
			}
			if source.Alternate != nil && source.Alternate.Column == columnIndex &&
				CarriedPartCount(state, source.Alternate.ItemIndex) > 0 {
				return rowIndex, columnIndex, source.Alternate.ItemIndex, true
			}
			if !fallbackFound && installed != shunt && columnIndex != 0 && CarriedPartCount(state, shunt) > 0 {
				fallbackFound = true
				fallbackRow, fallbackColumn = rowIndex, columnIndex
			}
		}
	}
	if fallbackFound {
		return fallbackRow, fallbackColumn, shunt, true
	}
	return 0, 0, 0, false
}

// normalizeLocker keeps fixed slot positions and unknown valid packed bytes.
// Save decoding validates the raw shape before normalization.
func normalizeLocker(state *State) {
	if len(state.Stores) != storeSlots {
		stores := make([]int, storeSlots)
		copy(stores, state.Stores)
		state.Stores = stores
	}
	for index, item := range state.Stores {
		if item < 0 || item > 0xFF {
			state.Stores[index] = 0
		}
	}
	state.Parts = bodySlots(*state)
	count := len(PartsForSale())
	if state.Screen == ScreenStores {
		count = storeSlots
	} else if state.Screen == ScreenCharacterStatus {
		count = PocketCount
	}
	if count == 0 || state.SelectedPartIndex < 0 || state.SelectedPartIndex >= count {
		state.SelectedPartIndex = 0
	}
}
