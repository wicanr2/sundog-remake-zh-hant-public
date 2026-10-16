package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// The ship system tables describe what each of the six systems is built from.
// The numbers come from the game's own new-game initializer rather than from a
// data table, so the decoder that produced this document reads p-code call
// sites; see tools/atari_st_ship_systems.py.

const (
	shipSystemCount = 6
	// partColumns is the number of part slots in a row. Column 0 is always the
	// control node and is written by the initializer rather than passed in.
	partColumns = 4
	// shuntItemIndex is the part that stands in for a missing one. It is
	// accepted in any column but the control node's.
	shuntItemIndex = 0x17
	// deadPartItemIndex is what a damaged part becomes when the repair screen
	// normalizes the system.
	deadPartItemIndex = 0x07
	// rowWorkingStatus is a row with every part in place; a shunt costs one
	// point and a wrong or missing part drops the row to zero.
	rowWorkingStatus = 4
	rowCount         = 4
	// maxSystemStatus is the sum over the four rows.
	maxSystemStatus = rowWorkingStatus * rowCount
)

// ShipSystemColumn is one part slot of a system, with the part the source says
// belongs in it.
type ShipSystemColumn struct {
	Column    int
	ItemIndex int
	ItemName  string
}

// ShipSystemAlternate is a system's optional second-choice part: a specific
// item that the source accepts in one specific column.
type ShipSystemAlternate struct {
	ItemIndex int
	ItemName  string
	Column    int
}

// ShipSystem is one of the six systems and the four parts it is built from.
type ShipSystem struct {
	Index     int
	Columns   []ShipSystemColumn
	Alternate *ShipSystemAlternate
	RawOffset int
}

// ShipSystemData is the validated view of the six ship systems.
type ShipSystemData struct {
	systems []ShipSystem
}

type shipSystemsDocument struct {
	Schema  int `json:"schema"`
	Systems []struct {
		Index     int    `json:"index"`
		RawOffset string `json:"rawOffset"`
		Columns   []struct {
			Column    int    `json:"column"`
			ItemIndex string `json:"itemIndex"`
			ItemName  string `json:"itemName"`
		} `json:"columns"`
		Alternate *struct {
			ItemIndex string `json:"itemIndex"`
			ItemName  string `json:"itemName"`
			Column    int    `json:"column"`
		} `json:"alternate"`
	} `json:"systems"`
}

// LoadShipSystemData validates the document before any of it can reach a rules
// layer. A composition that does not describe six systems of four columns each
// is a decoding failure, not something to work around.
func LoadShipSystemData(reader io.Reader) (ShipSystemData, error) {
	var document shipSystemsDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return ShipSystemData{}, fmt.Errorf("decode Atari ST ship system data: %w", err)
	}
	if document.Schema != 1 {
		return ShipSystemData{}, fmt.Errorf("unsupported Atari ST ship system data schema %d", document.Schema)
	}
	if len(document.Systems) != shipSystemCount {
		return ShipSystemData{}, fmt.Errorf("ship system data describes %d systems, want %d", len(document.Systems), shipSystemCount)
	}

	systems := make([]ShipSystem, 0, shipSystemCount)
	for position, entry := range document.Systems {
		if entry.Index != position {
			return ShipSystemData{}, fmt.Errorf("ship system %d declares index %d", position, entry.Index)
		}
		offset, err := strconv.ParseInt(entry.RawOffset, 0, 64)
		if err != nil || offset < 0 {
			return ShipSystemData{}, fmt.Errorf("ship system %d has an invalid raw offset %q", position, entry.RawOffset)
		}
		if len(entry.Columns) != partColumns {
			return ShipSystemData{}, fmt.Errorf("ship system %d has %d columns, want %d", position, len(entry.Columns), partColumns)
		}
		columns := make([]ShipSystemColumn, 0, partColumns)
		for column, source := range entry.Columns {
			if source.Column != column {
				return ShipSystemData{}, fmt.Errorf("ship system %d column %d declares position %d", position, column, source.Column)
			}
			item, err := strconv.ParseInt(source.ItemIndex, 0, 32)
			if err != nil || item < 0 || item > 0xFF {
				return ShipSystemData{}, fmt.Errorf("ship system %d column %d has an invalid item %q", position, column, source.ItemIndex)
			}
			if source.ItemName == "" {
				return ShipSystemData{}, fmt.Errorf("ship system %d column %d has no item name", position, column)
			}
			if item == shuntItemIndex {
				return ShipSystemData{}, fmt.Errorf("ship system %d column %d wants a shunt as its own part", position, column)
			}
			columns = append(columns, ShipSystemColumn{Column: column, ItemIndex: int(item), ItemName: source.ItemName})
		}

		system := ShipSystem{Index: position, Columns: columns, RawOffset: int(offset)}
		if entry.Alternate != nil {
			item, err := strconv.ParseInt(entry.Alternate.ItemIndex, 0, 32)
			if err != nil || item < 0 || item > 0xFF {
				return ShipSystemData{}, fmt.Errorf("ship system %d has an invalid alternate item %q", position, entry.Alternate.ItemIndex)
			}
			if entry.Alternate.Column <= 0 || entry.Alternate.Column >= partColumns {
				return ShipSystemData{}, fmt.Errorf("ship system %d has an alternate in column %d", position, entry.Alternate.Column)
			}
			if entry.Alternate.ItemName == "" {
				return ShipSystemData{}, fmt.Errorf("ship system %d has an unnamed alternate part", position)
			}
			system.Alternate = &ShipSystemAlternate{
				ItemIndex: int(item),
				ItemName:  entry.Alternate.ItemName,
				Column:    entry.Alternate.Column,
			}
		}
		systems = append(systems, system)
	}
	return ShipSystemData{systems: systems}, nil
}

