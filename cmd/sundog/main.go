package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wicanr2/sundog-remake/internal/appdir"
	"github.com/wicanr2/sundog-remake/internal/catalog"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	apple2source "github.com/wicanr2/sundog-remake/internal/source/apple2"
	"github.com/wicanr2/sundog-remake/internal/storage"
)

func main() {
	catalogPath := flag.String("catalog", "docs/i18n/apple2-text-catalog.json", "Apple II 來源文字 catalog")
	uiCatalogPath := flag.String("ui-catalog", "docs/i18n/sundog-ui-zh-Hant.json", "重製版繁中 UI overlay")
	savePath := flag.String("save", fmt.Sprintf("workplace/save/sundog-prototype-v%d-slot-1.json", game.CurrentSchema), "可寫入的重製版存檔槽")
	loadSave := flag.Bool("load", false, "啟動前載入可寫入的重製版存檔槽")
	libraryPath := flag.String("library", fmt.Sprintf("workplace/save/sundog-remake-v%d-library.json", game.CurrentSchema), "可寫入的重製版保存庫替代檔")
	demo := flag.Bool("demo", false, "執行 deterministic 垂直切片重播")
	atariAtlasManifest := flag.String("atari-atlas-manifest", "", "選用的 Atari ST normalized PNG atlas manifest")
	flag.Parse()

	// A packaged build starts from whatever directory the launcher happened to
	// use, so the read-only assets are looked for next to the executable as well,
	// and the writable files move under the user's own directory when the working
	// directory cannot take them.
	*catalogPath = appdir.Asset(*catalogPath)
	*uiCatalogPath = appdir.Asset(*uiCatalogPath)
	// 只有玩家沒指定路徑時才改寫。明確給了 -save／-library 的呼叫端
	// （replay 腳本、測試、錄影流程）拿到的永遠是它自己給的那個路徑。
	given := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if !given["save"] {
		*savePath = appdir.UserData("sundog-prototype-slot-1.json", *savePath)
	}
	if !given["library"] {
		*libraryPath = appdir.UserData("sundog-remake-library.json", *libraryPath)
	}

	labels, sourceText, err := loadCatalogs(*catalogPath, *uiCatalogPath)
	if err != nil {
		fatal(err)
	}
	reducer, err := game.NewReducer()
	if err != nil {
		fatal(err)
	}
	if *atariAtlasManifest != "" {
		registry, err := reducer.Themes.WithAtariSTAtlas(*atariAtlasManifest)
		if err != nil {
			fatal(fmt.Errorf("load Atari ST atlas: %w", err))
		}
		reducer.Themes = registry
	}
	reducer = reducer.WithAppleIIText(sourceText)
	reducer = reducer.WithLocalizer(func(key, fallback string) string {
		return localization.Lookup(labels, key, fallback)
	})
	state := game.NewState()
	if *loadSave {
		state, err = storage.Load(*savePath)
		if err != nil {
			fatal(err)
		}
		state.MessageKey = "ui.message.loaded"
	}
	if *demo {
		state = runDemo(state, reducer, labels, *savePath)
		return
	}
	if err := runInteractive(state, reducer, labels, sourceText, *savePath, *libraryPath); err != nil {
		fatal(err)
	}
}

func loadCatalogs(sourcePath, uiPath string) (catalog.Catalog, apple2source.TextCatalog, error) {
	load := func(path string) (catalog.Catalog, []byte, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return catalog.Catalog{}, nil, fmt.Errorf("open catalog %s: %w", path, err)
		}
		loaded, err := catalog.Load(bytes.NewReader(data))
		if err != nil {
			return catalog.Catalog{}, nil, err
		}
		return loaded, data, nil
	}
	base, sourceData, err := load(sourcePath)
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, err
	}
	sourceText, err := apple2source.LoadTextCatalog(bytes.NewReader(sourceData))
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, fmt.Errorf("validate Apple II source catalog: %w", err)
	}
	ui, _, err := load(uiPath)
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, err
	}
	merged, err := base.Merge(ui)
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, err
	}
	return merged, sourceText, nil
}

