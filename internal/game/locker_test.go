package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// TestNewGameCarriesWhatSetUpLockersPutsAboard pins the one part of the locker
// that is proven: XSTARTUP:0x32 fills five locker bytes with shunts and stocks
// the stores with five rapidheals, five burgers, a shield and a stinger.
func TestNewGameCarriesWhatSetUpLockersPutsAboard(t *testing.T) {
	state := NewState()
	if state.Shunts != 5 {
		t.Fatalf("a new game carries %d shunts, want 5", state.Shunts)
	}
	counts := map[string]int{}
	for _, item := range state.Stores {
		if item == 0 {
			continue
		}
		name, ok := ItemName(item)
		if !ok {
			t.Fatalf("stores hold item 0x%02x, which is not in the item table", item)
		}
		counts[name]++
	}
	for name, want := range map[string]int{"rapidheal": 5, "burger": 5, "shield": 1, "stinger": 1} {
		if counts[name] != want {
			t.Fatalf("stores hold %d × %s, want %d (%v)", counts[name], name, want, counts)
		}
	}
	if len(state.Stores) != storeSlots {
		t.Fatalf("stores block is %d slots, want %d", len(state.Stores), storeSlots)
	}
	// The shield byte keeps the high bits the source writes; only the low five
	// bits are the item code.
	if state.Stores[10]&^itemCodeMask == 0 {
		t.Fatalf("the shield slot lost the source's high bits: 0x%02x", state.Stores[10])
	}
}

// TestPartsForSaleCoversEverySystemPart keeps the counter's stock tied to what
// the systems are actually built from.
func TestPartsForSaleCoversEverySystemPart(t *testing.T) {
	parts := PartsForSale()
	if len(parts) == 0 {
		t.Fatal("the parts counter offers nothing")
	}
	offered := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		offered[part] = struct{}{}
		if _, ok := ItemPrice(part); !ok {
			t.Fatalf("part 0x%02x is offered without a price input", part)
		}
	}
	if _, ok := offered[atarist.ShuntItemIndex()]; !ok {
		t.Fatal("the parts counter does not stock shunts")
	}
	data, err := atarist.EmbeddedShipSystemData()
	if err != nil {
		t.Fatal(err)
	}
	for _, system := range data.Systems() {
		for _, column := range system.Columns {
			if _, ok := offered[column.ItemIndex]; !ok {
				t.Fatalf("system %d needs part 0x%02x, which is not for sale", system.Index, column.ItemIndex)
			}
		}
		if system.Alternate != nil {
			if _, ok := offered[system.Alternate.ItemIndex]; !ok {
				t.Fatalf("system %d's alternate 0x%02x is not for sale", system.Index, system.Alternate.ItemIndex)
			}
		}
	}
}

// TestBuyAPartThenInstallItRestoresTheRowFully walks the loop the player is
// meant to walk: a hit kills a part, the warehouse sells a replacement, and
// installing the real part brings the row back to four rather than the three a
// shunt leaves behind.
func TestBuyAPartThenInstallItRestoresTheRowFully(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	if !reducer.World.HasFacility(state.CityID, state.Position.X, state.Position.Y, world.FacilityWarehouse) {
		t.Fatalf("the start tile has no warehouse: %+v", state.Position)
	}
	state, err = reducer.Step(state, ActionOpenParts)
	if err != nil || state.Screen != ScreenParts {
		t.Fatalf("the parts counter did not open: state=%+v err=%v", state, err)
	}

	// Break the second column of the warp drive's first row, then buy exactly
	// the part that column wants.
	source, ok := sourceShipSystem(ShipSystemWarpDrive)
	if !ok {
		t.Fatal("warp drive part composition is unavailable")
	}
	wanted := source.Columns[1].ItemIndex
	for index, part := range PartsForSale() {
		if part == wanted {
			state.SelectedPartIndex = index
		}
	}
	state.Bank = 10000
	before := state
	state, err = reducer.Step(state, ActionBuyPart)
	if err != nil {
		t.Fatal(err)
	}
	if state.MessageKey != "ui.message.part_bought" || CarriedPartCount(state, wanted) != 1 {
		t.Fatalf("buying the part did not put it aboard: %+v", state)
	}
	// What a shop charges is the part's own value put through the star system's
	// price multiplier, so the price has to be asked of the shop the player is
	// standing in rather than of the part.
	price, _ := reducer.ShopItemPrice(before, wanted)
	if price <= 0 || state.Bank != 10000-price {
		t.Fatalf("the part cost %d, bank is %d", price, state.Bank)
	}

	state.Screen = ScreenEngineering
	state.SelectedSystemIndex = 0
	state.ShipSystems[0].Rows[0][1] = atarist.DeadPartItemIndex()
	state.ShipSystems[0].Status = shipSystemStatus(state.ShipSystems[0])
	if state.ShipSystems[0].Status != MaxShipSystemStatus()-4 {
		t.Fatalf("breaking one part left status %d, want %d", state.ShipSystems[0].Status, MaxShipSystemStatus()-4)
	}
	state, err = reducer.Step(state, ActionInstallPart)
	if err != nil {
		t.Fatal(err)
	}
	if state.MessageKey != "ui.message.part_installed" {
		t.Fatalf("installing the part reported %q", state.MessageKey)
	}
	if state.ShipSystems[0].Status != MaxShipSystemStatus() {
		t.Fatalf("the real part left status %d, want a whole system", state.ShipSystems[0].Status)
	}
	if CarriedPartCount(state, wanted) != 0 {
		t.Fatalf("installing did not spend the part: %d left", CarriedPartCount(state, wanted))
	}
}

