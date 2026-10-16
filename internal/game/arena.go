package game

// The ground combat arena is XDOCOMBA, eighteen procedures. It is where a
// street robbery ends up: XMOVEONG rolls random() mod 1000 == 21, offers a way
// out, and on failure hands the player to rand(2,6) opponents in a walled
// arena. Every number below is the original's, pinned in
// docs/atari-st/city-subsystems.json.
//
// What stays remake-owned is the clock. The original's arena is real time --
// handle_player and handle_thugs run off a background interrupt -- while the
// remake resolves a fight as a sequence of exchanges driven by the reducer.
// The stats, the damage, the shield and the incapacitation threshold are not
// remake-owned.

const (
	// arenaHealthDivisor turns health and strength into hit points:
	// health x ST / 90.
	arenaHealthDivisor = 90
	// arenaSpeedDivisor and arenaSpeedBase: DX / 5 + 3.
	arenaSpeedDivisor = 5
	arenaSpeedBase    = 3
	// arenaDownThreshold is what counts as out of the fight. Note it is not
	// zero: a fighter at 32 or below is down.
	arenaDownThreshold = 32
	// arenaDamageSpan is the multiplier a shot rolls: base x (9 + 0..3).
	arenaDamageBase = 9
	arenaDamageSpan = 4
	// The three damage bases, by what is doing the shooting.
	arenaScattergunDamage = 12
	arenaStingerDamage    = 5
	arenaThugDamage       = 3
	// arenaMinRecords and arenaMaxRecords bound the *records*, not the crowd.
	// XDOCOMBA:0x0a initialize sets local 0x1a to rand(2,6) for encounter type
	// 16 and a flat two for anything else, and record 1 is the player: the
	// loops that walk the opponents all run `for i := 2 to 0x1a`. So a mugging
	// is one to five opponents and everything else is exactly one.
	arenaMinRecords = 2
	arenaMaxRecords = 6
	// A thug's own numbers, also from init_people: hit points rand(70,250) and
	// speed random() mod 8 + 6, neither of which depends on the encounter type.
	arenaThugMinHitPoints = 70
	arenaThugMaxHitPoints = 250
	arenaThugSpeedSpan    = 8
	arenaThugSpeedBase    = 6
)

// Item ids the arena looks for in the four carry slots. These are the source
// item table's own numbers.
const (
	itemShield     = 11
	itemStinger    = 12
	itemScattergun = 13
	// itemDeadPart is what a spent shield leaves behind, the same marker the
	// ship's repair screen uses.
	itemDeadPart = 7
)

// ArenaThug is one opponent: the original's five-word record, plus where its
// sprite is. Speed is record word 1 — the *firing* speed, not the walking one;
// Move is what create_sprite was given.
type ArenaThug struct {
	HitPoints int  `json:"hitPoints"`
	Speed     int  `json:"speed"`
	Down      bool `json:"down"`
	// X and Y are the sprite; TargetX and TargetY are what move_sprite was
	// last told to walk to. Halted is stop_sprite.
	X       int  `json:"x"`
	Y       int  `json:"y"`
	TargetX int  `json:"targetX"`
	TargetY int  `json:"targetY"`
	Halted  bool `json:"halted"`
	// Move is the sprite speed create_sprite gave it, 6 + random() mod 4.
	Move int `json:"move"`
	// Cooldown is record word 3: one off a pass, and a shot resets it to
	// 14 - Speed. A thug that is down counts the same word away before its
	// sprite is taken off the field.
	Cooldown int `json:"cooldown"`
	// Gone is set once the sprite has been removed — fire_gun does it the
	// moment the hit points reach zero, handle_thugs when the counter runs out
	// or the thug walks off the field.
	Gone bool `json:"gone"`
}

