package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/catalog"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

func mouseTestStates() map[string]game.State {
	states := map[string]game.State{}
	for _, screen := range []game.Screen{game.ScreenTitle, game.ScreenCharacterLibrary, game.ScreenCharacterCreation, game.ScreenAttributeAllocation, game.ScreenCharacterStatus, game.ScreenShipboard, game.ScreenStores, game.ScreenGround, game.ScreenDepot, game.ScreenFuelPurchase, game.ScreenFTLBoost, game.ScreenInterior, game.ScreenSlots, game.ScreenArena, game.ScreenParts, game.ScreenTrade, game.ScreenNavigation, game.ScreenFlight, game.ScreenDialog, game.ScreenTubeStation, game.ScreenUniteller, game.ScreenTactical, game.ScreenEngineering, game.ScreenCargo, game.ScreenEquipment, game.ScreenOutcome, game.ScreenScore} {
		s := game.NewState()
		s.Screen = screen
		s.Layout = theme.AtariSTLayout
		s.CityID = "banville"
		states[string(screen)] = s
	}
	for page := game.TradePageMain; page <= game.TradePageSwapWarehouse; page++ {
		s := states["trade"]
		s.TradePage = page
		states["trade:"+fmt.Sprint(page)] = s
	}
	for page := game.TellerPageMain; page <= game.TellerPageTransferPick; page++ {
		s := states["uniteller"]
		s.TellerPage = page
		states["teller:"+fmt.Sprint(page)] = s
	}
	s := states["tube-station"]
	s.TubePage = game.TubePageConfirm
	states["tube:confirm"] = s
	return states
}

