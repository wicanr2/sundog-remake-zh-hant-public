package game

import "github.com/wicanr2/sundog-remake/internal/source/atarist"

// ShipSystemID identifies one of the six ship systems. The identities and their
// order are the original's: XDOREPAI's constant pool holds the six display
// names in this order — Warp Drives, Sub-c Engines, Ship's Guns, Tactical
// Disply (the original's own spelling), Force Shields, Pilotage. What each
// system is built from is decoded too: the parts, the alternate part, and the
// rules below all come from the game's own p-code. See
// docs/atari-st/ship-systems.json. Only the ID strings here are remake-owned.
type ShipSystemID string

const (
	ShipSystemWarpDrive       ShipSystemID = "warp-drive"
	ShipSystemSubcEngine      ShipSystemID = "subc-engine"
	ShipSystemShipGuns        ShipSystemID = "ship-guns"
	ShipSystemTacticalDisplay ShipSystemID = "tactical-display"
	ShipSystemForceShields    ShipSystemID = "force-shields"
	ShipSystemPilotage        ShipSystemID = "pilotage"
)

var (
	shipSystemSource   atarist.ShipSystemData
	shipSystemSourceOK bool
)

func init() {
	data, err := atarist.EmbeddedShipSystemData()
	if err != nil {
		return
	}
	shipSystemSource = data
	shipSystemSourceOK = true
}

// ShipSystem is one system's installed parts. Rows and columns are the source's
// own: four rows of four parts, each holding an item index from the Atari ST
// item table. Status is derived from them and is never stored as an independent
// truth.
type ShipSystem struct {
	ID   ShipSystemID `json:"id"`
	Rows [][]int      `json:"rows"`
	// LastHitSlot is the slot the enemy destroyed most recently, as a row-major
	// index. The original keeps it in byte 25 of the system record, which is
	// what SHIPLIB:0x21 update_attack writes when a part is blasted.
	LastHitSlot int `json:"lastHitSlot"`
	Status      int `json:"status"`
}

var shipSystemOrder = [...]ShipSystemID{
	ShipSystemWarpDrive,
	ShipSystemSubcEngine,
	ShipSystemShipGuns,
	ShipSystemTacticalDisplay,
	ShipSystemForceShields,
	ShipSystemPilotage,
}

// MaxShipSystemStatus is a system with all four rows intact.
func MaxShipSystemStatus() int { return atarist.MaxSystemStatus() }

// ShipSystemIDs returns the stable engineering-screen order.
func ShipSystemIDs() []ShipSystemID {
	ids := make([]ShipSystemID, len(shipSystemOrder))
	copy(ids, shipSystemOrder[:])
	return ids
}

// shipSystemIndex maps an identity onto its source index. The order is the
// order the original's new-game initializer uses.
func shipSystemIndex(id ShipSystemID) (int, bool) {
	for index, known := range shipSystemOrder {
		if known == id {
			return index, true
		}
	}
	return 0, false
}

// sourceShipSystem returns the part composition behind an identity.
func sourceShipSystem(id ShipSystemID) (atarist.ShipSystem, bool) {
	if !shipSystemSourceOK {
		return atarist.ShipSystem{}, false
	}
	index, ok := shipSystemIndex(id)
	if !ok {
		return atarist.ShipSystem{}, false
	}
	return shipSystemSource.System(index)
}

// ShipSystemLabelKey returns a UI catalog key without translating the stable
// ID used by saves and tests.
func ShipSystemLabelKey(id ShipSystemID) string {
	switch id {
	case ShipSystemWarpDrive:
		return "ui.system_warp_drive"
	case ShipSystemSubcEngine:
		return "ui.system_subc_engine"
	case ShipSystemShipGuns:
		return "ui.system_ship_guns"
	case ShipSystemTacticalDisplay:
		return "ui.system_tactical_display"
	case ShipSystemForceShields:
		return "ui.system_force_shields"
	case ShipSystemPilotage:
		return "ui.system_pilotage"
	default:
		return "ui.system_unknown"
	}
}

func isKnownShipSystemID(id ShipSystemID) bool {
	_, ok := shipSystemIndex(id)
	return ok
}

