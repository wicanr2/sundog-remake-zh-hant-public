package game

// Walking into a building in the original does not open a menu -- it puts the
// player inside a room and lets them keep walking. XMOVEINB is twenty-eight
// procedures of rooms, booths, counters, doors, a bartender, customers, and a
// slot machine in the corner.
//
// Which building a door leads to is the original's: terrain codes 11 to 14 are
// the four building doors and XMOVEONG:0x1e check_building turns them into
// interaction kinds 12, 13, 14 and 23 (rule city.facility-door in
// docs/atari-st/ship-rules.json). get_building then reads a per-kind table for
// the room's height, its two backdrop images, how many counter positions it
// has and a price (rule interior.kind-dispatch).
//
// That jump table has now been read through, so most of what follows is the
// original's: how many customers each building holds, how fast they arrive,
// which one has the slot machine and which way its counter runs. What stays
// remake-owned is the room's size and where the furniture sits in it. The
// original identifies the room by a style constant it hands make_zoom, and
// opens the window at the player's own cell on the city map -- there is no
// width and height to copy.

// InteractionKind is the original's own numbering for what a door opens onto.
type InteractionKind int

const (
	KindBarCounter    InteractionKind = 12
	KindEquipmentShop InteractionKind = 13
	KindPartsShop     InteractionKind = 14
	KindHotel         InteractionKind = 23
)

// interiorKindByTerrain is check_building's mapping from the code under the
// player's feet to the interaction kind.
var interiorKindByTerrain = map[int]InteractionKind{
	11: KindBarCounter,
	12: KindEquipmentShop,
	13: KindPartsShop,
	14: KindHotel,
}

// InteriorKindForTerrain reports which building a terrain code opens.
func InteriorKindForTerrain(code int) (InteractionKind, bool) {
	kind, ok := interiorKindByTerrain[code]
	return kind, ok
}

// InteriorLabelKey is the catalogue key naming a building kind.
func InteriorLabelKey(kind InteractionKind) string {
	switch kind {
	case KindBarCounter:
		return "ui.interior.kind.bar"
	case KindEquipmentShop:
		return "ui.interior.kind.equipment"
	case KindPartsShop:
		return "ui.interior.kind.parts"
	case KindHotel:
		return "ui.interior.kind.hotel"
	default:
		return "ui.interior.kind.unknown"
	}
}

const (
	// The original does not size the room from the caller. init_building's two
	// clamps are on the player's cell position on the city map -- update_position
	// writes the sprite coordinates divided by eight into those var parameters,
	// and make_zoom uses them to decide where the pop-up window opens. The room
	// itself is make_zoom's first argument, the per-building style constant.
	// The remake draws the room in a fixed panel, so the sizes below are its
	// own; these bounds are kept because a room outside them could not be
	// anchored the way the original anchors it.
	interiorMinWidth  = 2
	interiorMaxWidth  = 17
	interiorMinHeight = 2
	interiorMaxHeight = 7
	// interiorMinCustomers is the low end of get_building's rand(3, cap).
	interiorMinCustomers = 3
	// interiorArrivalNumerator is the 10 in "random() mod divisor < 10", the
	// test handle_customers makes on every update.
	interiorArrivalNumerator = 10
)

// interiorLayout is one building kind's room. CustomerCap, ArrivalDivisor,
// HasSlots, VerticalCounter and Backdrop are the original's, read out of
// get_building's jump table; the size and the furniture positions are the
// remake's, bounded by the clamp above.
type interiorLayout struct {
	Width, Height int
	Counter       Position
	Booths        []Position
	// HasSlots is the L12 flag get_building sets. Only the bar has it.
	HasSlots bool
	SlotsAt  Position
	// CustomerCap is how many people the room holds; ArrivalDivisor is the
	// divisor in the arrival roll, so a smaller number is a busier room.
	CustomerCap    int
	ArrivalDivisor int
	// VerticalCounter is the L16 flag, cleared only for the parts shop, which
	// makes its counter run the other way.
	VerticalCounter bool
	// Backdrop is the pair of image indices get_building loads for the room.
	Backdrop [2]int
}

var interiorLayouts = map[InteractionKind]interiorLayout{
	KindBarCounter: {
		Width: 11, Height: 7,
		Counter:  Position{X: 5, Y: 1},
		Booths:   []Position{{X: 1, Y: 4}, {X: 5, Y: 4}, {X: 9, Y: 4}},
		HasSlots: true, SlotsAt: Position{X: 9, Y: 1},
		CustomerCap: 7, ArrivalDivisor: 50,
		Backdrop: [2]int{29, 30},
	},
	KindEquipmentShop: {
		Width: 9, Height: 5,
		Counter:     Position{X: 4, Y: 1},
		CustomerCap: 4, ArrivalDivisor: 200,
		Backdrop: [2]int{25, 26},
	},
	KindPartsShop: {
		Width: 9, Height: 5,
		Counter:     Position{X: 4, Y: 1},
		CustomerCap: 4, ArrivalDivisor: 100,
		VerticalCounter: true,
		Backdrop:        [2]int{27, 28},
	},
	KindHotel: {
		Width: 9, Height: 6,
		Counter:     Position{X: 4, Y: 1},
		CustomerCap: 4, ArrivalDivisor: 300,
		Backdrop: [2]int{41, 42},
	},
}

