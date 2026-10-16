package game

import "github.com/wicanr2/sundog-remake/internal/world"

// Street encounters are four procedures of the original, and they replace the
// remake's own robbery roll.
//
//   - XMOVEONG:0x01 xmove_on_ground rolls the trigger
//     (encounter.trigger).
//   - XMOVEONG:0x34 set_itype reads the ground byte under the player, cancels
//     on a building, and picks three weights from a 0x00..0x14 jump table
//     (encounter.type-table).
//   - XMOVEONG:0x35 which(a, b, c) turns those weights into a kind, biased by
//     the player's cash (encounter.which).
//   - XMOVEONG:0x36 confirm(kind, threshold) suppresses each kind against a
//     threshold the star system carries (encounter.type-confirm).
//
// XDOINTER:0x26 start_interaction then gives the surviving kind its opening
// dialog frame. All of that is pinned; see the street.* and encounter.* rules
// in docs/atari-st/ship-rules.json and docs/atari-st/ground-rules.json, and
// the walk-through in docs/atari-st/ground-rules.md.
//
// How often the trigger comes up depends on which walk is running. The
// original rolls random() mod 1000 == 21 on every iteration of its real-time
// movement loop. Under the Atari ST presentation the remake now has that loop
// — the pixel walk in cmd/sundog-ebiten/stground.go — so there the roll is the
// original's, once per walking tick (streetTriggerRealTime). Under the two
// grid presentations there is no such loop and the roll is per step
// (streetTriggerIn), which is not a claim about the original's odds.
//
// What is still the remake's either way is the **rate of the tick**: the
// original's loop runs as fast as the p-machine goes and that has not been
// measured, so a tick here is one 60-frames-a-second update.
//
// The Enlie doubling is the original's in both, because it is a ratio rather
// than a rate: that system adds a second trigger roll (random() mod 1000 ==
// 57), so the chance there is twice everywhere else.

// The three street encounter kinds. They are interaction kinds, the same
// numbering the building doors use, which is why they reach the dialog
// interpreter unchanged.
const (
	// StreetKindMugger opens frame 0x41, "Give me your cash or die!", and is
	// the one that turns into a fight.
	StreetKindMugger = 16
	// StreetKindBeggar opens frame 0x31, "Can you spare N for an old man".
	StreetKindBeggar = 18
	// StreetKindDealer opens frame 0x21, "can I interest you in a deal".
	StreetKindDealer = 22
)

const (
	// streetTriggerIn is the remake's per-step chance of any encounter at all
	// on the grid walk. See the note above: that rate is remake-owned.
	streetTriggerIn = 40
	// streetTriggerRealTime is the original's own: random() mod 1000 on every
	// iteration of the movement loop.
	streetTriggerRealTime = 1000
	// streetEnlieTriggerFactor is the second trigger roll the jammed system
	// adds, expressed as a factor on the chance.
	streetEnlieTriggerFactor = 2
	// streetWhichSpan is which()'s own roll: random() mod 100.
	streetWhichSpan = 100
	// streetCashDivisor is how much of the player's cash biases the first
	// weight. MAINLIB_G33 is the high word of the cash pair, and set_money
	// splits money base-1000, so it is the thousands: a += thousands div 2.
	streetCashDivisor = 2
	// streetConfirmSpan is confirm()'s roll: random() mod 20 under the
	// system's threshold cancels the encounter.
	streetConfirmSpan = 20
	// streetForceSpan and streetForceEscape are the jammed system's last
	// rewrite: any surviving kind becomes a mugger unless random() mod 99 == 7.
	streetForceSpan   = 99
	streetForceEscape = 7
	// streetBuildingBias is what set_itype subtracts from a ground byte above
	// 15 before indexing the weight table. Building codes start at 34.
	streetBuildingBias = 16
	// streetTerrainMax is the last index of the weight table.
	streetTerrainMax = 0x14
	// streetEasterEggIn is the one-time developer credit's own gate:
	// passed(101) is made_roll(1), a one-in-a-hundred roll.
	streetEasterEggIn = 100
	// streetEasterEggAward is what frame 0xe1 hands over when the player
	// answers it correctly.
	streetEasterEggAward = 10000
	// streetBeggarMin and streetBeggarSpan bound what a beggar asks for:
	// set_up_price(rand(5, 45)).
	streetBeggarMin  = 5
	streetBeggarSpan = 41
)

// Opening dialog frames, from XDOINTER:0x26 start_interaction.
const (
	streetMuggerFrame = 0x41
	streetBeggarFrame = 0x31
	streetDealerFrame = 0x21
	// streetEasterEggFrame is the one-time frame that names the six people who
	// made the game.
	streetEasterEggFrame = 0xe0
)