func runDemo(state game.State, reducer game.Reducer, labels catalog.Catalog, savePath string) game.State {
	sequence := []game.Action{
		game.ActionSelectGame,
		game.ActionNewGame,
		game.ActionConfirmCharacterName,
		game.ActionOpenStatus,
		game.ActionEat,
		game.ActionRest,
		game.ActionBack,
		game.ActionOpenGround,
		game.ActionMoveEast,
		game.ActionOpenTrade,
		game.ActionBuyGems,
		game.ActionSellGems,
		game.ActionBack,
		game.ActionMoveWest,
		game.ActionOpenUniteller,
		game.ActionDepositCash,
		game.ActionTransferBank,
		game.ActionBack,
		game.ActionBack,
		game.ActionOpenGround,
		game.ActionLoadSupplies,
		game.ActionLoadCryogens,
		game.ActionBack,
		game.ActionOpenNavigation,
		game.ActionNextDestination,
		game.ActionPreviousDestination,
		game.ActionNextTargetPlanet,
		game.ActionPreviousTargetPlanet,
		game.ActionNextTargetSystem,
		game.ActionPreviousTargetSystem,
		game.ActionNavigate,
		game.ActionOpenGround,
		game.ActionRefuel,
		game.ActionDeliverMission,
		game.ActionBack,
		game.ActionOpenGround,
		game.ActionOpenUniteller,
		game.ActionWithdrawCash,
		game.ActionBack,
		game.ActionBack,
		game.ActionOpenTactical,
		game.ActionFire,
		game.ActionRechargeWeapon,
		game.ActionShieldsOff,
		game.ActionFullShields,
		game.ActionBack,
		game.ActionOpenEngineering,
		game.ActionOpenEquipment,
		game.ActionBack,
		game.ActionRepair,
		game.ActionBack,
		game.ActionOpenCargo,
		game.ActionLoadCargo,
		game.ActionUnloadCargo,
		game.ActionBack,
		game.ActionCyclePresentation,
		game.ActionSave,
	}
	for _, action := range sequence {
		if action == game.ActionConfirmCharacterName {
			state = game.SetCharacterNameDraft(state, "Zed")
		}
		var err error
		state, err = reducer.Step(state, action)
		if err != nil {
			fatal(err)
		}
		if action == game.ActionSave {
			if err := storage.Save(savePath, state); err != nil {
				fatal(err)
			}
		}
		printState(os.Stdout, state, labels, reducer)
	}
	return state
}

func runInteractive(state game.State, reducer game.Reducer, labels catalog.Catalog, sourceText apple2source.TextCatalog, savePath, libraryPath string) error {
	librarySlot := "slot-1"
	printInteractiveState(os.Stdout, state, labels, reducer, sourceText, libraryPath, librarySlot)
	scanner := bufio.NewScanner(os.Stdin)
	for !state.QuitRequested && scanner.Scan() {
		rawCommand := strings.TrimSpace(scanner.Text())
		if state.Screen == game.ScreenCharacterCreation {
			lowered := strings.ToLower(rawCommand)
			switch {
			case strings.HasPrefix(lowered, "name "):
				state = game.SetCharacterNameDraft(state, strings.TrimSpace(rawCommand[5:]))
				printState(os.Stdout, state, labels, reducer)
				continue
			case lowered == "confirm" || lowered == "enter" || lowered == "play":
				// Let commandAction map the confirmation below.
			case lowered == "b" || lowered == "back":
				// Let commandAction map the return action below.
			case rawCommand != "":
				state = game.SetCharacterNameDraft(state, rawCommand)
				printState(os.Stdout, state, labels, reducer)
				continue
			}
		}
		command := strings.ToLower(rawCommand)
		if command == "" {
			continue
		}
		if state.Screen == game.ScreenCharacterLibrary {
			if strings.HasPrefix(command, "slot ") {
				librarySlot = normalizeLibrarySlot(command[5:])
				state.MessageKey = "ui.message.library_slot_selected"
				printInteractiveState(os.Stdout, state, labels, reducer, sourceText, libraryPath, librarySlot)
				continue
			}
		}
		if command == "l" || command == "load" {
			loaded, libraryErr := storage.LoadGame(libraryPath, librarySlot)
			if libraryErr != nil {
				// Keep the original single writable slot as a migration-friendly
				// fallback for users who have not created the remake library yet.
				var err error
				loaded, err = storage.Load(savePath)
				if err != nil {
					if errors.Is(libraryErr, storage.ErrLibraryNotFound) {
						state.MessageKey = "ui.message.library_missing"
						printInteractiveState(os.Stdout, state, labels, reducer, sourceText, libraryPath, librarySlot)
						continue
					}
					fmt.Fprintf(os.Stdout, "%s %v\n", lookup(labels, "ui.message.load_rejected"), err)
					continue
				}
			}
			state = loaded
			state.MessageKey = "ui.message.loaded"
			printInteractiveState(os.Stdout, state, labels, reducer, sourceText, libraryPath, librarySlot)
			continue
		}
		action, ok := commandAction(command, state)
		if !ok {
			fmt.Fprintln(os.Stdout, lookup(labels, "ui.message.unknown_command"))
			continue
		}
		switch action {
		case game.ActionMakeLibrary:
			if err := storage.MakeLibrary(libraryPath); err != nil {
				fmt.Fprintf(os.Stdout, "%s %v\n", lookup(labels, "ui.message.library_rejected"), err)
				continue
			}
		case game.ActionStoreGame:
			if err := storage.StoreGame(libraryPath, librarySlot, state); err != nil {
				fmt.Fprintf(os.Stdout, "%s %v\n", lookup(labels, "ui.message.library_rejected"), err)
				continue
			}
		case game.ActionDeleteGame:
			if err := storage.DeleteGame(libraryPath, librarySlot); err != nil {
				fmt.Fprintf(os.Stdout, "%s %v\n", lookup(labels, "ui.message.library_rejected"), err)
				continue
			}
		}
		next, err := reducer.Step(state, action)
		if err != nil {
			fmt.Fprintf(os.Stdout, "%s %v\n", lookup(labels, "ui.message.action_rejected"), err)
			continue
		}
		state = next
		if action == game.ActionSave {
			if err := storage.Save(savePath, state); err != nil {
				return err
			}
		}
		printInteractiveState(os.Stdout, state, labels, reducer, sourceText, libraryPath, librarySlot)
	}
	return scanner.Err()
}

