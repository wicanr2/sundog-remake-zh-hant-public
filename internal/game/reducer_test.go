package game

import (
	"os"
	"reflect"
	"strings"
	"testing"

	apple2source "github.com/wicanr2/sundog-remake/internal/source/apple2"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// stepToWalkableNeighbour moves one cell in whichever direction the city's own
// map allows. Tests use it instead of a fixed direction because each of the 60
// cities has its own layout.
func stepToWalkableNeighbour(t *testing.T, reducer Reducer, state State) State {
	t.Helper()
	for _, action := range []Action{ActionMoveEast, ActionMoveWest, ActionMoveNorth, ActionMoveSouth} {
		moved, err := reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
		if moved.Position != state.Position {
			return moved
		}
	}
	t.Fatalf("no walkable neighbour of (%d,%d)", state.Position.X, state.Position.Y)
	return state
}

// stepOffTheFacilityCluster moves onto a neighbouring cell that carries no
// facility, so a test can exercise the "not here" branch of a ground action.
func stepOffTheFacilityCluster(t *testing.T, reducer Reducer, state State) State {
	t.Helper()
	for _, action := range []Action{ActionMoveEast, ActionMoveWest, ActionMoveNorth, ActionMoveSouth} {
		moved, err := reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
		if moved.Position == state.Position {
			continue
		}
		position := world.TilePosition{X: moved.Position.X, Y: moved.Position.Y}
		if len(reducer.World.FacilitiesAt(moved.CityID, position)) == 0 {
			return moved
		}
	}
	t.Fatalf("every neighbour of (%d,%d) carries a facility", state.Position.X, state.Position.Y)
	return state
}

func TestNormalPrototypePathAndThemeDoesNotMutateRules(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game banks the original's opening balance; these tests are about
	// what the bank does, not what it starts with.
	state.Bank = 0
	state.BankBalances = map[string]int{}

	for _, action := range []Action{ActionPlay, ActionOpenGround, ActionOpenTrade} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatalf("start city %q quoted nothing", state.CityID)
	}
	// The ring's first quote may cost more than a new purse holds; the
	// purse is topped up so the path goes through, the bank stays empty.
	state.Cash = Credits(quote.Price) + 100
	resetTradeTestFunds(&state)
	state, err = reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenTrade || WarehouseLotsOf(state, state.CityID)[0].Goods != quote.Goods || state.Gems != 0 || TradeFunds(state) != 100 || state.Bank != 0 {
		t.Fatalf("unexpected path state: %+v (quote %+v)", state, quote)
	}
	state.Fuel = 90
	before := state
	state, err = reducer.Step(state, ActionCyclePresentation)
	if err != nil {
		t.Fatal(err)
	}
	if got := PresentationOf(state); got.ID == PresentationOf(before).ID {
		t.Fatalf("呈現沒有換：%q", got.ID)
	}
	after := state
	after.Theme, after.Tileset, after.Layout = before.Theme, before.Tileset, before.Layout
	after.MessageKey = before.MessageKey
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("換呈現動到了遊戲規則：before=%+v after=%+v", before, state)
	}
}

// boardShip 走出去再走回船停的那一格。重製版現在只有這一條上船的路
// （re_enter_ship），`ActionBack` 在地面上不再回船。
func boardShip(t *testing.T, reducer Reducer, state State) State {
	t.Helper()
	if state.Screen != ScreenGround {
		t.Fatalf("boardShip 要從地面開始，現在是 %s", state.Screen)
	}
	for _, action := range []Action{ActionMoveEast, ActionMoveWest} {
		next, err := reducer.Step(state, action)
		if err != nil {
			t.Fatalf("走回船的 %s：%v", action, err)
		}
		state = next
	}
	if state.Screen != ScreenShipboard {
		t.Fatalf("走回船那一格之後在 %s，訊息 %q", state.Screen, state.MessageKey)
	}
	return state
}

func TestBackAndSave(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state = boardShip(t, reducer, state)
	for _, action := range []Action{ActionBack, ActionSave} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	if state.Screen != ScreenTitle || !state.Saved {
		t.Fatalf("unexpected back/save state: %+v", state)
	}
}

func TestEquipmentCatalogIsAReadOnlyEngineeringChildScreen(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	if _, err := reducer.Step(state, ActionOpenEquipment); err == nil {
		t.Fatal("equipment catalog opened outside engineering")
	}
	state, err = reducer.Step(state, ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionOpenEngineering)
	if err != nil {
		t.Fatal(err)
	}
	before := state
	state, err = reducer.Step(state, ActionOpenEquipment)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenEquipment || state.Fuel != before.Fuel || state.Shunts != before.Shunts {
		t.Fatalf("equipment catalog changed game state: before=%+v after=%+v", before, state)
	}
	state, err = reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenEngineering {
		t.Fatalf("equipment catalog did not return to engineering: %+v", state)
	}
}

func TestNewGameResetsRemakeStateFromCharacterLibrary(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game banks the original's opening balance; these tests are about
	// what the bank does, not what it starts with.
	state.Bank = 0
	state.BankBalances = map[string]int{}

	state.Cash = 1
	state.Bank = 2
	state.MissionComplete = true
	state = WithPresentation(state, theme.NextPresentation(PresentationOf(state).ID))
	look := PresentationOf(state)
	state, err = reducer.Step(state, ActionSelectGame)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionNewGame)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenCharacterCreation || state.Cash != NewState().Cash || state.Bank != NewState().Bank || state.MissionComplete {
		t.Fatalf("開新局沒有把重製版狀態歸零：%+v", state)
	}
	kept := PresentationOf(state)
	if kept.ID != look.ID {
		t.Fatalf("開新局把呈現從 %q 換成 %q", look.ID, kept.ID)
	}
	if state.Theme != kept.Chrome || state.Tileset != kept.Tileset || state.Layout != kept.Layout {
		t.Fatalf("開新局之後三個欄位不是同一個預設：%s/%s/%s", state.Theme, state.Tileset, state.Layout)
	}
	state = SetCharacterNameDraft(state, "莉亞")
	state, err = reducer.Step(state, ActionConfirmCharacterName)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenAttributeAllocation || state.Character != "莉亞" || state.CharacterDraft != "莉亞" {
		t.Fatalf("character creation did not commit the draft: %+v", state)
	}
	for index := range state.Attributes {
		if state.Attributes[index] != 30 || state.AttributeMax[index] != 30 {
			t.Fatalf("attribute %d did not start at 30: %+v", index, state)
		}
	}
	if state.AttributePoints != CharacterCreationPoints || state.AttributeSelected != AttrStrength {
		t.Fatalf("allocation did not start with the Atari ST pool: %+v", state)
	}
}

func TestCharacterAttributeAllocationAndWasteConfirmation(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenAttributeAllocation
	state.CharacterDraft = "莉亞"
	state.Attributes = newCharacterCreationAttributes()
	state.AttributeMax = state.Attributes
	state.AttributePoints = CharacterCreationPoints

	state, err = reducer.Step(state, ActionIncreaseAttribute)
	if err != nil || state.Attributes[AttrStrength] != 31 || state.AttributePoints != 439 {
		t.Fatalf("increase did not consume one point: %+v, %v", state, err)
	}
	state, err = reducer.Step(state, ActionNextAttribute)
	if err != nil || state.AttributeSelected != AttrIntelligence {
		t.Fatalf("next attribute did not select IQ: %+v, %v", state, err)
	}
	state, err = reducer.Step(state, ActionDecreaseAttribute)
	if err != nil || state.Attributes[AttrIntelligence] != 30 || state.AttributePoints != 439 {
		t.Fatalf("minimum attribute boundary failed: %+v, %v", state, err)
	}
	state, err = reducer.Step(state, ActionConfirmAttributes)
	if err != nil || state.Screen != ScreenAttributeAllocation || !state.AttributeWasteWarned {
		t.Fatalf("first waste confirmation did not warn: %+v, %v", state, err)
	}
	state, err = reducer.Step(state, ActionConfirmAttributes)
	if err != nil || state.Screen != ScreenShipboard || state.AttributeWasteWarned || state.AttributePoints != 0 || state.CharacterDraft != "" {
		t.Fatalf("second waste confirmation did not enter the game: %+v, %v", state, err)
	}
}