// Arena is a fight in progress.
type Arena struct {
	Active bool `json:"active"`
	// HitPoints and Speed are the player's, from health x ST / 90 and
	// DX / 5 + 3.
	HitPoints int `json:"hitPoints"`
	Speed     int `json:"speed"`
	// Gun is the item id the player is holding, 0 for bare hands. check_guns
	// moves a stinger or scattergun into the hand slot when the fight opens.
	Gun int `json:"gun"`
	// Shots is what is left in a scattergun. The carry byte packs the item id
	// in the low 5 bits and the remaining shots in the high 3, and check_guns
	// reads that second half for the scattergun alone — a stinger has no
	// count. Firing the last shot leaves the player bare-handed, which is the
	// branch fire_gun's "out of ammunition" cue belongs to.
	Shots int `json:"shots"`
	// ShieldSlot is which carry slot holds a shield, -1 for none;
	// ShieldCharges is what is left of it.
	ShieldSlot    int `json:"shieldSlot"`
	ShieldCharges int `json:"shieldCharges"`
	// Thugs is the crowd. Rolls counts draws so a replay reproduces.
	Thugs []ArenaThug `json:"thugs"`
	Rolls int         `json:"rolls"`
	// Won and Lost are set when the fight ends.
	Won  bool `json:"won"`
	Lost bool `json:"lost"`

	// --- the real-time state, XDOCOMBA's own locals ---

	// PlayerX and PlayerY are the player's sprite; TargetX and TargetY are
	// where the left button last pointed. Halted is stop_sprite.
	PlayerX int  `json:"playerX"`
	PlayerY int  `json:"playerY"`
	TargetX int  `json:"targetX"`
	TargetY int  `json:"targetY"`
	Halted  bool `json:"halted"`
	// Cooldown is the player's record word 3. It is only counted down while
	// the player is on the floor: that is the delay before the crowd helps
	// itself to what the player is carrying.
	Cooldown int `json:"cooldown"`
	// Mode is local 0x0c: 0 fighting, 1 holding, 2 scattering.
	Mode int `json:"mode"`
	// Fleeing is flag 0x32 — set when the encounter is already over (type 15),
	// which is what makes the leftover crowd run instead of shoot.
	Fleeing bool `json:"fleeing"`
	// DownPending is flag 0x30: the player is on the floor and the countdown
	// to being robbed is running.
	DownPending bool `json:"downPending"`
	// FireLatch is local 0x01. The right button has to be let go between
	// shots, so holding it down fires once.
	FireLatch bool `json:"fireLatch"`
	// Pulls counts trigger pulls for the scattergun, which spends a shot every
	// fourth one (local 0x19).
	Pulls int `json:"pulls"`
	// Escaped is local 0x05: the player walked off the field.
	Escaped bool `json:"escaped"`
	// Robbed is set the pass the crowd empties the carry slots, so the shell
	// has something to say about it.
	Robbed bool `json:"robbed"`
	// History is the ring handle_player writes the player's position into and
	// handle_thugs aims at. HistoryAt is the last one written.
	History   [arenaHistory]ArenaPoint `json:"history"`
	HistoryAt int                      `json:"historyAt"`
	// Ticks counts passes. The crowd moves on every second one, which is the
	// original's own two-tick timer.
	Ticks int `json:"ticks"`
	// Shot is the last ray fired, for the shell to draw. It lasts one pass.
	Shot ArenaShot `json:"shot"`
}

// ArenaShot is one ray, kept for the pass it was fired on so the shell can draw
// it. The original draws it as it traces, a point at a time, in colour 12 or 13
// for the player's two guns and 13 for a thug's.
type ArenaShot struct {
	Live   bool `json:"live"`
	FromX  int  `json:"fromX"`
	FromY  int  `json:"fromY"`
	ToX    int  `json:"toX"`
	ToY    int  `json:"toY"`
	Colour int  `json:"colour"`
	// Hit is who it landed on: 0 for the player, 1-based into Thugs for an
	// opponent, -1 for nobody.
	Hit int `json:"hit"`
}

// arenaRoll is the remake's reproducible draw, the same substitution the rest
// of the remake makes for the original's RNG.
func arenaRoll(arena *Arena, span int) int {
	if span <= 0 {
		return 0
	}
	arena.Rolls++
	mixed := uint32(arena.Rolls)*1664525 + 1013904223
	mixed ^= mixed >> 13
	return int(mixed % uint32(span))
}