func printInteractiveState(out *os.File, state game.State, labels catalog.Catalog, reducer game.Reducer, sourceText apple2source.TextCatalog, libraryPath, librarySlot string) {
	printState(out, state, labels, reducer)
	if state.Screen == game.ScreenEquipment {
		printEquipmentCatalog(out, labels, sourceText)
	}
	if state.Screen != game.ScreenCharacterLibrary {
		return
	}
	fmt.Fprintf(out, "%s: %s\n", lookup(labels, "ui.library_slot"), localization.LibrarySlotLabel(librarySlot))
	summaries, err := storage.ListLibrary(libraryPath)
	if err != nil {
		if errors.Is(err, storage.ErrLibraryNotFound) {
			fmt.Fprintln(out, lookup(labels, "ui.message.library_missing"))
			return
		}
		fmt.Fprintf(out, "%s %v\n", lookup(labels, "ui.message.library_list_rejected"), err)
		return
	}
	if len(summaries) == 0 {
		fmt.Fprintln(out, lookup(labels, "ui.library_slot_empty"))
		return
	}
	for _, summary := range summaries {
		fmt.Fprintf(out, "[%s] %s\n", localization.LibrarySlotLabel(summary.ID), summary.Character)
	}
}

func printEquipmentCatalog(out *os.File, labels catalog.Catalog, sourceText apple2source.TextCatalog) {
	keys := sourceText.KeysWithPrefix("apple2.equipment.")
	fmt.Fprintln(out, lookup(labels, "ui.equipment_note"))
	for index, key := range keys {
		label := lookup(labels, key)
		if translated, ok := sourceText.Localized(key); ok {
			label = translated
		}
		fmt.Fprintf(out, "%02d. %s", index+1, label)
		if index%2 == 1 || index == len(keys)-1 {
			fmt.Fprintln(out)
		} else {
			fmt.Fprint(out, "\t")
		}
	}
}

func normalizeLibrarySlot(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "slot-1"
	}
	allDigits := true
	for _, value := range raw {
		if value < '0' || value > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return "slot-" + raw
	}
	return raw
}

