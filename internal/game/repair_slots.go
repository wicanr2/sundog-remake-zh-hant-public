package game

// XDOREPAI:0x0b move_item is the repair window's mouse: press on a slot
// with a part and the part is in the hand (swap_items), following the
// cursor; press on another slot and, if it is empty and the part fits there
// (0x0a fits: the part is what the record expects in that column, or a
// shunt anywhere but the control column), the two swap and syscheck
// rewrites the record; releasing with a part still in hand puts it back
// where it came from. The remake keeps the hand in State.HandItem and does
// the three moves as reducer calls from the window's clicks.

// PickPocketPart selects a fixed body slot. Occupied destinations reject the
// drag and restore its source; removing an item never shifts another slot.
func (r Reducer) PickPocketPart(state State, pocket int) State {
	if pocket < 0 || pocket >= PocketCount {
		return state
	}
	parts := bodySlots(state)
	if state.HandItem != 0 {
		if parts[pocket] != 0 {
			return r.CancelHand(state)
		}
		parts[pocket] = state.HandItem
		clearHand(&state)
	} else if parts[pocket] != 0 {
		state.HandItem = parts[pocket]
		parts[pocket] = 0
		state.HandOrigin = ItemOrigin{Kind: itemOriginPocket, Index: pocket}
	}
	state.Parts = parts
	syncAwakeClockLifecycle(&state)
	return state
}

// DropHandIntoPocket puts the held part into the pockets.
func (r Reducer) DropHandIntoPocket(state State) State {
	if state.HandItem == 0 {
		return state
	}
	for index, item := range bodySlots(state) {
		if item == 0 {
			return r.PickPocketPart(state, index)
		}
	}
	state.MessageKey = "ui.message.carry_full"
	return state
}

// PocketCount is make_man's four pockets.
const PocketCount = 4

// MoveHandToSlot is the press on a system slot: an empty hand picks the
// slot's part up; a held part goes in when it fits, the slot's old part
// (blasted or not) coming out into the hand.
func (r Reducer) MoveHandToSlot(state State, row, column int) (State, bool) {
	system, ok := state.SelectedShipSystem()
	if !ok || row < 0 || row >= len(system.Rows) || column < 0 || column >= len(system.Rows[row]) {
		return state, false
	}
	source, found := sourceShipSystem(system.ID)
	if !found {
		return state, false
	}
	installed := system.Rows[row][column]
	if state.HandItem != 0 {
		if _, accepted := source.Accepts(column, state.HandItem); !accepted {
			state.MessageKey = "ui.message.shunt_does_not_fit"
			return state, false
		}
	} else if installed == 0 {
		return state, false
	}
	repaired := state.ShipSystems[state.SelectedSystemIndex]
	rows := make([][]int, len(repaired.Rows))
	for i := range repaired.Rows {
		rows[i] = append([]int(nil), repaired.Rows[i]...)
	}
	rows[row][column] = state.HandItem
	repaired.Rows = rows
	repaired.Status = shipSystemStatus(repaired)
	systems := append([]ShipSystem(nil), state.ShipSystems...)
	systems[state.SelectedSystemIndex] = repaired
	state.ShipSystems = systems
	state.HandItem = installed
	// The existing repair exchange is outside Stores READY. A successful
	// placement consumes the tracked drag; an extracted part is untracked.
	state.HandOrigin = ItemOrigin{}
	syncAwakeClockLifecycle(&state)
	if installed == 0 {
		state.MessageKey = "ui.message.part_installed"
	}
	return state, true
}