// streetWeights is set_itype's jump table: one row of three weights per ground
// code. Index 0x0f is the original's own hole -- it makes no call at all, so
// nothing can happen on that tile.
var streetWeights = [streetTerrainMax + 1][3]int{
	0x00: {2, 3, 5},
	0x01: {4, 7, 9},
	0x02: {5, 5, 15},
	0x03: {40, 40, 20},
	0x04: {20, 5, 15},
	0x05: {50, 30, 20},
	0x06: {8, 10, 15},
	0x07: {8, 10, 15},
	0x08: {8, 10, 15},
	0x09: {8, 10, 15},
	0x0a: {10, 10, 10},
	0x0b: {50, 30, 20},
	0x0c: {5, 5, 30},
	0x0d: {25, 5, 20},
	0x0e: {15, 30, 15},
	0x0f: {0, 0, 0},
	0x10: {60, 20, 20},
	0x11: {40, 10, 20},
	0x12: {10, 40, 40},
	0x13: {10, 10, 10},
	0x14: {5, 5, 10},
}

// streetWeightsFor returns the three weights a ground byte selects. The second
// result is false where the original makes no call.
func streetWeightsFor(groundByte int) ([3]int, bool) {
	index := groundByte
	if index > 0x0f {
		index -= streetBuildingBias
	}
	if index < 0 || index > streetTerrainMax || index == 0x0f {
		return [3]int{}, false
	}
	return streetWeights[index], true
}

// whichStreetKind is XMOVEONG:0x35 which(a, b, c). The weights are cumulative
// and the first one carries the cash bias, so a rich player walks into more
// muggers and the same walk offers fewer deals.
func whichStreetKind(roll int, weights [3]int, cashThousands int) int {
	a := weights[0] + cashThousands/streetCashDivisor
	b := weights[1] + a
	c := weights[2] + b
	switch {
	case roll < a:
		return StreetKindMugger
	case roll < b:
		return StreetKindBeggar
	case roll < c:
		return StreetKindDealer
	default:
		return 0
	}
}

// confirmStreetKind is XMOVEONG:0x36 confirm(kind, threshold): a kind that
// matches is cancelled when the roll falls under the system's threshold. Note
// the direction -- a higher threshold means fewer of that kind.
func confirmStreetKind(kind, target, roll, threshold int) int {
	if kind == target && roll < threshold {
		return 0
	}
	return kind
}

// streetThresholds returns the three suppression thresholds a system carries,
// in kind order: mugger, beggar, dealer. They are nibbles 3, 4 and 5 of the
// star system record, which the rest of the remake already knows as the
// encounter rating, the price level and the street deal rating.
func streetThresholds(system world.System) [3]int {
	return [3]int{system.EncounterRating, system.PriceLevel, system.StreetDealRating}
}

// StreetFrame is the dialog frame a kind opens with.
func StreetFrame(kind int) (int, bool) {
	switch kind {
	case StreetKindMugger:
		return streetMuggerFrame, true
	case StreetKindBeggar:
		return streetBeggarFrame, true
	case StreetKindDealer:
		return streetDealerFrame, true
	default:
		return 0, false
	}
}

// streetRoll is the city's own reproducible sequence, kept apart from the
// combat one so a fight cannot shift what a walk does.
func streetRoll(state *State, span int) int {
	if span <= 0 {
		return 0
	}
	state.StreetRolls++
	mixed := uint32(state.StreetRolls)*22695477 + 1
	mixed ^= mixed >> 15
	return int(mixed % uint32(span))
}

// resolveStreetEncounter runs one step's check and returns the kind that
// survives it, or 0 for an uneventful step.
func resolveStreetEncounter(state *State, system world.System, groundByte int, onBuilding bool) int {
	return resolveStreetEncounterAt(state, system, groundByte, onBuilding, streetTriggerIn)
}