// Interior is the room the player is standing in.
type Interior struct {
	Active bool            `json:"active"`
	Kind   InteractionKind `json:"kind"`
	// Position is where the player stands inside the room, in room cells.
	Position Position `json:"position"`
	// Width and Height are the room's walkable extent.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Counter, Booths and SlotsAt are the furniture the player can step up to.
	Counter  Position   `json:"counter"`
	Booths   []Position `json:"booths"`
	HasSlots bool       `json:"hasSlots"`
	SlotsAt  Position   `json:"slotsAt"`
	// Customers is how many people are in the room, and CustomerCap is how many
	// it holds. ArrivalDivisor is the divisor in the arrival roll.
	Customers      int `json:"customers"`
	CustomerCap    int `json:"customerCap"`
	ArrivalDivisor int `json:"arrivalDivisor"`
	// Rolls counts the arrival draws so a replay reproduces.
	Rolls int `json:"rolls"`
	// ReturnPosition is the city cell to put the player back on when they
	// leave, so a door does not move them.
	ReturnPosition Position `json:"returnPosition"`
}

// enterInterior builds the room for one building kind. The player starts at
// the door, which is the middle of the bottom wall.
func enterInterior(kind InteractionKind, from Position, seed int) Interior {
	layout, ok := interiorLayouts[kind]
	if !ok {
		layout = interiorLayouts[KindEquipmentShop]
	}
	booths := make([]Position, len(layout.Booths))
	copy(booths, layout.Booths)
	interior := Interior{
		Active:         true,
		Kind:           kind,
		Position:       Position{X: layout.Width / 2, Y: layout.Height - 1},
		Width:          clampInterior(layout.Width, interiorMinWidth, interiorMaxWidth),
		Height:         clampInterior(layout.Height, interiorMinHeight, interiorMaxHeight),
		Counter:        layout.Counter,
		Booths:         booths,
		HasSlots:       layout.HasSlots,
		SlotsAt:        layout.SlotsAt,
		CustomerCap:    layout.CustomerCap,
		ArrivalDivisor: layout.ArrivalDivisor,
		Rolls:          seed,
		ReturnPosition: from,
	}
	// get_building opens the room with rand(3, cap) people already in it.
	interior.Customers = interiorMinCustomers +
		interiorRoll(&interior, layout.CustomerCap-interiorMinCustomers+1)
	return interior
}

func clampInterior(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// interiorRoll is the remake's reproducible draw, standing in for the
// original's RNG the same way the rest of the remake does.
func interiorRoll(interior *Interior, span int) int {
	if span <= 0 {
		return 0
	}
	interior.Rolls++
	mixed := uint32(interior.Rolls)*2891336453 + 1640531527
	mixed ^= mixed >> 14
	return int(mixed % uint32(span))
}

// admitCustomer is handle_customers' arrival test: while the room is not full,
// one person walks in whenever random() mod divisor lands under ten. A smaller
// divisor is a busier room -- the bar's 50 against the hotel's 300.
func admitCustomer(interior *Interior) bool {
	if interior.Customers >= interior.CustomerCap {
		return false
	}
	if interiorRoll(interior, interior.ArrivalDivisor) >= interiorArrivalNumerator {
		return false
	}
	interior.Customers++
	return true
}

// AtDoor reports whether the player is standing on the room's exit.
func (i Interior) AtDoor() bool {
	return i.Position.Y >= i.Height-1 && i.Position.X == i.Width/2
}

// AtCounter reports whether the player is next to the counter.
func (i Interior) AtCounter() bool {
	return adjacent(i.Position, i.Counter)
}

// AtSlots reports whether the player is next to the slot machine.
func (i Interior) AtSlots() bool {
	return i.HasSlots && adjacent(i.Position, i.SlotsAt)
}

// BoothIndex reports which booth the player is next to, or -1.
func (i Interior) BoothIndex() int {
	for index, booth := range i.Booths {
		if adjacent(i.Position, booth) {
			return index
		}
	}
	return -1
}

// adjacent is a one-cell reach in any of the four directions, or standing on
// the cell itself.
func adjacent(a, b Position) bool {
	dx := a.X - b.X
	dy := a.Y - b.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx+dy <= 1
}

// moveInterior walks one cell and reports whether the step was legal. Walls
// stop the player; so does the furniture, which is what makes standing next to
// the counter a place rather than a cell.
func moveInterior(interior *Interior, dx, dy int) bool {
	next := Position{X: interior.Position.X + dx, Y: interior.Position.Y + dy}
	if next.X < 0 || next.Y < 0 || next.X >= interior.Width || next.Y >= interior.Height {
		return false
	}
	if next == interior.Counter {
		return false
	}
	if interior.HasSlots && next == interior.SlotsAt {
		return false
	}
	for _, booth := range interior.Booths {
		if next == booth {
			return false
		}
	}
	interior.Position = next
	return true
}
