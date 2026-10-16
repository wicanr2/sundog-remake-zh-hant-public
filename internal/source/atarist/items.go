package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

const (
	itemRecordCount = 32
	// itemNameFieldLimit is the widest name a record can hold. A name this long
	// leaves no room for the trailing value byte, which is why Value can be
	// absent rather than zero.
	itemNameFieldLimit = 15
)

// Entry is one record of the item or cargo table.
type Entry struct {
	Index     int
	ID        string
	Name      string
	RawOffset int
	// Value is the record's trailing byte, which XDOINTER:0x02 set_up_price
	// reads as a pricing input. HasValue is false for the one record whose name
	// fills the slot; the source has no number there and none is invented.
	Value    int
	HasValue bool
	// RawTailByte preserves record offset15 even when it overlaps the name.
	// XDOTRADI reads it unconditionally; HasValue retains the separate field's presence.
	RawTailByte int
}

// ItemData is the validated view of the 32-record item and cargo tables.
type ItemData struct {
	items  []Entry
	cargo  []Entry
	byID   map[string]Entry
	byName map[string]Entry
}

type rawEntry struct {
	Index       string `json:"index"`
	RawOffset   string `json:"rawOffset"`
	Name        string `json:"name"`
	NameLength  int    `json:"nameLength"`
	Value       *int   `json:"value"`
	ValueStatus string `json:"valueStatus"`
	RawTailByte *int   `json:"rawTailByte"`
}

type rawItemTable struct {
	RawOffset   string     `json:"rawOffset"`
	RecordSize  int        `json:"recordSize"`
	RecordCount int        `json:"recordCount"`
	Records     []rawEntry `json:"records"`
}

type itemDocument struct {
	Schema int          `json:"schema"`
	Items  rawItemTable `json:"items"`
	Cargo  rawItemTable `json:"cargo"`
}

// LoadItemData validates both tables before their names or values can reach a
// player-facing screen or a price.
func LoadItemData(reader io.Reader) (ItemData, error) {
	var document itemDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return ItemData{}, fmt.Errorf("decode Atari ST item data: %w", err)
	}
	if document.Schema != 1 {
		return ItemData{}, fmt.Errorf("unsupported Atari ST item data schema %d", document.Schema)
	}
	items, err := convertEntries(document.Items, "item")
	if err != nil {
		return ItemData{}, err
	}
	cargo, err := convertEntries(document.Cargo, "cargo")
	if err != nil {
		return ItemData{}, err
	}

	data := ItemData{
		items:  items,
		cargo:  cargo,
		byID:   make(map[string]Entry, len(items)+len(cargo)),
		byName: make(map[string]Entry, len(items)+len(cargo)),
	}
	for _, entry := range items {
		data.byID["atarist.item."+entry.ID] = entry
		data.byName[entry.Name] = entry
	}
	for _, entry := range cargo {
		data.byID["atarist.cargo."+entry.ID] = entry
	}
	return data, nil
}

func convertEntries(table rawItemTable, kind string) ([]Entry, error) {
	if table.RecordCount != itemRecordCount || len(table.Records) != itemRecordCount {
		return nil, fmt.Errorf("%s table declares %d records and carries %d, want %d", kind, table.RecordCount, len(table.Records), itemRecordCount)
	}
	entries := make([]Entry, 0, itemRecordCount)
	seen := make(map[string]struct{}, itemRecordCount)
	for index, record := range table.Records {
		declared, err := strconv.ParseInt(record.Index, 0, 32)
		if err != nil || int(declared) != index {
			return nil, fmt.Errorf("%s record %d declares index %q", kind, index, record.Index)
		}
		if record.Name == "" || len(record.Name) > itemNameFieldLimit {
			return nil, fmt.Errorf("%s record %d has an invalid name %q", kind, index, record.Name)
		}
		if record.NameLength != len(record.Name) {
			return nil, fmt.Errorf("%s record %d name length %d does not match %q", kind, index, record.NameLength, record.Name)
		}
		offset, err := strconv.ParseInt(record.RawOffset, 0, 64)
		if err != nil || offset < 0 {
			return nil, fmt.Errorf("%s record %d has an invalid raw offset %q", kind, index, record.RawOffset)
		}
		entry := Entry{Index: index, ID: slug(record.Name), Name: record.Name, RawOffset: int(offset)}
		if record.RawTailByte == nil || *record.RawTailByte < 0 || *record.RawTailByte > 255 {
			return nil, fmt.Errorf("%s record %d has missing or invalid raw tail byte", kind, index)
		}
		entry.RawTailByte = *record.RawTailByte
		switch record.ValueStatus {
		case "present":
			if record.Value == nil {
				return nil, fmt.Errorf("%s record %d claims a value and carries none", kind, index)
			}
			if *record.Value < 0 || *record.Value > 0xFF {
				return nil, fmt.Errorf("%s record %d has an out-of-range value %d", kind, index, *record.Value)
			}
			entry.Value = *record.Value
			entry.HasValue = true
			if entry.RawTailByte != entry.Value || len(record.Name) == itemNameFieldLimit {
				return nil, fmt.Errorf("%s record %d tail byte contradicts value field", kind, index)
			}
		case "absent":
			if record.Value != nil {
				return nil, fmt.Errorf("%s record %d claims no value and carries one", kind, index)
			}
			if len(record.Name) != itemNameFieldLimit || entry.RawTailByte != int(record.Name[itemNameFieldLimit-1]) {
				return nil, fmt.Errorf("%s record %d tail byte contradicts full name", kind, index)
			}
		default:
			return nil, fmt.Errorf("%s record %d has unknown value status %q", kind, index, record.ValueStatus)
		}
		if entry.ID == "" {
			return nil, fmt.Errorf("%s record %d name %q produced an empty identity", kind, index, record.Name)
		}
		if _, exists := seen[entry.ID]; exists {
			return nil, fmt.Errorf("%s table has duplicate identity %q", kind, entry.ID)
		}
		seen[entry.ID] = struct{}{}
		entries = append(entries, entry)
	}
	return entries, nil
}

// Items returns the 32 inventory items in source-table order.
func (d ItemData) Items() []Entry {
	result := make([]Entry, len(d.items))
	copy(result, d.items)
	return result
}

// Cargo returns the 32 cargo kinds in source-table order.
func (d ItemData) Cargo() []Entry {
	result := make([]Entry, len(d.cargo))
	copy(result, d.cargo)
	return result
}

// Entry looks a record up by its catalog key, such as atarist.cargo.gold.
func (d ItemData) Entry(key string) (Entry, bool) {
	entry, ok := d.byID[key]
	return entry, ok
}

// ItemByName resolves the part names the ship system tables refer to.
func (d ItemData) ItemByName(name string) (Entry, bool) {
	entry, ok := d.byName[name]
	return entry, ok
}