// resolveStreetEncounterAt is the same with the trigger's odds named, so the
// pixel walk can pass the original's 1-in-1000 and the grid walk its own.
func resolveStreetEncounterAt(state *State, system world.System, groundByte int, onBuilding bool, chanceIn int) int {
	jammed := system.ID == commJammedSystemID
	if jammed {
		chanceIn /= streetEnlieTriggerFactor
	}
	if streetRoll(state, chanceIn) != 0 {
		return 0
	}
	// check_building claims the tile before anything is rolled.
	if onBuilding {
		return 0
	}
	weights, ok := streetWeightsFor(groundByte)
	if !ok {
		return 0
	}
	kind := whichStreetKind(streetRoll(state, streetWhichSpan), weights, state.Cash/1000)
	thresholds := streetThresholds(system)
	for index, target := range [3]int{StreetKindMugger, StreetKindBeggar, StreetKindDealer} {
		kind = confirmStreetKind(kind, target, streetRoll(state, streetConfirmSpan), thresholds[index])
	}
	// Banville turns muggers and beggars away. The original checks the city
	// number straight after the three confirms — `MAINLIB_G2a` field (6,9)
	// against 0x3c, which is city 60, the colony. Dealers still come up: the
	// test names types 16 and 18 only.
	if state.CityID == world.MissionDestinationID &&
		(kind == StreetKindMugger || kind == StreetKindBeggar) {
		kind = 0
	}
	if jammed && kind != 0 && streetRoll(state, streetForceSpan) != streetForceEscape {
		kind = StreetKindMugger
	}
	return kind
}

// streetBeggarAsk is what a beggar opens with. The original calls
// set_up_price(rand(5, 45)), and set_up_price is the same price multiplier
// every shop uses, which the remake already has as Reducer.PriceScale.
func (r Reducer) streetBeggarAsk(state *State) int {
	ask := streetBeggarMin + streetRoll(state, streetBeggarSpan)
	return ask * r.PriceScale(*state)
}

// openStreetEncounter runs one step's check and puts whatever it turns up in
// front of the player. A mugger says his line before he draws -- xdo_combat
// calls bring_up_window before it sets the arena up -- so the fight starts when
// the player walks away from that conversation, not here. The exception is the
// original's own: in the jammed system a mugger skips the talking nine times
// out of ten.
func (r Reducer) openStreetEncounter(next *State) {
	r.openStreetEncounterAt(next, streetTriggerIn)
}

// openStreetEncounterRealTime is the check the pixel walk runs once a tick,
// with the original's own odds.
func (r Reducer) openStreetEncounterRealTime(next *State) {
	r.openStreetEncounterAt(next, streetTriggerRealTime)
}

func (r Reducer) openStreetEncounterAt(next *State, chanceIn int) {
	system, ok := r.World.System(next.SystemID)
	if !ok {
		return
	}
	groundByte, _, hasCode := GroundTerrainCode(*next, next.CityID, next.Position.X, next.Position.Y)
	if !hasCode {
		return
	}
	_, onBuilding := GroundBuildingFacilityAt(*next, world.TilePosition{X: next.Position.X, Y: next.Position.Y})
	kind := resolveStreetEncounterAt(next, system, groundByte, onBuilding, chanceIn)
	if kind == 0 {
		return
	}
	if kind == StreetKindMugger && system.ID == commJammedSystemID &&
		streetRoll(next, streetSilentMuggingSpan) != streetSilentMuggingTalk {
		beginArena(next, arenaRand(&next.Arena, arenaMinRecords, arenaMaxRecords))
		next.MessageKey = "ui.message.street_mugger"
		return
	}
	frame, ok := StreetFrame(kind)
	if !ok {
		return
	}
	if kind == StreetKindBeggar && !next.StreetEasterEggSeen && streetRoll(next, streetEasterEggIn) == 0 {
		// The beggar who is really the people who wrote the game. It happens
		// once a save, and answering him correctly is worth 10000 Cr.
		next.StreetEasterEggSeen = true
		frame = streetEasterEggFrame
	}
	opening, hasOpening := dialogOpeningForFrame(frame)
	if !hasOpening || !DialogAvailable() {
		// Without the dialog data there is nothing to show, so the encounter
		// reduces to the one thing that still has consequences.
		if kind == StreetKindMugger {
			beginArena(next, arenaRand(&next.Arena, arenaMinRecords, arenaMaxRecords))
			next.MessageKey = "ui.message.street_mugger"
		}
		return
	}
	next.Screen = ScreenDialog
	next.DialogBlock = opening.Block
	next.DialogLine = opening.Line
	next.DialogVerb = 0
	next.DialogVariant = next.DialogVariant + 1
	next.DialogSaid = ""
	next.GroundReturn = ScreenGround
	switch kind {
	case StreetKindMugger:
		next.StreetPending = StreetKindMugger
		next.MessageKey = "ui.message.street_mugger"
	case StreetKindBeggar:
		next.StreetAsk = r.streetBeggarAsk(next)
		next.MessageKey = "ui.message.street_beggar"
	case StreetKindDealer:
		next.MessageKey = "ui.message.street_dealer"
	}
}

const (
	// streetSilentMuggingSpan and streetSilentMuggingTalk are xdo_combat's own
	// escape: in the jammed system a mugger opens fire without a word unless
	// random() mod 10 == 4.
	streetSilentMuggingSpan = 10
	streetSilentMuggingTalk = 4
)
