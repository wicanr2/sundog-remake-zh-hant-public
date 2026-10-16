package game

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

const CurrentSchema = 44

// The purse a new game opens with: XSTARTUP:0x30 sets it to
// `random() mod 700 + 200`, so 200..899 Cr (ground-rules.json, cash.new-game).
const (
	// newGameCashMin/Span are XSTARTUP's G33[0] := random() mod 700 + 200
	// (segment offset 0x1c37).
	newGameCashMin  = 200
	newGameCashSpan = 700
	// The bank balances a new game opens with. XSTARTUP's world loop
	// (segment offset 0x1532) walks the twelve star-system records of
	// MAINLIB_G11f and writes the two money words of each:
	//
	//	record[6] = 3 + random() mod 3        // thousands
	//	if system = 1 then record[6] += 32    // Jondd, the start system
	//	record[7] = random() mod 1000         // the rest
	//
	// and then, after the loop, fills Enlie's two words with zero (0x15ba) —
	// that system has no bank, which is also why pick_location skips it. So a
	// balance is `record[6] × 1000 + record[7]`: 35,000–37,999 at Jondd,
	// 3,000–5,999 elsewhere, 0 at Enlie.
	//
	// Two independent measurements land inside that: the Uniteller capture's
	// "In bank: 36,306" and a memory dump's twelve records
	// (37,623 / 5,142 / 0 / 3,379 / 4,191 / 4,506 / 4,430 / 3,549 / 3,261 /
	// 5,955 / 3,822 / 3,828). See docs/atari-st/secondary-gamestate.md.
	newGameBankThousandsMin  = 3
	newGameBankThousandsSpan = 3
	newGameBankStartBonus    = 32
	newGameBankUnitsSpan     = 1000
)

// newGameBankBalances is XSTARTUP's loop over the twelve system records. The
// index rather than the id decides the two special cases, because that is what
// the original tests: system 1 is the start system, system 3 has no bank.
func newGameBankBalances(seed int64, systems []world.System) map[string]int {
	roll := newGameRoller(seed)
	balances := map[string]int{}
	for index, system := range systems {
		thousands := newGameBankThousandsMin + roll(newGameBankThousandsSpan)
		if index == 0 {
			thousands += newGameBankStartBonus
		}
		balances[system.ID] = thousands*1000 + roll(newGameBankUnitsSpan)
	}
	if len(systems) > tellerPickNoBank {
		balances[systems[tellerPickNoBank].ID] = 0
	}
	return balances
}

// newGameRoller is the remake's stand-in for MAINLIB:0x0b: a stream of values
// from one seed, so a seed gives the same world twice. The original's LCG is
// documented but not reproduced here — what matters for a balance is the
// distribution the loop above draws from, not the exact sequence.
func newGameRoller(seed int64) func(int) int {
	state := uint64(seed)*6364136223846793005 + 1442695040888963407
	return func(n int) int {
		state = state*6364136223846793005 + 1442695040888963407
		if n <= 0 {
			return 0
		}
		return int((state >> 33) % uint64(n))
	}
}

// NewGameSeed is the entropy a new game draws. The original takes it from the
// p-system clock: KERNEL:0x14 time writes both words of the LCG state
// MAINLIB_G101/G102 at new-game (ground-rules.json, rng.seed-new-game), which
// is why two new games do not open with the same purse. Everything else in the
// remake replaces the original's RNG with a counter kept in the save, but that
// substitution has nothing to count from before the save exists.
//
// Tests and replay scripts pin this so a run stays reproducible.
var NewGameSeed = func() int64 { return time.Now().UnixNano() }

// newGameCash draws the opening purse from one seed.
func newGameCash(seed int64) int {
	mixed := uint32(seed)*1664525 + 1013904223
	mixed ^= mixed >> 16
	return newGameCashMin + int(mixed%uint32(newGameCashSpan))
}

// CharacterNameMaxRunes is a remake UI safety limit. The original limit is
// not yet proven; this value prevents an unbounded draft from overflowing the
// small prototype character-creation panel.
const CharacterNameMaxRunes = 20

type Screen string

const (
	ScreenTitle               Screen = "title"
	ScreenCharacterLibrary    Screen = "character-library"
	ScreenCharacterCreation   Screen = "character-creation"
	ScreenAttributeAllocation Screen = "attribute-allocation"
	ScreenCharacterStatus     Screen = "character-status"
	ScreenShipboard           Screen = "shipboard"
	ScreenGround              Screen = "ground"
	ScreenTrade               Screen = "trade"
	ScreenNavigation          Screen = "navigation"
	ScreenUniteller           Screen = "uniteller"
	ScreenTactical            Screen = "tactical"
	ScreenFlight              Screen = "flight"
	ScreenEngineering         Screen = "engineering"
	ScreenEquipment           Screen = "equipment"
	ScreenStores              Screen = "stores"
	ScreenParts               Screen = "parts"
	ScreenCargo               Screen = "cargo"
	ScreenDialog              Screen = "dialog"
	ScreenOutcome             Screen = "outcome"
	// ScreenInterior is the inside of a building: the original's XMOVEINB.
	ScreenInterior Screen = "interior"
	// ScreenSlots is the machine in the corner of the bar: XSLOTS.
	ScreenSlots Screen = "slots"
	// ScreenArena is ground combat: XDOCOMBA.
	ScreenArena Screen = "arena"
	// ScreenScore is the final tally: XSTARTUP:0x0d show_score.
	ScreenScore Screen = "score"
	// ScreenTubeStation is the tube station's own window: XDOUNITE:0x09
	// do_tube_station. Walking into the building opens "Tube Station" with
	// "Select destination city" centred and pick_location's boxes under it.
	ScreenTubeStation Screen = "tube-station"
)