// Damage the original writes into a new game's rows. RowStatus subtracts each
// slot's cost from four, so an accepted alternate leaves the row at 3, and both
// an empty slot and a part the column does not accept drop it to 0 — which is
// exactly the shapes XSTARTUP:0x31 produces.
const (
	newGameAlternatePart = 23
	newGameDeadPart      = 7
	newGameEmptySlot     = 0
)

// newGameRows is XSTARTUP:0x31's row loop. Each of the four real rows takes
// random() mod 3, and a zero damages one column picked by random() mod 4.
//
// Which of the two shapes it writes is decided at 0x1acb by
// `(column = 0) or random()`, and what that means was settled by reading the
// interpreter's own routines (docs/atari-st/secondary-gamestate.md):
//
//   - `lor` ($A0) is `or.w d0,(a7)` — a bitwise or, not a logical one.
//   - `fjp` ($D4) is `btst #0,d0` — it branches on bit 0, not on the whole word.
//
// So the test is `(column == 0) | (random() & 1)`, and random()'s range never
// enters into it. False leaves the alternate part and row status 3; true goes
// on to draw once more, its bit 0 choosing between an empty slot and a dead
// part, both of which give row status 0. That makes status 3 come up on
// 3/4 × 1/2 = 3/8 of the damaged rows — the measured 8 of 23 is 34.8%.
//
// p-code does not short-circuit: `lor`'s second operand is a call, so the draw
// happens even when the column is 0. The rolls below keep that order.
func newGameRows(id ShipSystemID, roll func(int) int) [][]int {
	rows := intactRows(id)
	if rows == nil {
		return nil
	}
	for row := range rows {
		if roll(3) != 0 {
			continue
		}
		column := roll(4)
		if column >= len(rows[row]) {
			continue
		}
		odd := roll(2) == 1
		if column != 0 && !odd {
			rows[row][column] = newGameAlternatePart
			continue
		}
		if roll(2) == 1 {
			rows[row][column] = newGameEmptySlot
		} else {
			rows[row][column] = newGameDeadPart
		}
	}
	return rows
}

// intactRows builds the four rows a system starts with: every slot holding the
// part that belongs in it. It is what a repaired system looks like; a new game
// runs newGameRows over it.
func intactRows(id ShipSystemID) [][]int {
	source, ok := sourceShipSystem(id)
	if !ok {
		return nil
	}
	rows := make([][]int, atarist.RowCount())
	for row := range rows {
		slots := make([]int, len(source.Columns))
		for column, part := range source.Columns {
			slots[column] = part.ItemIndex
		}
		rows[row] = slots
	}
	return rows
}

// defaultShipSystems builds the six systems a new game starts with, damage and
// all. The seed makes it reproducible: the same seed gives the same ship.
func defaultShipSystems(seed int64) []ShipSystem {
	state := uint64(seed)*6364136223846793005 + 1442695040888963407
	roll := func(n int) int {
		state = state*6364136223846793005 + 1442695040888963407
		return int((state >> 33) % uint64(n))
	}
	systems := make([]ShipSystem, 0, len(shipSystemOrder))
	for _, id := range shipSystemOrder {
		system := ShipSystem{ID: id, Rows: newGameRows(id, roll)}
		system.Status = shipSystemStatus(system)
		systems = append(systems, system)
	}
	return systems
}

// IntactShipSystems is the six systems with every part where the source table
// puts it: the ship a new game would have had if XSTARTUP:0x31 had rolled no
// damage. It is for the diagnostic entry points that need a ship able to fly;
// a real new game goes through defaultShipSystems and keeps the damage.
func IntactShipSystems() []ShipSystem {
	systems := make([]ShipSystem, 0, len(shipSystemOrder))
	for _, id := range shipSystemOrder {
		system := ShipSystem{ID: id, Rows: intactRows(id)}
		system.Status = shipSystemStatus(system)
		systems = append(systems, system)
	}
	return systems
}

// shipSystemStatus sums the four row statuses, which is what the original
// stores as the system's own status.
func shipSystemStatus(system ShipSystem) int {
	source, ok := sourceShipSystem(system.ID)
	if !ok {
		return 0
	}
	total := 0
	for _, row := range system.Rows {
		total += source.RowStatus(row)
	}
	return total
}

