package game

import (
	"fmt"

	apple2source "github.com/wicanr2/sundog-remake/internal/source/apple2"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

const (
	maxCargoUnits   = 2 // manual capacity description; prototype cargo model
	maxFuel         = 255
	maxWeaponCharge = 255
	maxShieldLevel  = 255
	// The three readings that predate schema 29 now share the original's own
	// ceiling; Rest joined them. See internal/game/condition.go.
	maxCharacterHealth   = MaxCondition
	maxCharacterVigor    = MaxCondition
	maxNourishment       = MaxCondition
	prototypeShuntRepair = 25
)

type Reducer struct {
	Themes      theme.Registry
	World       world.Registry
	AppleIIText *apple2source.TextCatalog
	DialogText  *atarist.DialogData
	// Localize is set by a shell that owns a localization catalog. The rules
	// layer needs it only where original text embeds a place name mid-sentence,
	// which no message key can express.
	Localize func(key, fallback string) string
}

// WithDialogText 切換玩家可見的對話語料；對話游標仍留在遊戲狀態中，因此不改存檔格式。
func (r Reducer) WithDialogText(data atarist.DialogData) Reducer {
	r.DialogText = &data
	return r
}

// WithLocalizer attaches a player-visible lookup to the product adapter.
func (r Reducer) WithLocalizer(lookup func(key, fallback string) string) Reducer {
	r.Localize = lookup
	return r
}

// WithAppleIIText attaches the validated source manifest to the product
// adapter. The pointer is optional so pure reducer tests remain independent
// of repository files; the interactive shells always attach it.
func (r Reducer) WithAppleIIText(catalog apple2source.TextCatalog) Reducer {
	r.AppleIIText = &catalog
	return r
}

func NewReducer() (Reducer, error) {
	registry, err := theme.NewPrototypeRegistry()
	if err != nil {
		return Reducer{}, err
	}
	worldRegistry, err := world.NewSourceRegistry()
	if err != nil {
		return Reducer{}, err
	}
	dialog, _ := atarist.EmbeddedTranslatedDialogData()
	return Reducer{Themes: registry, World: worldRegistry, DialogText: &dialog}, nil
}

// Step applies one action. The planets are advanced afterwards rather than
// inside each case: XMOVEONS:0x07 check_planets runs off the clock, not off any
// particular action, and it does nothing at all until the day count changes.
func (r Reducer) Step(state State, action Action) (State, error) {
	next, err := r.step(state, action)
	if err != nil {
		return state, err
	}
	// SHIPLIB:1f commits the destination in the same update that finishes
	// mode2. Background and sleep callbacks must consume that transition too.
	// See ship-rules.md: natural arrival READY, not just the manual shortcut.
	if next.FlightTargetID != "" && next.FlightTicks > 0 && FlightArrived(next) {
		destination, ok := r.World.Destination(next.FlightTargetID)
		if !ok {
			return state, fmt.Errorf("unknown flight destination %q", next.FlightTargetID)
		}
		screen, message := next.Screen, next.MessageKey
		arriveAt(&next, destination)
		if screen != ScreenFlight {
			next.Screen, next.MessageKey = screen, message
		}
		if next.StatusReturn == ScreenFlight {
			next.StatusReturn = ScreenShipboard
		}
		if next.LibraryReturn == ScreenFlight {
			next.LibraryReturn = ScreenShipboard
		}
	}
	if next.TimeMinutes != state.TimeMinutes && next.Vigor <= 0 {
		// Early-return actions also have to consume the clock's death mode.
		interruptForConditions(&next)
	}
	if next.Sleep.Active && !state.Sleep.Active {
		// An atomic action may finish a scene transition after its clock
		// call. Present the interrupt before returning to the shell, and keep
		// the resulting view for Back rather than losing the flight route.
		if next.Screen != ScreenCharacterStatus {
			if ship, valid := sleepScene(next.Screen); valid {
				next.StatusReturn = next.Screen
				next.Sleep.ShipActive = ship
			}
		}
		next.Screen = ScreenCharacterStatus
		next.TacticalSlewing = false
		next.MessageKey = "ui.message.sleeping"
	}
	advancePlanetOrbits(&next, r.World.Planets())
	if next.TradeSession.Active && !tradeSessionContext(next) {
		finishTradeSession(&next)
	}
	if next.Screen == ScreenGround && state.Screen != ScreenGround && state.Screen != ScreenCharacterLibrary {
		resetGroundClock(&next)
	}
	syncAwakeClockLifecycle(&next)
	return next, nil
}

func (r Reducer) step(state State, action Action) (State, error) {
	if state.Schema != CurrentSchema {
		return State{}, fmt.Errorf("unsupported state schema %d", state.Schema)
	}
	if state.Sleep.Active && state.Screen == ScreenCharacterStatus {
		switch action {
		case ActionRest, ActionWake, ActionBack, ActionSleepTick, ActionVBLTick, ActionSave, ActionQuit, ActionOpenLibrary, ActionCyclePresentation:
		default:
			return state, nil
		}
		if action == ActionRest || action == ActionWake || action == ActionBack {
			next := state
			next.Sleep.WakeRequested = true
			next.Saved = false
			return next, nil
		}
	}
	if state.Screen == ScreenOutcome && state.OutcomeKey == "ui.outcome.failure" && action != ActionSave && action != ActionQuit {
		next := state
		next.Saved = false
		next.MessageKey = "ui.message.failure_terminal"
		return next, nil
	}
	if state.Screen == ScreenTrade && (state.TradePage == TradePageSellConfirm || state.TradePage == TradePageBuyDestination || state.TradePage == TradePageSwapWarehouse) {
		switch action {
		case ActionBuyGems, ActionSellGems, ActionTradeBuyPage, ActionTradeSellPage, ActionTradePass,
			ActionTradeExchange, ActionTradeWarehousePage, ActionTradeStorePage, ActionTradeFetchPage, ActionTradeSwapPage:
			return state, nil
		}
	}
	next := state
	normalizeShipSystems(&next)
	next.BankBalances = cloneBalances(state.BankBalances)
	next.SystemHeat = cloneHeat(state.SystemHeat)
	next.VisitedDestinations = append([]string(nil), state.VisitedDestinations...)
	// Every action clears the saved flag, because every action makes the save
	// on disk stale. Backing out is the exception: the manual's page 5 says the
	// title screen reports that the game was saved, and the walk from the save
	// to the title is a Back. Nothing about the game changes on the way.
	if action != ActionBack {
		next.Saved = false
	}
	switch action {
	case ActionPlay:
		if state.Screen != ScreenTitle {
			return state, fmt.Errorf("play is only valid on title screen")
		}
		next.Screen = ScreenShipboard
		next.MessageKey = "ui.message.boarded"
	case ActionSelectGame:
		if state.Screen != ScreenTitle {
			return state, fmt.Errorf("select game is only valid on title screen")
		}
		next.Screen = ScreenCharacterLibrary
		next.LibraryReturn = ""
		next.MessageKey = "ui.message.library"
	case ActionOpenLibrary:
		// The library is reachable from inside a game so a slot can be written
		// without ending the game first — the remake keeps more than one save,
		// which the original's one-game-one-place disk could not.
		switch state.Screen {
		case ScreenTitle, ScreenCharacterLibrary, ScreenCharacterCreation,
			ScreenAttributeAllocation, ScreenScore, ScreenOutcome:
			return state, fmt.Errorf("the library is opened from the title or from a game in progress")
		}
		next.LibraryReturn = state.Screen
		next.Screen = ScreenCharacterLibrary
		next.MessageKey = "ui.message.library"
	case ActionNewGame:
		if state.Screen != ScreenCharacterLibrary {
			return state, fmt.Errorf("new game is only valid in character library")
		}
		fresh := NewState()
		// 呈現是玩家跨局的偏好，不是這一局的狀態，所以整組帶過去；
		// 只帶配色會讓三個欄位變成沒人設計過的組合。
		fresh = WithPresentation(fresh, PresentationOf(state))
		fresh.CharacterDraft = ""
		fresh.Attributes = newCharacterCreationAttributes()
		fresh.AttributeMax = fresh.Attributes
		fresh.AttributePoints = CharacterCreationPoints
		fresh.AttributeSelected = AttrStrength
		fresh.AttributeWasteWarned = false
		fresh.Screen = ScreenCharacterCreation
		fresh.MessageKey = "ui.message.character_name"
		// XSTARTUP:init_planets re-rolls every planet's orbit angle for a new
		// game, so the sky a player starts under is never the factory one.
		normalizePlanetAngles(&fresh)
		fresh.PlanetAngles = rollPlanetAngles(&fresh, len(fresh.PlanetAngles))
		next = fresh
	case ActionConfirmCharacterName:
		if state.Screen != ScreenCharacterCreation {
			return state, fmt.Errorf("character name confirmation is only valid in character creation")
		}
		if !CharacterNameValid(state.CharacterDraft) {
			next.MessageKey = "ui.message.character_name_invalid"
			return next, nil
		}
		// The draft carries whatever the player typed, trailing space included.
		// This is where it stops being an edit and becomes the name.
		next.Character = normalizeCharacterName(state.CharacterDraft)
		next.CharacterDraft = next.Character
		next.Screen = ScreenAttributeAllocation
		next.MessageKey = "ui.message.attribute_points"
	case ActionPreviousAttribute, ActionNextAttribute:
		if state.Screen != ScreenAttributeAllocation {
			return state, fmt.Errorf("attribute selection is only valid during allocation")
		}
		delta := 1
		if action == ActionPreviousAttribute {
			delta = -1
		}
		next.AttributeSelected = (state.AttributeSelected + delta + AttributeCount) % AttributeCount
	case ActionIncreaseAttribute:
		if state.Screen != ScreenAttributeAllocation {
			return state, fmt.Errorf("attribute increase is only valid during allocation")
		}
		if state.AttributePoints > 0 {
			next.Attributes[state.AttributeSelected]++
			next.AttributeMax[state.AttributeSelected]++
			next.AttributePoints--
			next.AttributeWasteWarned = false
		}
	case ActionDecreaseAttribute:
		if state.Screen != ScreenAttributeAllocation {
			return state, fmt.Errorf("attribute decrease is only valid during allocation")
		}
		if state.Attributes[state.AttributeSelected] > characterCreationAttribute {
			next.Attributes[state.AttributeSelected]--
			next.AttributeMax[state.AttributeSelected]--
			next.AttributePoints++
			next.AttributeWasteWarned = false
		}
	case ActionConfirmAttributes:
		if state.Screen != ScreenAttributeAllocation {
			return state, fmt.Errorf("attribute confirmation is only valid during allocation")
		}
		if state.AttributePoints > 0 && !state.AttributeWasteWarned {
			next.AttributeWasteWarned = true
			next.MessageKey = "ui.message.attribute_waste"
			return next, nil
		}
		next.AttributePoints = 0
		next.AttributeWasteWarned = false
		next.CharacterDraft = ""
		next.Screen = ScreenShipboard
		next.MessageKey = "ui.message.character_created"
	case ActionLoadGame:
		if state.Screen != ScreenTitle && state.Screen != ScreenCharacterLibrary {
			return state, fmt.Errorf("load game is only valid on title or character library screen")
		}
		next.MessageKey = "ui.message.load_requested"
	case ActionMakeLibrary:
		if state.Screen != ScreenCharacterLibrary {
			return state, fmt.Errorf("make library is only valid in character library")
		}
		next.MessageKey = "ui.message.library_created"
	case ActionStoreGame:
		if state.Screen != ScreenTitle && state.Screen != ScreenCharacterLibrary {
			return state, fmt.Errorf("store game is only valid on title or character library screen")
		}
		next.MessageKey = "ui.message.game_stored"
	case ActionDeleteGame:
		if state.Screen != ScreenCharacterLibrary {
			return state, fmt.Errorf("delete game is only valid in character library")
		}
		next.MessageKey = "ui.message.game_deleted"
	case ActionOpenStatus:
		if state.Screen != ScreenShipboard && state.Screen != ScreenGround {
			return state, fmt.Errorf("character status is not available on %s screen", state.Screen)
		}
		next.StatusReturn = state.Screen
		next.Screen = ScreenCharacterStatus
		next.SelectedPartIndex = 0
		next.MessageKey = "ui.message.status"
	case ActionOpenDepot:
		if err := r.openDepot(&next); err != nil {
			return state, err
		}
	case ActionRepairHull:
		if state.Screen != ScreenDepot {
			return state, fmt.Errorf("hull repair requires depot menu")
		}
		if err := r.openFuelPurchase(&next, FuelPurchaseHull); err != nil {
			return state, err
		}
	case ActionLoadAuxFuel:
		if err := r.openFuelPurchase(&next, FuelPurchaseAux); err != nil {
			return state, err
		}
	case ActionFuelBuyFull, ActionFuelBuyOne, ActionFuelCancel:
		if state.Screen != ScreenFuelPurchase {
			return state, fmt.Errorf("fuel quantity is only available in the depot dialog")
		}
		if action == ActionFuelCancel {
			finishFuelPurchase(&next)
		} else {
			buyFuelQuantity(&next, action == ActionFuelBuyFull)
		}
	case ActionUnloadAuxFuel:
		if state.Screen != ScreenGround && state.Screen != ScreenCargo {
			return state, fmt.Errorf("auxiliary fuel is dropped on the ground or in the hold")
		}
		if AuxFuelTanks(state) <= 0 {
			next.MessageKey = "ui.message.no_aux_fuel"
			return next, nil
		}
		emptyFirstHoldOf(&next, HoldAuxFuel)
		next.MessageKey = "ui.message.aux_fuel_dropped"
	case ActionHand:
		if state.Screen != ScreenCharacterStatus && state.Screen != ScreenStores {
			return state, fmt.Errorf("the hand is only reachable in character status or Stores")
		}
		if state.HandItem != 0 {
			next = r.DropHandIntoPocket(state)
			if next.HandItem == 0 {
				next.MessageKey = "ui.message.stowed"
			}
			return next, nil
		}
		next = r.PickPocketPart(state, state.SelectedPartIndex)
		if next.HandItem != 0 {
			next.MessageKey = "ui.message.in_hand"
			return next, nil
		}
		next.MessageKey = "ui.message.nothing_to_hold"
	case ActionPreviousPocket, ActionNextPocket:
		if state.Screen != ScreenCharacterStatus {
			return state, fmt.Errorf("body selection requires character status")
		}
		delta := 1
		if action == ActionPreviousPocket {
			delta = -1
		}
		next.SelectedPartIndex = (state.SelectedPartIndex + delta + PocketCount) % PocketCount
	case ActionCancelItem:
		next = r.CancelHand(state)
	case ActionOpenStores:
		if state.Screen != ScreenShipboard {
			return state, fmt.Errorf("Stores is only available shipboard")
		}
		next.Screen = ScreenStores
		next.SelectedPartIndex = 0
	case ActionPreviousStore, ActionNextStore:
		if state.Screen != ScreenStores {
			return state, fmt.Errorf("Stores selection requires Stores")
		}
		delta := 1
		if action == ActionPreviousStore {
			delta = -1
		}
		next.SelectedPartIndex = (state.SelectedPartIndex + delta + storeSlots) % storeSlots
	case ActionTakeStore:
		if state.Screen != ScreenStores {
			return state, fmt.Errorf("Stores transfer requires Stores")
		}
		next = r.SelectStoreItem(state, state.SelectedPartIndex)
	case ActionRest:
		if state.Screen != ScreenCharacterStatus {
			return state, fmt.Errorf("rest is only valid in character status")
		}
		beginSleep(&next, false)
	case ActionVBLTick:
		switch state.Screen {
		case ScreenTitle, ScreenCharacterLibrary, ScreenCharacterCreation, ScreenAttributeAllocation, ScreenOutcome:
			return state, nil
		}
		advanceAwakeVBL(&next)
	case ActionGroundClockTick:
		if state.Screen != ScreenGround {
			return state, nil
		}
		advanceGroundClock(&next, true)
	case ActionSleepTick:
		r.advanceSleep(&next)
	case ActionWake:
		return state, nil
	case ActionEat:
		if state.Screen != ScreenCharacterStatus && state.Screen != ScreenStores {
			return state, fmt.Errorf("eat requires character status or Stores")
		}
		if r.AppleIIText != nil {
			if _, ok := r.AppleIIText.Entry(state.FoodItemKey); !ok {
				return state, fmt.Errorf("Apple II source catalog is missing food item %q", state.FoodItemKey)
			}
		}
		if state.HandItem == 0 || ItemCode(state.HandItem) != itemBurger {
			next = r.CancelHand(state)
			next.MessageKey = "ui.message.no_food"
			return next, nil
		}
		clearHand(&next)
		next.Nourishment = clamp(state.Nourishment+70, 0, maxNourishment)
		next.MessageKey = "ui.message.ate"
	case ActionOpenGround:
		if state.Screen != ScreenShipboard {
			return state, fmt.Errorf("ground movement is only valid shipboard")
		}
		// The original does not cut to the city: XLANDFX walks a horizon down
		// the screen first. That runs as an overlay rather than a screen of its
		// own, so the ground is the ground from the first tick and a replay
		// that ignores the animation still works. The renderer ticks it with
		// ActionLandingTick and the player can cut it short.
		next.Screen = ScreenGround
		next.SleepTheftProtected = false
		next.GroundWorld = false
		next.CityID = state.ShipCityID
		next.Position = state.ShipPosition
		// place_person 的步行下船分支：船首 (17,20) 加 (7,1)。
		ResetGroundView(&next, Position{24, 21})
		next.Landing = beginLanding(&next, state.CityID)
		next.MessageKey = "ui.message.landing"
	case ActionOpenTrade:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("trade is only available on ground movement screen")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityExchange) {
			next.MessageKey = "ui.message.need_exchange"
			return next, nil
		}
		next.Screen = ScreenTrade
		next.TradePage = TradePageEntrance
		r.openExchange(&next)
		next.MessageKey = "ui.message.exchange"
	case ActionOpenNavigation:
		if state.Screen != ScreenShipboard {
			return state, fmt.Errorf("navigation is only available shipboard")
		}
		if _, ok := r.World.Destination(state.NavigationTargetID); !ok {
			destination, err := r.World.CycleDestination("", 0)
			if err != nil {
				return state, err
			}
			selectDestination(&next, destination)
		}
		next.Screen = ScreenNavigation
		// The original opens on the sector chart and the player descends from
		// there, so that is where the route selector starts too.
		next.NavigationView = NavigationSector
		next.MessageKey = "ui.message.navigation"
	case ActionPreviousDestination, ActionNextDestination,
		ActionPreviousTargetPlanet, ActionNextTargetPlanet,
		ActionPreviousTargetSystem, ActionNextTargetSystem:
		if state.Screen != ScreenNavigation {
			return state, fmt.Errorf("destination selection is only valid on navigation screen")
		}
		delta := 1
		switch action {
		case ActionPreviousDestination, ActionPreviousTargetPlanet, ActionPreviousTargetSystem:
			delta = -1
		}
		var (
			destination world.Destination
			err         error
		)
		switch action {
		case ActionPreviousTargetSystem, ActionNextTargetSystem:
			destination, err = r.World.CycleTargetSystem(state.NavigationTargetID, delta)
			next.NavigationView = NavigationSector
		case ActionPreviousTargetPlanet, ActionNextTargetPlanet:
			destination, err = r.World.CycleTargetPlanet(state.NavigationTargetID, delta)
			next.NavigationView = NavigationSystem
		default:
			destination, err = r.World.CycleTargetCity(state.NavigationTargetID, delta)
			next.NavigationView = NavigationPlanet
		}
		if err != nil {
			return state, err
		}
		selectDestination(&next, destination)
		next.MessageKey = "ui.message.destination_selected"
	case ActionTalk:
		if state.GroundWorld {
			next.MessageKey = "ui.message.left_city"
			return next, nil
		}
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("talking is only available in ground movement")
		}
		if !DialogAvailable() {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		block, ok := r.dialogBlockAt(state)
		if !ok {
			next.MessageKey = "ui.message.nobody_here"
			return next, nil
		}
		next.Screen = ScreenDialog
		next.DialogBlock = block
		next.DialogLine = 0
		next.DialogVerb = 0
		next.DialogVariant = state.DialogVariant + 1
		next.DialogSaid = ""
		next.MessageKey = "ui.message.dialog_opened"
	case ActionPreviousVerb, ActionNextVerb:
		if state.Screen != ScreenDialog {
			return state, fmt.Errorf("verb selection is only valid during a conversation")
		}
		verbs := r.PlayerVerbs()
		if len(verbs) == 0 {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		delta := 1
		if action == ActionPreviousVerb {
			delta = -1
		}
		index := (state.DialogVerb + delta) % len(verbs)
		if index < 0 {
			index += len(verbs)
		}
		next.DialogVerb = index
		next.MessageKey = "ui.message.verb_selected"
	case ActionSayVerb:
		if state.Screen != ScreenDialog {
			return state, fmt.Errorf("speaking is only valid during a conversation")
		}
		verbs := r.PlayerVerbs()
		if state.DialogVerb < 0 || state.DialogVerb >= len(verbs) {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		next.DialogSaid = verbs[state.DialogVerb]
		if !r.dialogAdvance(&next) {
			next.Screen = ScreenGround
			next.DialogSaid = ""
			next.MessageKey = "ui.message.dialog_ended"
			return next, nil
		}
		next.MessageKey = "ui.message.dialog_said"
	case ActionOpenUniteller:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("uniteller is only available in ground movement")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityUniteller) {
			next.MessageKey = "ui.message.need_uniteller"
			return next, nil
		}
		next.Screen = ScreenUniteller
		next.MessageKey = "ui.message.uniteller"
	case ActionOpenTubeStation:
		// do_tube_station opens on the tube station's own door, not at the
		// Uniteller counter.
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("the tube station is only reachable on foot")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityTubeStation) {
			next.MessageKey = "ui.message.need_tube_station"
			return next, nil
		}
		next.Screen = ScreenTubeStation
		next.TubePage, next.TubeTargetID = TubePageList, ""
		next.MessageKey = "ui.message.tube_station"
	case ActionRideTube:
		// The TRANSPORT box on the confirmation page.
		if state.Screen != ScreenTubeStation {
			return state, fmt.Errorf("the tube is ridden from the tube station")
		}
		if state.TubePage != TubePageConfirm {
			return state, fmt.Errorf("no ride is selected yet")
		}
		next.MessageKey = r.tubeTransport(&next)
		return next, nil
	case ActionTubePick1, ActionTubePick2, ActionTubePick3, ActionTubePick4, ActionTubePick5, ActionTubePick6:
		if state.Screen != ScreenTubeStation || state.TubePage != TubePageList {
			return state, fmt.Errorf("the tube station's list is not up")
		}
		for i, pick := range tubePickActions {
			if pick == action {
				next.MessageKey = r.tubePick(&next, i)
				return next, nil
			}
		}
	case ActionPayFine:
		if state.Screen != ScreenUniteller {
			return state, fmt.Errorf("the fine is settled at the Uniteller counter")
		}
		fine := r.ParkingFine(state)
		if fine <= 0 {
			next.MessageKey = "ui.message.no_fine"
			return next, nil
		}
		if !payParkingFine(&next, fine) {
			next.MessageKey = "ui.message.no_funds"
			return next, nil
		}
		next.MessageKey = "ui.message.fine_paid"
		return next, nil
	case ActionDistressCall:
		if state.Screen != ScreenShipboard && state.Screen != ScreenFlight {
			return state, fmt.Errorf("the distress call is only on the pilotage console")
		}
		if !r.CanRequestFuel(state) {
			next.MessageKey = "ui.message.distress_unavailable"
			if state.FuelRescuePending {
				next.MessageKey = "ui.message.refuel_eta"
			}
			return next, nil
		}
		inCity := state.Screen == ScreenShipboard
		requestFuel(&next, inCity, inCity, func(n int) int { return combatRoll(&next, n) })
		next.MessageKey = "ui.message.refuel_eta"
		return next, nil
	case ActionOpenTactical:
		if state.Screen != ScreenShipboard {
			return state, fmt.Errorf("tactical is only available shipboard")
		}
		next.Screen = ScreenTactical
		// 重入只恢復顯示；貨艙閘門只決定是否產生新的遭遇。
		// 已確認契約：docs/atari-st/combat-reentry-fix-20260908.md。
		if state.CombatActive {
			next.MessageKey = "ui.message.tactical"
			break
		}
		if !PodDrawsEncounter(next) {
			// update_movement's fourth gate: a pirate only comes for a ship
			// that is carrying something. The display still opens — it is a
			// display — but the scanners stay empty.
			next.CombatActive = false
			next.CombatVictory = false
			next.MessageKey = "ui.message.tactical_empty_pod"
			if CargoUnits(next) > 0 {
				// Cryogens occupy the pod but fail the ordinary cargo gate.
				next.MessageKey = "ui.message.tactical"
			}
			break
		}
		// Which enemy turns up is the star system's own business: its encounter
		// setting fixes the class the rolls then widen.
		rating := 0
		if system, ok := r.World.System(state.SystemID); ok {
			rating = system.EncounterRating
		}
		// A system the player has been shooting up sends worse company. The
		// original applies its heat to the movement tick that decides whether an
		// encounter happens at all; this shell has no such tick, so the same
		// count lowers the encounter setting instead — the axis that decides
		// which class of ship turns up, and where lower means less safe.
		if heatDrawsAnEncounter(&next) {
			rating = clamp(rating-SystemHeat(next, next.SystemID), 0, rating)
		}
		beginEncounter(&next, rating)
		next.MessageKey = "ui.message.tactical"
	case ActionOpenEngineering:
		if state.Screen != ScreenShipboard {
			return state, fmt.Errorf("engineering is only available shipboard")
		}
		next.Screen = ScreenEngineering
		next.MessageKey = "ui.message.engineering"
	case ActionOpenEquipment:
		if state.Screen != ScreenEngineering {
			return state, fmt.Errorf("equipment catalog is only available in engineering")
		}
		if r.AppleIIText != nil && len(r.AppleIIText.KeysWithPrefix("apple2.equipment.")) == 0 {
			return state, fmt.Errorf("Apple II source catalog contains no equipment labels")
		}
		next.Screen = ScreenEquipment
		next.MessageKey = "ui.message.equipment"
	case ActionOpenCargo:
		if state.Screen != ScreenShipboard {
			return state, fmt.Errorf("cargo is only available shipboard")
		}
		next.Screen = ScreenCargo
		next.MessageKey = "ui.message.cargo"
	case ActionExitCargoPod:
		if state.Screen != ScreenCargo && state.Screen != ScreenShipboard {
			return state, fmt.Errorf("cargo pod exit is only available shipboard")
		}
		next.Screen, next.GroundWorld = ScreenGround, false
		next.GroundMode, next.CargoAttached = GroundInPod, false
		next.CityID, next.Position = state.CargoCityID, state.CargoPosition
		// Exact original place_person is unknown. Let the existing renderer
		// choose a traversable initial anchor; do not force a point in the
		// parked ship's collision pixels. Saved anchors still resume unchanged.
		next.GroundView = GroundView{}
		next.MessageKey = "ui.message.pod_exit"
	case ActionNavigate:
		if state.Screen != ScreenNavigation {
			return state, fmt.Errorf("navigation confirmation is only valid on navigation screen")
		}
		destination, ok := r.World.Destination(state.NavigationTargetID)
		if !ok {
			return state, fmt.Errorf("unknown navigation destination %q", state.NavigationTargetID)
		}
		if !PrimaryCity(destination.CityID) && !GroundScannerFitted(state) {
			next.MessageKey = "ui.message.ground_scanner_required"
			return next, nil
		}
		cost, ok := r.RouteCost(state)
		if !ok {
			return state, fmt.Errorf("no route to %q", state.NavigationTargetID)
		}
		// The three gates XSHOWMOV:0x06 start_jump puts in front of a jump, in
		// its own order: fuel, then range, then the warp drive itself.
		if next.Fuel <= 0 {
			// A full auxiliary tank empties itself into the main tank first;
			// only with none left is the jump out of fuel. This is the original's
			// own check_fuel behaviour (ship rule fuel.aux-tank-refill).
			if !drainAuxFuelTank(&next) {
				next.MessageKey = "ui.message.out_of_fuel"
				return next, nil
			}
			next.MessageKey = "ui.message.aux_fuel_drained"
		}
		if cost.JumpDistance > 0 {
			if !cost.WithinRange {
				next.MessageKey = "ui.message.beyond_maximum_jump"
				return next, nil
			}
			if shipSystemStatusByID(state, ShipSystemWarpDrive) <= 0 {
				next.MessageKey = "ui.message.warp_engines_blown"
				return next, nil
			}
		}
		// XSHOWMOV:0x0a do_in_system and 0x0d do_on_world both gate on the
		// sub-c engine, the way start_jump gates the jump on the warp drive.
		// Every trip flies at least one of those legs, so this always applies.
		if shipSystemStatusByID(state, ShipSystemSubcEngine) <= 0 {
			next.MessageKey = "ui.message.subc_engines_blown"
			return next, nil
		}
		// do_on_world's third gate (0x06a9): a take-off is refused while the
		// player's balance in this star system is in the red. Every trip starts
		// by lifting off a city, so every trip is subject to it.
		if state.Bank < 0 {
			next.MessageKey = "ui.message.unpaid_debts"
			return next, nil
		}
		planet, ok := r.World.Planet(destination.PlanetID)
		if !ok {
			return state, fmt.Errorf("destination %q has no planet", destination.ID)
		}
		legs := routeTicks(next, r.PlanetOrbit(next, planet), destination.PlanetID == state.PlanetID)
		departureFuel := routeDepartureFuelCost(next, legs)
		if next.Fuel < departureFuel {
			next.MessageKey = "ui.message.insufficient_fuel"
			return next, nil
		}
		next.Fuel -= departureFuel
		// XSHOWMOV:0a writes mode2's ETA to G51; it does not add that estimate
		// to the main clock. This leg advances through the awake minute slot.
		// Retained atomic world/warp timing remains a separate approximation.
		arcETA := legs.ArcTicks + legs.ArcTicks/flightEtaDivisor
		advanceTime(&next, cost.TotalMinutes-arcETA)
		if cost.JumpDistance > 0 {
			// XSHOWMOV:06 restores slot 2 at the end of the atomic warp.
			saved := next.AwakeClock.Background.PeriodTicks(ClockIdleShip)
			next.AwakeClock.Background.Stop(ClockIdleShip)
			_ = next.AwakeClock.Background.Start(ClockIdleShip, saved)
		}
		// XLANDFX runs the ascent here: this is where the ship actually leaves
		// the city it is parked in. It used to play when the player came back
		// aboard, which is not the same moment — boarding is re_enter_ship.
		next.Landing = beginTakeOff(&next, state.CityID)
		// Lifting off turns the shields off and says so (0x0729). The original
		// composes that line out of whatever was actually on; this shell has
		// one message slot, so it only reports the case that changes anything.
		shieldsWereUp := next.ShieldsActive
		next.ShieldsActive = false
		// The trip is not over when it is paid for. The jump between stars is
		// instantaneous, but the leg inside the destination system is flown,
		// and the original interpolates it from wherever the ship already is.
		// A trip between two cities of the same planet has no in-system leg —
		// the original routes that through its own city-to-city path rather
		// than through the flight, and the interpolation would have nothing to
		// interpolate.
		if destination.PlanetID == state.PlanetID {
			arriveAt(&next, destination)
			if shieldsWereUp {
				next.MessageKey = "ui.message.shields_off_for_flight"
			}
			return next, nil
		}
		beginInSystemFlight(&next, r.PlanetOrbit(next, planet))
		next.FlightTargetID = destination.ID
		next.Screen = ScreenFlight
		next.MessageKey = "ui.message.under_way"
		if shieldsWereUp {
			next.MessageKey = "ui.message.shields_off_for_flight"
		}
	case ActionFlightResume:
		if state.Screen != ScreenFlight {
			return state, fmt.Errorf("flight resumes only on the flight screen")
		}
		if state.ShipControls.Subc {
			next.MessageKey = "ui.message.under_way"
			return next, nil
		}
		if state.Fuel <= 0 {
			next.MessageKey = "ui.message.out_of_fuel"
			return next, nil
		}
		destination, ok := r.World.Destination(state.FlightTargetID)
		if !ok {
			return state, fmt.Errorf("unknown flight destination %q", state.FlightTargetID)
		}
		planet, ok := r.World.Planet(destination.PlanetID)
		if !ok {
			return state, fmt.Errorf("unknown flight planet %q", destination.PlanetID)
		}
		// R reselects the destination through XSHOWMOV's numerical initializer
		// from the live polar position. Rescue never starts the engine, and
		// this entry does not repeat world/warp charges.
		beginInSystemFlight(&next, r.PlanetOrbit(next, planet))
		next.MessageKey = "ui.message.under_way"
		return next, nil
	case ActionFlightTick, ActionFlightEngage:
		if state.Screen != ScreenFlight {
			return state, fmt.Errorf("flight only advances on the flight screen")
		}
		// One press is one tick of update_movement. Engage runs the rest of the
		// trip in a single step; that shortcut is the remake's, for a shell
		// that has no clock of its own.
		//
		// A tick that empties an auxiliary tank into the main one says so, and
		// that line outranks "under way": the player has just lost a lot of the
		// pod and needs to know.
		refuelled := advanceInSystemFlight(&next)
		if action == ActionFlightEngage {
			for updates := 0; !FlightArrived(next) && next.ShipControls.Subc && updates < next.FlightTicks*32+32; updates++ {
				if advanceInSystemFlight(&next) {
					refuelled = true
				}
			}
		}
		if !FlightArrived(next) {
			next.MessageKey = "ui.message.under_way"
			if !next.ShipControls.Subc {
				next.MessageKey = "ui.message.out_of_fuel"
			}
			if refuelled {
				next.MessageKey = "ui.message.aux_fuel_drained"
			}
			return next, nil
		}
		destination, ok := r.World.Destination(next.FlightTargetID)
		if !ok {
			return state, fmt.Errorf("unknown flight destination %q", next.FlightTargetID)
		}
		arriveAt(&next, destination)
	case ActionDepositCash:
		if state.Screen != ScreenUniteller {
			return state, fmt.Errorf("deposit is only valid at uniteller")
		}
		if state.Cash == 0 {
			next.MessageKey = "ui.message.no_cash"
			return next, nil
		}
		next.Bank += state.Cash
		next.Cash = 0
		storeCurrentBank(&next)
		next.MessageKey = "ui.message.deposited"
	case ActionWithdrawCash:
		if state.Screen != ScreenUniteller {
			return state, fmt.Errorf("withdraw is only valid at uniteller")
		}
		if state.Bank == 0 {
			next.MessageKey = "ui.message.no_bank_balance"
			return next, nil
		}
		next.Cash += state.Bank
		next.Bank = 0
		storeCurrentBank(&next)
		next.MessageKey = "ui.message.withdrawn"
	case ActionTransferBank:
		if state.Screen != ScreenUniteller {
			return state, fmt.Errorf("transfer is only valid at uniteller")
		}
		destination, ok := r.World.Destination(state.NavigationTargetID)
		if !ok {
			return state, fmt.Errorf("unknown transfer destination %q", state.NavigationTargetID)
		}
		if destination.SystemID == state.SystemID {
			next.MessageKey = "ui.message.transfer_same_system"
			return next, nil
		}
		if state.Bank == 0 {
			next.MessageKey = "ui.message.no_bank_balance"
			return next, nil
		}
		storeCurrentBank(&next)
		next.BankBalances[destination.SystemID] += state.Bank
		next.Bank = 0
		storeCurrentBank(&next)
		next.MessageKey = "ui.message.transferred"
	case ActionGroundTick:
		// One iteration of xmove_on_ground: the walker has already moved (the
		// shell owns the sprite), so what is left is the encounter roll the
		// original makes on every pass. Nothing else changes, message included
		// — a quiet tick has to look like no action at all.
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("the ground tick only runs on the city ground")
		}
		if !state.GroundWorld {
			r.openStreetEncounterRealTime(&next)
		}
	case ActionMoveNorth, ActionMoveSouth, ActionMoveWest, ActionMoveEast:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("movement is only valid on ground screen")
		}
		target := state.Position
		switch action {
		case ActionMoveNorth:
			target.Y--
		case ActionMoveSouth:
			target.Y++
		case ActionMoveWest:
			target.X--
		case ActionMoveEast:
			target.X++
		}
		target = GroundWrap(state, target)
		advanceGroundClock(&next, true)
		if next.Sleep.Active || next.Screen == ScreenOutcome {
			return next, nil
		}
		if !state.GroundWorld {
			if terrain, ok := GroundTerrainAt(state, state.CityID, target.X, target.Y); ok && !world.IsPassable(terrain) {
				next.MessageKey = "ui.message.cannot_move"
				return next, nil
			}
		}
		next.Position = target
		SyncCargoGroundPosition(&next)
		next.Heading = heading(action)
		next.GroundView = GroundView{}
		if state.GroundWorld {
			offset := Position{16, 16}
			switch action {
			case ActionMoveNorth:
				offset.Y = 31
			case ActionMoveSouth:
				offset.Y = 0
			case ActionMoveWest:
				offset.X = 31
			case ActionMoveEast:
				offset.X = 0
			}
			if !EnterGroundCity(&next, offset) {
				ResetGroundView(&next, Position{16, 16})
				next.MessageKey = "ui.message.moved"
			}
			return next, nil
		}
		leftCity, err := LeaveGroundCityChecked(&next, Position{target.X - 4, target.Y - 2})
		if err != nil {
			return state, err
		}
		if leftCity {
			return next, nil
		}
		if next.CityID == next.ShipCityID && next.Position == next.ShipPosition {
			offset := Position{17, 20}
			if next.GroundMode == GroundInPod {
				offset.X = 23
			}
			BoardGroundShip(&next, offset, action == ActionMoveWest || action == ActionMoveEast)
			return next, nil
		}
		next.MessageKey = "ui.message.moved"
		r.openStreetEncounter(&next)
	case ActionRefuel:
		if err := r.openFuelPurchase(&next, FuelPurchaseMain); err != nil {
			return state, err
		}
	case ActionOpenFTLBoost:
		if err := r.openFTLBoost(&next); err != nil {
			return state, err
		}
	case ActionInstallFTLBoost:
		if state.Screen != ScreenFTLBoost {
			return state, fmt.Errorf("FTL installation requires its engineering quote")
		}
		installFTLBoost(&next)
	case ActionOpenParts:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("the parts counter is only available on ground movement screen")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityWarehouse) {
			next.MessageKey = "ui.message.need_warehouse"
			return next, nil
		}
		if len(PartsForSale()) == 0 {
			return state, fmt.Errorf("ship part data is unavailable")
		}
		next.Screen = ScreenParts
		next.MessageKey = "ui.message.parts_counter"
	case ActionPreviousPart, ActionNextPart:
		if state.Screen != ScreenParts {
			return state, fmt.Errorf("part selection is only valid at the parts counter")
		}
		delta := 1
		if action == ActionPreviousPart {
			delta = -1
		}
		cyclePart(&next, delta)
		next.MessageKey = "ui.message.part_selected"
	case ActionBuyPart:
		if state.Screen != ScreenParts {
			return state, fmt.Errorf("buying parts is only valid at the parts counter")
		}
		part, ok := state.SelectedPart()
		if !ok {
			return state, fmt.Errorf("the parts counter has no selected part")
		}
		price, ok := r.ShopItemPrice(state, part)
		if !ok {
			return state, fmt.Errorf("part 0x%02x has no price input in the source table", part)
		}
		if state.Cash+state.Bank < price {
			next.MessageKey = "ui.message.insufficient_cash"
			return next, nil
		}
		// Body slots are fixed. Refuse a purchase before charging when none
		// can receive the part; the shop's other prototype rules are unchanged.
		if part != atarist.ShuntItemIndex() && CarriedItemCount(state) >= PocketCount {
			next.MessageKey = "ui.message.carry_full"
			return next, nil
		}
		// Same order as the exchange, per the manual: the local bank first,
		// then cash.
		bankSpend := price
		if bankSpend > state.Bank {
			bankSpend = state.Bank
		}
		next.Bank -= bankSpend
		next.Cash -= price - bankSpend
		storeCurrentBank(&next)
		addCarriedPart(&next, part)
		advanceTime(&next, 5)
		next.MessageKey = "ui.message.part_bought"
	case ActionInstallPart:
		if state.Screen != ScreenEngineering {
			return state, fmt.Errorf("installing a part is only valid in engineering")
		}
		system, ok := state.SelectedShipSystem()
		if !ok {
			return state, fmt.Errorf("engineering has no selected ship system")
		}
		if system.Status >= MaxShipSystemStatus() {
			next.MessageKey = "ui.message.repair_not_needed"
			return next, nil
		}
		row, column, part, ok := installablePart(state, system)
		if !ok {
			next.MessageKey = "ui.message.no_matching_part"
			return next, nil
		}
		if !takeCarriedPart(&next, part) {
			next.MessageKey = "ui.message.no_matching_part"
			return next, nil
		}
		repaired := next.ShipSystems[next.SelectedSystemIndex]
		repaired.Rows[row][column] = part
		repaired.Status = shipSystemStatus(repaired)
		next.ShipSystems[next.SelectedSystemIndex] = repaired
		advanceTime(&next, 10)
		next.MessageKey = "ui.message.part_installed"
	case ActionLoadSupplies:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("supplies can only be loaded on ground screen")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityWarehouse) {
			next.MessageKey = "ui.message.need_warehouse"
			return next, nil
		}
		if CargoLotsFree(state) <= 0 {
			next.MessageKey = "ui.message.cargo_full"
			return next, nil
		}
		loadHold(&next, Hold{Kind: HoldSupplies})
		next.MessageKey = "ui.message.supplies_loaded"
	case ActionLoadCryogens:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("cryogens can only be loaded on ground screen")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityWarehouse) {
			next.MessageKey = "ui.message.need_warehouse"
			return next, nil
		}
		if CargoLotsFree(state) <= 0 {
			next.MessageKey = "ui.message.cargo_full"
			return next, nil
		}
		loadHold(&next, Hold{Kind: HoldCryogens})
		next.MessageKey = "ui.message.cryogens_loaded"
	case ActionDeliverMission:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("mission delivery is only valid on ground screen")
		}
		if state.GroundWorld || !r.GroundHasFacility(state, world.FacilityColony) {
			next.MessageKey = "ui.message.need_colony"
			return next, nil
		}
		if state.CityID != world.MissionDestinationID {
			next.MessageKey = "ui.message.wrong_destination"
			return next, nil
		}
		next.MessageKey = "ui.message.delivery_at_exchange"
	case ActionBuyGems:
		if state.Screen != ScreenTrade {
			return state, fmt.Errorf("buy is only valid at trade exchange")
		}
		// The purchase itself; the original's quote page in front of it is
		// ActionTradeBuyPage, and a purchase closes it.
		next.TradePage = TradePageMain
		quote, ok := r.World.Market(state.CityID)
		if !ok {
			return state, fmt.Errorf("no market quote for city %q", state.CityID)
		}
		// The quote's key belongs to whichever source table produced it, so the
		// guard has to ask that table. It used to ask the Apple II catalog
		// unconditionally, from when the only cargo was that catalog's
		// gems/crystals -- once the Atari ST cargo table landed, every quote
		// carried an atarist.cargo.* key the Apple II catalog has never heard
		// of, and **every purchase was refused**.
		if err := r.checkCargoKeyIsNamed(quote.ItemKey); err != nil {
			return state, err
		}
		if len(state.Quotes) > 0 {
			// The exchange proper: the ring's current quote (do_buying).
			key, _ := r.buyQuote(&next)
			next.MessageKey = key
			return next, nil
		}
		price := quote.BuyPrice
		if TradeFunds(state) < price {
			next.MessageKey = "ui.message.insufficient_cash"
			return next, nil
		}
		if CargoLotsFree(state) <= 0 {
			next.MessageKey = "ui.message.cargo_full"
			return next, nil
		}
		spendTradeFunds(&next, price)
		next.Gems++
		loadHold(&next, Hold{Kind: HoldGoods, ItemKey: quote.ItemKey})
		next.MessageKey = "ui.message.bought"
	case ActionTellerDepositPage, ActionTellerWithdrawPage, ActionTellerTransferPage:
		if state.Screen != ScreenUniteller {
			return state, fmt.Errorf("the teller's keypad is only valid at uniteller")
		}
		next.TellerAmount = 0
		switch action {
		case ActionTellerDepositPage:
			if state.Cash == 0 {
				next.MessageKey = "ui.message.no_cash"
				return next, nil
			}
			next.TellerPage = TellerPageDeposit
		case ActionTellerWithdrawPage:
			if state.Bank == 0 {
				next.MessageKey = "ui.message.no_bank_balance"
				return next, nil
			}
			next.TellerPage = TellerPageWithdraw
		default:
			if state.Bank == 0 {
				next.MessageKey = "ui.message.no_bank_balance"
				return next, nil
			}
			// do_transfer: pick_location first, the keypad after.
			next.TellerPage, next.TellerPickPage, next.TellerTransferID = TellerPageTransferPick, 0, ""
		}
		next.MessageKey = "ui.message.teller_amount"
	case ActionTellerPick1, ActionTellerPick2, ActionTellerPick3, ActionTellerPick4, ActionTellerPick5, ActionTellerPick6, ActionTellerOthers:
		if state.Screen != ScreenUniteller || state.TellerPage != TellerPageTransferPick {
			return state, fmt.Errorf("the transfer list is not up")
		}
		if action == ActionTellerOthers {
			if _, others := r.TellerPickList(state); others {
				next.TellerPickPage = 1 - state.TellerPickPage
			}
			break
		}
		for i, pick := range tellerPickActions {
			if pick == action {
				r.tellerPick(&next, i)
			}
		}
	case ActionTellerAmountAll, ActionTellerAmount100, ActionTellerAmount1000, ActionTellerAmount10000, ActionTellerAmount50000, ActionTellerAmountClear:
		if state.Screen != ScreenUniteller || state.TellerPage == TellerPageMain {
			return state, fmt.Errorf("the keypad is not up")
		}
		tellerKey(&next, action)
	case ActionTellerConfirm:
		if state.Screen != ScreenUniteller || state.TellerPage == TellerPageMain {
			return state, fmt.Errorf("the keypad is not up")
		}
		if err := r.tellerConfirm(&next); err != nil {
			return state, err
		}
	case ActionTradeBuyPage, ActionTradeSellPage:
		// Sell enters pick_item: pod mode chooses Warehouse/Pod; foot mode
		// goes straight to the warehouse list. Buying opens the quote.
		if state.Screen != ScreenTrade {
			return state, fmt.Errorf("the exchange pages are only valid at trade exchange")
		}
		if action == ActionTradeBuyPage {
			if !TradePurchaseFits(state) {
				next.MessageKey = purchaseFullMessage(state)
				break
			}
			next.TradePage = TradePageBuy
			next.MessageKey = "ui.message.trade_quote"
		} else {
			next.TradePage = TradePageSellWarehouse
			if state.GroundMode == GroundInPod {
				next.TradePage = TradePageSellSource
			}
			next.MessageKey = "ui.message.trade_sell_which"
		}
	case ActionTradePass:
		if state.Screen != ScreenTrade {
			return state, fmt.Errorf("pass is only valid at trade exchange")
		}
		// do_buying's Pass: the next quote of the ring stays on the page.
		passQuote(&next)
		next.MessageKey = "ui.message.trade_passed"
	case ActionTradeSwapPage:
		if state.Screen != ScreenTrade || state.TradePage != TradePageWarehouse {
			return state, fmt.Errorf("swap starts at the warehouse menu")
		}
		next.TradePage = TradePageSwapPod
		next.TradeSaleSlot, next.TradeSalePrice = 0, 0
		next.MessageKey = "ui.trade.swap_pod"
	case ActionTradeExchange, ActionTradeWarehousePage, ActionTradeStorePage, ActionTradeFetchPage:
		if state.Screen != ScreenTrade {
			return state, fmt.Errorf("the exchange pages are only valid at trade exchange")
		}
		switch action {
		case ActionTradeExchange:
			next.TradePage = TradePageMain
		case ActionTradeWarehousePage:
			next.TradePage = TradePageWarehouse
		case ActionTradeStorePage:
			next.TradePage = TradePageStore
		default:
			next.TradePage = TradePageFetch
		}
	case ActionTradePick1, ActionTradePick2, ActionTradePick3, ActionTradePick4:
		if state.Screen != ScreenTrade {
			return state, fmt.Errorf("picking a lot is only valid at trade exchange")
		}
		index := map[Action]int{ActionTradePick1: 0, ActionTradePick2: 1, ActionTradePick3: 2, ActionTradePick4: 3}[action]
		if state.TradePage == TradePageBuyDestination {
			if index < 2 {
				next.MessageKey, _ = r.finishTradePurchase(&next, index == 1)
			}
			break
		}
		if state.TradePage == TradePageSellSource {
			if index == 0 {
				next.TradePage = TradePageSellWarehouse
			} else if index == 1 {
				next.TradePage = TradePageSell
			}
			break
		}
		if state.TradePage == TradePageSellConfirm {
			if index == 1 {
				cancelTradeSale(&next)
			} else if index == 0 && TradeSaleAvailable(state) {
				next.MessageKey, _ = r.confirmTradeSale(&next)
				if next.TradePage == TradePageSellConfirm {
					cancelTradeSale(&next)
				}
			}
			break
		}
		var key string
		var ok bool
		switch state.TradePage {
		case TradePageSwapPod:
			if index >= CargoLots || state.Holds[index].Kind != HoldGoods || state.Holds[index].Goods <= 0 {
				next.MessageKey = "ui.message.no_cargo"
				break
			}
			next.TradeSaleSlot, next.TradeSalePrice = index+5, 0
			next.TradePage = TradePageSwapWarehouse
			next.MessageKey = "ui.trade.swap_warehouse"
		case TradePageSwapWarehouse:
			key, ok = r.swapLot(&next, state.TradeSaleSlot-5, index)
		case TradePageSell:
			key, ok = r.prepareTradeSale(&next, index+5)
		case TradePageSellWarehouse:
			key, ok = r.prepareTradeSale(&next, index+1)
		case TradePageStore:
			key, ok = r.storeLot(&next, index)
		case TradePageFetch:
			key, ok = r.fetchLot(&next, index)
		default:
			return state, fmt.Errorf("no lot list is up")
		}
		if state.TradePage == TradePageSwapPod {
			break
		}
		next.MessageKey = key
		if ok && state.TradePage != TradePageSell && state.TradePage != TradePageSellWarehouse {
			next.TradePage = TradePageWarehouse
			if state.TradePage == TradePageSwapWarehouse {
				next.TradeSaleSlot, next.TradeSalePrice = 0, 0
			}
		}
	case ActionSellGems:
		if state.Screen != ScreenTrade {
			return state, fmt.Errorf("sell is only valid at trade exchange")
		}
		index := firstGoodsHold(state)
		if index < 0 {
			next.MessageKey = "ui.message.no_cargo"
			return next, nil
		}
		next.MessageKey, _ = r.prepareTradeSale(&next, index+5)
	case ActionTacticalMove:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("tactical move is only valid on tactical screen")
		}
		// do_guns: from here on the mouse steers (Reducer.Slew) until a
		// button ends it (ActionTacticalStop).
		advanceTime(&next, 1)
		next.TacticalSlewing = true
		next.MessageKey = "ui.message.tactical_slewing"
	case ActionTacticalStop:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("tactical stop is only valid on tactical screen")
		}
		next.TacticalSlewing = false
		next.SlewBearing, next.SlewPitch = 0, 0
		next.MessageKey = "ui.message.tactical_moved"
	case ActionTractor:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("the tractor beam is only valid on the tactical screen")
		}
		switch runTractor(&next) {
		case TractorOutOfRange:
			next.MessageKey = "ui.message.tractor_out_of_range"
		case TractorNoRoom:
			next.MessageKey = "ui.message.tractor_no_room"
		case TractorOutOfFuel:
			next.MessageKey = "ui.message.out_of_fuel"
		default:
			next.MessageKey = "ui.message.cargo_loaded"
		}
		return next, nil
	case ActionJettisonHold1, ActionJettisonHold2, ActionJettisonAll:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("the jettison bolts are only valid on the tactical screen")
		}
		target := JettisonHold1
		switch action {
		case ActionJettisonHold2:
			target = JettisonHold2
		case ActionJettisonAll:
			target = JettisonAll
		}
		if runJettison(&next, target) == 0 {
			next.MessageKey = "ui.message.jettison_empty"
			return next, nil
		}
		// Dumping cargo wipes the system's record of you
		// (system.heat-cleared-by-jettison).
		clearSystemHeat(&next)
		next.MessageKey = "ui.message.bolts_armed"
		return next, nil
	case ActionHail:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("hailing is only valid on the tactical screen")
		}
		switch runHail(&next) {
		case HailNoShip:
			next.MessageKey = "ui.message.no_ship_to_hail"
			return next, nil
		case HailJammed:
			next.MessageKey = "ui.message.comm_jammed"
			return next, nil
		}
		// do_comm is not a subsystem of its own: it hands the dialog
		// interpreter a set of parameters and lets it run (comm.dialog-setup).
		opening, ok := dialogOpeningForFrame(hailDialogFrame)
		if !ok {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		next.DialogBlock = opening.Block
		next.DialogLine = opening.Line
		next.Screen = ScreenDialog
		next.MessageKey = "ui.message.hailing"
		return next, nil
	case ActionFire:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("fire is only valid on tactical screen")
		}
		// XDOFIGHT:0x06 fire_it needs both: guns that still work and a charge
		// to spend. A wrecked gun system cannot fire at all.
		if shipSystemStatusByID(state, ShipSystemShipGuns) <= 0 {
			next.MessageKey = "ui.message.guns_blown"
			return next, nil
		}
		if state.WeaponCharge == 0 {
			next.MessageKey = "ui.message.weapon_empty"
			return next, nil
		}
		if !state.CombatActive {
			next.WeaponCharge = 0
			next.MessageKey = "ui.message.no_enemy"
			break
		}
		// Firing in space heats the system up (system.heat-raised-by-firing).
		raiseSystemHeat(&next)
		// fire_it lands the shot only when the enemy is inside the cannon's
		// aiming window; a miss still spends the charge.
		_, cannonWindow := aimTolerances(next)
		if aimError(next) > cannonWindow {
			next.WeaponCharge = 0
			next.MessageKey = "ui.message.cannon_missed"
			break
		}
		// The charge the shot carries is the charge it spends, so the damage is
		// worked out before the weapon is emptied. What limits the rate of fire
		// is therefore the recharge, which the guns supply.
		damage := playerShotDamage(&next)
		next.WeaponCharge = 0
		next.EnemyHull = clamp(state.EnemyHull-damage, 0, next.EnemyMaxHull)
		if next.EnemyHull == 0 {
			next.CombatActive = false
			next.CombatVictory = true
			next.MessageKey = "ui.message.enemy_destroyed"
		} else {
			next.MessageKey = "ui.message.fired"
		}
	case ActionFireLaser:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("fire is only valid on tactical screen")
		}
		// The laser fires through the same gate as the cannon: working guns
		// and a charge to spend (fire_it checks them before it asks which
		// button was pressed).
		if shipSystemStatusByID(state, ShipSystemShipGuns) <= 0 {
			next.MessageKey = "ui.message.guns_blown"
			return next, nil
		}
		if state.WeaponCharge == 0 {
			next.MessageKey = "ui.message.weapon_empty"
			return next, nil
		}
		if !state.CombatActive {
			next.WeaponCharge = 0
			next.MessageKey = "ui.message.no_enemy"
			return next, nil
		}
		raiseSystemHeat(&next)
		// A miss still spends the charge: fire_it clears it on every shot,
		// hit or not.
		damage, hit := playerLaserShot(&next)
		next.WeaponCharge = 0
		if !hit {
			next.MessageKey = "ui.message.laser_missed"
			break
		}
		next.EnemyHull = clamp(state.EnemyHull-damage, 0, next.EnemyMaxHull)
		if next.EnemyHull == 0 {
			next.CombatActive = false
			next.CombatVictory = true
			next.MessageKey = "ui.message.enemy_destroyed"
		} else {
			next.MessageKey = "ui.message.laser_fired"
		}
	case ActionRechargeWeapon:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("weapon recharge is only valid on tactical screen")
		}
		// The guns supply the charge, so a shot-up weapon system recharges the
		// weapon more slowly. That link is the source's.
		next.ShipControls.Guns = true
		next.MessageKey = "ui.message.weapon_recharged"
	case ActionShieldsOff:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("shield control is only valid on tactical screen")
		}
		next.ShieldsActive = false
		next.ShieldSetting = 0
		next.MessageKey = "ui.message.shields_off"
	case ActionFullShields:
		if state.Screen != ScreenTactical {
			return state, fmt.Errorf("shield control is only valid on tactical screen")
		}
		next.ShieldsActive = true
		next.ShieldSetting = maxShieldLevel
		next.MessageKey = "ui.message.full_shields"
	case ActionPreviousSystem, ActionNextSystem:
		if state.Screen != ScreenEngineering {
			return state, fmt.Errorf("ship system selection is only valid in engineering")
		}
		delta := 1
		if action == ActionPreviousSystem {
			delta = -1
		}
		cycleSelectedShipSystem(&next, delta)
		next.MessageKey = "ui.message.system_selected"
	case ActionRepairSystem:
		if state.Screen != ScreenEngineering {
			return state, fmt.Errorf("ship system repair is only valid in engineering")
		}
		if r.AppleIIText != nil {
			if _, ok := r.AppleIIText.Entry("apple2.equipment.shunt"); !ok {
				return state, fmt.Errorf("Apple II source catalog is missing shunt equipment")
			}
		}
		system, ok := state.SelectedShipSystem()
		if !ok {
			return state, fmt.Errorf("engineering has no selected ship system")
		}
		if system.Status >= MaxShipSystemStatus() {
			next.MessageKey = "ui.message.repair_not_needed"
			return next, nil
		}
		if state.Shunts <= 0 {
			next.MessageKey = "ui.message.no_shunt"
			return next, nil
		}
		// A shunt stands in for a broken part everywhere but the control node's
		// column, so a system can be damaged and still refuse one.
		repaired := next.ShipSystems[next.SelectedSystemIndex]
		if !installShunt(&repaired) {
			next.MessageKey = "ui.message.shunt_does_not_fit"
			return next, nil
		}
		next.ShipSystems[next.SelectedSystemIndex] = repaired
		next.Shunts--
		next.MessageKey = "ui.message.system_repaired"
	case ActionRepair, ActionRepairWeapon, ActionRepairShields:
		if state.Screen != ScreenEngineering {
			return state, fmt.Errorf("repair is only valid in engineering")
		}
		if r.AppleIIText != nil {
			if _, ok := r.AppleIIText.Entry("apple2.equipment.shunt"); !ok {
				return state, fmt.Errorf("Apple II source catalog is missing shunt equipment")
			}
		}
		value := state.HullCondition
		messageKey := "ui.message.repaired"
		switch action {
		case ActionRepairWeapon:
			value = state.WeaponCharge
			messageKey = "ui.message.weapon_repaired"
		case ActionRepairShields:
			value = state.ShieldLevel
			messageKey = "ui.message.shields_repaired"
		}
		if value >= repairCeiling(action) {
			next.MessageKey = "ui.message.repair_not_needed"
			return next, nil
		}
		if state.Shunts <= 0 {
			next.MessageKey = "ui.message.no_shunt"
			return next, nil
		}
		next.Shunts--
		repairedValue := clamp(value+repairAmount(action), 0, repairCeiling(action))
		switch action {
		case ActionRepairWeapon:
			next.WeaponCharge = repairedValue
		case ActionRepairShields:
			next.ShieldLevel = repairedValue
		default:
			next.HullCondition = repairedValue
		}
		next.MessageKey = messageKey
	case ActionLoadCargo:
		if state.Screen != ScreenCargo {
			return state, fmt.Errorf("cargo loading is only valid in cargo screen")
		}
		if CargoLotsFree(state) <= 0 {
			next.MessageKey = "ui.message.cargo_full"
			return next, nil
		}
		loadHold(&next, Hold{Kind: HoldGoods})
		next.MessageKey = "ui.message.cargo_loaded"
	case ActionUnloadCargo:
		if state.Screen != ScreenCargo {
			return state, fmt.Errorf("cargo unloading is only valid in cargo screen")
		}
		if CargoUnits(state) == 0 {
			next.MessageKey = "ui.message.no_cargo"
			return next, nil
		}
		emptyFirstHoldOf(&next, HoldGoods, HoldSupplies, HoldCryogens)
		next.MessageKey = "ui.message.cargo_unloaded"
	case ActionEnterBuilding:
		if state.Screen != ScreenGround {
			return state, fmt.Errorf("entering a building is only valid on ground screen")
		}
		code, _, ok := GroundTerrainCode(state, state.CityID, state.Position.X, state.Position.Y)
		if state.GroundWorld {
			ok = false
		}
		if !ok {
			next.MessageKey = "ui.message.no_door_here"
			return next, nil
		}
		kind, ok := InteriorKindForTerrain(code)
		if !ok {
			next.MessageKey = "ui.message.no_door_here"
			return next, nil
		}
		next.Interior = enterInterior(kind, state.Position, state.Interior.Rolls)
		next.Screen = ScreenInterior
		next.GroundReturn = ScreenGround
		next.MessageKey = InteriorLabelKey(kind)
	case ActionLeaveBuilding:
		if state.Screen != ScreenInterior {
			return state, fmt.Errorf("leaving a building is only valid inside one")
		}
		if !state.Interior.AtDoor() {
			next.MessageKey = "ui.message.not_at_door"
			return next, nil
		}
		next.Position = state.Interior.ReturnPosition
		next.Interior = Interior{}
		next.Screen = ScreenGround
		next.MessageKey = "ui.message.left_building"
	case ActionInteriorNorth, ActionInteriorSouth, ActionInteriorWest, ActionInteriorEast:
		if state.Screen != ScreenInterior {
			return state, fmt.Errorf("interior movement is only valid inside a building")
		}
		dx, dy := interiorStep(action)
		interior := state.Interior
		if !moveInterior(&interior, dx, dy) {
			next.MessageKey = "ui.message.cannot_move"
			return next, nil
		}
		// handle_customers runs on every update, so a step is also a chance for
		// somebody to walk in.
		admitCustomer(&interior)
		next.Interior = interior
		advanceTime(&next, 1)
		switch {
		case interior.AtCounter():
			next.MessageKey = "ui.message.at_counter"
		case interior.AtSlots():
			next.MessageKey = "ui.message.at_slots"
		case interior.BoothIndex() >= 0:
			next.MessageKey = "ui.message.at_booth"
		case interior.AtDoor():
			next.MessageKey = "ui.message.at_door"
		default:
			next.MessageKey = "ui.message.moved"
		}
	case ActionUseCounter:
		if state.Screen != ScreenInterior {
			return state, fmt.Errorf("the counter is only reachable inside a building")
		}
		if !state.Interior.AtCounter() {
			next.MessageKey = "ui.message.not_at_counter"
			return next, nil
		}
		if !DialogAvailable() {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		frame, ok := DialogBlockForInterior(state.Interior.Kind)
		if !ok {
			next.MessageKey = "ui.message.nobody_here"
			return next, nil
		}
		opening, ok := dialogOpeningForFrame(frame)
		if !ok {
			next.MessageKey = "ui.message.nobody_here"
			return next, nil
		}
		next.GroundReturn = ScreenInterior
		next.Screen = ScreenDialog
		next.DialogBlock = opening.Block
		next.DialogLine = opening.Line
		next.DialogVerb = 0
		next.DialogSaid = ""
		next.MessageKey = "ui.message.dialog_opened"
	case ActionSitAtBooth:
		if state.Screen != ScreenInterior {
			return state, fmt.Errorf("booths are only reachable inside a building")
		}
		if state.Interior.BoothIndex() < 0 {
			next.MessageKey = "ui.message.not_at_booth"
			return next, nil
		}
		if !DialogAvailable() {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		booth, ok := dialogOpeningForFrame(boothDialogFrame)
		if !ok {
			next.MessageKey = "ui.message.dialog_unavailable"
			return next, nil
		}
		next.GroundReturn = ScreenInterior
		next.Screen = ScreenDialog
		next.DialogBlock = booth.Block
		next.DialogLine = booth.Line
		next.DialogVerb = 0
		next.DialogSaid = ""
		next.MessageKey = "ui.message.dialog_opened"
	case ActionOpenSlots:
		if state.Screen != ScreenInterior {
			return state, fmt.Errorf("the machine is only reachable inside a building")
		}
		if !state.Interior.AtSlots() {
			next.MessageKey = "ui.message.not_at_slots"
			return next, nil
		}
		next.Slots = SlotMachine{Bet: slotBetForState(state, r.World), Rolls: state.Slots.Rolls}
		next.Screen = ScreenSlots
		next.MessageKey = "ui.message.slots_open"
	case ActionSpinSlots:
		if state.Screen != ScreenSlots {
			return state, fmt.Errorf("spinning is only valid at the machine")
		}
		if state.Cash < state.Slots.Bet {
			next.MessageKey = "ui.message.no_credit"
			return next, nil
		}
		change := spinSlots(&next, next.Attribute(AttrLuck))
		next.Cash += change
		if next.Cash < 0 {
			next.Cash = 0
		}
		switch {
		case next.Slots.Matched >= 3:
			next.MessageKey = "ui.message.slots_jackpot"
		case next.Slots.Matched == 2:
			next.MessageKey = "ui.message.slots_pair"
		default:
			next.MessageKey = "ui.message.slots_lost"
		}
	case ActionArenaAttack:
		if state.Screen != ScreenArena {
			return state, fmt.Errorf("attacking is only valid in the arena")
		}
		if !state.Arena.Active {
			next.MessageKey = "ui.message.arena_over"
			return next, nil
		}
		next.MessageKey = resolveArenaAttack(&next)
	case ActionArenaFlee:
		if state.Screen != ScreenArena {
			return state, fmt.Errorf("fleeing is only valid in the arena")
		}
		next.Arena = Arena{}
		next.Screen = ScreenGround
		next.MessageKey = "ui.message.arena_fled"
	case ActionLandingTick:
		if state.Landing.Phase == LandingIdle {
			return state, fmt.Errorf("no landing is running")
		}
		landing := state.Landing
		if !advanceLanding(&landing) {
			next.Landing = Landing{}
			next.MessageKey = "ui.message.landed"
			return next, nil
		}
		next.Landing = landing
	case ActionSkipLanding:
		if state.Landing.Phase == LandingIdle {
			return state, fmt.Errorf("no landing is running")
		}
		next.Landing = Landing{}
		next.MessageKey = "ui.message.landed"
	case ActionOpenScore:
		next.Screen = ScreenScore
		next.MessageKey = "ui.message.score"
	case ActionBack:
		next.TacticalSlewing = false
		if next.Screen == ScreenFTLBoost {
			next.Screen, next.MessageKey = ScreenGround, "ui.message.back"
			if next.GroundReturn == ScreenDepot {
				next.Screen = ScreenDepot
			}
			next.GroundReturn = ScreenGround
			break
		}
		if next.Screen == ScreenDepot {
			next.Screen, next.GroundReturn, next.MessageKey = ScreenGround, ScreenGround, "ui.message.back"
			break
		}
		if next.Screen == ScreenFuelPurchase {
			finishFuelPurchase(&next)
			break
		}
		if next.Screen == ScreenTrade && next.TradePage != TradePageEntrance {
			// A page closes back to the one before it: quote and lots to
			// Buy/Sell, Buy/Sell and the warehouse to Exchange/Warehouse,
			// the warehouse's pickers to the warehouse.
			switch next.TradePage {
			case TradePageBuyDestination:
				// write pick_item's -1 input prefers pod, with fallback.
				next.MessageKey, _ = r.finishTradePurchase(&next, true)
			case TradePageSellConfirm:
				cancelTradeSale(&next)
			case TradePageSwapPod, TradePageSwapWarehouse:
				next.TradeSaleSlot, next.TradeSalePrice = 0, 0
				next.TradePage = TradePageWarehouse
			case TradePageStore, TradePageFetch:
				next.TradePage = TradePageWarehouse
			case TradePageBuy, TradePageSell, TradePageSellSource, TradePageSellWarehouse:
				next.TradePage = TradePageMain
			default:
				next.TradePage = TradePageEntrance
			}
			break
		}
		if next.Screen == ScreenTubeStation && next.TubePage == TubePageConfirm {
			// The right button on the confirmation page goes back to the list,
			// the way it goes back a step everywhere else in the station.
			next.TubePage, next.TubeTargetID = TubePageList, ""
			next.MessageKey = "ui.message.tube_station"
			break
		}
		if next.Screen == ScreenUniteller && next.TellerPage == TellerPageTransferPick {
			// pick_location on the right button returns nothing: back to the
			// teller's boxes.
			next.TellerPage, next.TellerTransferID = TellerPageMain, ""
			break
		}
		if next.Screen == ScreenUniteller && next.TellerPage != TellerPageMain {
			// Leaving the keypad is what moves the amount (move_cash).
			if err := r.tellerConfirm(&next); err != nil {
				return state, err
			}
			break
		}
		next = back(next)
	case ActionCyclePresentation:
		// One axis, not three. The colours, the tile art and the layout move
		// together because that is the only way every reachable combination is
		// one someone drew: the Atari ST layout paints from the player's disk
		// and ignores the other two, so as separate axes two of the three
		// controls did nothing there — and one pairing drew the remake's map
		// over the original's city.
		current := theme.SnapPresentation(state.Theme, r.TilesetOf(state), state.Layout)
		next.Theme, next.Tileset, next.Layout = presentationTriple(theme.NextPresentation(current.ID))
		next.MessageKey = "ui.message.presentation_changed"
	case ActionSave:
		next.Saved = true
		next.MessageKey = "ui.message.saved"
	case ActionQuit:
		next.QuitRequested = true
		next.MessageKey = "ui.message.quit"
	default:
		return state, fmt.Errorf("unknown action %q", action)
	}
	if isCombatTurnAction(action) {
		// 每次正式戰術更新先持續相位，再耗能及敵艦更新。
		// 已發射的未命中也必須走到這裡，不能略過整拍。
		advanceShipPhase(&next)
		updateShipEnergyFromFuel(&next, state.Fuel, func(n int) int { return combatRoll(&next, n) })
		// The enemy steers before it shoots, the order set_course and
		// update_attack run in. Closing in cuts both ways: the same distance
		// feeds the player's damage, the enemy's accuracy and the enemy's
		// damage, so the enemy's own course decides how dangerous the fight is.
		if next.CombatActive {
			steerEnemy(&next)
			resolveEnemyFire(&next)
		}
	}
	if action != ActionBack && next.Screen != ScreenOutcome && (next.Vigor <= 0 || next.HullCondition <= 0) {
		next.Sleep = SleepState{}
		next.Screen = ScreenOutcome
		next.OutcomeKey = "ui.outcome.failure"
		next.CombatActive = false
		next.MessageKey = "ui.message.defeat"
	}
	return next, nil
}