type Action string

const (
	ActionPlay       Action = "play"
	ActionSelectGame Action = "select-game"
	// ActionOpenLibrary opens the save library from inside a game, so a slot
	// can be written without leaving the ship. Back returns to the screen it
	// was opened from (State.LibraryReturn).
	ActionOpenLibrary Action = "open-library"
	// ActionGroundTick is one iteration of xmove_on_ground's loop: the pixel
	// walk drives it while the button is held, and the street encounter rolls
	// on it the way the original rolls on every loop iteration.
	ActionGroundTick           Action = "ground-tick"
	ActionNewGame              Action = "new-game"
	ActionConfirmCharacterName Action = "confirm-character-name"
	ActionPreviousAttribute    Action = "previous-attribute"
	ActionNextAttribute        Action = "next-attribute"
	ActionIncreaseAttribute    Action = "increase-attribute"
	ActionDecreaseAttribute    Action = "decrease-attribute"
	ActionConfirmAttributes    Action = "confirm-attributes"
	ActionLoadGame             Action = "load-game"
	ActionMakeLibrary          Action = "make-library"
	ActionStoreGame            Action = "store-game"
	ActionDeleteGame           Action = "delete-game"
	ActionOpenStatus           Action = "open-status"
	ActionRest                 Action = "rest"
	ActionSleepTick            Action = "sleep-tick"
	ActionVBLTick              Action = "vbl-tick"
	ActionGroundClockTick      Action = "ground-clock-tick"
	ActionWake                 Action = "wake"
	ActionHand                 Action = "hand"
	ActionLoadAuxFuel          Action = "load-aux-fuel"
	ActionUnloadAuxFuel        Action = "unload-aux-fuel"
	ActionEat                  Action = "eat"
	ActionOpenStores           Action = "open-stores"
	ActionPreviousStore        Action = "previous-store"
	ActionNextStore            Action = "next-store"
	ActionTakeStore            Action = "take-store"
	ActionCancelItem           Action = "cancel-item"
	ActionPreviousPocket       Action = "previous-pocket"
	ActionNextPocket           Action = "next-pocket"
	ActionMoveNorth            Action = "move-north"
	ActionMoveSouth            Action = "move-south"
	ActionMoveWest             Action = "move-west"
	ActionMoveEast             Action = "move-east"
	ActionRefuel               Action = "refuel"
	ActionOpenGround           Action = "open-ground"
	ActionOpenTrade            Action = "open-trade"
	ActionOpenNavigation       Action = "open-navigation"
	ActionPreviousDestination  Action = "previous-destination"
	ActionNextDestination      Action = "next-destination"
	// The source world has 12 systems, 18 planets and 58 cities, so the route
	// selector walks that hierarchy instead of one flat 58-entry list.
	ActionPreviousTargetPlanet Action = "previous-target-planet"
	ActionNextTargetPlanet     Action = "next-target-planet"
	ActionPreviousTargetSystem Action = "previous-target-system"
	ActionNextTargetSystem     Action = "next-target-system"
	ActionOpenUniteller        Action = "open-uniteller"
	ActionOpenTactical         Action = "open-tactical"
	ActionOpenEngineering      Action = "open-engineering"
	ActionOpenEquipment        Action = "open-equipment"
	ActionOpenCargo            Action = "open-cargo"
	ActionOpenParts            Action = "open-parts"
	ActionPreviousPart         Action = "previous-part"
	ActionNextPart             Action = "next-part"
	ActionBuyPart              Action = "buy-part"
	ActionInstallPart          Action = "install-part"
	ActionTalk                 Action = "talk"
	ActionPreviousVerb         Action = "previous-verb"
	ActionNextVerb             Action = "next-verb"
	ActionSayVerb              Action = "say-verb"
	ActionLoadSupplies         Action = "load-supplies"
	ActionLoadCryogens         Action = "load-cryogens"
	ActionDeliverMission       Action = "deliver-mission"
	ActionNavigate             Action = "navigate"
	ActionFlightTick           Action = "flight-tick"
	ActionFlightEngage         Action = "flight-engage"
	ActionFlightResume         Action = "flight-resume"
	ActionDepositCash          Action = "deposit-cash"
	ActionWithdrawCash         Action = "withdraw-cash"
	ActionTransferBank         Action = "transfer-bank"
	ActionBuyGems              Action = "buy-gems"
	ActionSellGems             Action = "sell-gems"
	ActionTacticalMove         Action = "tactical-move"
	ActionTacticalStop         Action = "tactical-stop"
	ActionTradePass            Action = "trade-pass"
	ActionTradeExchange        Action = "trade-exchange"
	ActionTradeWarehousePage   Action = "trade-warehouse-page"
	ActionTradeStorePage       Action = "trade-store-page"
	ActionTradeFetchPage       Action = "trade-fetch-page"
	ActionTradeSwapPage        Action = "trade-swap-page"
	ActionTradePick1           Action = "trade-pick-1"
	ActionTradePick2           Action = "trade-pick-2"
	ActionTradePick3           Action = "trade-pick-3"
	ActionTradePick4           Action = "trade-pick-4"
	ActionTradeBuyPage         Action = "trade-buy-page"
	ActionTellerDepositPage    Action = "teller-deposit-page"
	ActionTellerWithdrawPage   Action = "teller-withdraw-page"
	ActionTellerTransferPage   Action = "teller-transfer-page"
	ActionTellerAmountAll      Action = "teller-amount-all"
	ActionTellerAmount100      Action = "teller-amount-100"
	ActionTellerAmount1000     Action = "teller-amount-1000"
	ActionTellerAmount10000    Action = "teller-amount-10000"
	ActionTellerAmount50000    Action = "teller-amount-50000"
	ActionTellerAmountClear    Action = "teller-amount-clear"
	ActionTellerConfirm        Action = "teller-confirm"
	ActionTellerPick1          Action = "teller-pick-1"
	ActionTellerPick2          Action = "teller-pick-2"
	ActionTellerPick3          Action = "teller-pick-3"
	ActionTellerPick4          Action = "teller-pick-4"
	ActionTellerPick5          Action = "teller-pick-5"
	ActionTellerPick6          Action = "teller-pick-6"
	ActionTellerOthers         Action = "teller-others"
	ActionTradeSellPage        Action = "trade-sell-page"
	ActionFire                 Action = "fire"
	ActionFireLaser            Action = "fire-laser"
	ActionTractor              Action = "tractor"
	// do_jettison's menu is hold 1, hold 2, or both — the pod is two records,
	// so the choice is by slot, not by what happens to be in them.
	ActionJettisonHold1     Action = "jettison-hold-1"
	ActionJettisonHold2     Action = "jettison-hold-2"
	ActionJettisonAll       Action = "jettison-all"
	ActionHail              Action = "hail"
	ActionDistressCall      Action = "distress-call"
	ActionPayFine           Action = "pay-fine"
	ActionOpenTubeStation   Action = "open-tube-station"
	ActionRideTube          Action = "ride-tube"
	ActionTubePick1         Action = "tube-pick-1"
	ActionTubePick2         Action = "tube-pick-2"
	ActionTubePick3         Action = "tube-pick-3"
	ActionTubePick4         Action = "tube-pick-4"
	ActionTubePick5         Action = "tube-pick-5"
	ActionTubePick6         Action = "tube-pick-6"
	ActionRechargeWeapon    Action = "recharge-weapon"
	ActionShieldsOff        Action = "shields-off"
	ActionFullShields       Action = "full-shields"
	ActionPreviousSystem    Action = "previous-system"
	ActionNextSystem        Action = "next-system"
	ActionRepairSystem      Action = "repair-system"
	ActionRepair            Action = "repair"
	ActionRepairWeapon      Action = "repair-weapon"
	ActionRepairShields     Action = "repair-shields"
	ActionLoadCargo         Action = "load-cargo"
	ActionUnloadCargo       Action = "unload-cargo"
	ActionBack              Action = "back"
	ActionCyclePresentation Action = "cycle-presentation"
	// ActionOpenOptions is answered by the app, not the reducer: the options
	// card is presentation and settings, never game state.
	ActionOpenOptions Action = "open-options"
	// ActionAboutSunDog shows the original's credits picture; app-level too.
	ActionAboutSunDog Action = "about-sundog"
	ActionSave        Action = "save"
	ActionQuit        Action = "quit"
	// Building interiors.
	ActionEnterBuilding Action = "enter-building"
	ActionLeaveBuilding Action = "leave-building"
	ActionInteriorNorth Action = "interior-north"
	ActionInteriorSouth Action = "interior-south"
	ActionInteriorWest  Action = "interior-west"
	ActionInteriorEast  Action = "interior-east"
	ActionUseCounter    Action = "use-counter"
	ActionSitAtBooth    Action = "sit-at-booth"
	// Slot machine.
	ActionOpenSlots Action = "open-slots"
	ActionSpinSlots Action = "spin-slots"
	// Ground combat arena.
	ActionArenaAttack Action = "arena-attack"
	ActionArenaFlee   Action = "arena-flee"
	// Landing and take-off animation.
	ActionLandingTick Action = "landing-tick"
	ActionSkipLanding Action = "skip-landing"
	// Final score.
	ActionOpenScore Action = "open-score"
)