// ShipSystemRowStatus reports one row's status so a shell can show which row is
// carrying the damage.
func ShipSystemRowStatus(system ShipSystem, row int) int {
	source, ok := sourceShipSystem(system.ID)
	if !ok || row < 0 || row >= len(system.Rows) {
		return 0
	}
	return source.RowStatus(system.Rows[row])
}

// ShipSystemPartName returns the source name of the part in a slot, so a shell
// can name what is installed rather than print a number.
func ShipSystemPartName(system ShipSystem, row, column int) (string, bool) {
	if row < 0 || row >= len(system.Rows) {
		return "", false
	}
	slots := system.Rows[row]
	if column < 0 || column >= len(slots) {
		return "", false
	}
	items, err := atarist.EmbeddedItemData()
	if err != nil {
		return "", false
	}
	table := items.Items()
	item := slots[column]
	if item < 0 || item >= len(table) {
		return "", false
	}
	return table[item].Name, true
}

// normalizeShipSystems gives a restored save a deterministic six-system shape
// and recomputes every status from the parts actually installed. A save can
// carry stale numbers; the parts are the truth.
func normalizeShipSystems(state *State) {
	existing := make(map[ShipSystemID]ShipSystem, len(state.ShipSystems))
	for _, system := range state.ShipSystems {
		if !isKnownShipSystemID(system.ID) {
			continue
		}
		existing[system.ID] = system
	}
	systems := make([]ShipSystem, 0, len(shipSystemOrder))
	for _, id := range shipSystemOrder {
		system := ShipSystem{ID: id, Rows: intactRows(id)}
		if previous, ok := existing[id]; ok && validShipSystemRows(previous.Rows) {
			// The rows have to be copied, not aliased. A reducer step starts
			// from a copy of the state, but a copied struct still shares the
			// slices underneath it — mutating a row in place would reach back
			// into the caller's state and quietly break the reducer's promise
			// that stepping does not touch its input.
			system.Rows = copyShipSystemRows(previous.Rows)
			system.LastHitSlot = previous.LastHitSlot
		}
		if system.LastHitSlot < 0 || system.LastHitSlot >= atarist.RowCount()*atarist.PartColumns() {
			system.LastHitSlot = 0
		}
		system.Status = shipSystemStatus(system)
		systems = append(systems, system)
	}
	state.ShipSystems = systems
	if state.SelectedSystemIndex < 0 || state.SelectedSystemIndex >= len(state.ShipSystems) {
		state.SelectedSystemIndex = 0
	}
}

// copyShipSystemRows gives a system its own grid.
func copyShipSystemRows(rows [][]int) [][]int {
	result := make([][]int, len(rows))
	for index, row := range rows {
		result[index] = make([]int, len(row))
		copy(result[index], row)
	}
	return result
}

// validShipSystemRows rejects a save whose grid is not the shape the source
// describes, rather than indexing into it and hoping.
func validShipSystemRows(rows [][]int) bool {
	if len(rows) != atarist.RowCount() {
		return false
	}
	for _, row := range rows {
		if len(row) != atarist.PartColumns() {
			return false
		}
		for _, item := range row {
			if item < 0 || item > 0xFF {
				return false
			}
		}
	}
	return true
}

// SelectedShipSystem returns a copy so callers cannot mutate state without a
// reducer action.
func (state State) SelectedShipSystem() (ShipSystem, bool) {
	if state.SelectedSystemIndex < 0 || state.SelectedSystemIndex >= len(state.ShipSystems) {
		return ShipSystem{}, false
	}
	return state.ShipSystems[state.SelectedSystemIndex], true
}

func cycleSelectedShipSystem(state *State, delta int) {
	if len(state.ShipSystems) == 0 {
		return
	}
	index := (state.SelectedSystemIndex + delta) % len(state.ShipSystems)
	if index < 0 {
		index += len(state.ShipSystems)
	}
	state.SelectedSystemIndex = index
}