func TestMouseControlsExposeKeyboardActionsAndGeometry(t *testing.T) {
	states := mouseTestStates()
	model := map[string][]mouseControl{}
	for name, state := range states {
		a := app{state: state, mouseCard: mouseControlCard{open: true}}
		choices := a.mouseChoices()
		seen := map[game.Action]bool{}
		for _, b := range choices {
			seen[b.action] = true
		}
		for _, key := range polledKeys {
			if action, ok := keyAction(key, state); ok && action != game.ActionQuit && !seen[action] {
				t.Fatalf("%s missing %s", name, action)
			}
		}
		for _, preset := range theme.Presentations() {
			a.state = game.WithPresentation(state, preset)
			for _, size := range [][2]int{{1280, 800}, {1600, 1000}, {1400, 900}} {
				a.setViewport(size[0], size[1])
				for page := 0; page*mousePageSize < len(choices); page++ {
					a.mouseCard.page = page
					buttons := a.mouseButtons()
					for i, b := range buttons {
						if b.x < 0 || b.y < 0 || b.x+b.w > 320 || b.y+b.h > 200 {
							t.Fatalf("outside viewport: %+v", b)
						}
						x, y := (b.x+b.w/2)*a.viewScale()+a.originX, (b.y+b.h/2)*a.viewScale()+a.originY
						got, ok := a.mouseControlAt(x, y)
						if !ok || got.id != b.id || got.action != b.action {
							t.Fatalf("%s hit mismatch %+v %+v", name, b, got)
						}
						for _, other := range buttons[:i] {
							if b.x < other.x+other.w && b.x+b.w > other.x && b.y < other.y+other.h && b.y+b.h > other.y {
								t.Fatalf("overlap %+v %+v", b, other)
							}
						}
					}
				}
			}
		}
		a.mouseCard.page = 0
		model[name] = a.mouseChoices()
	}
	if path := os.Getenv("SUNDOG_MOUSE_MODEL"); path != "" {
		// Test metadata only; the game has no injected state or diagnostic port.
		out := map[string][]map[string]string{}
		for name, choices := range model {
			for _, b := range choices {
				out[name] = append(out[name], map[string]string{"id": b.id, "action": string(b.action)})
			}
		}
		raw, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMouseControlsFiveLocalesHaveEveryLabel(t *testing.T) {
	for _, locale := range []string{"zh-Hant", "zh-Hans", "ja", "ko", "en"} {
		f, err := os.Open(filepath.Join("../../docs/i18n", "sundog-ui-"+locale+".json"))
		if err != nil {
			t.Fatal(err)
		}
		labels, err := catalog.Load(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		for name, state := range mouseTestStates() {
			a := app{state: state, labels: labels, mouseCard: mouseControlCard{open: true}}
			for _, system := range []bool{false, true} {
				a.mouseCard.system = system
				for _, b := range a.mouseChoices() {
					if strings.HasPrefix(b.label, "[") || b.label == "" {
						t.Fatalf("%s %s missing label %+v", locale, name, b)
					}
				}
			}
		}
	}
}

func TestMouseControlsDelegateRulesAndPreserveSaveDomain(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	a := app{reducer: r, state: game.NewState(), mouseCard: mouseControlCard{open: true}}
	before := a.state
	a.activateMouseControl(mouseControl{id: "system"})
	a.activateMouseControl(mouseControl{id: "context"})
	a.activateMouseControl(mouseControl{id: "close"})
	if !reflect.DeepEqual(before, a.state) {
		t.Fatal("opening/closing the card changes game state")
	}
	for _, action := range []game.Action{game.ActionSelectGame, game.ActionNewGame} {
		a.activateMouseControl(mouseControl{id: "action", action: action})
	}
	a.activateMouseControl(mouseControl{id: "name"})
	a.activateMouseControl(mouseControl{id: "char:z"})
	a.activateMouseControl(mouseControl{id: "char:e"})
	a.activateMouseControl(mouseControl{id: "char:d"})
	a.activateMouseControl(mouseControl{id: "space"})
	a.activateMouseControl(mouseControl{id: "char:q"})
	a.activateMouseControl(mouseControl{id: "erase"})
	a.activateMouseControl(mouseControl{id: "name-confirm"})
	if a.state.Character != "zed" || a.state.Screen != game.ScreenAttributeAllocation {
		t.Fatal("mouse character creation", a.state.Character, a.state.Screen)
	}
	before = a.state
	a.activateMouseControl(mouseControl{id: "action", action: game.ActionIncreaseAttribute})
	want, err := r.Step(before, game.ActionIncreaseAttribute)
	if err != nil || !reflect.DeepEqual(want, a.state) {
		t.Fatal("mouse and reducer attribute results differ")
	}
}

func TestMouseControlsRespectTerminalSleepAndDiskGates(t *testing.T) {
	a := app{state: game.NewState(), mouseCard: mouseControlCard{open: true}}
	a.state.Screen = game.ScreenOutcome
	a.state.OutcomeKey = "ui.outcome.failure"
	for _, system := range []bool{false, true} {
		a.mouseCard.system = system
		for _, b := range a.mouseChoices() {
			if b.action != "" && b.action != game.ActionSave && b.action != game.ActionQuit {
				t.Fatal("terminal bypass", b)
			}
		}
	}
	a.state.Screen = game.ScreenCharacterStatus
	a.state.OutcomeKey = ""
	a.state.Sleep.Active = true
	a.mouseCard.system = false
	if got := a.mouseContextActions(); !reflect.DeepEqual(got, []game.Action{game.ActionWake}) {
		t.Fatal(got)
	}
	a.state = game.NewState()
	a.showOptions = true
	a.options.diskOnly = true
	for _, system := range []bool{false, true} {
		a.mouseCard.system = system
		for _, b := range a.mouseChoices() {
			if b.action != "" && b.action != game.ActionQuit {
				t.Fatal("disk gate bypass", b)
			}
		}
	}
}

func TestMouseDiskBrowserUnicodeAndInvalidInput(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "原版資料"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"陽犬號.ST", "wrong.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("invalid disk"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := app{state: game.NewState(), mouseCard: mouseControlCard{open: true, mode: "disk"}, showOptions: true, options: optionsCard{diskOnly: true}}
	a.readMouseDirectory(dir)
	if len(a.mouseCard.files) != 2 || !a.mouseCard.files[0].directory || a.mouseCard.files[1].name != "陽犬號.ST" {
		t.Fatal(a.mouseCard.files)
	}
	a.activateMouseControl(mouseControl{id: "file:1"})
	if a.originalDisk != nil || !a.showOptions || !a.mouseCard.open || a.mouseCard.errorKey != "ui.options.disk_failed" {
		t.Fatal("invalid disk bypasses bootstrap")
	}
	a.readMouseDirectory(filepath.Join(dir, "missing"))
	if a.mouseCard.errorKey != "ui.mouse.directory_error" {
		t.Fatal("directory error missing")
	}
}

func TestMouseSpecialActionsProducePlayerVisibleChanges(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	a := app{reducer: r, state: game.NewState(), mouseCard: mouseControlCard{open: true}}
	clickAction := func(action game.Action) {
		t.Helper()
		for _, b := range a.mouseChoices() {
			if b.action == action {
				a.activateMouseControl(b)
				return
			}
		}
		t.Fatalf("no mouse button for %s", action)
	}
	// Valid, bounded room fixture. This is a UI/rule test, not a native route.
	a.state.Screen = game.ScreenInterior
	a.state.Interior = game.Interior{Active: true, Width: 5, Height: 5, Position: game.Position{X: 2, Y: 3}, Rolls: 7}
	clickAction(game.ActionInteriorNorth)
	if a.state.Interior.Position != (game.Position{X: 2, Y: 2}) {
		t.Fatal("room move failed", a.state.Interior.Position)
	}
	// Slots use their own controlled random counter; do not select a lucky roll.
	a.state.Screen = game.ScreenSlots
	a.state.Slots = game.SlotMachine{Bet: game.SlotBet(1, 1), Rolls: 7}
	a.state.Cash = 10000
	cash, bet := a.state.Cash, a.state.Slots.Bet
	clickAction(game.ActionSpinSlots)
	if !a.state.Slots.Spun || a.state.Slots.Rolls <= 7 || a.state.Cash != cash-bet+a.state.Slots.Payout {
		t.Fatal("slot spin not dispatched")
	}
	a.state = game.NewState()
	a.state.Screen = game.ScreenParts
	index := a.state.SelectedPartIndex
	clickAction(game.ActionNextPart)
	if a.state.SelectedPartIndex == index {
		t.Fatal("part selector failed")
	}
	part, ok := a.state.SelectedPart()
	if !ok {
		t.Fatal("selected part unavailable")
	}
	price, ok := r.ShopItemPrice(a.state, part)
	if !ok {
		t.Fatal("source item price unavailable")
	}
	funds := a.state.Bank + a.state.Cash
	count := game.CarriedPartCount(a.state, part)
	clickAction(game.ActionBuyPart)
	if a.state.MessageKey != "ui.message.part_bought" || a.state.Bank+a.state.Cash != funds-price || game.CarriedPartCount(a.state, part) != count+1 {
		t.Fatal("part purchase not dispatched")
	}
}

func TestMouseHoldDoesNotRepeatTransactions(t *testing.T) {
	for _, action := range []game.Action{game.ActionBuyPart, game.ActionBuyGems, game.ActionDeleteGame, game.ActionNewGame, game.ActionQuit, game.ActionFire, game.ActionSave, game.ActionSpinSlots} {
		if mouseRepeatable(action) {
			t.Fatal("transaction repeats while held", action)
		}
	}
	for _, action := range []game.Action{game.ActionIncreaseAttribute, game.ActionDecreaseAttribute, game.ActionNextPart, game.ActionPreviousDestination, game.ActionInteriorNorth} {
		if !mouseRepeatable(action) {
			t.Fatal("selector cannot be held", action)
		}
	}
}

func TestMouseArenaTriggerDoesNotBecomeBack(t *testing.T) {
	for _, preset := range theme.Presentations() {
		for name, state := range mouseTestStates() {
			for _, active := range []bool{false, true} {
				a := app{state: game.WithPresentation(state, preset)}
				a.state.Arena.Active = active
				want := state.Screen == game.ScreenArena && preset.Layout == theme.AtariSTLayout && active
				if a.arenaOwnsRightButton() != want {
					t.Fatalf("%s %s active%v arena input owner", preset.ID, name, active)
				}
			}
		}
	}
}