// Heading is which way the character last walked. The original's character
// status window has a compass, and a compass needs something to point with.
type Heading string

const (
	HeadingNorth Heading = "north"
	HeadingSouth Heading = "south"
	HeadingWest  Heading = "west"
	HeadingEast  Heading = "east"
)

// CargoLots is how many lots the cargo pod holds. The manual says two, and
// says a lot may be given over to auxiliary fuel instead of goods.
const CargoLots = 2

// CarrySlots is how many objects the character can carry on the body. Four is
// the original's number, not a remake choice: MAINLIB_G3f is a byte array and
// two independent writers clamp the index to 0..3 — XDOINTER:0x28 item_check
// tests set membership against the constant {0,1,2,3}, and WINDOWLI:0x04
// swap_items uses a range test. See docs/atari-st/ground-rules.md.
const CarrySlots = 4

// NavigationView names one of the original's three route maps.
type NavigationView string

const (
	// NavigationSector is the three-dimensional star chart: one circle per star
	// system, its size standing for the distance along the third axis.
	NavigationSector NavigationView = "sector"
	// NavigationSystem is the star system map: orbits as circles and the
	// planets on them.
	NavigationSystem NavigationView = "system"
	// NavigationPlanet is the planet map: its cities as squares.
	NavigationPlanet NavigationView = "planet"
)

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// LegacyPod is the cargo pod as schema 29 and earlier stored it: counts by
// kind plus a list of goods lots. Schema 30 replaced it with State.Holds, two
// records in slot order (hold.go). These fields stay so an older save can be
// read; migrate rebuilds Holds from them and then zeroes them.
type LegacyPod struct {
	LegacyCargoUnits   int    `json:"cargoUnits,omitempty"`
	LegacySupplies     int    `json:"supplies,omitempty"`
	LegacyCryogens     int    `json:"cryogens,omitempty"`
	LegacyAuxFuelTanks int    `json:"auxFuelTanks,omitempty"`
	LegacySalvageLots  int    `json:"salvageLots,omitempty"`
	LegacyCargoItemKey string `json:"cargoItemKey,omitempty"`
	LegacyPodLots      []Lot  `json:"pod,omitempty"`
}

