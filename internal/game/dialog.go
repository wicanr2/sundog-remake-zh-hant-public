package game

import (
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// The dialog corpus is original: the lines the other party says and the verbs
// the player picks from both come from the Atari ST source tables. What is
// remake-owned is the *flow* — which block a facility opens, and what the other
// party says after the player picks a verb.
//
// That flow is remake-owned by choice, not for want of evidence: the original's
// frame interpreter is decoded (docs/atari-st/dialog-vm.json, eleven pinned
// signatures) and so is its category-to-frame table. Running it would mean
// carrying the frame VM, its precode/postcode opcodes and its attribute rolls.
//
// The four buildings are the exception. Their doors are the original's terrain
// codes and their conversations are the original's frames, so
// DialogBlockForInterior pairs them the way the original does. Only the street
// facilities still use the remake's own pairing.
const (
	// playerVerbPhrase is stock phrase 0x00: the 25 verbs the source table
	// gives the player to choose from.
	playerVerbPhrase = 0x00
	// okReference is the fixed literal the running game substitutes for 0x22.
	okReference = 0x22
	// moneyReference and itemReference are filled from the local market, which
	// is what the lines quoting them are about.
	moneyReference = 0x24
	itemReference  = 0x25
	// placeReference is XDOINTER:0x0f get_place. Its p-code, read from the
	// game's own SYSTEM.STARTUP, tosses a coin and then picks either a random
	// star system (1..12) or a random planet (1..18), retrying while the pick
	// equals the player's current one. So the name it yields is always
	// somewhere the player is not.
	placeReference = 0x23
)

var (
	dialogSource     atarist.DialogData
	dialogTranslated atarist.DialogData
	dialogReady      bool
)

func init() {
	source, err := atarist.EmbeddedDialogData()
	if err != nil {
		return
	}
	translated, err := atarist.EmbeddedTranslatedDialogData()
	if err != nil {
		return
	}
	dialogSource = source
	dialogTranslated = translated
	dialogReady = true
}

// DialogAvailable reports whether the dialog corpus loaded. A shell can hide
// the talk action rather than offer one that always fails.
func DialogAvailable() bool { return dialogReady }

// PlayerVerbs returns the localized verbs the player can say. They are the
// alternatives of source stock phrase 0x00.
func (r Reducer) PlayerVerbs() []string {
	if !dialogReady {
		return nil
	}
	data := r.localizedDialog()
	phrases := data.StockPhrases()
	if playerVerbPhrase >= len(phrases) {
		return nil
	}
	block, _ := data.Block(0x10)
	verbs := make([]string, 0, len(phrases[playerVerbPhrase].Alternatives))
	for index := range phrases[playerVerbPhrase].Alternatives {
		text, err := data.Expand(block, phrases[playerVerbPhrase].Alternatives[index], atarist.FirstAlternative, nil)
		if err != nil || text == "" {
			continue
		}
		verbs = append(verbs, text)
	}
	return verbs
}

// DialogLine returns the localized line the other party is saying, with the
// references the running game supplies filled in from the player's situation.
func (r Reducer) DialogLine(state State) (string, error) {
	return dialogLineText(r.localizedDialog(), state, r.dialogResolver(state))
}

func (r Reducer) localizedDialog() atarist.DialogData {
	if r.DialogText != nil {
		return *r.DialogText
	}
	return dialogTranslated
}

// DialogSourceLine returns the same line in its original English. It backs a
// review path, not a player path: the player only ever sees the translation.
func (r Reducer) DialogSourceLine(state State) (string, error) {
	return dialogLineText(dialogSource, state, r.dialogResolver(state))
}

func dialogLineText(data atarist.DialogData, state State, resolve atarist.Resolver) (string, error) {
	if !dialogReady {
		return "", fmt.Errorf("dialog corpus is unavailable")
	}
	block, ok := data.Block(state.DialogBlock)
	if !ok {
		return "", fmt.Errorf("unknown dialog block 0x%02x", state.DialogBlock)
	}
	if state.DialogLine < 0 || state.DialogLine >= len(block.Lines) {
		return "", fmt.Errorf("dialog block 0x%02x has no line %d", state.DialogBlock, state.DialogLine)
	}
	return data.ExpandLine(block, state.DialogLine, dialogChooser(state.DialogVariant), resolve)
}

// dialogChooser picks alternatives from a counter carried in the save, so the
// same conversation replays identically while still varying between lines. The
// original picks at random; a reproducible counter keeps replays and
// screenshots verifiable.
func dialogChooser(variant int) atarist.Chooser {
	return func(n int) int {
		if n <= 0 {
			return 0
		}
		index := variant % n
		if index < 0 {
			index += n
		}
		return index
	}
}

// dialogResolver fills the references the running game supplies, from the
// player's own situation. Anything it cannot source stays visible as a marker
// rather than being quietly invented.
func (r Reducer) dialogResolver(state State) atarist.Resolver {
	return func(reference int) (string, bool) {
		switch reference {
		case okReference:
			if r.Localize != nil {
				return r.Localize("ui.dialog.ok", "OK"), true
			}
			return "OK", true
		case moneyReference:
			// A beggar names his own figure -- set_up_price runs before the
			// frame opens -- so that one wins over the local market quote.
			if state.StreetAsk > 0 {
				return fmt.Sprintf("%d Cr", state.StreetAsk), true
			}
			if quote, ok := r.World.Market(state.CityID); ok {
				return fmt.Sprintf("%d Cr", quote.BuyPrice), true
			}
		case itemReference:
			quote, ok := r.World.Market(state.CityID)
			if !ok {
				return "", false
			}
			return r.cargoName(quote.ItemKey), true
		case placeReference:
			return r.otherPlaceName(state)
		}
		return "", false
	}
}

// DialogBlockForFacility maps a place to the conversation it opens. The block
// numbers are source data and the text in each block is what justifies the
// pairing. The mapping itself is remake-owned because the remake's facilities
// are not the original's: the original dispatches on the terrain code under the
// player (11-14 are the four building doors, giving interaction kinds 12, 13,
// 14 and 23 — bar counter, equipment shop, parts shop, hotel; see the
// city.facility-door rule in docs/atari-st/ship-rules.json), and those places
// live inside XMOVEINB, which DialogBlockForInterior serves instead. This
// function is only for the facilities the remake places on the street.
func DialogBlockForFacility(facility world.Facility) (int, bool) {
	switch facility {
	case world.FacilityExchange:
		return 0x20, true // haggling over an item
	case world.FacilityUniteller:
		return 0x80, true // buying-or-selling small talk
	case world.FacilitySpaceport:
		return 0x30, true // traffic control
	case world.FacilityWarehouse:
		return 0xA0, true // ship parts advice
	case world.FacilityColony:
		return 0xB0, true // idle conversation
	default:
		return 0, false
	}
}

// dialogBlockAt returns the conversation available at the player's tile. A cell
// that is one of the city's decoded buildings speaks as that building — the
// building's identity comes from the original map, while the remake's own
// facility cluster on the landing cell is only the fallback.
func (r Reducer) dialogBlockAt(state State) (int, bool) {
	position := world.TilePosition{X: state.Position.X, Y: state.Position.Y}
	if facility, ok := GroundBuildingFacilityAt(state, position); ok {
		if block, ok := DialogBlockForFacility(facility); ok {
			return block, true
		}
	}
	for _, facility := range r.GroundFacilitiesAt(state, position) {
		if block, ok := DialogBlockForFacility(facility); ok {
			return block, true
		}
	}
	return 0, false
}

// dialogAdvance moves the conversation on after the player says something. The
// original picks the reply by running the frame VM — decoded, see
// docs/atari-st/dialog-rendered.md — which branches on attribute rolls, the
// NPC's attitude and a precode bitfield. This walks the block's lines in order
// instead, and is explicitly remake-owned.
func (r Reducer) dialogAdvance(state *State) bool {
	block, ok := r.localizedDialog().Block(state.DialogBlock)
	if !ok {
		return false
	}
	state.DialogVariant++
	if state.DialogLine+1 >= len(block.Lines) {
		return false
	}
	state.DialogLine++
	return true
}

// dialogStateValid reports whether a restored save still points at a line that
// exists. A save made against a different corpus revision must not leave the
// player staring at a broken screen.
func dialogStateValid(state State) error {
	if !dialogReady {
		return fmt.Errorf("dialog corpus is unavailable")
	}
	block, ok := dialogTranslated.Block(state.DialogBlock)
	if !ok {
		return fmt.Errorf("unknown dialog block 0x%02x", state.DialogBlock)
	}
	if state.DialogLine < 0 || state.DialogLine >= len(block.Lines) {
		return fmt.Errorf("dialog block 0x%02x has no line %d", state.DialogBlock, state.DialogLine)
	}
	return nil
}

// otherPlaceName reproduces XDOINTER:0x0f get_place: a coin toss chooses the
// star system list or the planet list, and the pick is retried while it equals
// where the player already is. The remake draws from the same dialog variant
// counter instead of the original's RNG so a conversation replays identically.
func (r Reducer) otherPlaceName(state State) (string, bool) {
	useSystems := state.DialogVariant%2 == 0
	step := state.DialogVariant / 2

	if useSystems {
		candidates := make([]System, 0, 12)
		for _, system := range r.World.Systems() {
			if system.ID != state.SystemID {
				candidates = append(candidates, System{ID: system.ID, Name: system.Name})
			}
		}
		if len(candidates) == 0 {
			return "", false
		}
		pick := candidates[abs(step)%len(candidates)]
		return r.localize("ui.system."+pick.ID, pick.Name), true
	}

	candidates := make([]System, 0, 18)
	for _, planet := range r.World.Planets() {
		if planet.ID != state.PlanetID {
			candidates = append(candidates, System{ID: planet.ID, Name: planet.Name})
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	pick := candidates[abs(step)%len(candidates)]
	return r.localize("ui.planet."+pick.ID, pick.Name), true
}

// System pairs an identity with a display name for the place picker. It mirrors
// world.System and world.Planet, which differ only in the fields not needed here.
type System struct {
	ID   string
	Name string
}

// localize resolves a player-visible key when a shell has supplied a lookup.
// Without one the source name is used, which keeps the rules layer usable in
// tests that carry no catalog.
func (r Reducer) localize(key, fallback string) string {
	if r.Localize == nil {
		return fallback
	}
	return r.Localize(key, fallback)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// boothDialogFrame is the conversation the person at a bar booth opens. The
// original's interaction kind 21 starts at frame 0x30, which chains straight to
// 0x80 -- "Hello. Care to sit down".
const boothDialogFrame = 0x80

// DialogBlockForInterior maps a building's counter to the conversation behind
// it. Unlike DialogBlockForFacility these pairings are the original's: the four
// building doors give interaction kinds 12, 13, 14 and 23, and the frame each
// kind starts at is decoded in docs/atari-st/dialog-rendered.md -- kind 12 at
// 0x10, kind 13 at 0x50, kind 14 at 0x51, kind 23 at 0x60.
func DialogBlockForInterior(kind InteractionKind) (int, bool) {
	switch kind {
	case KindBarCounter:
		return 0x10, true
	case KindEquipmentShop:
		return 0x50, true
	case KindPartsShop:
		return 0x51, true
	case KindHotel:
		return 0x60, true
	default:
		return 0, false
	}
}