// Systems returns the six systems in source order.
func (d ShipSystemData) Systems() []ShipSystem {
	result := make([]ShipSystem, len(d.systems))
	for index, system := range d.systems {
		result[index] = system
		result[index].Columns = make([]ShipSystemColumn, len(system.Columns))
		copy(result[index].Columns, system.Columns)
		if system.Alternate != nil {
			alternate := *system.Alternate
			result[index].Alternate = &alternate
		}
	}
	return result
}

// System returns one system by its source index.
func (d ShipSystemData) System(index int) (ShipSystem, bool) {
	if index < 0 || index >= len(d.systems) {
		return ShipSystem{}, false
	}
	return d.Systems()[index], true
}

// Accepts reports whether a part may occupy a column of this system, and the
// row status it leaves behind. It reproduces the two checks the original makes:
// SHIPLIB:0x0e update_system decides whether a row still works, and
// XDOREPAI:0x0a fits decides whether the player may drop a part into a slot.
//
// A shunt keeps the row alive but costs it a point, and it is refused in the
// control node's column. A wrong part leaves the row dead.
func (s ShipSystem) Accepts(column, itemIndex int) (cost int, ok bool) {
	if column < 0 || column >= len(s.Columns) {
		return 0, false
	}
	if itemIndex == shuntItemIndex {
		if column == 0 {
			return 0, false
		}
		return 1, true
	}
	if itemIndex == s.Columns[column].ItemIndex {
		return 0, true
	}
	if s.Alternate != nil && s.Alternate.Column == column && s.Alternate.ItemIndex == itemIndex {
		return 0, true
	}
	return 0, false
}

// RowStatus is the status of one row of installed parts: four when every part
// belongs, less one for each shunt, and zero as soon as one slot holds
// something that does not belong there.
func (s ShipSystem) RowStatus(row []int) int {
	if len(row) != len(s.Columns) {
		return 0
	}
	status := rowWorkingStatus
	for column, item := range row {
		cost, ok := s.Accepts(column, item)
		if !ok {
			return 0
		}
		status -= cost
	}
	if status < 0 {
		return 0
	}
	return status
}

// ShuntItemIndex is the item every system accepts in place of a missing part,
// outside the control node's column.
func ShuntItemIndex() int { return shuntItemIndex }

// DeadPartItemIndex is what the original leaves in a slot whose part is broken.
func DeadPartItemIndex() int { return deadPartItemIndex }

// MaxSystemStatus is a system with all four rows intact.
func MaxSystemStatus() int { return maxSystemStatus }

// RowCount is the number of part rows a system carries.
func RowCount() int { return rowCount }

// PartColumns is the number of part slots in a row.
func PartColumns() int { return partColumns }