type State struct {
	Schema               int        `json:"schema"`
	Screen               Screen     `json:"screen"`
	Character            string     `json:"character"`
	CharacterDraft       string     `json:"characterDraft"`
	AttributePoints      int        `json:"attributePoints"`
	AttributeSelected    int        `json:"attributeSelected"`
	AttributeWasteWarned bool       `json:"attributeWasteWarned"`
	Location             string     `json:"location"`
	SystemID             string     `json:"systemId"`
	PlanetID             string     `json:"planetId"`
	CityID               string     `json:"cityId"`
	Position             Position   `json:"position"`
	GroundWorld          bool       `json:"groundWorld"`
	GroundView           GroundView `json:"groundView"`
	// ShipCityID/ShipPosition is where the SunDog is parked. The original keeps
	// the ship's own city, column, row and sub-cell pixel offset in MAINLIB
	// G4a–G4e, and XMOVEONG:0x07 put_ship only draws it when that city is the
	// one being walked. The remake keeps the city and the cell; a grid has no
	// sub-cell offset to keep.
	ShipCityID          string       `json:"shipCityId"`
	ShipPosition        Position     `json:"shipPosition"`
	Cash                int          `json:"cash"`
	Bank                int          `json:"bank"`
	Gems                int          `json:"gems"`
	Fuel                int          `json:"fuel"`
	WeaponCharge        int          `json:"weaponCharge"`
	ShieldLevel         int          `json:"shieldLevel"`
	HullCondition       int          `json:"hullCondition"`
	ShipSystems         []ShipSystem `json:"shipSystems"`
	SelectedSystemIndex int          `json:"selectedSystemIndex"`
	Shunts              int          `json:"shunts"`
	Parts               []int        `json:"parts"`
	Stores              []int        `json:"stores"`
	SelectedPartIndex   int          `json:"selectedPartIndex"`
	ShieldsActive       bool         `json:"shieldsActive"`
	ShieldSetting       int          `json:"shieldSetting"`
	ShipControls        ShipControls `json:"shipControls"`
	ShipPhase           int          `json:"shipPhase"`
	CurrentSpeed        int          `json:"currentSpeed"`
	TargetSpeed         int          `json:"targetSpeed"`
	GroundMode          GroundMode   `json:"groundMode"`
	CargoAttached       bool         `json:"cargoAttached"`
	CargoCityID         string       `json:"cargoCityId"`
	CargoPosition       Position     `json:"cargoPosition"`
	TractorActive       bool         `json:"tractorActive"`
	FuelRescuePending   bool         `json:"fuelRescuePending"`
	FuelRescueDue       int          `json:"fuelRescueDue"`
	FuelJettisonPending bool         `json:"fuelJettisonPending"`
	ShipRefresh         bool         `json:"shipRefresh"`
	EnemyClosing        int          `json:"enemyClosing"`
	FuelPurchase        FuelPurchase `json:"fuelPurchase"`
	JumpRange           int          `json:"jumpRange"`
	CombatRolls         int          `json:"combatRolls"`
	// The four condition readings the manual's page 6 lists, in its order.
	// They live in MAINLIB_G3d as four bytes, so the scale here is the
	// original's 0-255 rather than a percentage: the decay tables are indexed
	// by value/0x40 and rescaling would blur the four bands. See the ground
	// rules condition.decay and condition.vigor-collapse.
	Vigor       int    `json:"vigor"`
	Rest        int    `json:"rest"`
	Health      int    `json:"health"`
	Nourishment int    `json:"nourishment"`
	FoodUnits   int    `json:"foodUnits"`
	FoodItemKey string `json:"foodItemKey"`
	// LegacyPod carries the pod fields schema 29 and earlier wrote. Only
	// migrate reads them, and it clears them once Holds is built, so a game
	// saved from here on carries the holds and nothing else.
	LegacyPod
	// Holds is the cargo pod's two holds, in slot order — the original's Gec
	// (hold.go). What is in them replaces the counters this shell used to keep
	// (goods, auxiliary fuel, salvage, and the mission's two lots): counts by
	// kind cannot say which hold the bolts blow.
	Holds               [CargoLots]Hold `json:"holds"`
	SuppliesDelivered   int             `json:"suppliesDelivered"`
	CryogensDelivered   int             `json:"cryogensDelivered"`
	MissionComplete     bool            `json:"missionComplete"`
	Colony              ColonyProgress  `json:"colony"`
	ColonySite          ColonySite      `json:"colonySite"`
	BankBalances        map[string]int  `json:"bankBalances"`
	VisitedDestinations []string        `json:"visitedDestinations"`
	DialogBlock         int             `json:"dialogBlock"`
	DialogLine          int             `json:"dialogLine"`
	DialogVerb          int             `json:"dialogVerb"`
	DialogVariant       int             `json:"dialogVariant"`
	DialogSaid          string          `json:"dialogSaid"`
	NavigationTargetID  string          `json:"navigationTargetId"`
	NavigationTarget    string          `json:"navigationTarget"`
	// NavigationView is which of the original's three route maps is on screen.
	// The original has a sector chart, a system map and a planet map, and the
	// player descends through them; here the map follows whichever level of the
	// selector was last moved.
	NavigationView NavigationView `json:"navigationView"`
	// HandItem is the complete packed byte of the dragged object; ItemCode
	// selects its source item record. Zero is an empty hand.
	HandItem int `json:"handItem"`
	// HandOrigin records the cleared slot while an object is being dragged.
	HandOrigin ItemOrigin `json:"handOrigin"`
	// Heading is what the compass points at: the direction the character last
	// walked in.
	Heading Heading `json:"heading"`
	// StreetRolls counts the street-encounter draws a city walk has made, so
	// the sequence is reproducible rather than wall-clock random.
	StreetRolls int `json:"streetRolls"`
	// StreetEasterEggSeen records the one-time gate on the developer credit a
	// beggar can open (G53 bit 0x7d). Once it has been shown, the original
	// never offers it again.
	StreetEasterEggSeen bool `json:"streetEasterEggSeen"`
	// StreetPending is the encounter kind waiting for the conversation in
	// front of it to end. Only a mugger sets it: he talks first and fights
	// afterwards.
	StreetPending int `json:"streetPending"`
	// StreetAsk is what a beggar opened with, so the shell can name the figure
	// he is asking for.
	StreetAsk           int             `json:"streetAsk"`
	StatusReturn        Screen          `json:"statusReturn"`
	Sleep               SleepState      `json:"sleep"`
	AwakeClock          AwakeClockState `json:"awakeClock"`
	SleepTheftProtected bool            `json:"sleepTheftProtected"`
	// LibraryReturn is the screen the save library was opened from, so Back
	// goes there instead of to the title. Empty means it was opened from the
	// title, which is where the original's Select Game lives.
	LibraryReturn      Screen `json:"libraryReturn,omitempty"`
	EnemyHull          int    `json:"enemyHull"`
	EnemyMaxHull       int    `json:"enemyMaxHull"`
	EnemyType          int    `json:"enemyType"`
	EnemyFireCountdown int    `json:"enemyFireCountdown"`
	CombatRange        int    `json:"combatRange"`
	EnemyCourseMode    int    `json:"enemyCourseMode"`
	// EnemyBearing and EnemyPitch are the two axes the original tracks the
	// enemy on (MAINLIB_Gd0 and Gcf, 0–0x3ff); EnemyDriftBearing and
	// EnemyDriftPitch are the per-tick drifts set_course gives them
	// (Gd3 and Gd2). They feed the tactical scanners.
	EnemyBearing      int `json:"enemyBearing"`
	EnemyPitch        int `json:"enemyPitch"`
	EnemyDriftBearing int `json:"enemyDriftBearing"`
	EnemyDriftPitch   int `json:"enemyDriftPitch"`
	// TacticalSlewing is the original's do_guns loop: the mouse steers the
	// ship until a button ends it. SlewBearing and SlewPitch are the rates
	// the last pass read (SHIPLIB_Ge and Gd).
	TacticalSlewing bool `json:"tacticalSlewing,omitempty"`
	SlewBearing     int  `json:"slewBearing,omitempty"`
	SlewPitch       int  `json:"slewPitch,omitempty"`
	// TradePage preserves the exchange page, including the source choice
	// and warehouse sale/swap lists. Values are defined in cargo.go.
	TradePage    int          `json:"tradePage,omitempty"`
	TradeSession TradeSession `json:"tradeSession"`
	// TradeSaleSlot identifies a selected lot (1..4 warehouse, 5..6 pod).
	// On TradePageSwapWarehouse it is the pending pod source, with price 0.
	TradeSaleSlot    int  `json:"tradeSaleSlot"`
	TradeSalePrice   int  `json:"tradeSalePrice"`
	TradePurchaseLot *Lot `json:"tradePurchaseLot,omitempty"`
	// The exchange's economy (exchange.go): the quote ring, each city's supply
	// and demand, each city's warehouse. The pod itself is State.Holds.
	Quotes          []Lot                 `json:"quotes,omitempty"`
	QuoteIndex      int                   `json:"quoteIndex,omitempty"`
	Markets         map[string]CityMarket `json:"markets,omitempty"`
	Warehouses      map[string][]Lot      `json:"warehouses"`
	TradeRolls      int                   `json:"tradeRolls,omitempty"`
	TradeClassShift int                   `json:"tradeClassShift"`
	LastSalePrice   int                   `json:"lastSalePrice,omitempty"`
	// TellerPage is the Uniteller's move_cash keypad when it is up
	// (XDOUNITE:0x0c): which of deposit, withdraw and transfer it is
	// composing an amount for; TellerAmount is the amount so far.
	TellerPage   int `json:"tellerPage,omitempty"`
	TellerAmount int `json:"tellerAmount,omitempty"`
	// TellerPickPage is which page of pick_location\'s transfer list is up;
	// TellerTransferID is the system the list picked for the keypad.
	TellerPickPage   int    `json:"tellerPickPage,omitempty"`
	TellerTransferID string `json:"tellerTransferId,omitempty"`
	// TubePage is which of do_tube_station's two pages is up, and TubeTargetID
	// the city the first one picked for the second.
	TubePage          int    `json:"tubePage,omitempty"`
	TubeTargetID      string `json:"tubeTargetId,omitempty"`
	FlightAngle       int    `json:"flightAngle"`
	FlightRadius      int    `json:"flightRadius"`
	FlightFromAngle   int    `json:"flightFromAngle"`
	FlightFromRadius  int    `json:"flightFromRadius"`
	FlightDeltaAngle  int    `json:"flightDeltaAngle"`
	FlightDeltaRadius int    `json:"flightDeltaRadius"`
	FlightTick        int    `json:"flightTick"`
	FlightTicks       int    `json:"flightTicks"`
	FlightTargetID    string `json:"flightTargetId"`
	// PlanetAngles is where each of the eighteen planets currently sits on its
	// orbit, keyed by the source table's index. The original keeps the same
	// thing in its planet records: the angle is live state, re-rolled at the
	// start of a new game and advanced daily. See internal/game/orbit.go.
	PlanetAngles []int `json:"planetAngles"`
	// OrbitDay is the day count the orbits were last advanced on, which is what
	// check_planets compares against to decide whether to run.
	OrbitDay int `json:"orbitDay"`
	// PodParkedAt is the clock reading when the ship was last left on a city
	// street, which is what the parking fine is measured from. Negative means
	// the meter has not started — a new game is like that, the player is still
	// aboard. See internal/game/fine.go.
	PodParkedAt int `json:"podParkedAt"`
	// SystemHeat is how much trouble the player has caused in each star system.
	// The original keeps the same count in nibble 7 of the system record; see
	// internal/game/heat.go.
	SystemHeat map[string]int `json:"systemHeat"`
	// SystemCoordinates is this game's star chart: the original re-rolls the
	// twelve systems' packed coordinates at new-game (internal/game/sector.go),
	// and every jump is measured on the result. Saves from before schema 22
	// were played on the factory chart and keep it.
	SystemCoordinates world.SystemCoordinates `json:"systemCoordinates"`
	CombatActive      bool                    `json:"combatActive"`
	CombatVictory     bool                    `json:"combatVictory"`
	TimeMinutes       int                     `json:"timeMinutes"`
	Theme             theme.ID                `json:"theme"`
	Tileset           theme.TilesetID         `json:"tileset"`
	Layout            theme.LayoutID          `json:"layout"`
	// Attributes and AttributeMax are the original's five: ST, IQ, DX, CH, LK.
	Attributes   [AttributeCount]int `json:"attributes"`
	AttributeMax [AttributeCount]int `json:"attributeMax"`
	// BanvilleCargo and Deaths are the two score inputs the original keeps in
	// MAINLIB_G41 and G42.
	BanvilleCargo int `json:"banvilleCargo"`
	Deaths        int `json:"deaths"`
	// Interior, Slots, Arena and Landing are the four subsystems that live
	// below the city ground: XMOVEINB, XSLOTS, XDOCOMBA and XLANDFX.
	Interior Interior    `json:"interior"`
	Slots    SlotMachine `json:"slots"`
	Arena    Arena       `json:"arena"`
	Landing  Landing     `json:"landing"`
	// GroundReturn is the screen to go back to when a subsystem closes.
	GroundReturn  Screen `json:"groundReturn"`
	MessageKey    string `json:"messageKey"`
	OutcomeKey    string `json:"outcomeKey"`
	Saved         bool   `json:"saved"`
	QuitRequested bool   `json:"quitRequested"`
}