func TestCharacterAttributeAllocationBackPreservesDraftAndDistribution(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenAttributeAllocation
	state.CharacterDraft = "莉亞"
	state.Attributes = newCharacterCreationAttributes()
	state.AttributeMax = state.Attributes
	state.AttributePoints = CharacterCreationPoints
	state, _ = reducer.Step(state, ActionIncreaseAttribute)
	state, err = reducer.Step(state, ActionBack)
	if err != nil || state.Screen != ScreenCharacterCreation || state.CharacterDraft != "莉亞" || state.Attributes[AttrStrength] != 31 || state.AttributePoints != 439 {
		t.Fatalf("back did not preserve the allocation draft: %+v, %v", state, err)
	}
}

func TestCharacterNameConfirmationRejectsEmptyDraft(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state, err = reducer.Step(state, ActionSelectGame)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionNewGame)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionConfirmCharacterName)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenCharacterCreation || state.MessageKey != "ui.message.character_name_invalid" {
		t.Fatalf("empty character draft was not rejected: %+v", state)
	}
}

func TestCharacterNameDraftHandlesCJKInputAndBackspace(t *testing.T) {
	state := NewState()
	state.CharacterDraft = ""
	state = AppendCharacterNameDraft(state, []rune("陽犬號"))
	if state.CharacterDraft != "陽犬號" || !CharacterNameValid(state.CharacterDraft) {
		t.Fatalf("CJK draft was not retained: %+v", state)
	}
	state = BackspaceCharacterNameDraft(state)
	if state.CharacterDraft != "陽犬" {
		t.Fatalf("backspace removed the wrong rune: %q", state.CharacterDraft)
	}
}

func TestLoadGameRequestIsLimitedToTitleScreens(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state, err = reducer.Step(state, ActionLoadGame)
	if err != nil || state.MessageKey != "ui.message.load_requested" {
		t.Fatalf("title load request failed: state=%+v err=%v", state, err)
	}
	state, err = reducer.Step(state, ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reducer.Step(state, ActionLoadGame); err == nil {
		t.Fatal("load request was accepted outside title screens")
	}
}

func TestCharacterLibraryOperationsExposeLocalizedRemakeActions(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state, err = reducer.Step(state, ActionSelectGame)
	if err != nil {
		t.Fatal(err)
	}
	for action, message := range map[Action]string{
		ActionMakeLibrary: "ui.message.library_created",
		ActionStoreGame:   "ui.message.game_stored",
		ActionDeleteGame:  "ui.message.game_deleted",
	} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("library action %s: %v", action, err)
		}
		if state.Screen != ScreenCharacterLibrary || state.MessageKey != message {
			t.Fatalf("library action %s produced unexpected state: %+v", action, state)
		}
	}
}

func TestTradeBankPriorityAndSaleIncome(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := gameStateAtTrade(t, reducer)
	state.GroundMode = GroundInPod // 受控資金案明示載具與目的槽。
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatalf("trade city %q quoted nothing", state.CityID)
	}
	// The bank holds less than the price, so the rest has to come out of cash.
	state.Bank = Credits(quote.Price) - 5
	state.Cash = 70
	resetTradeTestFunds(&state)
	state, err = reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionTradePick2)
	if err != nil {
		t.Fatal(err)
	}
	if state.Bank != 0 || state.Cash != 70 || TradeFunds(state) != 65 || state.Gems != 1 {
		t.Fatalf("buy did not use bank then cash: %+v (quote %+v)", state, quote)
	}
	state, err = reducer.Step(state, ActionSellGems)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionTradePick1)
	if err != nil {
		t.Fatal(err)
	}
	// 原版買賣在同一可用金額累計，離開才分回Cash／Bank。
	if state.LastSalePrice <= 0 || state.LastSalePrice >= quote.Price || TradeFunds(state) != 65+Credits(state.LastSalePrice) || state.Cash != 70 || state.Gems != 0 {
		t.Fatalf("sale did not add to session funds: %+v (quote %+v)", state, quote)
	}
	if CargoItemKey(state) != "" {
		t.Fatalf("empty cargo did not clear source item key: %q", CargoItemKey(state))
	}
}

func TestTradeStoresAtariSTSourceItemIdentity(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := gameStateAtTrade(t, reducer)
	state.GroundMode = GroundInPod
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatalf("trade city %q quoted nothing", state.CityID)
	}
	quoted, _ := reducer.World.CargoKindByCode(quote.Goods)
	state.Cash = Credits(quote.Price) + 1
	resetTradeTestFunds(&state)
	state, err = reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionTradePick2)
	if err != nil {
		t.Fatal(err)
	}
	if CargoItemKey(state) != quoted.Key {
		t.Fatalf("trade did not retain the source item identity: %+v (quote %+v)", state, quote)
	}
	kind, ok := reducer.World.CargoKind(CargoItemKey(state))
	if !ok {
		t.Fatalf("stored identity %q is not a source cargo record", CargoItemKey(state))
	}
	if reducer.CargoName(state) != kind.Name {
		t.Fatalf("without a localizer the cargo name should be the source name: %q", reducer.CargoName(state))
	}
}

func TestGroundFacilitiesGateActionsAndMarketPriceIsLocationAware(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// A new game banks the original's opening balance; these tests are about
	// what the bank does, not what it starts with.
	state.Bank = 0
	state.BankBalances = map[string]int{}

	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	away := stepOffTheFacilityCluster(t, reducer, state)
	away, err = reducer.Step(away, ActionOpenTrade)
	if err != nil || away.Screen != ScreenGround || away.MessageKey != "ui.message.need_exchange" {
		t.Fatalf("trade opened away from exchange: state=%+v err=%v", away, err)
	}
	state, err = reducer.Step(state, ActionOpenTrade)
	if err != nil {
		t.Fatal(err)
	}
	quote, ok := CurrentQuote(state)
	if !ok {
		t.Fatalf("start city %q quoted nothing", state.CityID)
	}
	// The bank opens empty, so the price comes off the purse instead.
	state.Cash = Credits(quote.Price) + 50
	resetTradeTestFunds(&state)
	cashBefore := state.Cash
	state, err = reducer.Step(state, ActionBuyGems)
	if err != nil || WarehouseLotsOf(state, state.CityID)[0].Goods != quote.Goods || state.Gems != 0 || TradeFunds(state) != cashBefore-Credits(quote.Price) || state.Cash != cashBefore {
		t.Fatalf("start market purchase changed unexpectedly: state=%+v err=%v quote=%+v", state, err, quote)
	}
}

func TestEveryMarketCargoComesFromTheSourceTable(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{})
	for _, destination := range reducer.World.Destinations() {
		state := NewState()
		state.Screen = ScreenTrade
		state.CityID = destination.CityID
		state.GroundMode = GroundInPod
		state.SystemID = destination.SystemID
		state.Bank = 100000
		state.Holds = [CargoLots]Hold{}
		state.Gems = 0
		reducer.openExchange(&state)
		quote, ok := CurrentQuote(state)
		if !ok {
			t.Fatalf("exchange at %s quoted nothing", destination.ID)
		}
		state, err = reducer.Step(state, ActionBuyGems)
		if err != nil {
			t.Fatalf("buy at %s: %v", destination.ID, err)
		}
		state, err = reducer.Step(state, ActionTradePick2)
		if err != nil {
			t.Fatal(err)
		}
		if kind, _ := reducer.World.CargoKindByCode(quote.Goods); CargoItemKey(state) != kind.Key {
			t.Fatalf("market source identity did not reach state at %s: state=%+v quote=%+v", destination.ID, state, quote)
		}
		if _, ok := reducer.World.CargoKind(CargoItemKey(state)); !ok {
			t.Fatalf("market at %s quoted %q, which is not a source cargo record", destination.ID, CargoItemKey(state))
		}
		seen[CargoItemKey(state)] = struct{}{}
	}
	// Sixty exchanges quoting eight goods each cover the table many times
	// over; a few kinds may still sit out of every first quote.
	if want := len(reducer.World.CargoKinds()); len(seen)*2 < want {
		t.Fatalf("market path exposed %d cargo kinds, want at least half of the %d the source table prices", len(seen), want)
	}
}