// RouteCost estimates the selected trip. Coordinates and ETA formulas are
// source-derived; mode2 movement uses the saved phase. Damaged engines, active
// energy systems and world/warp composition prevent a whole-trip exact claim.
func (r Reducer) RouteCost(state State) (world.RouteCost, bool) {
	cost, ok := r.World.RouteCost(state.SystemID, state.NavigationTargetID, state.JumpRange, state.SystemCoordinates)
	if !ok {
		return cost, false
	}
	// The registry cannot work the in-system leg out: it does not know where
	// the ship is, and both of the original's figures depend on that.
	destination, ok := r.World.Destination(state.NavigationTargetID)
	if !ok {
		return cost, true
	}
	planet, ok := r.World.Planet(destination.PlanetID)
	if !ok {
		return cost, true
	}
	if destination.ID == state.CityID {
		return cost, true
	}
	legs := routeTicks(state, r.PlanetOrbit(state, planet), destination.PlanetID == state.PlanetID)
	cost.FuelCost = routeFuelCost(state, legs)
	// The clock figure is the original's ETA, which is computed from the
	// undamaged tick count even when the engine will stretch the trip. Being
	// late when the sub-c engine is wrecked is the original's behaviour, not a
	// rounding slip.
	cost.TotalMinutes += legs.Minutes
	return cost, true
}