// landingPosition is where the player stands in a city. The cell comes from the
// city's own decoded map: the exchange building, the one building every one of
// the 60 cities has.
func landingPosition(cityID string) Position {
	tile := world.LandingPosition(cityID)
	return Position{X: tile.X, Y: tile.Y}
}

func NewState() State {
	newGameBalances := newGameBankBalances(NewGameSeed(), world.FactorySystems())
	state := State{
		Schema:          CurrentSchema,
		AwakeClock:      newAwakeClock(),
		ColonySite:      initialColonySite(),
		TradeClassShift: newGameTradeClassShift(NewGameSeed()),
		Warehouses:      newGameWarehouses(NewGameSeed()),
		Screen:          ScreenTitle,
		Character:       "Zed",
		CharacterDraft:  "Zed",
		Location:        defaultStartLabel,
		SystemID:        world.StartSystemID,
		PlanetID:        world.StartPlanetID,
		CityID:          world.StartDestinationID,
		Position:        landingPosition(world.StartDestinationID),
		ShipCityID:      world.StartDestinationID,
		ShipPosition:    landingPosition(world.StartDestinationID),
		// The balance words are 0 on disk for all twelve systems; XSTARTUP's
		// world loop fills them at new-game (see newGameBankBalances).
		Cash:                newGameCash(NewGameSeed()),
		SystemCoordinates:   JitterSystemCoordinates(NewGameSeed(), world.FactorySystems()),
		Bank:                newGameBalances[world.StartSystemID],
		BankBalances:        newGameBalances,
		VisitedDestinations: []string{world.StartDestinationID},
		Theme:               theme.DefaultPresentation().Chrome,
		Tileset:             theme.DefaultPresentation().Tileset,
		Layout:              theme.DefaultPresentation().Layout,
		// The original writes no fuel at all in set_new_game, so a new game
		// starts empty and LIFTOFF answers "Out of fuel" until the player buys
		// some at a depot (docs/atari-st/ship-rules.md, worklist D6).
		Fuel: 0,
		// The parking meter has not started: the player is still aboard, and
		// the original's Uniteller shows no PAY FINE on a fresh game (the
		// eighth round's capture has TRANSFER／WITHDRAW／DEPOSIT).
		PodParkedAt:  parkingMeterUnstarted,
		WeaponCharge: 100,
		// Keep the existing explicit starting values until the original
		// starting charge is verified; a larger range does not fill a tank.
		ShieldLevel:   100,
		ShieldSetting: 100,
		ShipControls:  ShipControls{Guns: true},
		GroundMode:    GroundOnFoot,
		CargoAttached: true,
		CargoCityID:   world.StartDestinationID,
		CargoPosition: landingPosition(world.StartDestinationID),
		JumpRange:     world.MaxJumpDistance(),
		HullCondition: MaxHullCondition(),
		ShipSystems:   defaultShipSystems(NewGameSeed()),
		Shunts:        newGameShunts(),
		Stores:        defaultShipStores(),
		Parts:         make([]int, PocketCount),
		ShieldsActive: true,
		// A new game fills all four condition readings: XSTARTUP:0x30 writes
		// 0xff into each of G3d's four bytes.
		Vigor:              MaxCondition,
		Rest:               MaxCondition,
		Health:             MaxCondition,
		Nourishment:        MaxCondition,
		FoodUnits:          0, // Historical UI field; food comes from physical item bytes.
		FoodItemKey:        "apple2.ship.food",
		NavigationTargetID: world.MissionDestinationID,
		NavigationTarget:   defaultMissionLabel,
		NavigationView:     NavigationSector,
		Heading:            HeadingNorth,
		StatusReturn:       ScreenShipboard,
		CombatRange:        initialCombatRange,
		Attributes:         newAttributes(),
		AttributeMax:       newAttributes(),
		GroundReturn:       ScreenGround,
		MessageKey:         "ui.message.ready",
	}
	// The ship starts where its planet is. Filling that in here rather than
	// only on load is what keeps a new game and a reloaded one the same state.
	normalizeWorldFields(&state)
	// A fresh state carries the source table's angles until a new game rolls
	// them; a loaded save gets the same seeding in normalizePlanetAngles, so the
	// two agree.
	normalizePlanetAngles(&state)
	return state
}