func commandAction(command string, state game.State) (game.Action, bool) {
	screen := state.Screen
	if screen == game.ScreenOutcome && state.OutcomeKey == "ui.outcome.failure" {
		switch command {
		case "q", "quit":
			return game.ActionQuit, true
		case "s", "save":
			return game.ActionSave, true
		}
		return "", false
	}
	if command == "q" || command == "quit" {
		return game.ActionQuit, true
	}
	if command == "f8" || command == "look" || command == "theme" {
		return game.ActionCyclePresentation, true
	}
	// The library is reachable from a game in progress, so a slot can be
	// written without ending the game; the reducer refuses it on the menus.
	if command == "library" || command == "f5" {
		return game.ActionOpenLibrary, true
	}
	switch screen {
	case game.ScreenTitle:
		switch command {
		case "p", "play":
			return game.ActionPlay, true
		case "c", "select", "select-game":
			return game.ActionSelectGame, true
		case "store", "store-game":
			return game.ActionStoreGame, true
		case "q", "quit":
			return game.ActionQuit, true
		}
	case game.ScreenCharacterLibrary:
		switch command {
		case "n", "new":
			return game.ActionNewGame, true
		case "m", "make", "make-library":
			return game.ActionMakeLibrary, true
		case "s", "store", "store-game":
			return game.ActionStoreGame, true
		case "d", "delete", "delete-game":
			return game.ActionDeleteGame, true
		case "l", "load":
			return game.ActionLoadGame, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenCharacterCreation:
		switch command {
		case "confirm", "enter", "play", "p":
			return game.ActionConfirmCharacterName, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenCharacterStatus:
		switch command {
		case "h", "hand":
			return game.ActionHand, true
		case "left", "previous":
			return game.ActionPreviousPocket, true
		case "right", "next":
			return game.ActionNextPocket, true
		case "rest", "r":
			return game.ActionRest, true
		case "eat", "e":
			return game.ActionEat, true
		case "back", "b":
			return game.ActionBack, true
		}
	case game.ScreenShipboard:
		switch command {
		case "stores":
			return game.ActionOpenStores, true
		case "g", "ground":
			return game.ActionOpenGround, true
		case "n", "navigation", "nav":
			return game.ActionOpenNavigation, true
		case "x", "tactical":
			return game.ActionOpenTactical, true
		case "e", "engineering", "engine":
			return game.ActionOpenEngineering, true
		case "c", "cargo":
			return game.ActionOpenCargo, true
		case "h", "status", "character":
			return game.ActionOpenStatus, true
		case "d", "distress":
			return game.ActionDistressCall, true
		case "s", "save":
			return game.ActionSave, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenStores:
		switch command {
		case "left", "previous":
			return game.ActionPreviousStore, true
		case "right", "next":
			return game.ActionNextStore, true
		case "enter", "take":
			return game.ActionTakeStore, true
		case "h", "hand":
			return game.ActionHand, true
		case "e", "eat":
			return game.ActionEat, true
		case "cancel":
			return game.ActionCancelItem, true
		case "back":
			return game.ActionBack, true
		}
	case game.ScreenInterior:
		switch command {
		case "w", "north":
			return game.ActionInteriorNorth, true
		case "a", "west":
			return game.ActionInteriorWest, true
		case "s", "south":
			return game.ActionInteriorSouth, true
		case "d", "east":
			return game.ActionInteriorEast, true
		case "c", "counter":
			return game.ActionUseCounter, true
		case "sit", "booth":
			return game.ActionSitAtBooth, true
		case "g", "slots":
			return game.ActionOpenSlots, true
		case "e", "leave":
			return game.ActionLeaveBuilding, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenSlots:
		switch command {
		case "spin", "p":
			return game.ActionSpinSlots, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenArena:
		switch command {
		case "f", "fight", "attack":
			return game.ActionArenaAttack, true
		case "x", "flee":
			return game.ActionArenaFlee, true
		}
	case game.ScreenScore:
		switch command {
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenGround:
		switch command {
		case "e", "enter":
			return game.ActionEnterBuilding, true
		case "w", "north":
			return game.ActionMoveNorth, true
		case "a", "west":
			return game.ActionMoveWest, true
		case "s", "south":
			return game.ActionMoveSouth, true
		case "d", "east":
			return game.ActionMoveEast, true
		case "t", "trade":
			return game.ActionOpenTrade, true
		case "u", "uniteller", "bank":
			return game.ActionOpenUniteller, true
		case "tube", "station":
			return game.ActionOpenTubeStation, true
		case "talk", "c":
			return game.ActionTalk, true
		case "r", "refuel", "fuel":
			return game.ActionRefuel, true
		case "p", "supplies":
			return game.ActionLoadSupplies, true
		case "k", "cryogens":
			return game.ActionLoadCryogens, true
		case "deliver", "colony":
			return game.ActionDeliverMission, true
		case "parts", "y":
			return game.ActionOpenParts, true
		case "h", "status", "character":
			return game.ActionOpenStatus, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenParts:
		switch command {
		case "buy", "b":
			return game.ActionBuyPart, true
		case "next", "right":
			return game.ActionNextPart, true
		case "prev", "previous", "left":
			return game.ActionPreviousPart, true
		case "back":
			return game.ActionBack, true
		}
	case game.ScreenTrade:
		switch command {
		case "buy", "b":
			return game.ActionBuyGems, true
		case "sell":
			return game.ActionSellGems, true
		case "back":
			return game.ActionBack, true
		}
	case game.ScreenFlight:
		switch command {
		case "t", "tick", "step":
			return game.ActionFlightTick, true
		case "e", "engage", "go":
			return game.ActionFlightEngage, true
		}
	case game.ScreenNavigation:
		switch command {
		case "go", "navigate", "confirm":
			return game.ActionNavigate, true
		case "next", "right":
			return game.ActionNextDestination, true
		case "prev", "previous", "left":
			return game.ActionPreviousDestination, true
		case "pn", "planet-next":
			return game.ActionNextTargetPlanet, true
		case "pp", "planet-prev":
			return game.ActionPreviousTargetPlanet, true
		case "sn", "system-next":
			return game.ActionNextTargetSystem, true
		case "sp", "system-prev":
			return game.ActionPreviousTargetSystem, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenDialog:
		switch command {
		case "next", "right":
			return game.ActionNextVerb, true
		case "prev", "previous", "left":
			return game.ActionPreviousVerb, true
		case "say", "go", "confirm":
			return game.ActionSayVerb, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenUniteller:
		switch command {
		case "deposit", "d":
			return game.ActionDepositCash, true
		case "withdraw", "w":
			return game.ActionWithdrawCash, true
		case "transfer", "t":
			return game.ActionTransferBank, true
		case "fine", "f":
			return game.ActionPayFine, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenTubeStation:
		// do_tube_station 的框：1–6 各一座城市。
		switch command {
		case "1":
			return game.ActionTubePick1, true
		case "2":
			return game.ActionTubePick2, true
		case "3":
			return game.ActionTubePick3, true
		case "4":
			return game.ActionTubePick4, true
		case "5":
			return game.ActionTubePick5, true
		case "6":
			return game.ActionTubePick6, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenTactical:
		switch command {
		case "move", "m":
			return game.ActionTacticalMove, true
		case "fire", "f":
			return game.ActionFire, true
		case "tractor", "t":
			return game.ActionTractor, true
		case "jettison", "j1":
			return game.ActionJettisonHold1, true
		case "jettison-2", "j2":
			return game.ActionJettisonHold2, true
		case "jettison-all", "ja":
			return game.ActionJettisonAll, true
		case "hail", "comm":
			return game.ActionHail, true
		case "charge", "recharge":
			return game.ActionRechargeWeapon, true
		case "shields-off", "off":
			return game.ActionShieldsOff, true
		case "shields-on", "full-shields", "on":
			return game.ActionFullShields, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenEngineering:
		switch command {
		case "prev-system", "previous-system", "system-prev", "left":
			return game.ActionPreviousSystem, true
		case "next-system", "system-next", "right":
			return game.ActionNextSystem, true
		case "repair-system", "system-repair", "selected-system", "e":
			return game.ActionRepairSystem, true
		case "i", "equipment", "devices":
			return game.ActionOpenEquipment, true
		case "p", "install", "install-part":
			return game.ActionInstallPart, true
		case "repair-weapon", "weapon", "rw", "w":
			return game.ActionRepairWeapon, true
		case "repair-shields", "shields", "rs", "s":
			return game.ActionRepairShields, true
		case "repair", "repair-hull", "hull", "rh", "r":
			return game.ActionRepair, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenEquipment:
		switch command {
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenCargo:
		switch command {
		case "load", "l":
			return game.ActionLoadCargo, true
		case "unload", "u":
			return game.ActionUnloadCargo, true
		case "b", "back":
			return game.ActionBack, true
		}
	case game.ScreenOutcome:
		switch command {
		case "b", "back", "enter", "continue":
			return game.ActionBack, true
		}
	}
	return "", false
}

func printState(out *os.File, state game.State, labels catalog.Catalog, reducer game.Reducer) {
	group, _ := reducer.GroupOf(state)
	fmt.Fprintf(out, "\n=== %s ===\n", lookup(labels, "ui.title"))
	location := displayLocation(labels, state.CityID, state.Location)
	target := localization.Destination(labels, state.NavigationTargetID, state.NavigationTarget)
	system := localization.System(labels, state.SystemID, state.SystemID)
	planet := localization.Planet(labels, state.PlanetID, state.PlanetID)
	city := localization.City(labels, state.CityID, state.CityID)
	fmt.Fprintf(out, "%s: %s | %s: %s | %s: %d | %s: %d | %s: %d\n",
		lookup(labels, "ui.location"), location,
		lookup(labels, "ui.theme"), lookup(labels, group.DisplayNameKey),
		lookup(labels, "ui.cash"), state.Cash,
		lookup(labels, "ui.bank"), state.Bank,
		reducer.CargoName(state), state.Gems)
	fmt.Fprintf(out, "%s: %d | %s: %d | %s: %d/%d | %s: %d/%d | %s: %d | %s: %d/2\n",
		lookup(labels, "ui.fuel"), state.Fuel,
		lookup(labels, "ui.weapon_charge"), state.WeaponCharge,
		lookup(labels, "ui.shield_level"), state.ShieldLevel, state.ShieldSetting,
		lookup(labels, "ui.hull_condition"), state.HullCondition, game.MaxHullCondition(),
		lookup(labels, "ui.shunts"), state.Shunts,
		lookup(labels, "ui.cargo_units"), game.CargoUnits(state))
	if state.Screen == game.ScreenDialog {
		if line, err := reducer.DialogLine(state); err == nil {
			fmt.Fprintf(out, "%s：%s\n", lookup(labels, "ui.dialog"), line)
		}
		if state.DialogSaid != "" {
			fmt.Fprintf(out, "%s：%s\n", lookup(labels, "ui.dialog_said"), state.DialogSaid)
		}
		if verbs := reducer.PlayerVerbs(); state.DialogVerb >= 0 && state.DialogVerb < len(verbs) {
			fmt.Fprintf(out, "%s：%s\n", lookup(labels, "ui.dialog_verb"), verbs[state.DialogVerb])
		}
	}
	if state.Screen == game.ScreenNavigation {
		if cost, ok := reducer.RouteCost(state); ok {
			line := fmt.Sprintf("%s: %s %d | %s: %d%s",
				lookup(labels, "ui.route_cost"), lookup(labels, "ui.fuel"), cost.FuelCost,
				lookup(labels, "ui.time"), cost.TotalMinutes, lookup(labels, "ui.minutes"))
			if cost.JumpDistance > 0 {
				line += fmt.Sprintf(" | %s: %d/%d", lookup(labels, "ui.route_jump"), cost.JumpDistance, state.JumpRange)
			}
			fmt.Fprintln(out, line)
		}
	}
	if state.Screen == game.ScreenParts {
		printPartsCounter(out, labels, state, reducer)
	}
	if selected, ok := state.SelectedShipSystem(); ok {
		fmt.Fprintf(out, "%s: %s %d/%d\n", lookup(labels, "ui.repair_system"),
			lookup(labels, game.ShipSystemLabelKey(selected.ID)), selected.Status, game.MaxShipSystemStatus())
		if state.Screen == game.ScreenEngineering {
			printShipSystemGrid(out, labels, selected)
		}
	}
	fmt.Fprintf(out, "%s: %d | %s: %d | %s: %d | %s: %d | %s: %d\n",
		lookup(labels, "ui.health"), state.Health,
		lookup(labels, "ui.vigor"), state.Vigor,
		lookup(labels, "ui.nourishment"), state.Nourishment,
		lookup(labels, foodLabelKey(state)), game.CarriedFoodCount(state),
		lookup(labels, "ui.supplies"), game.SuppliesAboard(state))
	fmt.Fprintf(out, "%s: %d | %s: %d%% | %s: %t | %s: %d\n",
		lookup(labels, "ui.cryogens"), game.CryogensAboard(state),
		lookup(labels, "ui.enemy_hull"), game.EnemyHullPercent(state),
		lookup(labels, "ui.mission_complete"), state.MissionComplete,
		lookup(labels, "ui.time"), state.TimeMinutes)
	if state.Screen == game.ScreenFlight {
		angle, radius := game.FlightPosition(state)
		fmt.Fprintf(out, "%s: %d%% | %s: %d/%d | %s: %d,%d\n",
			lookup(labels, "ui.flight_progress"), game.FlightProgress(state),
			lookup(labels, "ui.flight_tick"), state.FlightTick, state.FlightTicks,
			lookup(labels, "ui.flight"), angle, radius)
	}
	fmt.Fprintf(out, "%s: %s\n", lookup(labels, "ui.system"), system)
	fmt.Fprintf(out, "%s: %s | %s: %s | %s: %s\n",
		lookup(labels, "ui.planet"), planet,
		lookup(labels, "ui.city"), city,
		lookup(labels, "ui.navigation_target"), target)
	fmt.Fprintf(out, "%s: %s\n", lookup(labels, "ui.character"), state.Character)
	if state.Screen == game.ScreenCharacterCreation {
		fmt.Fprintf(out, "%s: %s\n", lookup(labels, "ui.character_name"), state.CharacterDraft)
	}
	fmt.Fprintf(out, "%s：(%d，%d) | %s：%d 分鐘\n", lookup(labels, "ui.position"), state.Position.X, state.Position.Y, lookup(labels, "ui.time"), state.TimeMinutes)
	fmt.Fprintf(out, "%s\n", lookup(labels, state.MessageKey))
	if state.OutcomeKey != "" {
		fmt.Fprintf(out, "%s: %s\n", lookup(labels, "ui.outcome"), lookup(labels, state.OutcomeKey))
	}
	switch state.Screen {
	case game.ScreenTitle:
		fmt.Fprintln(out, "[p]", lookup(labels, "ui.play_sundog"), "[c]", lookup(labels, "ui.select_game"), "[l]", lookup(labels, "ui.load_game"), "[q]", lookup(labels, "ui.quit"))
	case game.ScreenCharacterLibrary:
		fmt.Fprintln(out, "[n]", lookup(labels, "ui.new_game"), "[m]", lookup(labels, "ui.make_library"), "[s]", lookup(labels, "ui.store_game"), "[l]", lookup(labels, "ui.load_game"), "[d]", lookup(labels, "ui.delete_game"), "[槽位 N]", lookup(labels, "ui.library_slot"), "[b]", lookup(labels, "ui.back"))
	case game.ScreenCharacterCreation:
		fmt.Fprintln(out, "[name <文字>]", lookup(labels, "ui.character_name"), "[confirm]", lookup(labels, "ui.character_name_confirm"), "[b]", lookup(labels, "ui.back"))
	case game.ScreenCharacterStatus:
		fmt.Fprintln(out, "[r]", lookup(labels, "ui.rest"), "[e]", lookup(labels, "ui.eat"), "[b]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenShipboard:
		fmt.Fprintln(out, "[g]", lookup(labels, "ui.ground"), "[n]", lookup(labels, "ui.navigation"), "[x]", lookup(labels, "ui.tactical"), "[e]", lookup(labels, "ui.engineering"), "[c]", lookup(labels, "ui.cargo_hold"), "[h]", lookup(labels, "ui.character_status"), "[d]", lookup(labels, "ui.action.distress"), "[s]", lookup(labels, "ui.save_game"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenGround:
		fmt.Fprintln(out, "[w/a/s/d]", lookup(labels, "ui.move"), "[t]", lookup(labels, "ui.trade_exchange"), "[y]", lookup(labels, "ui.open_parts"), "[u]", lookup(labels, "ui.uniteller"), "[talk]", lookup(labels, "ui.talk"), "[p]", lookup(labels, "ui.load_supplies"), "[k]", lookup(labels, "ui.load_cryogens"), "[deliver]", lookup(labels, "ui.deliver_mission"), "[h]", lookup(labels, "ui.character_status"), "[b]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenInterior:
		fmt.Fprintln(out, "[w/a/s/d]", lookup(labels, "ui.move"), "[c]", lookup(labels, "ui.interior.counter"), "[sit]", lookup(labels, "ui.interior.booth"), "[g]", lookup(labels, "ui.interior.slots"), "[e]", lookup(labels, "ui.interior.door"), "[b]", lookup(labels, "ui.back"))
	case game.ScreenSlots:
		fmt.Fprintln(out, "[spin]", lookup(labels, "ui.slots.spin"), "[b]", lookup(labels, "ui.back"))
	case game.ScreenArena:
		fmt.Fprintln(out, "[f]", lookup(labels, "ui.arena.weapon"), "[x]", lookup(labels, "ui.message.arena_fled"))
	case game.ScreenScore:
		fmt.Fprintln(out, "[b]", lookup(labels, "ui.back"))
	case game.ScreenDialog:
		fmt.Fprintln(out, "[prev/next]", lookup(labels, "ui.verb_previous_next"), "[say]", lookup(labels, "ui.dialog_say"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenTrade:
		fmt.Fprintln(out, "[buy]", lookup(labels, "ui.buy"), "[sell]", lookup(labels, "ui.sell"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenParts:
		fmt.Fprintln(out, "[prev/next]", lookup(labels, "ui.parts_select"), "[buy]", lookup(labels, "ui.buy_part"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenNavigation:
		fmt.Fprintln(out, "[sp/sn]", lookup(labels, "ui.target_system_previous_next"), "[pp/pn]", lookup(labels, "ui.target_planet_previous_next"), "[prev/next]", lookup(labels, "ui.target_city_previous_next"), "[go]", lookup(labels, "ui.navigate"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenUniteller:
		fmt.Fprintln(out, "[deposit]", lookup(labels, "ui.deposit"), "[withdraw]", lookup(labels, "ui.withdraw"), "[transfer]", lookup(labels, "ui.transfer"), "[f]", lookup(labels, "ui.action.pay_fine"), "[u]", lookup(labels, "ui.action.tube"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenTactical:
		fmt.Fprintln(out, "[move]", lookup(labels, "ui.tactical_move"), "[fire]", lookup(labels, "ui.fire"), "[t]", lookup(labels, "ui.action.tractor"), "[j1/j2/ja]", lookup(labels, "ui.action.jettison"), "[hail]", lookup(labels, "ui.action.hail"), "[charge]", lookup(labels, "ui.recharge"), "[off]", lookup(labels, "ui.shields_off"), "[on]", lookup(labels, "ui.full_shields"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenEngineering:
		fmt.Fprintln(out, "[left/right]", lookup(labels, "ui.system_previous_next"), "[p]", lookup(labels, "ui.install_part"), "[e]", lookup(labels, "ui.repair_system"), "[w]", lookup(labels, "ui.repair_weapon"), "[s]", lookup(labels, "ui.repair_shields"), "[r]", lookup(labels, "ui.repair_hull"), "[i]", lookup(labels, "ui.equipment"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenEquipment:
		fmt.Fprintln(out, "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenCargo:
		fmt.Fprintln(out, "[load]", lookup(labels, "ui.load_cargo"), "[unload]", lookup(labels, "ui.unload_cargo"), "[back]", lookup(labels, "ui.back"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
	case game.ScreenOutcome:
		if state.OutcomeKey == "ui.outcome.failure" {
			fmt.Fprintln(out, "[save]", lookup(labels, "ui.outcome_failure_terminal"), "[q]", lookup(labels, "ui.quit"))
		} else {
			fmt.Fprintln(out, "[back]", lookup(labels, "ui.outcome_return"), "[f8]", lookup(labels, "ui.theme_cycle"), "[f9]", lookup(labels, "ui.tileset_cycle"))
		}
	}
}

func displayLocation(labels catalog.Catalog, id, fallback string) string {
	if value := localization.Destination(labels, id, ""); value != "" {
		return value
	}
	return fallback
}

// printPartsCounter shows the part on offer, what it costs and how many are
// already aboard.
func printPartsCounter(out *os.File, labels catalog.Catalog, state game.State, reducer game.Reducer) {
	part, ok := state.SelectedPart()
	if !ok {
		return
	}
	name, _ := game.ItemName(part)
	price, _ := reducer.ShopItemPrice(state, part)
	fmt.Fprintf(out, "%s: %s | %s: %d | %s: %d\n",
		lookup(labels, "ui.parts_counter"),
		localization.Lookup(labels, "ui.item."+itemSlug(name), name),
		lookup(labels, "ui.parts_price"), price,
		lookup(labels, "ui.parts_carried"), game.CarriedPartCount(state, part))
	stores := make([]string, 0, len(state.Stores))
	for _, item := range state.Stores {
		if item == 0 {
			continue
		}
		stored, ok := game.ItemName(item)
		if !ok {
			continue
		}
		stores = append(stores, localization.Lookup(labels, "ui.item."+itemSlug(stored), stored))
	}
	if len(stores) > 0 {
		fmt.Fprintf(out, "%s: %s\n", lookup(labels, "ui.ship_stores"), strings.Join(stores, "、"))
	}
}

// printShipSystemGrid shows what the selected system is actually built from.
// The four rows are the source's own redundancy: each holds the same four
// parts, and the system keeps working while any of them is intact.
func printShipSystemGrid(out *os.File, labels catalog.Catalog, system game.ShipSystem) {
	for row := range system.Rows {
		parts := make([]string, 0, len(system.Rows[row]))
		for column := range system.Rows[row] {
			name, ok := game.ShipSystemPartName(system, row, column)
			if !ok {
				name = "?"
			}
			parts = append(parts, localization.Lookup(labels, "ui.item."+itemSlug(name), name))
		}
		fmt.Fprintf(out, "  %d) %s [%d]\n", row+1, strings.Join(parts, " · "), game.ShipSystemRowStatus(system, row))
	}
}

// itemSlug turns a source part name into its catalog key suffix, the same way
// the catalog itself was built.
func itemSlug(name string) string {
	var builder strings.Builder
	dash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			dash = false
		default:
			if !dash {
				builder.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func lookup(labels catalog.Catalog, key string) string {
	if value, ok := labels.Lookup(key); ok {
		return value
	}
	return "[missing:" + key + "]"
}

func foodLabelKey(state game.State) string {
	if state.FoodItemKey != "" {
		return state.FoodItemKey
	}
	return "apple2.ship.food"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