// ArenaHitPoints is what init_people gives the player: health x ST / 90, with
// a floor at one point above the down threshold. Without the floor a weak
// character would arrive already unconscious.
func ArenaHitPoints(health, strength int) int {
	points := health * strength / arenaHealthDivisor
	if points <= arenaDownThreshold {
		points = arenaDownThreshold + 1
	}
	return points
}

// ArenaSpeed is DX / 5 + 3.
func ArenaSpeed(dexterity int) int {
	return dexterity/arenaSpeedDivisor + arenaSpeedBase
}

// carrySlots is the original's four body slots. WINDOWLI make_man and the
// native Stores chain prove that the dragged object is separate.
func carrySlots(state State) [CarrySlots]int {
	var slots [CarrySlots]int
	copy(slots[:], state.Parts)
	return slots
}

// writeCarrySlots preserves the dragged object and fixed body positions.
func writeCarrySlots(state *State, slots [CarrySlots]int) {
	state.Parts = append([]int(nil), slots[:]...)
}

// arenaShieldSlot is check_shield: the first carry slot holding item 11, and
// how many charges are left in its high three bits.
func arenaShieldSlot(slots [CarrySlots]int) (slot, charges int) {
	for index, value := range slots {
		if value != 0 && value%32 == itemShield {
			return index, value / 32
		}
	}
	return -1, 0
}

// arenaDrawGun is check_guns: find a stinger or scattergun in the carry slots
// and swap it into slot 0, the hand. The original does this the moment the
// fight opens, so whatever the player was holding gets displaced.
// It also returns what the high 3 bits of that byte hold: the scattergun's
// remaining shots. A stinger carries no count, so it comes back as 0.
func arenaDrawGun(slots *[CarrySlots]int) (gun, shots int) {
	for index, value := range slots {
		id := value % 32
		if value == 0 || (id != itemStinger && id != itemScattergun) {
			continue
		}
		if index != 0 {
			slots[0], slots[index] = slots[index], slots[0]
		}
		if id == itemScattergun {
			return id, value / 32
		}
		return id, 0
	}
	return 0, 0
}

// arenaShotDamage is fire_gun's damage: base x (9 + 0..3).
func arenaShotDamage(arena *Arena, base int) int {
	return base * (arenaDamageBase + arenaRoll(arena, arenaDamageSpan))
}

// arenaPlayerDamageBase is what the player's current weapon hits for.
func arenaPlayerDamageBase(gun int) int {
	switch gun {
	case itemScattergun:
		return arenaScattergunDamage
	case itemStinger:
		return arenaStingerDamage
	default:
		// Bare hands. The original arms the player from the carry slots and
		// has no unarmed branch in fire_gun, so this figure is remake-owned.
		return arenaThugDamage
	}
}

// applyArenaDamage takes points off a fighter and reports whether that put them
// down. Negative totals are clamped to zero first, as fire_gun does.
func applyArenaDamage(points *int, damage int) bool {
	*points -= damage
	if *points < 0 {
		*points = 0
	}
	return *points <= arenaDownThreshold
}

// rollThug draws one opponent the way init_people does: hit points anywhere in
// 70..250, a firing speed in 6..13 and a walking speed in 6..9, none of which
// depends on what kind of encounter this is. `index` is which opponent it is,
// because only the first is dropped at random — the rest stand off the centre.
func rollThug(arena *Arena, index int) ArenaThug {
	span := arenaThugMaxHitPoints - arenaThugMinHitPoints + 1
	thug := ArenaThug{
		HitPoints: arenaThugMinHitPoints + arenaRoll(arena, span),
		Speed:     arenaThugSpeedBase + arenaRoll(arena, arenaThugSpeedSpan),
		Move:      arenaThugMoveBase + arenaRoll(arena, arenaThugMoveSpan),
	}
	// The opening cooldown is rand(11, 19) minus the speed, so a fast thug gets
	// its first shot away sooner.
	thug.Cooldown = arenaRand(arena, arenaOpeningCooldownLow, arenaOpeningCooldownHigh) - thug.Speed
	thug.X, thug.Y = arenaSeat(arena, index)
	thug.TargetX, thug.TargetY = thug.X, thug.Y
	return thug
}