// repairCeiling and repairAmount keep the three older shunt repairs working on
// their own scales. The hull is on the source's scale now; the weapon and the
// shields are still remake-owned prototype levels.
func repairCeiling(action Action) int {
	switch action {
	case ActionRepairWeapon:
		return maxWeaponCharge
	case ActionRepairShields:
		return maxShieldLevel
	default:
		return MaxHullCondition()
	}
}

func repairAmount(action Action) int {
	if action == ActionRepair {
		// A quarter of the hull, which on the source's scale is four points
		// rather than the prototype's twenty-five.
		return MaxHullCondition() / 4
	}
	return prototypeShuntRepair
}

func isCombatTurnAction(action Action) bool {
	switch action {
	case ActionTacticalMove, ActionFire, ActionFireLaser, ActionRechargeWeapon, ActionShieldsOff, ActionFullShields:
		return true
	default:
		return false
	}
}

// advanceTime updates the four readings only on hour boundaries, as
// MAINLIB:35's 0bff -> 0cf1 gate requires. Status sleep preserves Rest;
// awake Rest0/Vigor0 is dispatched only after the full increment completes.
func advanceTime(state *State, minutes int) {
	hours := (state.TimeMinutes%60 + minutes) / 60
	advanceClockMinutes(state, minutes)
	if minutes > 0 && hours > 0 && !state.Sleep.Active {
		interruptForConditions(state)
	}
}

