package game

import "fmt"

// Stores/body transfer rules: banville-restoration-spec.md, Stores READY.
// These are remake slot identities, never original RAM addresses.
type ItemOrigin struct {
	Kind  string `json:"kind"`
	Index int    `json:"index"`
}

const (
	itemOriginStores = "stores"
	itemOriginPocket = "pocket"
	itemBurger       = 8
)

func bodySlots(state State) []int {
	slots := make([]int, PocketCount)
	copy(slots, state.Parts)
	return slots
}

// CarriedItemCount counts occupied body slots, excluding the dragged object.
func CarriedItemCount(state State) int {
	count := 0
	for _, item := range state.Parts {
		if item != 0 {
			count++
		}
	}
	return count
}

// CarriedFoodCount excludes the ship's remote stores.
func CarriedFoodCount(state State) int {
	count := 0
	for _, item := range append(bodySlots(state), state.HandItem) {
		if item != 0 && ItemCode(item) == itemBurger {
			count++
		}
	}
	return count
}

func clearHand(state *State) {
	state.HandItem = 0
	state.HandOrigin = ItemOrigin{}
	syncAwakeClockLifecycle(state)
}

// CancelHand restores the complete byte without moving any other slot.
func (r Reducer) CancelHand(state State) State {
	if state.HandItem == 0 {
		return state
	}
	o := state.HandOrigin
	switch o.Kind {
	case itemOriginStores:
		if o.Index < 0 || o.Index >= len(state.Stores) || state.Stores[o.Index] != 0 {
			return state
		}
		state.Stores = append([]int(nil), state.Stores...)
		state.Stores[o.Index] = state.HandItem
	case itemOriginPocket:
		if o.Index < 0 || o.Index >= PocketCount {
			return state
		}
		slots := bodySlots(state)
		if slots[o.Index] != 0 {
			return state
		}
		slots[o.Index] = state.HandItem
		state.Parts = slots
	default:
		// Legacy repair exchanges do not supply a Stores/body origin.
		return state
	}
	clearHand(&state)
	return state
}

// SelectStoreItem is a click on a fixed Stores slot. An occupied destination
// rejects a held object and cancels the drag, as original move_item does.
func (r Reducer) SelectStoreItem(state State, index int) State {
	if state.Screen != ScreenStores || index < 0 || index >= len(state.Stores) {
		return state
	}
	state.SelectedPartIndex = index
	item := state.Stores[index]
	if state.HandItem != 0 {
		if item != 0 {
			return r.CancelHand(state)
		}
		state.Stores = append([]int(nil), state.Stores...)
		state.Stores[index] = state.HandItem
		clearHand(&state)
	} else if item != 0 {
		state.Stores = append([]int(nil), state.Stores...)
		state.Stores[index] = 0
		state.HandItem = item
		state.HandOrigin = ItemOrigin{Kind: itemOriginStores, Index: index}
	}
	syncAwakeClockLifecycle(&state)
	return state
}

// ValidateItemState rejects malformed current saves rather than synthesizing
// slots or discarding unknown but valid packed item bytes.
func ValidateItemState(state State) error {
	if len(state.Stores) != storeSlots || len(state.Parts) != PocketCount {
		return fmt.Errorf("invalid Stores/body slot count")
	}
	for _, item := range append(append([]int{state.HandItem}, state.Stores...), state.Parts...) {
		if item < 0 || item > 255 {
			return fmt.Errorf("invalid packed item byte %d", item)
		}
	}
	o := state.HandOrigin
	if o.Kind == "" {
		if o.Index != 0 {
			return fmt.Errorf("invalid empty item origin")
		}
		return nil
	}
	if state.HandItem == 0 {
		return fmt.Errorf("item origin without held item")
	}
	var slots []int
	switch o.Kind {
	case itemOriginStores:
		slots = state.Stores
	case itemOriginPocket:
		slots = state.Parts
	default:
		return fmt.Errorf("unknown item origin %q", o.Kind)
	}
	if o.Index < 0 || o.Index >= len(slots) || slots[o.Index] != 0 {
		return fmt.Errorf("invalid or occupied item origin")
	}
	return nil
}