// brokenSlots lists the slots of a row that hold something which does not
// belong there.
func brokenSlots(source atarist.ShipSystem, row []int) []int {
	broken := make([]int, 0, len(row))
	for column, item := range row {
		if _, accepted := source.Accepts(column, item); !accepted {
			broken = append(broken, column)
		}
	}
	return broken
}

// shuntTarget picks the slot a shunt goes into. The original lets the player
// drop a shunt into any broken slot but the control node's; which one is worth
// the shunt is the player's judgement there and the remake's here, so it picks
// the row closest to working again. A row whose control node is gone is skipped
// because no number of shunts can bring it back.
func shuntTarget(system ShipSystem) (row, column int, ok bool) {
	source, found := sourceShipSystem(system.ID)
	if !found {
		return 0, 0, false
	}
	bestCount := 0
	for rowIndex, slots := range system.Rows {
		broken := brokenSlots(source, slots)
		if len(broken) == 0 || broken[0] == 0 {
			continue
		}
		if bestCount == 0 || len(broken) < bestCount {
			bestCount = len(broken)
			row, column, ok = rowIndex, broken[0], true
		}
	}
	return row, column, ok
}

// installShunt puts a shunt into the first broken slot of a system. It reports
// false when nothing is broken, or when the only damage is in the control
// node's column, where the original does not let a shunt go.
func installShunt(system *ShipSystem) bool {
	row, column, ok := shuntTarget(*system)
	if !ok {
		return false
	}
	system.Rows[row][column] = atarist.ShuntItemIndex()
	system.Status = shipSystemStatus(*system)
	return true
}

// breakShipSystemPart is what damage does to a system: it kills the part in the
// first slot that still works, leaving the dead-part marker the original uses.
// Which system takes the hit is decoded — SHIPLIB:0x21 rolls random() mod 7,
// six systems and the hull (rule attack.target-roll), and records the blasted
// slot in the record's byte 25 (attack.last-hit-slot). Picking the first live
// slot instead of rolling is remake-owned and deterministic, so a replay lands
// on the same state.
func breakShipSystemPart(state *State, id ShipSystemID) {
	source, ok := sourceShipSystem(id)
	if !ok {
		return
	}
	for index := range state.ShipSystems {
		if state.ShipSystems[index].ID != id {
			continue
		}
		system := &state.ShipSystems[index]
		for row, slots := range system.Rows {
			// Within a row the control node is hit last. The original does not
			// say which slot a hit lands on; taking the shuntable parts first
			// is remake-owned and keeps a damaged row repairable, which is what
			// carrying shunts is for.
			for _, column := range append(shuntableColumns(len(slots)), 0) {
				if _, accepted := source.Accepts(column, slots[column]); !accepted {
					continue
				}
				system.Rows[row][column] = atarist.DeadPartItemIndex()
				system.Status = shipSystemStatus(*system)
				state.SelectedSystemIndex = index
				return
			}
		}
		return
	}
}

// shuntableColumns lists the columns a shunt may occupy, in order.
func shuntableColumns(width int) []int {
	columns := make([]int, 0, width)
	for column := 1; column < width; column++ {
		columns = append(columns, column)
	}
	return columns
}

// alternatePartFitted reports whether a system is carrying its alternate part
// in the slot that part belongs in. This is MAINLIB_G53 bit 16 + n, which
// SHIPLIB:0x0e update_system recomputes on every call: it clears the bit, then
// sets it when the designated slot holds the alternate and that row is not
// dead. Two systems have no alternate at all, so their bits are always clear.
//
// Only the ship's guns' bit does visible work so far -- fire_it reads it to
// pick the shot's base -- but the other three are the same shape.
func alternatePartFitted(state State, id ShipSystemID) bool {
	source, ok := sourceShipSystem(id)
	if !ok || source.Alternate == nil {
		return false
	}
	column := source.Alternate.Column
	for _, system := range state.ShipSystems {
		if system.ID != id {
			continue
		}
		for index, row := range system.Rows {
			if column < 0 || column >= len(row) {
				continue
			}
			if row[column] != source.Alternate.ItemIndex {
				continue
			}
			// "that row is not dead": update_system only sets the bit when the
			// row it found the part in still has status.
			if ShipSystemRowStatus(system, index) > 0 {
				return true
			}
		}
	}
	return false
}