// Migrate validates the current remake state. Old saves lack the persistent
// scheduler and attachment fields and are explicitly unsupported.
func Migrate(state State) (State, error) {
	if err := ValidateAwakeClockState(state); err != nil {
		return State{}, err
	}
	if err := ValidateSleepState(state); err != nil {
		return State{}, err
	}
	if err := ValidateFuelState(state); err != nil {
		return State{}, err
	}
	if err := ValidateTradeState(state); err != nil {
		return State{}, err
	}
	if err := ValidateAttributeState(state); err != nil {
		return State{}, err
	}
	if state.Character == "" {
		state.Character = "Zed"
	}
	if state.NavigationTarget == "" {
		state.NavigationTarget = defaultMissionLabel
	}
	if state.StatusReturn == "" {
		state.StatusReturn = ScreenShipboard
	}
	normalizeShipSystems(&state)
	normalizeLocker(&state)
	normalizeWorldFields(&state)
	normalizeAttributeAllocation(&state)
	normalizePlanetAngles(&state)
	if state.GroundReturn == "" {
		state.GroundReturn = ScreenGround
	}
	return state, nil
}

// SetCharacterNameDraft is the shell-facing, deterministic input boundary for
// the remake-owned character creation screen. It does not claim the original
// name length or character encoding rules.
func SetCharacterNameDraft(state State, draft string) State {
	state.CharacterDraft = normalizeCharacterName(draft)
	return state
}