// TestAtariSTCargoTableExtendsTheAppleIICargoList keeps the cross-platform
// corroboration honest. The Apple II cargo list was decoded from Apple II bytes
// and the Atari ST table from ST bytes, independently; the ST table turns out to
// open with the same sixteen names in the same order, and only then adds its own
// sixteen. That is what lets a schema 7 save's cargo identity be carried across
// by name instead of being guessed, so a drift here has to fail loudly.
func TestAtariSTCargoTableExtendsTheAppleIICargoList(t *testing.T) {
	data, err := os.ReadFile("../../docs/i18n/apple2-text-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	sourceText, err := apple2source.LoadTextCatalog(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	items, err := atarist.EmbeddedItemData()
	if err != nil {
		t.Fatal(err)
	}
	cargo := items.Cargo()
	appleKeys := sourceText.KeysWithPrefix("apple2.cargo.")
	if len(appleKeys) == 0 {
		t.Fatal("Apple II source catalog carries no cargo entries")
	}
	if len(cargo) < len(appleKeys) {
		t.Fatalf("Atari ST cargo table has %d records, fewer than the Apple II list's %d", len(cargo), len(appleKeys))
	}
	// KeysWithPrefix is in raw-offset order, which for a fixed-stride source
	// table is table order.
	for index, key := range appleKeys {
		entry, ok := sourceText.Entry(key)
		if !ok {
			t.Fatalf("Apple II catalog lost entry %q", key)
		}
		want := cargo[index].Name
		// The only name that differs across the two platforms.
		if entry.Source == "ship's fuel" && want == "fuel" {
			continue
		}
		if entry.Source != want {
			t.Fatalf("cargo index %d is %q on Apple II and %q on Atari ST", index, entry.Source, want)
		}
	}
}

func TestFoodStatusUsesAppleIISourceIdentity(t *testing.T) {
	data, err := os.ReadFile("../../docs/i18n/apple2-text-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	sourceText, err := apple2source.LoadTextCatalog(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	reducer = reducer.WithAppleIIText(sourceText)
	state := NewState()
	state.Screen = ScreenCharacterStatus
	state, err = reducer.Step(state, ActionEat)
	if err != nil {
		t.Fatal(err)
	}
	if state.FoodItemKey != "apple2.ship.food" || state.FoodUnits != 0 {
		t.Fatalf("food source identity did not survive status action: %+v", state)
	}
}

func TestRefuelRequiresDepotAndUsesBankBeforeCash(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	state.Fuel = 0
	state.Bank = 100
	state.Cash = 100
	away := stepOffTheFacilityCluster(t, reducer, state)
	away, err = reducer.Step(away, ActionRefuel)
	if err != nil || away.MessageKey != "ui.message.need_depot" || away.Screen != ScreenGround || away.Fuel != 0 {
		t.Fatalf("refuel opened away from depot: state=%+v err=%v", away, err)
	}
	depot, ok := reducer.World.FacilityPosition(state.CityID, world.FacilityDepot)
	if !ok {
		t.Fatal("source city has no depot")
	}
	state.Position = Position{depot.X, depot.Y}
	for _, action := range []Action{ActionRefuel, ActionFuelBuyOne, ActionFuelCancel} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Source system1 caller: one16-unit segment costs157.
	if state.Fuel != 16 || state.Bank != 0 || state.Cash != 43 || state.MessageKey != "ui.message.refueled" {
		t.Fatalf("refuel did not use bank then cash: state=%+v err=%v", state, err)
	}
	state.Bank, state.Cash = 3000, 0
	for _, action := range []Action{ActionRefuel, ActionFuelBuyFull, ActionFuelCancel} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Fuel != 255 || state.Bank != 645 || state.Cash != 0 {
		t.Fatalf("full purchase: fuel=%d bank=%d cash=%d", state.Fuel, state.Bank, state.Cash)
	}
}

func TestManualBackedShipSystemsAndUnitellerPath(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	// 此案例測航程／任務，先滿足原版非首城降落所需的掃描器。
	fitGroundScannerForTest(t, &state)
	// A new game banks the original's opening balance; these tests are about
	// what the bank does, not what it starts with.
	state.Bank = 0
	state.BankBalances = map[string]int{}

	for _, action := range []Action{ActionPlay, ActionOpenGround, ActionOpenUniteller, ActionDepositCash} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// Depositing the whole purse puts exactly that much in the local bank.
	if state.Screen != ScreenUniteller || state.Cash != 0 || state.Bank != NewState().Cash {
		t.Fatalf("deposit did not use local bank: %+v", state)
	}
	state, err = reducer.Step(state, ActionWithdrawCash)
	if err != nil {
		t.Fatal(err)
	}
	if state.Cash != NewState().Cash || state.Bank != 0 {
		t.Fatalf("withdraw did not return local bank balance: %+v", state)
	}
	state, err = reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	state = boardShip(t, reducer, state)
	// A new game starts empty; the fuel arithmetic below is what this test pins.
	state.Fuel = maxFuel
	beforeFlight := state.TimeMinutes
	for _, action := range []Action{ActionOpenNavigation, ActionNavigate} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// Drahew/Banville's existing world-hop charge4 is retained separately
	// from mode2. Its full phase composition remains Issue #6; this test pins
	// the remake's compatibility boundary, not original total-fuel parity.
	if state.Screen != ScreenShipboard || state.Fuel != maxFuel-4 || state.TimeMinutes-beforeFlight != 11 {
		t.Fatalf("navigation changed the retained world-hop charge and clock: %+v", state)
	}
	state = armForEncounter(state)
	state, err = reducer.Step(state, ActionOpenTactical)
	if err != nil {
		t.Fatal(err)
	}
	state.EnemyBearing, state.EnemyPitch = aimCentre, aimCentre
	state, err = reducer.Step(state, ActionFire)
	if err != nil {
		t.Fatal(err)
	}
	// A shot empties the weapon; what refills it is the guns, on a later turn.
	if state.WeaponCharge != 0 {
		t.Fatalf("fire did not empty the weapon charge: %+v", state)
	}
	state, err = reducer.Step(state, ActionShieldsOff)
	if err != nil {
		t.Fatal(err)
	}
	if state.ShieldsActive {
		t.Fatal("shields-off did not change tactical state")
	}
}

// TestShieldsAbsorbFirstAndOverflowBreaksSomething follows update_attack: a
// shot the shields can hold stops there, and one they cannot rolls a target for
// every 96 points that get through.
func TestShieldsAbsorbFirstAndOverflowBreaksSomething(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// Shields deep enough to swallow the whole shot: nothing else moves. The
	// enemy reloads between shots and can miss, so the turn a shot arrives on
	// is not fixed; what is being tested is where it lands.
	hullBefore := state.HullCondition
	absorbed := false
	for turn := 0; turn < 60 && !absorbed; turn++ {
		state.ShieldLevel = 1000
		state.ShieldSetting = 1000
		state, err = reducer.Step(state, ActionTacticalMove)
		if err != nil {
			t.Fatal(err)
		}
		if state.MessageKey != "ui.message.enemy_fired_shields" {
			continue
		}
		absorbed = true
		if state.HullCondition != hullBefore {
			t.Fatalf("a shot the shields could hold still reached the ship: %+v", state)
		}
		if state.ShieldLevel >= 1000 {
			t.Fatalf("the shields absorbed nothing: %d", state.ShieldLevel)
		}
	}
	if !absorbed {
		t.Fatalf("no shot was absorbed in sixty turns: %+v", state)
	}

	// With no shields at all, a shot that lands has to reach the ship.
	state.ShieldsActive = false
	landed := false
	for turn := 0; turn < 60 && !landed; turn++ {
		state.ShieldLevel = 0
		state.ShieldSetting = 0
		before := state
		state, err = reducer.Step(state, ActionTacticalMove)
		if err != nil {
			t.Fatal(err)
		}
		if state.MessageKey == "ui.message.enemy_fired_shields" {
			t.Fatalf("an unshielded shot was still absorbed: %+v", state)
		}
		if state.MessageKey != "ui.message.enemy_fired_hull" && state.MessageKey != "ui.message.enemy_fired_system" {
			continue
		}
		landed = true
		if state.HullCondition == before.HullCondition && shipSystemStatusTotal(state) == shipSystemStatusTotal(before) {
			t.Fatalf("an unshielded shot damaged nothing: %+v", state)
		}
	}
	if !landed {
		t.Fatalf("no shot landed in sixty turns: %+v", state)
	}
}

// TestVictoryStopsTheEnemyResponse keeps the remake-owned combat loop honest:
// a destroyed enemy does not get a parting shot.
func TestVictoryStopsTheEnemyResponse(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.EnemyHull = 1
	state.WeaponCharge = 100
	state.EnemyBearing, state.EnemyPitch = aimCentre, aimCentre
	hullBefore := state.HullCondition
	state, err = reducer.Step(state, ActionFire)
	if err != nil {
		t.Fatal(err)
	}
	if !state.CombatVictory || state.CombatActive || state.EnemyHull != 0 || state.HullCondition != hullBefore {
		t.Fatalf("victory still received an enemy response: %+v", state)
	}
}

func shipSystemStatusTotal(state State) int {
	total := 0
	for _, system := range state.ShipSystems {
		total += system.Status
	}
	return total
}

// TestADeadSystemPassesTheShotToTheHull is the rule that keeps a wrecked
// system from soaking damage forever: update_attack sends the shot to the hull
// when the system it rolled is already at zero.
func TestADeadSystemPassesTheShotToTheHull(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// Every system is out, so there is nothing left for a shot to break.
	for index := range state.ShipSystems {
		for row := range state.ShipSystems[index].Rows {
			state.ShipSystems[index].Rows[row][0] = 0x07
		}
		state.ShipSystems[index].Status = 0
	}
	state.ShieldLevel = 0
	state.ShieldSetting = 0
	state.ShieldsActive = false
	// The enemy reloads and can miss, so the shot that reaches the hull is not
	// always the next turn's. What is being tested is where it lands, not when.
	var hullBefore, hullAfter int
	landed := false
	for turn := 0; turn < 60 && !landed; turn++ {
		hullBefore = state.HullCondition
		state, err = reducer.Step(state, ActionTacticalMove)
		if err != nil {
			t.Fatal(err)
		}
		if state.MessageKey == "ui.message.enemy_fired_system" {
			t.Fatalf("a shot broke a part on a ship with nothing left to break: %+v", state)
		}
		if state.MessageKey == "ui.message.enemy_fired_hull" {
			hullAfter = state.HullCondition
			landed = true
		}
	}
	if !landed {
		t.Fatalf("no shot reached the hull in sixty turns: %+v", state)
	}
	// Every 96 points that get through cost the hull one to three, and a shot
	// is clamped to 20..300, so the loss is bounded on both sides.
	minimumHits := (minimumShotDamage + overflowPerPart - 1) / overflowPerPart
	maximumHits := (maximumShotDamage + overflowPerPart - 1) / overflowPerPart
	if loss := hullBefore - hullAfter; loss < minimumHits || loss > maximumHits*hullLossSpan {
		t.Fatalf("hull lost %d, want %d..%d", loss, minimumHits, maximumHits*hullLossSpan)
	}
}

func TestRepairConsumesValidatedShuntAndPartiallyRestoresHull(t *testing.T) {
	data, err := os.ReadFile("../../docs/i18n/apple2-text-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	sourceText, err := apple2source.LoadTextCatalog(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	reducer = reducer.WithAppleIIText(sourceText)
	state := NewState()
	// The hull is on the source's own scale now, so a quarter of it is four
	// points rather than the prototype's twenty-five.
	quarter := MaxHullCondition() / 4
	state.HullCondition = MaxHullCondition() - quarter - 1
	state.Shunts = 1
	for _, action := range []Action{ActionPlay, ActionOpenEngineering, ActionRepair} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	if state.HullCondition != MaxHullCondition()-1 || state.Shunts != 0 || state.MessageKey != "ui.message.repaired" {
		t.Fatalf("shunt repair did not consume one unit or restore a quarter of the hull: %+v", state)
	}
	state, err = reducer.Step(state, ActionRepair)
	if err != nil {
		t.Fatal(err)
	}
	if state.HullCondition != MaxHullCondition()-1 || state.MessageKey != "ui.message.no_shunt" {
		t.Fatalf("empty shunt inventory was not rejected: %+v", state)
	}
}

func TestRepairAtFullHullDoesNotConsumeShunt(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state, err = reducer.Step(state, ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionOpenEngineering)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionRepair)
	if err != nil {
		t.Fatal(err)
	}
	// A new game carries the five shunts set_up_lockers puts in the locker.
	if state.Shunts != newGameShunts() || state.HullCondition != MaxHullCondition() || state.MessageKey != "ui.message.repair_not_needed" {
		t.Fatalf("full-hull repair changed state: %+v", state)
	}
}

func TestRepairConsumesShuntsForWeaponAndShieldSystems(t *testing.T) {
	data, err := os.ReadFile("../../docs/i18n/apple2-text-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	sourceText, err := apple2source.LoadTextCatalog(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	reducer = reducer.WithAppleIIText(sourceText)
	state := NewState()
	state.WeaponCharge = 40
	state.ShieldLevel = 50
	state.Shunts = 2
	for _, action := range []Action{ActionPlay, ActionOpenEngineering} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state, err = reducer.Step(state, ActionRepairWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if state.WeaponCharge != 65 || state.Shunts != 1 || state.MessageKey != "ui.message.weapon_repaired" {
		t.Fatalf("weapon shunt repair did not update the targeted system: %+v", state)
	}
	state, err = reducer.Step(state, ActionRepairShields)
	if err != nil {
		t.Fatal(err)
	}
	if state.ShieldLevel != 75 || state.Shunts != 0 || state.MessageKey != "ui.message.shields_repaired" {
		t.Fatalf("shield shunt repair did not update the targeted system: %+v", state)
	}
	state, err = reducer.Step(state, ActionRepairWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if state.WeaponCharge != 65 || state.Shunts != 0 || state.MessageKey != "ui.message.no_shunt" {
		t.Fatalf("targeted repair consumed or changed state without a shunt: %+v", state)
	}
}

func TestShipSystemSelectionAndRepairUsesSixSystemModel(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	if len(state.ShipSystems) != 6 {
		t.Fatalf("new state has %d ship systems, want 6: %+v", len(state.ShipSystems), state.ShipSystems)
	}
	if selected, ok := state.SelectedShipSystem(); !ok || selected.ID != ShipSystemWarpDrive || selected.Status != MaxShipSystemStatus() {
		t.Fatalf("unexpected initial selected ship system: %+v %t", selected, ok)
	}
	for _, action := range []Action{ActionPlay, ActionOpenEngineering, ActionPreviousSystem} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	selected, ok := state.SelectedShipSystem()
	if !ok || selected.ID != ShipSystemPilotage {
		t.Fatalf("previous-system did not wrap to pilotage: %+v %t", selected, ok)
	}
	// Break one part of system 3, then shunt it: a shunt keeps the row alive
	// but costs it a point, so the system comes back to 15 rather than 16.
	state.SelectedSystemIndex = 3
	state.ShipSystems[3].Rows[0][2] = 0x07
	state.ShipSystems[3].Status = 12
	state.Shunts = 1
	state, err = reducer.Step(state, ActionRepairSystem)
	if err != nil {
		t.Fatal(err)
	}
	if state.SelectedSystemIndex != 3 || state.ShipSystems[3].Status != MaxShipSystemStatus()-1 ||
		state.Shunts != 0 || state.MessageKey != "ui.message.system_repaired" {
		t.Fatalf("shunt repair did not restore the row at a cost: %+v", state.ShipSystems[3])
	}
	if state.ShipSystems[3].Rows[0][2] != 0x17 {
		t.Fatalf("the shunt did not land in the broken slot: %+v", state.ShipSystems[3].Rows)
	}
	state, err = reducer.Step(state, ActionRepairSystem)
	if err != nil {
		t.Fatal(err)
	}
	if state.ShipSystems[3].Status != MaxShipSystemStatus()-1 || state.Shunts != 0 || state.MessageKey != "ui.message.no_shunt" {
		t.Fatalf("selected system repair changed state without a shunt: %+v", state)
	}
}

// TestShuntIsRefusedInTheControlNodeColumn keeps the one placement rule the
// original enforces: every column takes a shunt except the control node's.
func TestShuntIsRefusedInTheControlNodeColumn(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenEngineering} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.SelectedSystemIndex = 0
	for row := range state.ShipSystems[0].Rows {
		state.ShipSystems[0].Rows[row][0] = 0x07
	}
	state.ShipSystems[0].Status = 0
	state.Shunts = 2
	state, err = reducer.Step(state, ActionRepairSystem)
	if err != nil {
		t.Fatal(err)
	}
	if state.Shunts != 2 || state.MessageKey != "ui.message.shunt_does_not_fit" {
		t.Fatalf("a shunt was spent on the control node column: %+v", state)
	}
}

// TestASystemHitMarksTheSlotAndRecordsIt covers the two things update_attack
// writes when a real part is destroyed: the part is marked in place with the
// blasted bit, and the system remembers which slot took it.
func TestASystemHitMarksTheSlotAndRecordsIt(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.ShieldLevel = 0
	state.ShieldSetting = 0
	state.ShieldsActive = false
	// 以原版最近距離、可開火的交戰初態驗受損標記與最後命中槽。
	// 固定既有counter，不選種子；新遭遇等待／遠距離命中另由原生與GUI驗證。
	state.CombatRange = minimumCombatRange
	state.EnemyFireCountdown = 1
	hullBefore := state.HullCondition
	for attempt := 0; attempt < 12; attempt++ {
		state, err = reducer.Step(state, ActionTacticalMove)
		if err != nil {
			t.Fatal(err)
		}
		for _, system := range state.ShipSystems {
			for row := range system.Rows {
				for column, installed := range system.Rows[row] {
					if !isBlastedPart(installed) {
						continue
					}
					// The marker keeps the part's own code underneath.
					if installed&0x7F == 0 {
						t.Fatalf("the blasted marker lost the part it replaced: 0x%02x", installed)
					}
					if system.Status >= MaxShipSystemStatus() {
						t.Fatalf("a blasted part left the system whole: %+v", system)
					}
					wantSlot := row*len(system.Rows[row]) + column
					if system.LastHitSlot != wantSlot {
						// Another slot may have been hit later; only require
						// that the recorded slot is itself blasted.
						recordedRow := system.LastHitSlot / len(system.Rows[row])
						recordedColumn := system.LastHitSlot % len(system.Rows[row])
						if !isBlastedPart(system.Rows[recordedRow][recordedColumn]) {
							t.Fatalf("the recorded slot %d is not blasted: %+v", system.LastHitSlot, system)
						}
					}
					return
				}
			}
		}
		if state.HullCondition <= 0 {
			break
		}
	}
	t.Fatalf("no shot reached a ship system in twelve turns (hull %d → %d)", hullBefore, state.HullCondition)
}

// TestWeaponAndShieldRechargeFollowTheirSystems pins the link update_levels
// makes: the guns supply the weapon and the force shields supply the shields,
// so a damaged system recharges more slowly.
func TestWeaponAndShieldRechargeFollowTheirSystems(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	state = armForEncounter(state)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.WeaponCharge = 0
	state.CombatActive = false
	// 父層先增加相位；由1進2是原版艦砲充能的有效相位。
	state.ShipPhase = 1
	whole, err := reducer.Step(state, ActionRechargeWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if whole.WeaponCharge != MaxShipSystemStatus()*weaponRechargePerStatus {
		t.Fatalf("intact guns recharged %d, want %d", whole.WeaponCharge, MaxShipSystemStatus()*weaponRechargePerStatus)
	}

	// Take the guns down to one working row and the recharge has to follow.
	damaged := state
	gunsIndex := 2
	if damaged.ShipSystems[gunsIndex].ID != ShipSystemShipGuns {
		t.Fatalf("system %d is %q, want the ship's guns", gunsIndex, damaged.ShipSystems[gunsIndex].ID)
	}
	for row := 1; row < len(damaged.ShipSystems[gunsIndex].Rows); row++ {
		damaged.ShipSystems[gunsIndex].Rows[row][1] = 0x07
	}
	damaged.ShipSystems[gunsIndex].Status = shipSystemStatus(damaged.ShipSystems[gunsIndex])
	damaged, err = reducer.Step(damaged, ActionRechargeWeapon)
	if err != nil {
		t.Fatal(err)
	}
	if damaged.WeaponCharge != 4*weaponRechargePerStatus {
		t.Fatalf("shot-up guns recharged %d, want %d", damaged.WeaponCharge, 4*weaponRechargePerStatus)
	}
}

func TestCargoLimitIsKeptSeparateFromTheme(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenCargo, ActionLoadCargo, ActionLoadCargo} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	if CargoUnits(state) != 2 {
		t.Fatalf("cargo did not reach manual prototype capacity: %+v", state)
	}
	state, err = reducer.Step(state, ActionLoadCargo)
	if err != nil {
		t.Fatal(err)
	}
	if CargoUnits(state) != 2 || state.MessageKey != "ui.message.cargo_full" {
		t.Fatalf("cargo overflow was not rejected: %+v", state)
	}
}

func TestManualStatusMissionAndCombatPrototypePath(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	// 此案例測航程／任務，先滿足原版非首城降落所需的掃描器。
	fitGroundScannerForTest(t, &state)
	for _, action := range []Action{
		ActionPlay,
		ActionOpenStatus,
		ActionEat,
		ActionRest,
		ActionWake,
		ActionSleepTick,
		ActionBack,
		ActionOpenGround,
		ActionLoadSupplies,
		ActionLoadCryogens,
	} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
		if action == ActionWake {
			for i := 0; i < 32; i++ {
				state, err = reducer.Step(state, ActionSleepTick)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	state = boardShip(t, reducer, state)
	for _, action := range []Action{
		ActionOpenNavigation,
		ActionNavigate,
		ActionOpenGround,
		ActionDeliverMission,
	} {
		if action == ActionDeliverMission {
			// 任務規則測試的設施前置狀態；正常步行另有整合驗證。
			state.Position = Position{9, 5} // 原始第60張地圖的交易所格。
			state.GroundView = GroundView{}
		}
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	if state.Schema != CurrentSchema || state.Screen != ScreenGround || state.MissionComplete ||
		state.MessageKey != "ui.message.delivery_at_exchange" || CargoUnits(state) != 2 || state.BanvilleCargo != 0 {
		t.Fatalf("retired one-batch mission shortcut still runs: %+v", state)
	}
	// 船艦第一個callback即可醒來，未跨分鐘；飛行11分鐘。
	// 此fixture没有VBL輸入，地面移動不再憑每格增加5分鐘。
	if state.FoodUnits != 0 || state.Health != MaxCondition || state.TimeMinutes != 11 {
		t.Fatalf("status/rest path did not preserve expected prototype state: %+v", state)
	}
	// Drahew and Banville share the Jondd system, so the mission route keeps
	// one local bank instead of the two the retired prototype geography had.
	if state.BankBalances[world.StartSystemID] != state.Bank || state.VisitedDestinations[len(state.VisitedDestinations)-1] != world.MissionDestinationID {
		t.Fatalf("navigation did not record the source-backed local bank and visited destination: %+v", state)
	}

	// 以下是獨立的戰鬥原型 fixture，不用交付捷徑返回船艙。
	state.Screen = ScreenShipboard
	state = armForEncounter(state)
	state, err = reducer.Step(state, ActionOpenTactical)
	if err != nil {
		t.Fatal(err)
	}
	// Firing empties the weapon, so the player alternates shots with a turn
	// that lets the guns refill it. How many rounds that takes depends on the
	// enemy the star system rolled, so the loop runs to a victory rather than
	// to a fixed count.
	// 此測試核對勝利狀態的串接，戰術命中與傷害另有原版切片測試。
	// 明確固定敵方殘餘船殼，避免以任務路徑使用的亂數碰巧決定勝負。
	state.EnemyHull = 1
	for range 200 {
		// The mouse would hold the enemy on the cross-hair; the test aims
		// directly.
		state.EnemyBearing, state.EnemyPitch = aimCentre, aimCentre
		state, err = reducer.Step(state, ActionFire)
		if err != nil {
			t.Fatal(err)
		}
		if state.CombatVictory || state.Screen == ScreenOutcome {
			break
		}
		state, err = reducer.Step(state, ActionTacticalMove)
		if err != nil {
			t.Fatal(err)
		}
		if state.Screen == ScreenOutcome {
			break
		}
	}
	if !state.CombatVictory || state.CombatActive || state.EnemyHull != 0 {
		t.Fatalf("combat did not reach the victory node: %+v", state)
	}
}

func TestSurvivalFailureReachesLocalizedTerminalOutcomeAndCanBeSaved(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Health = 1
	state.Vigor = 1
	state.Nourishment = 0
	// The first background minute crosses the original's hourly gate.
	state.TimeMinutes = 59
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	state = stepToWalkableNeighbour(t, reducer, state)
	for i := 0; i < 32; i++ {
		state, err = reducer.Step(state, ActionVBLTick)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Screen != ScreenOutcome || state.OutcomeKey != "ui.outcome.failure" || state.MessageKey != "ui.message.defeat" {
		t.Fatalf("survival failure did not reach outcome screen: %+v", state)
	}
	state, err = reducer.Step(state, ActionBack)
	if err != nil || state.Screen != ScreenOutcome || state.OutcomeKey != "ui.outcome.failure" || state.MessageKey != "ui.message.failure_terminal" {
		t.Fatalf("failure outcome back escaped terminal state: state=%+v err=%v", state, err)
	}
	state, err = reducer.Step(state, ActionSave)
	if err != nil || state.Screen != ScreenOutcome || !state.Saved {
		t.Fatalf("failure outcome did not save terminal state: state=%+v err=%v", state, err)
	}
	state, err = reducer.Step(state, ActionCyclePresentation)
	if err != nil || state.Screen != ScreenOutcome || state.MessageKey != "ui.message.failure_terminal" {
		t.Fatalf("failure outcome allowed non-terminal action: state=%+v err=%v", state, err)
	}
}

func TestNavigationSelectionAndIndependentLocalBanks(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	// A new game banks the original's opening balance; these tests are about
	// what the bank does, not what it starts with.
	state.Bank = 0
	state.BankBalances = map[string]int{}

	for _, action := range []Action{ActionPlay, ActionOpenNavigation, ActionNextDestination} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// The mission colony is the last record in the source city table, so the
	// next destination wraps to the first one.
	if state.NavigationTargetID != world.StartDestinationID || state.NavigationTargetID == world.MissionDestinationID {
		t.Fatalf("next destination did not change the selected route: %+v", state)
	}
	state, err = reducer.Step(state, ActionPreviousDestination)
	if err != nil {
		t.Fatal(err)
	}
	if state.NavigationTargetID != world.MissionDestinationID {
		t.Fatalf("previous destination did not return to mission route: %+v", state)
	}
	// Drahew and Banville share the Jondd system, so an independent local bank
	// only appears once the route leaves that system.
	state, foreignSystemID := selectForeignDestination(t, reducer, state)
	foreignDestinationID := state.NavigationTargetID
	state, err = reducer.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	// The target is in another system, so the trip has an in-system leg to fly;
	// the assertions below are about the arrival, not the departure.
	state, err = reducer.Step(state, ActionFlightEngage)
	if err != nil {
		t.Fatal(err)
	}
	if state.SystemID != foreignSystemID || state.Bank != 0 || state.BankBalances[world.StartSystemID] != 0 || !contains(state.VisitedDestinations, foreignDestinationID) {
		t.Fatalf("navigation did not load an independent local bank: %+v", state)
	}

	state, err = reducer.Step(state, ActionOpenGround)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionOpenUniteller)
	if err != nil {
		t.Fatal(err)
	}
	purse := state.Cash
	state, err = reducer.Step(state, ActionDepositCash)
	if err != nil {
		t.Fatal(err)
	}
	// The whole purse lands in the bank of the system the player is standing
	// in; the home system's balance must not move.
	if state.BankBalances[foreignSystemID] != purse || state.BankBalances[world.StartSystemID] != 0 || state.Cash != 0 {
		t.Fatalf("deposit did not update only the current local bank: %+v", state)
	}
}

func TestTransferMovesCurrentBankToSelectedForeignSystem(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	// A new game banks the original's opening balance; these tests are about
	// what the bank does, not what it starts with.
	state.Bank = 0
	state.BankBalances = map[string]int{}

	for _, action := range []Action{ActionPlay, ActionOpenNavigation} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state, foreignSystemID := selectForeignDestination(t, reducer, state)
	for _, action := range []Action{ActionBack, ActionOpenGround, ActionOpenUniteller, ActionDepositCash, ActionTransferBank} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	if state.Cash != 0 || state.Bank != 0 || state.BankBalances[world.StartSystemID] != 0 || state.BankBalances[foreignSystemID] != NewState().Cash {
		t.Fatalf("transfer did not move the current local bank: %+v", state)
	}
	if state.MessageKey != "ui.message.transferred" {
		t.Fatalf("transfer did not report success: %q", state.MessageKey)
	}

	state, err = reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	state = boardShip(t, reducer, state)
	state, err = reducer.Step(state, ActionOpenNavigation)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	// The target is in another system, so the trip has an in-system leg to fly;
	// the assertions below are about the arrival, not the departure.
	state, err = reducer.Step(state, ActionFlightEngage)
	if err != nil {
		t.Fatal(err)
	}
	if state.Bank != NewState().Cash || state.SystemID != foreignSystemID {
		t.Fatalf("destination bank was not loaded after navigation: %+v", state)
	}
	state, err = reducer.Step(state, ActionOpenGround)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionOpenUniteller)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionTransferBank)
	if err != nil {
		t.Fatal(err)
	}
	if state.Bank != NewState().Cash || state.MessageKey != "ui.message.transfer_same_system" {
		t.Fatalf("same-system transfer should be rejected without changing balance: %+v", state)
	}
}

func TestGroundMovementBlocksObstaclesWithoutAdvancingTime(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}

	// A grid command moves the player without inventing elapsed VBL inputs.
	moved := stepToWalkableNeighbour(t, reducer, state)
	if moved.Position == state.Position || moved.TimeMinutes != state.TimeMinutes {
		t.Fatalf("walking did not move or invented elapsed time: before=%+v after=%+v", state, moved)
	}

	// A blocked neighbour changes nothing at all — not the position, not the
	// clock. Which direction that is depends on the city, so it is looked up in
	// the city's own map rather than written down here.
	blocked, direction, found := blockedNeighbour(moved)
	if !found {
		t.Skip("this city has no blocked cell next to the player")
	}
	after, err := reducer.Step(moved, direction)
	if err != nil {
		t.Fatal(err)
	}
	if after.Position != moved.Position || after.TimeMinutes != moved.TimeMinutes ||
		after.MessageKey != "ui.message.cannot_move" {
		t.Fatalf("walking into the blocked cell (%d,%d) changed state: before=%+v after=%+v",
			blocked.X, blocked.Y, moved, after)
	}
}

// blockedNeighbour finds a cell next to the player that the collision contract
// refuses, and the action that walks into it.
func blockedNeighbour(state State) (world.TilePosition, Action, bool) {
	candidates := []struct {
		dx, dy int
		action Action
	}{
		{dx: 1, action: ActionMoveEast},
		{dx: -1, action: ActionMoveWest},
		{dy: -1, action: ActionMoveNorth},
		{dy: 1, action: ActionMoveSouth},
	}
	for _, candidate := range candidates {
		position := world.TilePosition{X: state.Position.X + candidate.dx, Y: state.Position.Y + candidate.dy}
		terrain, ok := world.TerrainAt(state.CityID, position.X, position.Y)
		if ok && !world.IsPassable(terrain) {
			return position, candidate.action, true
		}
	}
	return world.TilePosition{}, "", false
}

// tradeTestFunds is enough bank for any quote. A quote costs its price word
// times a hundred (the original's scale), and a new game starts with 200-899 in
// cash and nothing banked — so the original expects the player to withdraw
// first, and tests that are about trading rather than about affording it say so
// by banking this much up front.
const tradeTestFunds = 100000

func gameStateAtTrade(t *testing.T, reducer Reducer) State {
	t.Helper()
	state := NewState()
	state.Bank = tradeTestFunds
	var err error
	for _, action := range []Action{ActionPlay, ActionOpenGround, ActionOpenTrade} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatal(err)
		}
	}
	return state
}

// selectForeignDestination presses the normal next-system action until the
// selected route leaves the current star system. The state must already be on
// the navigation screen. It returns the updated state and the selected system.
func selectForeignDestination(t *testing.T, reducer Reducer, state State) (State, string) {
	t.Helper()
	if state.Screen != ScreenNavigation {
		t.Fatalf("selectForeignDestination needs the navigation screen, got %q", state.Screen)
	}
	var err error
	for attempt := 0; attempt < 64; attempt++ {
		destination, ok := reducer.World.Destination(state.NavigationTargetID)
		if ok && destination.SystemID != state.SystemID {
			return state, destination.SystemID
		}
		state, err = reducer.Step(state, ActionNextTargetSystem)
		if err != nil {
			t.Fatalf("step %s: %v", ActionNextTargetSystem, err)
		}
	}
	t.Fatalf("no destination outside system %q was reachable by normal selection", state.SystemID)
	return state, ""
}

// TestDialogVerticalSliceUsesSourceTextOnBothSides walks the whole chain the
// dialog feature exists for: the player stands at a facility, opens a
// conversation, picks one of the source table's own player verbs, and gets a
// line back. Both halves are original text rendered in Traditional Chinese.
func TestDialogVerticalSliceUsesSourceTextOnBothSides(t *testing.T) {
	if !DialogAvailable() {
		t.Skip("dialog corpus is unavailable")
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// The game lands on the planet's landing square; the haggling block is
	// the exchange building's, so walk there first.
	if cityMap, ok := world.CityMap(state.CityID); ok {
		for _, building := range cityMap.Buildings() {
			if building.Facility == "exchange" {
				state.Position = Position{X: building.Row, Y: building.Column}
			}
		}
	}
	state, err = reducer.Step(state, ActionTalk)
	if err != nil {
		t.Fatalf("talk: %v", err)
	}
	if state.Screen != ScreenDialog {
		t.Fatalf("talk did not open a conversation: %+v", state)
	}
	// The exchange tile opens the haggling block.
	if state.DialogBlock != 0x20 {
		t.Fatalf("dialog block = 0x%02x, want 0x20", state.DialogBlock)
	}
	line, err := reducer.DialogLine(state)
	if err != nil || line == "" {
		t.Fatalf("dialog line = %q, %v", line, err)
	}
	if !hasCJK(line) {
		t.Fatalf("dialog line is not Traditional Chinese: %q", line)
	}

	verbs := reducer.PlayerVerbs()
	if len(verbs) < 2 {
		t.Fatalf("player verbs = %v", verbs)
	}
	state, err = reducer.Step(state, ActionNextVerb)
	if err != nil {
		t.Fatal(err)
	}
	if state.DialogVerb != 1 {
		t.Fatalf("verb index = %d, want 1", state.DialogVerb)
	}
	before := state.DialogLine
	state, err = reducer.Step(state, ActionSayVerb)
	if err != nil {
		t.Fatal(err)
	}
	if state.DialogSaid != verbs[1] {
		t.Fatalf("player said %q, want %q", state.DialogSaid, verbs[1])
	}
	if state.DialogLine == before {
		t.Fatalf("the other party did not answer: %+v", state)
	}
	reply, err := reducer.DialogLine(state)
	if err != nil || reply == line {
		t.Fatalf("reply = %q (previous %q), err %v", reply, line, err)
	}

	state, err = reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenGround || state.DialogSaid != "" {
		t.Fatalf("leaving the conversation did not return to the ground: %+v", state)
	}
}

func TestTalkNeedsSomebodyToTalkTo(t *testing.T) {
	if !DialogAvailable() {
		t.Skip("dialog corpus is unavailable")
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	// The landing cell carries the whole facility cluster, so move off it.
	state = stepOffTheFacilityCluster(t, reducer, state)
	state, err = reducer.Step(state, ActionTalk)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenGround || state.MessageKey != "ui.message.nobody_here" {
		t.Fatalf("talking to nobody did not report it: %+v", state)
	}
}

func hasCJK(value string) bool {
	for _, r := range value {
		if r >= 0x3400 && r <= 0x9fff {
			return true
		}
	}
	return false
}

// TestPlaceReferenceNamesSomewhereElse pins the behaviour read out of the
// game's own p-code: XDOINTER:0x0f get_place picks a random star system or
// planet and retries while the pick is where the player already is, so the
// name it yields is never the player's current location.
func TestPlaceReferenceNamesSomewhereElse(t *testing.T) {
	if !DialogAvailable() {
		t.Skip("dialog corpus is unavailable")
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	seenSystems, seenPlanets := 0, 0
	for variant := 0; variant < 24; variant++ {
		state.DialogVariant = variant
		name, ok := reducer.otherPlaceName(state)
		if !ok || name == "" {
			t.Fatalf("variant %d produced no place", variant)
		}
		current, _ := reducer.World.Destination(state.CityID)
		if name == current.SystemName && variant%2 == 0 {
			t.Fatalf("variant %d named the player's own system %q", variant, name)
		}
		if name == current.PlanetName && variant%2 == 1 {
			t.Fatalf("variant %d named the player's own planet %q", variant, name)
		}
		if variant%2 == 0 {
			seenSystems++
		} else {
			seenPlanets++
		}
	}
	if seenSystems == 0 || seenPlanets == 0 {
		t.Fatalf("the coin toss never reached both lists: %d systems, %d planets", seenSystems, seenPlanets)
	}
}

// TestCharacterNameKeepsInteriorSpaces covers what typing a two-word name
// actually looks like: one rune at a time. Trimming the draft on every
// keystroke eats the space the moment it arrives, so the next letter lands
// against the previous word and the space can never survive.
func TestCharacterNameKeepsInteriorSpaces(t *testing.T) {
	state := NewState()
	state.Screen = ScreenCharacterCreation
	state.CharacterDraft = ""
	for _, r := range "Mary Jo" {
		state = AppendCharacterNameDraft(state, []rune{r})
	}
	if state.CharacterDraft != "Mary Jo" {
		t.Fatalf("draft = %q, want %q", state.CharacterDraft, "Mary Jo")
	}
}

// A leading space is still refused, and a trailing one is an edit rather than
// part of the name: it goes away when the name is confirmed, not while typing.
func TestCharacterNameTrimsTheEdgesOnlyAtConfirmation(t *testing.T) {
	state := NewState()
	state.Screen = ScreenCharacterCreation
	state.CharacterDraft = ""
	state = AppendCharacterNameDraft(state, []rune{' ', ' '})
	if state.CharacterDraft != "" {
		t.Fatalf("a draft opened with a space: %q", state.CharacterDraft)
	}
	for _, r := range "Mary Jo " {
		state = AppendCharacterNameDraft(state, []rune{r})
	}
	if state.CharacterDraft != "Mary Jo " {
		t.Fatalf("draft = %q, want the trailing space kept while typing", state.CharacterDraft)
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	next, err := reducer.Step(state, ActionConfirmCharacterName)
	if err != nil {
		t.Fatal(err)
	}
	if next.Character != "Mary Jo" {
		t.Fatalf("confirmed name = %q, want %q", next.Character, "Mary Jo")
	}
}

// A draft of nothing but spaces is not a name.
func TestCharacterNameOfSpacesIsRejected(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenCharacterCreation
	state.CharacterDraft = "   "
	next, err := reducer.Step(state, ActionConfirmCharacterName)
	if err != nil {
		t.Fatal(err)
	}
	if next.Screen != ScreenCharacterCreation || next.Character != state.Character {
		t.Fatalf("a name of spaces was accepted: screen=%q character=%q", next.Screen, next.Character)
	}
	if next.MessageKey != "ui.message.character_name_invalid" {
		t.Fatalf("message = %q, want the invalid-name message", next.MessageKey)
	}
}

// Buying at the exchange is one of the paths the manual lists as complete, and
// it was dead: the guard asked the Apple II source catalog to name a cargo key
// that belongs to the Atari ST table, so every purchase came back as an error
// and the shell printed "this command is not valid on this screen".
//
// This walks the path a player walks — land, open the exchange, buy, sell — and
// checks the hold actually changes. A test that only asserted "the action
// returns no error" would not have caught it either, because the action did
// return an error; nothing was walking this path.
func TestBuyingAndSellingAtTheExchangeMovesCargo(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenShipboard
	for _, action := range []Action{ActionOpenGround, ActionSkipLanding, ActionOpenTrade} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
	}
	if state.Screen != ScreenTrade {
		t.Fatalf("the exchange did not open; screen is %q", state.Screen)
	}
	state.Cash = 100000
	resetTradeTestFunds(&state)
	bought, err := reducer.Step(state, ActionBuyGems)
	if err != nil {
		t.Fatalf("buying was refused: %v", err)
	}
	if WarehouseLotsOf(bought, bought.CityID)[0].Goods == 0 || CargoUnits(bought) != CargoUnits(state) {
		t.Fatal("步行買貨應進倉庫，pod保持")
	}
	if TradeFunds(bought) >= TradeFunds(state) {
		t.Fatal("the purchase cost nothing")
	}
	sold, err := reducer.Step(bought, ActionTradeSellPage)
	if err != nil {
		t.Fatal(err)
	}
	sold, err = reducer.Step(sold, ActionTradePick1)
	if err != nil {
		t.Fatal(err)
	}
	sold, err = reducer.Step(sold, ActionTradePick1)
	if err != nil {
		t.Fatal(err)
	}
	if WarehouseLotsOf(sold, sold.CityID)[0].Goods != 0 || CargoUnits(sold) != CargoUnits(bought) {
		t.Fatal("倉庫售出未清槽或改動pod")
	}
}

// TestTheLaserSpendsTheChargeHitOrMiss pins fire_it's own bookkeeping: the
// charge is cleared on every laser shot, and the enemy answers either way.
func TestTheLaserSpendsTheChargeHitOrMiss(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	base := NewState()
	base = armForEncounter(base)
	for _, action := range []Action{ActionPlay, ActionOpenTactical} {
		base, err = reducer.Step(base, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	base.CombatActive = true
	base.EnemyType = 1
	base.EnemyHull = 500
	base.EnemyMaxHull = 500
	// A huge reload counter keeps the enemy's answer from overwriting the
	// message under test; the answer itself is not what this test pins.
	base.EnemyFireCountdown = 1 << 20
	sawHit, sawMiss := false, false
	for seed := 0; seed < 200 && !(sawHit && sawMiss); seed++ {
		state := base
		state.WeaponCharge = 100
		state.CombatRolls = seed * 7
		// Half the trials are aimed, half are a full turn off.
		state.EnemyBearing, state.EnemyPitch = aimCentre, aimCentre
		if seed%2 == 1 {
			state.EnemyBearing = 0
		}
		state, err = reducer.Step(state, ActionFireLaser)
		if err != nil {
			t.Fatal(err)
		}
		if state.WeaponCharge != 0 {
			t.Fatalf("laser left charge %d behind", state.WeaponCharge)
		}
		// The enemy's answer overwrites the step message (the cannon lives
		// with the same order), so hit and miss are told apart by the hull.
		if state.EnemyHull < 500 {
			sawHit = true
		} else {
			sawMiss = true
		}
	}
	if !sawHit || !sawMiss {
		t.Fatalf("laser outcomes not both seen (hit=%v miss=%v)", sawHit, sawMiss)
	}
}

// TestTheExchangeOpensAQuoteBeforeItSells pins do_exchange's shape: Buy puts
// up the quote page, the quote's Buy is the purchase, Pass and Back close
// the page; Sell lists the pod's lots first.
func TestTheExchangeOpensAQuoteBeforeItSells(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround, ActionOpenTrade} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	cargoBefore := CargoUnits(state)
	state, err = reducer.Step(state, ActionTradeBuyPage)
	if err != nil || state.TradePage != TradePageBuy || CargoUnits(state) != cargoBefore {
		t.Fatalf("the quote page did not open: %v page=%d", err, state.TradePage)
	}
	first, _ := CurrentQuote(state)
	state, err = reducer.Step(state, ActionTradePass)
	second, _ := CurrentQuote(state)
	if err != nil || state.TradePage != TradePageBuy || second == first {
		t.Fatalf("Pass did not move the ring on: %v page=%d %+v -> %+v", err, state.TradePage, first, second)
	}
	state.Cash = Credits(second.Price) + 10
	resetTradeTestFunds(&state)
	state, err = reducer.Step(state, ActionBuyGems)
	if err != nil || state.TradePage != TradePageMain || CargoUnits(state) != cargoBefore || WarehouseLotsOf(state, state.CityID)[0].Goods != second.Goods {
		t.Fatalf("the quote's Buy did not buy and close: %v page=%d cargo %d", err, state.TradePage, CargoUnits(state))
	}
	state, err = reducer.Step(state, ActionTradeSellPage)
	if err != nil || state.TradePage != TradePageSellWarehouse {
		t.Fatalf("the lot list did not open: %v page=%d", err, state.TradePage)
	}
	state, err = reducer.Step(state, ActionBack)
	if err != nil || state.Screen != ScreenTrade || state.TradePage != TradePageMain {
		t.Fatalf("Back did not close the lot list: %v %s page=%d", err, state.Screen, state.TradePage)
	}
}

// TestTheTellerKeypadComposesAnAmount pins move_cash: the keys add up, ALL
// takes what is available, the sum is capped there, and leaving the keypad
// (Back, the right button) is what moves the money.
func TestTheTellerKeypadComposesAnAmount(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, action := range []Action{ActionPlay, ActionOpenGround} {
		state, err = reducer.Step(state, action)
		if err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	state.Screen = ScreenUniteller
	state.Cash, state.Bank = 1500, 0
	state, err = reducer.Step(state, ActionTellerDepositPage)
	if err != nil || state.TellerPage != TellerPageDeposit {
		t.Fatalf("deposit page: %v page=%d", err, state.TellerPage)
	}
	for _, key := range []Action{ActionTellerAmount1000, ActionTellerAmount100, ActionTellerAmount100} {
		state, _ = reducer.Step(state, key)
	}
	if state.TellerAmount != 1200 {
		t.Fatalf("keypad sum %d, want 1200", state.TellerAmount)
	}
	state, _ = reducer.Step(state, ActionTellerAmount1000)
	if state.TellerAmount != 1500 {
		t.Fatalf("keypad sum %d not capped at the cash 1500", state.TellerAmount)
	}
	state, _ = reducer.Step(state, ActionTellerAmountClear)
	state, _ = reducer.Step(state, ActionTellerAmount100)
	state, err = reducer.Step(state, ActionBack)
	if err != nil || state.Screen != ScreenUniteller || state.TellerPage != TellerPageMain || state.Cash != 1400 || state.Bank != 100 {
		t.Fatalf("leaving the keypad did not deposit 100: %v %s page=%d cash=%d bank=%d", err, state.Screen, state.TellerPage, state.Cash, state.Bank)
	}
	state, _ = reducer.Step(state, ActionTellerWithdrawPage)
	state, _ = reducer.Step(state, ActionTellerAmountAll)
	state, _ = reducer.Step(state, ActionTellerConfirm)
	if state.Cash != 1500 || state.Bank != 0 {
		t.Fatalf("ALL then confirm did not withdraw everything: cash=%d bank=%d", state.Cash, state.Bank)
	}
}

// Page 5 of the manual: after saving, the title screen says so. The flag every
// other action clears has to survive the one step that takes the player there.
func TestTheTitleScreenReportsASavedGame(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenShipboard
	saved, err := reducer.Step(state, ActionSave)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Saved {
		t.Fatal("saving did not set the flag")
	}
	title, err := reducer.Step(saved, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if title.Screen != ScreenTitle {
		t.Fatalf("screen = %q, want the title", title.Screen)
	}
	if title.MessageKey != "ui.message.saved" {
		t.Fatalf("message = %q, want the saved line", title.MessageKey)
	}
}

func TestAnUnsavedGameSaysNothingOnTheTitleScreen(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenShipboard
	title, err := reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if title.MessageKey == "ui.message.saved" {
		t.Fatal("an unsaved game claimed it was saved")
	}
}

// The save library is reachable from inside a game: the remake keeps eight
// slots where the original's disk kept one game in one place, so storing is a
// choice of slot and has to be possible without ending the game first. Back
// returns to the screen it was opened from.
func TestTheLibraryOpensFromAGameAndComesBackToIt(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state, err = reducer.Step(state, ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := reducer.Step(state, ActionOpenLibrary)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Screen != ScreenCharacterLibrary || opened.LibraryReturn != ScreenShipboard {
		t.Fatalf("the library did not open over the game: %s / %s", opened.Screen, opened.LibraryReturn)
	}
	// Storing works from there, which is the point of the entry.
	stored, err := reducer.Step(opened, ActionStoreGame)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Screen != ScreenCharacterLibrary {
		t.Fatalf("storing left the library: %s", stored.Screen)
	}
	back, err := reducer.Step(stored, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if back.Screen != ScreenShipboard || back.LibraryReturn != "" {
		t.Fatalf("back did not return to the game: %s / %s", back.Screen, back.LibraryReturn)
	}
}

// Opened from the title through Select Game, Back still goes to the title.
func TestTheLibraryFromTheTitleStillBacksToTheTitle(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state, err := reducer.Step(NewState(), ActionSelectGame)
	if err != nil {
		t.Fatal(err)
	}
	if state.LibraryReturn != "" {
		t.Fatalf("select game left a return screen: %s", state.LibraryReturn)
	}
	back, err := reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if back.Screen != ScreenTitle {
		t.Fatalf("back from the title's library went to %s", back.Screen)
	}
}

// The menus have no game to store, so the in-game entry is refused there.
func TestTheLibraryEntryIsRefusedOnTheMenus(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, screen := range []Screen{ScreenTitle, ScreenCharacterLibrary, ScreenCharacterCreation, ScreenAttributeAllocation} {
		state := NewState()
		state.Screen = screen
		if _, err := reducer.Step(state, ActionOpenLibrary); err == nil {
			t.Fatalf("the library opened over %s", screen)
		}
	}
}