// MAINLIB:35's clock/condition writes without foreground interrupt dispatch.
func advanceClockMinutes(state *State, minutes int) {
	if minutes <= 0 {
		return
	}
	ticks := (state.TimeMinutes%60 + minutes) / 60
	asleep := state.Sleep.Active
	state.TimeMinutes += minutes
	for i := 0; i < ticks; i++ {
		if advanceConditions(state, asleep) {
			// The reducer's terminal gate uses Vigor, not Health. Sleep
			// finishes its Rest writer before it performs that gate.
			state.MessageKey = "ui.message.collapsed"
		}
		advanceAttributes(state)
		if state.Sleep.Active && state.Rest == 0 {
			state.Sleep.Forced = true
		}
	}
}

// heading turns a move into the direction the compass then points at.
func heading(action Action) Heading {
	switch action {
	case ActionMoveSouth:
		return HeadingSouth
	case ActionMoveWest:
		return HeadingWest
	case ActionMoveEast:
		return HeadingEast
	default:
		return HeadingNorth
	}
}

func move(position Position, action Action) Position {
	switch action {
	case ActionMoveNorth:
		if position.Y > 0 {
			position.Y--
		}
	case ActionMoveSouth:
		if position.Y < world.MapHeight()-1 {
			position.Y++
		}
	case ActionMoveWest:
		if position.X > 0 {
			position.X--
		}
	case ActionMoveEast:
		if position.X < world.MapWidth()-1 {
			position.X++
		}
	}
	return position
}