// AppendCharacterNameDraft adds text input from an interactive shell while
// retaining a bounded, valid UTF-8 draft for the CJK UI.
//
// The draft is deliberately not trimmed here. Trimming per keystroke eats the
// space the moment it is typed, so the next letter lands against the previous
// word and a name like 「Mary Jo」 can never be typed at all. Trailing space is
// trimmed where it stops being an edit and becomes the name: at confirmation.
// A leading space is still refused, so the draft never opens with one.
func AppendCharacterNameDraft(state State, input []rune) State {
	current := []rune(state.CharacterDraft)
	for _, value := range input {
		if len(current) >= CharacterNameMaxRunes || unicode.IsControl(value) {
			continue
		}
		if len(current) == 0 && unicode.IsSpace(value) {
			continue
		}
		current = append(current, value)
	}
	draft := string(current)
	if !utf8.ValidString(draft) {
		return state
	}
	state.CharacterDraft = draft
	return state
}

func BackspaceCharacterNameDraft(state State) State {
	current := []rune(state.CharacterDraft)
	if len(current) > 0 {
		current = current[:len(current)-1]
	}
	state.CharacterDraft = string(current)
	return state
}

func CharacterNameValid(name string) bool {
	name = normalizeCharacterName(name)
	return name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= CharacterNameMaxRunes
}

