package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/wicanr2/sundog-remake/internal/game"
)

// --- The people in a room: XMOVEINB's sprites over the room picture ---
//
// The original's room is the zoom's inside (158×98, window 3) with the
// picture painted over it; the furniture is not drawn on top, it is in the
// picture, and the window table only says where it is so the people can
// find it (XMOVEINB:0x12 get_building takes the counter from windows 29/30
// for the bar, 41/42 for the hotel, 27/28 for the equipment shop -- the one
// vertical counter -- and 25/26 for the parts shop; the doors are the
// centres of windows 23 and 24, (7,44) and (7,58)).
//
// People are sprites (0x0f create_person): born at the door in one of
// patterns 6–9, colour 14, and walked (move_sprite) to a seat that 0x09
// find_booth or 0x0a find_counter picked: a booth seat is the centre of one
// of windows 31–38 (four booths, two seats each, along the bottom of the
// bar); a counter seat is a random spot along the counter's middle line.
// Person 0 is the bartender (created with the counter flag) and the talk
// starts when the player is within five of them (0x02 handle_bartender).
//
// The remake's room state is a grid (Interior.Width × Height, the counter
// and booths as cells); this file maps that grid onto the room's pixels and
// seats the customers the state counts at the original's seat positions.
// A customer the state has just admitted walks in from the door
// (create_person births at the door, move_sprite carries them to the seat);
// the walk takes stRoomWalkFrames display frames, the remake's pacing.

type stRoomRect struct{ x0, x1, y0, y1 int }

// stRoomCounter is get_building's counter rectangle per kind (window table,
// relative to the inside corner); vertical is L16 cleared.
var stRoomCounter = map[game.InteractionKind]struct {
	rect     stRoomRect
	vertical bool
}{
	game.KindBarCounter:    {stRoomRect{7, 135, 26, 28}, false},
	game.KindHotel:         {stRoomRect{7, 148, 27, 29}, false},
	game.KindEquipmentShop: {stRoomRect{54, 56, 11, 93}, true},
	game.KindPartsShop:     {stRoomRect{7, 148, 59, 61}, false},
}

// stBoothSeats are the centres of windows 31–38.
var stBoothSeats = [][2]int{{10, 92}, {32, 92}, {44, 92}, {66, 92}, {78, 92}, {100, 92}, {112, 92}, {134, 92}}

const (
	stRoomDoorX, stRoomDoorY   = 7, 58
	stRoomInnerW, stRoomInnerH = 158, 98
	// stRoomSpriteW/H: the doubled 7-row pattern, so a point is its centre.
	stRoomSpriteW, stRoomSpriteH = 14, 14
	// stPersonPen: create_person births in sprite colour 14, which the
	// sprite handler maps to hardware entry 11 (laanwj sprite_color_map).
	stPersonPen          = 11
	stPersonPatterns     = 6 // create_person: 6 + random mod 4
	stPersonPatternCount = 4
)

// stRoomSeats lists where customers sit for a kind, in the order the remake
// fills them: booths first at the bar, then spots along the counter.
func stRoomSeats(kind game.InteractionKind, count int) [][2]int {
	seats := [][2]int{}
	if kind == game.KindBarCounter {
		seats = append(seats, stBoothSeats...)
	}
	counter, ok := stRoomCounter[kind]
	if !ok {
		return seats
	}
	// The original rolls a random spot along the counter; the remake spaces
	// them evenly so a seat never lands on another.
	for i := 0; i < count; i++ {
		t := (2*i + 1)
		if counter.vertical {
			x := (counter.rect.x0 + counter.rect.x1) / 2
			y := counter.rect.y0 + (counter.rect.y1-counter.rect.y0)*t/(2*count)
			seats = append(seats, [2]int{x, y})
		} else {
			y := (counter.rect.y0 + counter.rect.y1) / 2
			x := counter.rect.x0 + (counter.rect.x1-counter.rect.x0)*t/(2*count)
			seats = append(seats, [2]int{x, y})
		}
	}
	return seats
}

// stRoomPlayer maps the remake's grid cell onto the room: the door column is
// the door, the far column the far wall, rows between the top of the
// picture's floor and the booths.
func stRoomPlayer(interior game.Interior) (int, int) {
	w, h := interior.Width-1, interior.Height-1
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	x := stRoomDoorX + interior.Position.X*(stRoomInnerW-14-stRoomDoorX)/w
	y := 14 + interior.Position.Y*(stRoomInnerH-14-14)/h
	return x, y
}

// drawSTRoomPeople draws the bartender, the customers and the player over
// the room picture.
func (a *app) drawSTRoomPeople(screen *ebiten.Image, window stZoomWindow) {
	interior := a.state.Interior
	if !interior.Active || a.groundSprites == nil {
		return
	}
	baseX, baseY := window.x0+1, window.y0+1
	at := func(pattern, x, y int, ink color.RGBA) {
		a.drawSTSprite(screen, pattern, baseX+x-stRoomSpriteW/2, baseY+y-stRoomSpriteH/2, ink)
	}
	pen := stColour(stPersonPen)
	if counter, ok := stRoomCounter[interior.Kind]; ok {
		// The bartender stands at the middle of the counter.
		at(stPersonPatterns, (counter.rect.x0+counter.rect.x1)/2, (counter.rect.y0+counter.rect.y1)/2, pen)
	}
	seats := stRoomSeats(interior.Kind, interior.CustomerCap)
	for i := 0; i < interior.Customers && i < len(seats); i++ {
		pattern := stPersonPatterns + (i*3+int(interior.Kind))%stPersonPatternCount
		x, y := seats[i][0], seats[i][1]
		if walked := a.frames - a.roomArrivalFrame(i); walked < stRoomWalkFrames {
			x = stRoomDoorX + (x-stRoomDoorX)*walked/stRoomWalkFrames
			y = stRoomDoorY + (y-stRoomDoorY)*walked/stRoomWalkFrames
		}
		at(pattern, x, y, pen)
	}
	x, y := stRoomPlayer(interior)
	at(walkerPattern(a.state.Heading, a.state.Position), x, y, color.RGBA{255, 255, 255, 255})
}

// stRoomWalkFrames is how long a customer's walk in from the door takes
// (display frames; the remake's pacing for move_sprite's steps).
const stRoomWalkFrames = 90

// stRoomArrivals remembers the frame each seated customer appeared on, so
// the walk in can be drawn; keyed by the room the player is in.
type stRoomArrivals struct {
	room   string
	frames map[int]int
}

func (a *app) roomArrivalFrame(seat int) int {
	room := fmt.Sprintf("%s:%d,%d", a.state.CityID, a.state.Interior.ReturnPosition.X, a.state.Interior.ReturnPosition.Y)
	if a.roomArrivals.room != room || a.roomArrivals.frames == nil {
		a.roomArrivals = stRoomArrivals{room: room, frames: map[int]int{}}
	}
	frame, ok := a.roomArrivals.frames[seat]
	if !ok {
		frame = a.frames
		a.roomArrivals.frames[seat] = frame
	}
	for extra := range a.roomArrivals.frames {
		if extra >= a.state.Interior.Customers {
			delete(a.roomArrivals.frames, extra)
		}
	}
	return frame
}