func back(state State) State {
	if state.Screen == ScreenStores || state.Screen == ScreenCharacterStatus || state.Screen == ScreenEngineering {
		state = (Reducer{}).CancelHand(state)
	}
	switch state.Screen {
	case ScreenCharacterLibrary:
		if state.LibraryReturn != "" {
			state.Screen = state.LibraryReturn
			state.LibraryReturn = ""
		} else {
			state.Screen = ScreenTitle
		}
	case ScreenCharacterCreation:
		state.Screen = ScreenCharacterLibrary
	case ScreenAttributeAllocation:
		state.Screen = ScreenCharacterCreation
		state.AttributeWasteWarned = false
		state.MessageKey = "ui.message.character_name"
	case ScreenCharacterStatus:
		if state.StatusReturn == "" {
			state.Screen = ScreenShipboard
		} else {
			state.Screen = state.StatusReturn
		}
	case ScreenTrade:
		state.Screen = ScreenGround
	case ScreenParts:
		state.Screen = ScreenGround
	case ScreenUniteller:
		state.Screen = ScreenGround
	case ScreenDialog:
		if state.GroundReturn == ScreenInterior && state.Interior.Active {
			state.Screen = ScreenInterior
		} else {
			state.Screen = ScreenGround
		}
		state.DialogSaid = ""
		state.StreetAsk = 0
		// A mugger says his line first and then draws. bring_up_window runs
		// before the arena is set up, so walking away from that conversation is
		// where the fight starts.
		if state.StreetPending == StreetKindMugger {
			state.StreetPending = 0
			beginArena(&state, arenaRand(&state.Arena, arenaMinRecords, arenaMaxRecords))
		}
	case ScreenInterior:
		state.Position = state.Interior.ReturnPosition
		state.Interior = Interior{}
		state.Screen = ScreenGround
	case ScreenSlots:
		state.Screen = ScreenInterior
	case ScreenArena:
		state.Screen = ScreenGround
	case ScreenScore:
		state.Screen = ScreenShipboard
	case ScreenGround:
		// 地面是根畫面，沒有上一層。回船要走到船那一格（re_enter_ship），
		// 不是按一個鍵就到。
		state.MessageKey = "ui.message.walk_to_ship"
		return state
	case ScreenShipboard:
		state.Screen = ScreenTitle
		if state.Saved {
			// Page 5: "The title screen will show that the game has been saved."
			state.MessageKey = "ui.message.saved"
			return state
		}
	case ScreenNavigation, ScreenFlight, ScreenTactical, ScreenEngineering, ScreenCargo, ScreenStores:
		state.Screen = ScreenShipboard
	case ScreenEquipment:
		state.Screen = ScreenEngineering
	case ScreenOutcome:
		state.Screen = ScreenShipboard
	default:
		state.Screen = ScreenTitle
	}
	state.MessageKey = "ui.message.back"
	return state
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func selectDestination(state *State, destination world.Destination) {
	state.NavigationTargetID = destination.ID
	state.NavigationTarget = destination.Label
}

func storeCurrentBank(state *State) {
	if state.BankBalances == nil {
		state.BankBalances = make(map[string]int)
	}
	state.BankBalances[state.SystemID] = state.Bank
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneBalances(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

// arriveAt puts the ship down at a destination: a flown trip, so the ship comes
// too and the player steps out of it.
func arriveAt(state *State, destination world.Destination) {
	arrive(state, destination, true)
}

// arriveByTube is the Uniteller's tube ride. The player arrives, the ship does
// not: it stays parked in the city it was flown to, which is what makes the
// ride a different thing from a flight. Every city's landing cell carries a
// Uniteller (internal/world/facilities.go), so the ride is never one-way.
func arriveByTube(state *State, destination world.Destination) {
	arrive(state, destination, false)
}

// arrive is the tail both ways share, so neither can drift from the other.
func arrive(state *State, destination world.Destination, withShip bool) {
	storeCurrentBank(state)
	// The parking meter starts when the ship touches down. The original stamps
	// it when the player steps out of the pod; this shell has no separate act
	// of parking, so landing is the moment it uses (see internal/game/fine.go).
	state.PodParkedAt = state.TimeMinutes
	state.SystemID = destination.SystemID
	state.PlanetID = destination.PlanetID
	state.CityID = destination.CityID
	state.Location = destination.Label
	if _, exists := state.BankBalances[destination.SystemID]; !exists {
		state.BankBalances[destination.SystemID] = 0
	}
	state.Bank = state.BankBalances[destination.SystemID]
	state.Position = landingPosition(destination.CityID)
	state.GroundWorld = false
	state.GroundView = GroundView{}
	if !contains(state.VisitedDestinations, destination.ID) {
		state.VisitedDestinations = append(state.VisitedDestinations, destination.ID)
	}
	state.FlightTargetID = ""
	if !withShip {
		// 人到了，船留在原地——所以是站在街上，不是站在船艙裡。
		state.Screen = ScreenGround
		state.MessageKey = "ui.message.tube_arrived"
		return
	}
	state.ShipCityID = destination.CityID
	state.Position = groundLandingPosition(state, destination.CityID)
	state.ShipPosition = state.Position
	if state.CargoAttached {
		state.CargoCityID, state.CargoPosition = state.ShipCityID, state.ShipPosition
	}
	state.GroundWorld = false
	state.GroundView = GroundView{}
	state.Screen = ScreenShipboard
	state.MessageKey = "ui.message.navigated"
}

// TilesetOf is the tile art a state resolves to. A save written before the two
// axes split records no tileset, so it falls back to the one its theme used to
// carry — which is what it looked like when it was written.
func (r Reducer) TilesetOf(state State) theme.TilesetID {
	if state.Tileset != "" {
		return state.Tileset
	}
	if fallback, ok := r.Themes.DefaultTileset(state.Theme); ok {
		return fallback
	}
	return theme.AppleIITiles
}

// GroupOf resolves a state into something drawable.
func (r Reducer) GroupOf(state State) (theme.Group, bool) {
	return r.Themes.Pair(state.Theme, r.TilesetOf(state))
}

// interiorStep turns an interior movement action into a one-cell delta.
func interiorStep(action Action) (dx, dy int) {
	switch action {
	case ActionInteriorNorth:
		return 0, -1
	case ActionInteriorSouth:
		return 0, 1
	case ActionInteriorWest:
		return -1, 0
	case ActionInteriorEast:
		return 1, 0
	default:
		return 0, 0
	}
}

// resolveArenaAttack is the keyboard's way into a fight that is otherwise
// driven by the pointer: aim at the nearest opponent, pull the trigger and run
// one pass. The pointer path is Reducer.StepArena.
func resolveArenaAttack(state *State) string {
	arena := &state.Arena
	x, y, ok := arenaNearestThug(arena)
	if !ok {
		// Nobody left to aim at. Still run the pass, which is what closes the
		// fight out.
		return arenaPass(state, ArenaInput{PointerX: arena.PlayerX, PointerY: arena.PlayerY})
	}
	arena.FireLatch = true
	return arenaPass(state, ArenaInput{PointerX: x, PointerY: y, Firing: true})
}

// beginArena opens a fight against the crowd a street encounter produced.
// `records` is XDOCOMBA local 0x1a, which counts the player as well: record 1
// is the player and the opponents are 2..records, so a mugging's rand(2,6)
// means one to five of them.
//
// Every number is the original's. init_people gives each opponent rand(70,250)
// hit points, a firing speed of random() mod 8 + 6 and a walking speed of
// 6 + random() mod 4; the first one is dropped anywhere on the field and the
// rest stand off the centre by (15, 11).
func beginArena(state *State, records int) {
	records = clampInt(records, arenaMinRecords, arenaMaxRecords)
	arena := Arena{
		Active:    true,
		HitPoints: ArenaHitPoints(state.Health, state.Attribute(AttrStrength)),
		Speed:     ArenaSpeed(state.Attribute(AttrDexterity)),
		Rolls:     state.Arena.Rolls,
		Thugs:     make([]ArenaThug, records-1),
		PlayerX:   arenaCentreX,
		PlayerY:   arenaCentreY,
		TargetX:   arenaCentreX,
		TargetY:   arenaCentreY,
		Halted:    true,
		FireLatch: true,
	}
	for index := range arena.History {
		arena.History[index] = ArenaPoint{X: arena.PlayerX, Y: arena.PlayerY}
	}
	slots := carrySlots(*state)
	arena.Gun, arena.Shots = arenaDrawGun(&slots)
	arena.ShieldSlot, arena.ShieldCharges = arenaShieldSlot(slots)
	writeCarrySlots(state, slots)
	for index := range arena.Thugs {
		arena.Thugs[index] = rollThug(&arena, index)
	}
	state.Arena = arena
	state.Screen = ScreenArena
}