func normalizeCharacterName(name string) string {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) {
		return ""
	}
	runes := []rune(name)
	if len(runes) > CharacterNameMaxRunes {
		runes = runes[:CharacterNameMaxRunes]
	}
	return string(runes)
}

func normalizeWorldFields(state *State) {
	if state.SystemID == "" {
		state.SystemID = world.StartSystemID
	}
	if state.PlanetID == "" {
		state.PlanetID = world.StartPlanetID
	}
	if state.CityID == "" {
		state.CityID = world.StartDestinationID
	}
	if state.Location == "" {
		state.Location = destinationLabel(state.CityID, defaultStartLabel)
	}
	if state.NavigationTargetID == "" {
		state.NavigationTargetID = world.MissionDestinationID
	}
	if state.NavigationTarget == "" {
		state.NavigationTarget = destinationLabel(state.NavigationTargetID, defaultMissionLabel)
	}
	if state.FoodItemKey == "" {
		state.FoodItemKey = "apple2.ship.food"
	}
	// The ship has to be parked somewhere. A save from before schema 27 carries
	// no ship place at all, and one whose ship city is not a city any more is
	// no better, so both fall back to the landing square of the city the save
	// is in.
	if state.ShipCityID == "" {
		state.ShipCityID = state.CityID
		state.ShipPosition = landingPosition(state.CityID)
	}
	// A save written against the old shared prototype map can hold a position
	// that the city's real map blocks, or that is outside it. Put the player
	// back on the landing cell rather than leaving them stuck.
	if _, mapped := world.CityMap(state.CityID); mapped && !state.GroundWorld && !state.GroundView.Valid {
		if terrain, ok := GroundTerrainAt(*state, state.CityID, state.Position.X, state.Position.Y); !ok || !world.IsPassable(terrain) {
			if !GroundPositionValid(*state, state.Position) || (ok && !world.IsPassable(terrain)) {
				state.Position = landingPosition(state.CityID)
			}
		}
	}
	if !GroundPositionValid(*state, state.Position) {
		state.Position = GroundWrap(*state, state.Position)
		state.GroundView = GroundView{}
	}
	if state.GroundView.Valid {
		pitch := GroundPitch(*state)
		view := state.GroundView
		position := GroundWrap(*state, Position{view.Origin.X + view.Anchor.X/pitch, view.Origin.Y + view.Anchor.Y/pitch})
		if view.Anchor.X < 0 || view.Anchor.X >= 320 || view.Anchor.Y < 0 || view.Anchor.Y >= 200 || position != state.Position {
			state.GroundView = GroundView{}
		}
	}
	// A ship with no place in its star system cannot be projected, and neither
	// can one whose radius is zero: it would sit on the star. Either way the
	// planet it is standing on is where it belongs.
	if state.FlightRadius <= 0 {
		if planet, ok := geographyPlanet(state.PlanetID); ok {
			state.FlightAngle = planet.Orbit.Angle
			state.FlightRadius = planet.Orbit.Radius
		}
	}
	if state.BankBalances == nil {
		state.BankBalances = map[string]int{state.SystemID: state.Bank}
	}
	state.BankBalances[state.SystemID] = state.Bank
	if len(state.VisitedDestinations) == 0 {
		state.VisitedDestinations = []string{state.CityID}
	}
	if state.Screen == ScreenDialog {
		if err := dialogStateValid(*state); err != nil {
			state.Screen = ScreenShipboard
			state.DialogBlock = 0
			state.DialogLine = 0
			state.DialogVerb = 0
			state.DialogSaid = ""
			state.MessageKey = "ui.message.dialog_unavailable"
		}
	}
}

func normalizeAttributeAllocation(state *State) {
	if state.AttributeSelected < 0 || state.AttributeSelected >= AttributeCount {
		state.AttributeSelected = AttrStrength
	}
	if state.AttributePoints < 0 {
		state.AttributePoints = 0
	}
}

// presentationTriple spreads a preset over the three fields the state still
// carries. Keeping the fields means a save written before the presets, the
// settings file and every draw path all keep reading one place; what changed
// is that only a preset's triple can get in.
func presentationTriple(preset theme.Presentation) (theme.ID, theme.TilesetID, theme.LayoutID) {
	return preset.Chrome, preset.Tileset, preset.Layout
}

// PresentationOf is the preset a state is showing.
func PresentationOf(state State) theme.Presentation {
	return theme.SnapPresentation(state.Theme, state.Tileset, state.Layout)
}

// WithPresentation returns state showing preset. The caller decides the
// message; the options card and the reducer both go through here so the three
// fields cannot drift apart.
func WithPresentation(state State, preset theme.Presentation) State {
	state.Theme, state.Tileset, state.Layout = presentationTriple(preset)
	return state
}