// TestInstallingWithoutAMatchingPartKeepsWhatIsAboard covers the refusal path,
// which has to leave the hold and the system alone.
func TestInstallingWithoutAMatchingPartKeepsWhatIsAboard(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenEngineering
	state.Shunts = 0
	state.SelectedSystemIndex = 0
	// Only the control node is gone, and a shunt never fits there.
	state.ShipSystems[0].Rows[0][0] = atarist.DeadPartItemIndex()
	state.ShipSystems[0].Status = shipSystemStatus(state.ShipSystems[0])
	before := state.ShipSystems[0].Status
	state, err = reducer.Step(state, ActionInstallPart)
	if err != nil {
		t.Fatal(err)
	}
	if state.MessageKey != "ui.message.no_matching_part" {
		t.Fatalf("a missing part reported %q", state.MessageKey)
	}
	if state.ShipSystems[0].Status != before || CarriedItemCount(state) != 0 {
		t.Fatalf("the refusal changed state: %+v", state.ShipSystems[0])
	}
}

// TestShopPriceFollowsTheSystemsOwnLevel is the property the price rule exists
// for: the same part is dearer in an expensive system than in a cheap one, and
// the multiplier never leaves the band the original clamps it to.
func TestShopPriceFollowsTheSystemsOwnLevel(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	var cheapest, dearest world.System
	for _, system := range reducer.World.Systems() {
		if cheapest.ID == "" || system.PriceLevel < cheapest.PriceLevel {
			cheapest = system
		}
		if dearest.ID == "" || system.PriceLevel > dearest.PriceLevel {
			dearest = system
		}
	}
	if cheapest.PriceLevel >= dearest.PriceLevel {
		t.Fatalf("every system has price level %d", cheapest.PriceLevel)
	}

	part := PartsForSale()[0]
	low := NewState()
	low.SystemID = cheapest.ID
	high := NewState()
	high.SystemID = dearest.ID
	lowPrice, ok := reducer.ShopItemPrice(low, part)
	if !ok || lowPrice <= 0 {
		t.Fatalf("the cheap system quotes %d", lowPrice)
	}
	highPrice, _ := reducer.ShopItemPrice(high, part)
	if highPrice <= lowPrice {
		t.Fatalf("%s quotes %d and %s quotes %d", cheapest.ID, lowPrice, dearest.ID, highPrice)
	}

	for _, system := range reducer.World.Systems() {
		state := NewState()
		state.SystemID = system.ID
		scale := reducer.PriceScale(state)
		if scale < priceScaleMinimum || scale > priceScaleMaximum {
			t.Fatalf("system %s scales prices by %d", system.ID, scale)
		}
	}
}

// TestAnOptionalPartCostsLessOnceTheShipHasOne pins the odd half of the rule:
// the four optional parts are cheaper to the player who already carries one,
// and everything else is dearer to the player who does not.
func TestAnOptionalPartCostsLessOnceTheShipHasOne(t *testing.T) {
	optional := tradeInFirstItem
	withNone, ok := itemPrice(optional, false)
	if !ok {
		t.Fatalf("item %d has no price", optional)
	}
	withOne, _ := itemPrice(optional, true)
	if withOne >= withNone {
		t.Fatalf("owning one made item %d cost %d instead of %d", optional, withOne, withNone)
	}

	plain := PartsForSale()[0]
	if plain >= tradeInFirstItem && plain <= tradeInLastItem {
		t.Skip("the first part for sale is one of the optional four")
	}
	plainNone, _ := itemPrice(plain, false)
	plainOne, _ := itemPrice(plain, true)
	if plainNone != plainOne*plainNewFactor {
		t.Fatalf("item %d costs %d without one and %d with one", plain, plainNone, plainOne)
	}
}
