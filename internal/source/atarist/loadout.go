package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// The new-game loadout is what XSTARTUP:0x32 set_up_lockers writes into the
// game state: a run of shunts and the ship's stores. The decoder that produced
// this document reads those p-code writes; see
// tools/atari_st_new_game_loadout.py.

// LoadoutKind is the shape of one write. An item write puts a part or a store
// item in place; a range write puts a number in a state byte that is not an
// item at all, which is how the hull is initialized.
type LoadoutKind string

const (
	LoadoutItem   LoadoutKind = "item"
	LoadoutRange  LoadoutKind = "range"
	LoadoutUnread LoadoutKind = "unrecognised"
)

// LoadoutWrite is one write of the new-game initializer.
type LoadoutWrite struct {
	Kind        LoadoutKind
	StateOffset int
	Count       int
	// RawValue keeps the byte as written. The item code is its low five bits
	// and the remaining charges are its high three bits, so the shield's 0xe0
	// means seven. See docs/atari-st/ground-rules.md.
	RawValue int
	ItemCode int
	ItemName string
	// Minimum and Maximum bound a range write. The original rolls within them;
	// which value it picks is not a constant to reproduce.
	Minimum int
	Maximum int
	// Meaning names what a range write initializes, when that is known.
	Meaning string
}

// LoadoutData is the validated view of the new-game loadout.
type LoadoutData struct {
	writes []LoadoutWrite
}

type loadoutDocument struct {
	Schema int `json:"schema"`
	Writes []struct {
		Kind        string `json:"kind"`
		StateOffset string `json:"stateOffset"`
		Count       int    `json:"count"`
		RawValue    string `json:"rawValue"`
		ItemIndex   string `json:"itemIndex"`
		ItemName    string `json:"itemName"`
		Minimum     int    `json:"minimum"`
		Maximum     int    `json:"maximum"`
		Meaning     string `json:"meaning"`
	} `json:"writes"`
}

// LoadLoadoutData validates the document before any of it reaches a rules layer.
func LoadLoadoutData(reader io.Reader) (LoadoutData, error) {
	var document loadoutDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return LoadoutData{}, fmt.Errorf("decode Atari ST loadout data: %w", err)
	}
	if document.Schema != 1 {
		return LoadoutData{}, fmt.Errorf("unsupported Atari ST loadout data schema %d", document.Schema)
	}
	if len(document.Writes) == 0 {
		return LoadoutData{}, fmt.Errorf("loadout data carries no writes")
	}

	writes := make([]LoadoutWrite, 0, len(document.Writes))
	for index, entry := range document.Writes {
		offset, err := strconv.ParseInt(entry.StateOffset, 0, 32)
		if err != nil || offset < 0 || offset > 0xFF {
			return LoadoutData{}, fmt.Errorf("loadout write %d has an invalid state offset %q", index, entry.StateOffset)
		}
		write := LoadoutWrite{StateOffset: int(offset), Count: 1}
		switch entry.Kind {
		case "unrecognised":
			write.Kind = LoadoutUnread
			writes = append(writes, write)
			continue
		case "randomRange":
			if entry.Maximum < entry.Minimum || entry.Minimum < 0 {
				return LoadoutData{}, fmt.Errorf("loadout write %d has range %d..%d", index, entry.Minimum, entry.Maximum)
			}
			write.Kind = LoadoutRange
			write.Minimum = entry.Minimum
			write.Maximum = entry.Maximum
			write.Meaning = entry.Meaning
			writes = append(writes, write)
			continue
		case "fillchar", "store":
			write.Kind = LoadoutItem
		default:
			return LoadoutData{}, fmt.Errorf("loadout write %d has unknown kind %q", index, entry.Kind)
		}
		if entry.Count <= 0 {
			return LoadoutData{}, fmt.Errorf("loadout write %d writes %d bytes", index, entry.Count)
		}
		raw, err := strconv.ParseInt(entry.RawValue, 0, 32)
		if err != nil || raw < 0 || raw > 0xFF {
			return LoadoutData{}, fmt.Errorf("loadout write %d has an invalid raw value %q", index, entry.RawValue)
		}
		code, err := strconv.ParseInt(entry.ItemIndex, 0, 32)
		if err != nil || code < 0 || code > itemCodeMask {
			return LoadoutData{}, fmt.Errorf("loadout write %d has an invalid item index %q", index, entry.ItemIndex)
		}
		if int(raw)&itemCodeMask != int(code) {
			return LoadoutData{}, fmt.Errorf("loadout write %d raw value %q does not carry item %q", index, entry.RawValue, entry.ItemIndex)
		}
		if entry.ItemName == "" {
			return LoadoutData{}, fmt.Errorf("loadout write %d has no item name", index)
		}
		write.Count = entry.Count
		write.RawValue = int(raw)
		write.ItemCode = int(code)
		write.ItemName = entry.ItemName
		writes = append(writes, write)
	}
	return LoadoutData{writes: writes}, nil
}

// itemCodeMask is the five bits an item code occupies in a stored byte.
const itemCodeMask = 0x1F

// Writes returns the loadout in source order.
func (d LoadoutData) Writes() []LoadoutWrite {
	result := make([]LoadoutWrite, len(d.writes))
	copy(result, d.writes)
	return result
}

// ShuntCount is how many shunts a new game carries.
func (d LoadoutData) ShuntCount() int {
	total := 0
	for _, write := range d.writes {
		if write.Kind == LoadoutItem && write.ItemCode == shuntItemIndex {
			total += write.Count
		}
	}
	return total
}

// Stores lays the item writes out as the block the original fills, using the
// lowest state offset among them as slot zero. Shunts are counted separately
// and the hull is not an item, so neither reaches the stores; slots the
// initializer does not touch stay empty.
func (d LoadoutData) Stores(size int) []int {
	if size <= 0 {
		return nil
	}
	base, ok := d.storesBase()
	if !ok {
		return make([]int, size)
	}
	stores := make([]int, size)
	for _, write := range d.writes {
		if write.Kind != LoadoutItem || write.ItemCode == shuntItemIndex {
			continue
		}
		for slot := 0; slot < write.Count; slot++ {
			index := write.StateOffset - base + slot
			if index < 0 || index >= size {
				continue
			}
			stores[index] = write.RawValue
		}
	}
	return stores
}

// Complete reports whether every write in the run was decoded.
func (d LoadoutData) Complete() bool {
	for _, write := range d.writes {
		if write.Kind == LoadoutUnread {
			return false
		}
	}
	return true
}

// HullRange is what a new game rolls the hull between. The original picks a
// value in it at random; a consumer that wants a reproducible new game has to
// choose, and that choice is the consumer's to make and to mark.
func (d LoadoutData) HullRange() (minimum, maximum int, ok bool) {
	for _, write := range d.writes {
		if write.Kind == LoadoutRange {
			return write.Minimum, write.Maximum, true
		}
	}
	return 0, 0, false
}

func (d LoadoutData) storesBase() (int, bool) {
	base := 0
	found := false
	for _, write := range d.writes {
		if write.Kind != LoadoutItem || write.ItemCode == shuntItemIndex {
			continue
		}
		if !found || write.StateOffset < base {
			base = write.StateOffset
			found = true
		}
	}
	return base, found
}
