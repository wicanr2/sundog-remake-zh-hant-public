package main

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"

	"github.com/wicanr2/sundog-remake/internal/catalog"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/storage"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// subcTarget is the middle of the Sub-C Engines row as it is actually drawn:
// right column, fourth row of the shipboard window. The window is sized around
// its own labels, so the point has to be worked out from the same code that
// draws it — written out as numbers it went stale the moment the window moved.
func subcTarget(a *app) (int, int) {
	window, columnWidth := a.shipboardWindow()
	return window.columnX(subcStationColumn, columnWidth) + columnWidth/2,
		window.rowY(subcStationRow, shipboardRowStep) + shipboardRowStep/2
}

func statusApp(t *testing.T) *app {
	t.Helper()
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	labels, face := auditCatalogAndFace(t)
	state := game.NewState()
	state.Screen = game.ScreenCharacterStatus
	return &app{reducer: reducer, state: state, labels: labels, face: face}
}

func shipboardApp(t *testing.T) *app {
	t.Helper()
	labels, face := auditCatalogAndFace(t)
	state := game.NewState()
	state.Screen = game.ScreenShipboard
	return &app{state: state, labels: labels, face: face}
}

func TestGroundDerivedDirectEntriesAreSteadyFrames(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{"ground", "trade", "uniteller", "tube-station", "dialog", "mission", "outcome"} {
		t.Run(start, func(t *testing.T) {
			state, err := startState(game.NewState(), reducer, start)
			if err != nil {
				t.Fatal(err)
			}
			if state.Landing.Phase != game.LandingIdle {
				t.Fatalf("-start %s landing phase = %q, want steady frame", start, state.Landing.Phase)
			}
		})
	}
}

func TestDefaultTitlePreservesNewGameResources(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	initial := game.NewState()
	state, err := startState(initial, reducer, "title")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initial, state) {
		t.Fatal("正式標題入口改寫了新局狀態")
	}
	state, err = reducer.Step(state, game.ActionPlay)
	if err != nil {
		t.Fatal(err)
	}
	if state.Fuel != 0 || !reflect.DeepEqual(state.ShipSystems, initial.ShipSystems) {
		t.Fatal("直接開始改寫了新局燃料或船艦損傷")
	}
	state, err = startState(initial, reducer, "character-creation")
	if err != nil {
		t.Fatal(err)
	}
	if state.Fuel != 0 {
		t.Fatal("另建角色必須保持零燃料")
	}
}

func TestShipboardPanelMouseActionUsesAppleIIEvidenceBackedSubcTarget(t *testing.T) {
	a := shipboardApp(t)
	x, y := subcTarget(a)
	action, ok := a.shipboardPanelMouseAction(x, y)
	if !ok {
		t.Fatal("Sub-C Engines panel target was not recognized")
	}
	if action != game.ActionOpenEngineering {
		t.Fatalf("Sub-C Engines panel target action = %q, want %q", action, game.ActionOpenEngineering)
	}
}

func TestShipboardPanelMouseActionLeavesUnknownTargetsPending(t *testing.T) {
	a := shipboardApp(t)
	window, columnWidth := a.shipboardWindow()
	targetX, targetY := subcTarget(a)
	topRow := window.rowY(0, shipboardRowStep) + shipboardRowStep/2
	for _, point := range [][2]int{
		{panelTextX, targetY},                           // 左方框，不是視窗
		{targetX, topRow},                               // 同一欄的第一列
		{window.columnX(0, columnWidth) + 2, targetY},   // 同一列的左欄
		{targetX, window.rowY(5, shipboardRowStep) + 2}, // 同一欄的最後一列
		{window.x + window.w + 4, targetY},              // 視窗外面
	} {
		if action, ok := a.shipboardPanelMouseAction(point[0], point[1]); ok || action != "" {
			t.Fatalf("unknown shipboard target (%d,%d) = (%q,%v), want no mapping", point[0], point[1], action, ok)
		}
	}
}

func TestRemakePanelMouseActionMatchesVisibleActions(t *testing.T) {
	// The x values come from the panel rectangle, not from where the panel used
	// to be. They were hard-coded at 30/50/110 -- inside the old panel at x=5 --
	// so moving the panel to the right of the scene made every one of them miss.
	const (
		panelMidX   = panelX + panelW/2
		panelLeftX  = panelX + panelW/4
		panelRightX = panelX + panelW*3/4
	)
	// buttonRowY 是左方框按鈕列的 y；讀數列不在那裡，也沒有點擊帶。
	buttonRowY := func(screen game.Screen, row int) int {
		y, ok := panelRowYFor(screen, row)
		if !ok {
			t.Fatalf("%s 第 %d 列不是按鈕列", screen, row)
		}
		return y
	}
	tests := []struct {
		name   string
		screen game.Screen
		x, y   int
		want   game.Action
	}{
		{"標題開始遊玩", game.ScreenTitle, 100, 74, game.ActionPlay},
		{"標題選擇角色", game.ScreenTitle, 100, 90, game.ActionSelectGame},
		{"保存庫建立", game.ScreenCharacterLibrary, 100, libraryKeysY + 4, game.ActionMakeLibrary},
		// 休息在第 6 列、進食在第 7 列（見 screenPanelLines）。
		{"角色狀態休息", game.ScreenCharacterStatus, panelMidX, buttonRowY(game.ScreenCharacterStatus, 6) + 4, game.ActionRest},
		{"角色狀態進食", game.ScreenCharacterStatus, panelMidX, buttonRowY(game.ScreenCharacterStatus, 7) + 4, game.ActionEat},
		{"地面補給", game.ScreenGround, panelMidX, groundRowY0 + 3*groundRowStep + 4, game.ActionLoadSupplies},
		{"交易出售", game.ScreenTrade, panelMidX, buttonRowY(game.ScreenTrade, 6) + 4, game.ActionSellGems},
		{"導航上一個星系", game.ScreenNavigation, panelLeftX, routeRowY + 0*routeRowStep + 4, game.ActionPreviousTargetSystem},
		{"導航下一個星系", game.ScreenNavigation, panelRightX, routeRowY + 0*routeRowStep + 4, game.ActionNextTargetSystem},
		{"導航上一顆行星", game.ScreenNavigation, panelLeftX, routeRowY + 1*routeRowStep + 4, game.ActionPreviousTargetPlanet},
		{"導航下一顆行星", game.ScreenNavigation, panelRightX, routeRowY + 1*routeRowStep + 4, game.ActionNextTargetPlanet},
		{"導航上一個城市", game.ScreenNavigation, panelLeftX, routeRowY + 2*routeRowStep + 4, game.ActionPreviousDestination},
		{"導航下一個城市", game.ScreenNavigation, panelRightX, routeRowY + 2*routeRowStep + 4, game.ActionNextDestination},
		{"銀行提領", game.ScreenUniteller, panelMidX, buttonRowY(game.ScreenUniteller, 3) + 4, game.ActionWithdrawCash},
		{"戰術移動", game.ScreenTactical, panelMidX, buttonRowY(game.ScreenTactical, 5) + 4, game.ActionTacticalMove},
		{"戰術充能", game.ScreenTactical, panelMidX, buttonRowY(game.ScreenTactical, 7) + 4, game.ActionRechargeWeapon},
		{"結局返回", game.ScreenOutcome, 100, 120, game.ActionBack},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := remakePanelMouseAction(test.screen, test.x, test.y)
			if !ok || got != test.want {
				t.Fatalf("remakePanelMouseAction(%q, %d, %d) = (%q, %v), want (%q, true)", test.screen, test.x, test.y, got, ok, test.want)
			}
		})
	}
}

func TestFailureOutcomeOnlyMapsTerminalSave(t *testing.T) {
	failure := game.State{Screen: game.ScreenOutcome, OutcomeKey: "ui.outcome.failure"}
	if action, ok := keyAction(ebiten.KeyS, failure); !ok || action != game.ActionSave {
		t.Fatalf("failure outcome S = (%q, %v), want terminal save", action, ok)
	}
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyEscape} {
		if action, ok := keyAction(key, failure); ok || action != "" {
			t.Fatalf("failure outcome key %v = (%q, %v), want no return", key, action, ok)
		}
	}
	success := game.State{Screen: game.ScreenOutcome, OutcomeKey: "ui.outcome.success"}
	if action, ok := keyAction(ebiten.KeyEscape, success); !ok || action != game.ActionBack {
		t.Fatalf("success outcome Escape = (%q, %v), want return", action, ok)
	}
}

func TestFailureOutcomeMouseActionsAreDisabled(t *testing.T) {
	a := app{state: game.State{Screen: game.ScreenOutcome, OutcomeKey: "ui.outcome.failure"}}
	if action := a.mouseActionAt(100, 120); action != "" {
		t.Fatalf("failure outcome click = %q, want no action", action)
	}
}

func TestMouseActionAtLeavesStaticPanelContentNeutral(t *testing.T) {
	a := app{state: game.State{Screen: game.ScreenTitle}}
	if got := a.mouseActionAt(28, 48); got != "" {
		t.Fatalf("static title content click = %q, want no action", got)
	}
}

func TestMouseActionAtPrioritizesVisibleGroundActionOverMap(t *testing.T) {
	a := app{state: game.State{Screen: game.ScreenGround, Position: game.Position{X: 3, Y: 2}}}
	// 補給是地面面板的第 3 列。座標從面板幾何推出來，不寫死。
	if got := a.mouseActionAt(panelX+panelW/2, groundRowY0+3*groundRowStep+4); got != game.ActionLoadSupplies {
		t.Fatalf("ground visible action click = %q, want %q", got, game.ActionLoadSupplies)
	}
}

func TestModernGroundDisplayPointerUsesHighResolutionMapAndVisiblePanel(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	a := app{
		reducer: reducer,
		state: game.State{
			Screen:   game.ScreenGround,
			Theme:    theme.AppleIIOriginal,
			Tileset:  theme.ModernTiles,
			Position: game.Position{X: 3, Y: 2},
		},
	}
	group, ok := reducer.Themes.Pair(theme.AppleIIOriginal, theme.ModernTiles)
	if !ok || group.Raster == nil {
		t.Fatal("Modern Apple high-resolution raster group is unavailable")
	}
	tileSize := tilePixels * displayScale
	mapX, mapY := modernGroundOriginX*displayScale, modernGroundOriginY*displayScale
	if got := a.mouseActionAtDisplay(mapX+5*tileSize+tileSize/2, mapY+2*tileSize+tileSize/2); got != game.ActionMoveEast {
		t.Fatalf("high-resolution map click = %q, want %q", got, game.ActionMoveEast)
	}
	panelX := (panelX + 20) * displayScale
	panelY := (groundRowY0 + 3*groundRowStep + 4) * displayScale
	if got := a.mouseActionAtDisplay(panelX, panelY); got != game.ActionLoadSupplies {
		t.Fatalf("high-resolution panel click = %q, want %q", got, game.ActionLoadSupplies)
	}
}

func TestEbitenLayoutUsesNativeHighResolutionOutput(t *testing.T) {
	if width, height := (&app{}).Layout(0, 0); width != displayWidth || height != displayHeight {
		t.Fatalf("Layout() = %dx%d, want %dx%d", width, height, displayWidth, displayHeight)
	}
}

func TestVisibleMousePathFromTitleToEvidenceBackedEngineering(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	labels, face := auditCatalogAndFace(t)
	a := app{state: game.NewState(), labels: labels, face: face}
	state, err := reducer.Step(a.state, a.mouseActionAt(100, 74))
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != game.ScreenShipboard {
		t.Fatalf("title visible click screen = %q, want %q", state.Screen, game.ScreenShipboard)
	}
	a.state = state
	targetX, targetY := subcTarget(&a)
	state, err = reducer.Step(a.state, a.mouseActionAt(targetX, targetY))
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != game.ScreenEngineering {
		t.Fatalf("Sub-C panel click screen = %q, want %q", state.Screen, game.ScreenEngineering)
	}
}

func TestFooterHelpWrapsWithoutHidingThemeName(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	registry, err := theme.NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// 每一個預設都要在下方那條說明裡讀得到名字：玩家按了 F8 之後，那行是唯一
	// 告訴他現在是哪一套的地方。
	for _, preset := range theme.Presentations() {
		t.Run(string(preset.ID), func(t *testing.T) {
			group, ok := registry.Pair(preset.Chrome, preset.Tileset)
			if !ok {
				t.Fatalf("預設 %q 的配色與圖塊配不起來", preset.ID)
			}
			state := game.WithPresentation(game.State{Screen: game.ScreenShipboard}, preset)
			help := footerHelp(labels, state, group)
			lines := localization.WrapText(face, help, footerHelpWidth, footerHelpMaxLines)
			if len(lines) == 0 || strings.HasSuffix(lines[len(lines)-1], "…") {
				t.Fatalf("下方說明被截斷了：%#v", lines)
			}
			name := lookupLabel(labels, preset.DisplayNameKey, string(preset.ID))
			if !strings.Contains(strings.Join(lines, " "), name) {
				t.Fatalf("下方說明看不到呈現名稱 %q：%#v", name, lines)
			}
		})
	}
}

func TestEquipmentNoteWrapsWithoutEllipsis(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	note := lookupLabel(labels, "ui.equipment_note", "")
	lines := localization.WrapText(face, note, equipmentNoteWidth, equipmentNoteMaxLines)
	if len(lines) == 0 || strings.HasSuffix(lines[len(lines)-1], "…") {
		t.Fatalf("equipment source note was truncated: %#v", lines)
	}
	if got, want := strings.ReplaceAll(strings.Join(lines, ""), " ", ""), strings.ReplaceAll(note, " ", ""); got != want {
		t.Fatalf("equipment source note lost content: got %q, want %q", got, want)
	}
}

func auditCatalogAndFace(t *testing.T) (catalog.Catalog, font.Face) {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", ".."))
	labels, _, err := loadCatalogs(
		filepath.Join(root, "docs", "i18n", "apple2-text-catalog.json"),
		filepath.Join(root, "docs", "i18n", "sundog-ui-zh-Hant.json"),
	)
	if err != nil {
		t.Fatal(err)
	}
	fontBytes, err := os.ReadFile(filepath.Join(root, "assets", "fonts", "NotoSansCJKtc-Regular.otf"))
	if err != nil {
		t.Fatal(err)
	}
	fontData, err := opentype.Parse(fontBytes)
	if err != nil {
		t.Fatal(err)
	}
	face, err := opentype.NewFace(fontData, &opentype.FaceOptions{Size: 8, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		t.Fatal(err)
	}
	return labels, face
}

// TestPolledKeysCoverEveryMappedKey walks every screen and every key value and
// fails if keyAction maps a key that Update never polls. That gap is invisible
// to a keyAction unit test: the mapping is correct while the key is dead in the
// running game.
func TestPolledKeysCoverEveryMappedKey(t *testing.T) {
	polled := make(map[ebiten.Key]struct{}, len(polledKeys)+len(separatelyPolledKeys))
	for _, key := range polledKeys {
		polled[key] = struct{}{}
	}
	for _, key := range separatelyPolledKeys {
		polled[key] = struct{}{}
	}
	screens := []game.Screen{
		game.ScreenTitle,
		game.ScreenCharacterLibrary,
		game.ScreenCharacterCreation,
		game.ScreenCharacterStatus,
		game.ScreenShipboard,
		game.ScreenGround,
		game.ScreenTrade,
		game.ScreenNavigation,
		game.ScreenFlight,
		game.ScreenUniteller,
		game.ScreenTactical,
		game.ScreenEngineering,
		game.ScreenEquipment,
		game.ScreenCargo,
		game.ScreenOutcome,
	}
	for _, screen := range screens {
		state := game.State{Screen: screen}
		for key := ebiten.Key(0); key <= ebiten.KeyMax; key++ {
			action, ok := keyAction(key, state)
			if !ok {
				continue
			}
			if _, polledOK := polled[key]; !polledOK {
				t.Errorf("screen %q maps key %v to %q but Update never polls it", screen, key, action)
			}
		}
	}
}

// TestPanelRowsClearTheStatusStrip is the rule the ground and engineering
// panels used to break: the status-message strip is shared, it is drawn over
// whatever the panel left there, and its first line of text starts at 138. A
// panel row is eight pixels of ink starting one below its y, so the last row
// has to sit at panelLastRowY or above. The screens with a full-width scene
// are excluded because they are the ones that do not draw the strip at all.
func TestPanelRowsClearTheStatusStrip(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	base := game.NewState()
	screens := []game.Screen{
		game.ScreenCharacterStatus,
		game.ScreenGround,
		game.ScreenTrade,
		game.ScreenParts,
		game.ScreenNavigation,
		game.ScreenUniteller,
		game.ScreenEngineering,
		game.ScreenCargo,
		game.ScreenDialog,
		game.ScreenInterior,
		game.ScreenSlots,
		game.ScreenArena,
		game.ScreenScore,
	}
	for _, screen := range screens {
		t.Run(string(screen), func(t *testing.T) {
			state := base
			state.Screen = screen
			a := app{reducer: reducer, state: state, labels: labels, face: face}
			lines := a.screenPanelLines()
			if len(lines) == 0 {
				t.Fatalf("%s panel has no rows", screen)
			}
			last, ok := panelRowYFor(screen, len(lines)-1)
			if !ok {
				// 整頁都是讀數的畫面（零件櫃台）沒有按鈕列，也就碰不到訊息框。
				return
			}
			if screen == game.ScreenGround {
				last = groundRowY0 + (len(lines)-1)*groundRowStep
			}
			if screen == game.ScreenNavigation {
				last = routeRowY + (len(lines)-1)*routeRowStep
			}
			if last > panelLastRowY {
				t.Fatalf("%s last panel row at y=%d, want at most %d so it clears the status strip at %d",
					screen, last, panelLastRowY, statusStripY)
			}
		})
	}
}

// everyScreen lists the screens the frame draws for, so a layout rule that has
// to hold everywhere can say so instead of naming a handful.
func everyScreen() []game.Screen {
	return []game.Screen{
		game.ScreenTitle, game.ScreenCharacterLibrary, game.ScreenCharacterCreation,
		game.ScreenCharacterStatus, game.ScreenShipboard, game.ScreenGround,
		game.ScreenTrade, game.ScreenNavigation, game.ScreenUniteller,
		game.ScreenTactical, game.ScreenFlight, game.ScreenEngineering,
		game.ScreenEquipment, game.ScreenParts, game.ScreenCargo,
		game.ScreenDialog, game.ScreenOutcome, game.ScreenInterior,
		game.ScreenSlots, game.ScreenArena, game.ScreenScore,
	}
}

// 頁尾的鍵位提示畫在原版的下方訊息框裡，寬度只有 frameMessageW。它是用 FitText
// 裁的，所以放不下不會噴錯，只會無聲吃掉最後幾個鍵——這條規則就是為了不讓它無聲。
func TestEveryFooterKeyStripFitsTheMessageBox(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	group, ok := reducer.Themes.Group(theme.AppleIIOriginal)
	if !ok {
		t.Fatal("預設配色不在註冊表裡")
	}
	base := game.NewState()
	width := frameMessageW - 10
	for _, screen := range everyScreen() {
		// 選單與終局畫面還是用整行寬的舊頁尾，這條規則只管畫在訊息框裡的那幾張。
		if !inGameFrame(screen) {
			continue
		}
		state := base
		state.Screen = screen
		// WrapText 自己會裁到 maxLines，所以要問它「不設限會折成幾列」，
		// 否則永遠得到兩列，這條規則就等於沒寫。
		strip := footerLabel(labels, state)
		if got := len(localization.WrapText(face, strip, width, 99)); got > 2 {
			t.Errorf("%s 的鍵位提示要 %d 列才放得下 %d 像素寬的訊息框：%q", screen, got, width, strip)
		}
		help := footerHelp(labels, state, group)
		if got := len(localization.WrapText(face, help, width, 99)); got > 2 {
			t.Errorf("%s 的操作提示要 %d 列：%q", screen, got, help)
		}
	}
}

// 使用者要的是「版面像原版，但字要清晰」。清晰的反面就是被 FitText 裁成刪節號，
// 所以每一列都要在它被畫的那個框裡放得下：讀數列在浮動視窗（143 寬），按鈕列在
// 左方框（68 寬）。
func TestNoPanelRowIsCutToAnEllipsis(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	base := game.NewState()
	for _, screen := range everyScreen() {
		if !inGameFrame(screen) {
			continue
		}
		state := base
		state.Screen = screen
		a := app{reducer: reducer, state: state, labels: labels, face: face}
		readouts := panelReadoutLines(screen)
		for index, line := range a.screenPanelLines() {
			width := panelTextW
			where := "左方框"
			if screen != game.ScreenGround && screen != game.ScreenNavigation && index < readouts {
				width, where = floatTextW, "浮動視窗"
			}
			if localization.FitText(face, line, width) != line {
				t.Errorf("%s 第 %d 列在%s（%d 像素）裡被裁掉：%q", screen, index, where, width, line)
			}
		}
	}
}

// 浮動視窗是蓋在視埠上的小視窗，不是另一張畫面：它要留在視埠內緣，而且每一列
// 都要畫在它自己的框裡。
func TestEveryFloatingWindowStaysInsideTheViewport(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	inside := func(name string, window floatWindow) {
		t.Helper()
		if window.x < floatX || window.x+window.w > floatX+floatW {
			t.Errorf("%s 的視窗橫向是 %d..%d，視埠內緣是 %d..%d",
				name, window.x, window.x+window.w, floatX, floatX+floatW)
		}
		if window.y < floatY || window.y+window.h > floatY+floatH {
			t.Errorf("%s 的視窗縱向是 %d..%d，視埠內緣是 %d..%d",
				name, window.y, window.y+window.h, floatY, floatY+floatH)
		}
	}
	for _, screen := range everyScreen() {
		if !inGameFrame(screen) || panelReadoutLines(screen) == 0 {
			continue
		}
		state := game.NewState()
		state.Screen = screen
		a := app{reducer: reducer, state: state, labels: labels, face: face}
		lines := a.screenPanelLines()
		window := a.readoutWindow(lines)
		inside(string(screen), window)
		readouts := clampInt(panelReadoutLines(screen), 0, len(lines))
		last := window.rowY(readouts-1, floatRowStep) + 8
		if last > window.y+window.h {
			t.Errorf("%s 的最後一列讀數畫到 y=%d，視窗只到 %d", screen, last, window.y+window.h)
		}
	}

	shipboard := shipboardApp(t)
	window, columnWidth := shipboard.shipboardWindow()
	inside("shipboard", window)
	if right := window.columnX(1, columnWidth) + columnWidth; right > window.x+window.w {
		t.Errorf("艙位右欄畫到 x=%d，視窗只到 %d", right, window.x+window.w)
	}
	if bottom := window.rowY(5, shipboardRowStep) + 8; bottom > window.y+window.h {
		t.Errorf("艙位最後一列畫到 y=%d，視窗只到 %d", bottom, window.y+window.h)
	}
}

// 圖示列排在浮動視窗最後一排，不能壓到讀數列，也不能掉出視窗。視窗的大小隨內容變，
// 所以這條要用實際算出來的那個視窗問，不是問常數。
func TestAttributeReadoutKeepsZeroAndHidesLuck(t *testing.T) {
	a := statusApp(t)
	a.state.Attributes = [game.AttributeCount]int{0, 255, 4, 9, 123}
	line := a.attributeLine()
	if !strings.Contains(line, "ST 0") || !strings.Contains(line, "IQ 255") ||
		!strings.Contains(line, "LK "+a.lookup("ui.attribute.hidden")) || strings.Contains(line, "123") {
		t.Fatalf("attribute readout: %s", line)
	}
}

func TestTheCarriedIconRowStaysInsideTheFloatingWindow(t *testing.T) {
	a := statusApp(t)
	window := a.readoutWindow(a.screenPanelLines())
	readouts := panelReadoutLines(game.ScreenCharacterStatus)
	lastReadout := window.rowY(readouts-1, floatRowStep) + 8
	if window.bottomRowY() < lastReadout {
		t.Fatalf("圖示列從 y=%d 起，讀數列畫到 %d", window.bottomRowY(), lastReadout)
	}
	if window.bottomRowY()+originalmedia.ItemIconSize > window.y+window.h {
		t.Fatalf("圖示列畫到 y=%d，浮動視窗只到 %d",
			window.bottomRowY()+originalmedia.ItemIconSize, window.y+window.h)
	}
}

// 圖示列一件一格、不重疊，而且排不下的那幾件寧可不畫，也不畫到視窗外面。
func TestTheCarriedIconRowGivesEachItemItsOwnCell(t *testing.T) {
	a := statusApp(t)
	a.state.HandItem = 0x0c
	a.state.Parts = []int{0x17, 0x18}
	row := a.carriedIconRow(a.readoutWindow(a.screenPanelLines()))
	if len(row) != 3 {
		t.Fatalf("身上三件卻排了 %d 格", len(row))
	}
	if row[0].item != 0x0c {
		t.Fatalf("排頭是 0x%02x，應該是手上那件", row[0].item)
	}
	for index := 1; index < len(row); index++ {
		gap := row[index].x - row[index-1].x
		if gap < originalmedia.ItemIconSize+1 {
			t.Fatalf("第 %d 格與前一格只差 %d 像素，圖示會黏在一起", index, gap)
		}
	}

	// 塞爆它：每一格都得留在視窗裡、讓開指南針，多出來的不畫。
	a.state.Parts = make([]int, 64)
	for index := range a.state.Parts {
		a.state.Parts[index] = 0x17
	}
	window := a.readoutWindow(a.screenPanelLines())
	right := window.x + window.w - floatCompassInset
	for _, cell := range a.carriedIconRow(window) {
		if cell.x < window.x || cell.x+originalmedia.ItemIconSize > right {
			t.Fatalf("有一格畫在 x=%d，可用的範圍是 %d..%d", cell.x, window.x, right)
		}
	}
}

// 沒給 -atari-disk 就沒有圖示。這是預設的建置，不能因此壞掉。
func TestItemIconsAreAbsentWithoutADisk(t *testing.T) {
	a := statusApp(t)
	if _, ok := a.itemIcon(0x17); ok {
		t.Fatal("沒有原版磁碟卻拿到了圖示")
	}
	a.drawCarriedIcons(nil, a.readoutWindow(a.screenPanelLines()))
}

// 存起來的物品位元組帶旗標，查表前要先遮掉——不遮的話 0x37 會查到表外。
func TestAnItemByteIsMaskedBeforeItIsLookedUp(t *testing.T) {
	for _, test := range []struct{ stored, want int }{
		{0x17, 0x17}, {0x37, 0x17}, {0xf7, 0x17}, {0x20, 0x00},
	} {
		if got := game.ItemCode(test.stored); got != test.want {
			t.Errorf("ItemCode(0x%02x) = 0x%02x，應該是 0x%02x", test.stored, got, test.want)
		}
	}
}

// 量表標籤是逐字直書的，一列一個字九像素，所以字數決定它會不會掉出右邊那個框。
func TestEveryGaugeLabelFitsTheGaugeBox(t *testing.T) {
	barTop := frameGaugeY + 6
	barHeight := frameGaugeH - 46
	bottom := frameGaugeY + frameGaugeH - 2
	root := filepath.Join("..", "..", "docs", "i18n")
	for _, locale := range []string{"zh-Hant", "zh-Hans", "ja", "ko", "en"} {
		t.Run(locale, func(t *testing.T) {
			sourceName := "apple2-text-" + locale + ".json"
			if locale == "zh-Hant" {
				sourceName = "apple2-text-catalog.json"
			}
			labels, _, err := loadCatalogs(filepath.Join(root, sourceName),
				filepath.Join(root, "sundog-ui-"+locale+".json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"ui.gauge.fuel", "ui.gauge.guns", "ui.gauge.shield", "ui.gauge.hull"} {
				label, ok := labels.Lookup(key)
				if !ok {
					t.Fatalf("量表標籤 %s 不在詞條表裡", key)
				}
				last := barTop + barHeight + 3 + (len([]rune(label))-1)*9 + 8
				if last > bottom {
					t.Errorf("%s（%s）直書到 y=%d，量表框只到 %d", key, label, last, bottom)
				}
			}
		})
	}
}

// 面板的最後一列不只要避開訊息框，還要留在左方框裡面：字是八像素的墨，畫在
// y 的下一列起算，所以最後一列的 y 加九不能越過框底。
func TestThePanelLastRowStaysInsideTheLeftBox(t *testing.T) {
	if panelLastRowY+9 > frameLeftY+frameLeftH {
		t.Fatalf("最後一列畫到 y=%d，左方框只到 %d", panelLastRowY+9, frameLeftY+frameLeftH)
	}
}

// The engineering panel's own rows: four meters, the selected system with its
// meter, the shunt row, then the source note on the last usable row.
func TestEngineeringPanelRowsClearTheStatusStrip(t *testing.T) {
	lastMeter := engineeringRowY0 + 3*engineeringRowStep
	if lastMeter+9 > engineeringSelectedY {
		t.Fatalf("last meter row ends at %d, selected system starts at %d", lastMeter+9, engineeringSelectedY)
	}
	if engineeringSelectedY+9 > engineeringShuntY {
		t.Fatalf("selected system row ends at %d, shunt row starts at %d", engineeringSelectedY+9, engineeringShuntY)
	}
	if engineeringShuntY+9 > engineeringRowsY {
		t.Fatalf("shunt row ends at %d, the row statuses start at %d", engineeringShuntY+9, engineeringRowsY)
	}
	if engineeringRowsY > panelLastRowY {
		t.Fatalf("row statuses sit at %d, past the last usable row %d", engineeringRowsY, panelLastRowY)
	}
	// 量表跟標籤都要留在左方框裡。
	if engineeringMeterX+engineeringMeterWidth > panelX+panelW-4 {
		t.Fatalf("量表畫到 x=%d，左方框只到 %d", engineeringMeterX+engineeringMeterWidth, panelX+panelW-4)
	}
	if panelLastRowY+9 >= statusStripY+1 {
		t.Fatalf("the note ends at %d, the status strip text starts at %d", panelLastRowY+9, statusStripY+1)
	}
}

// TestViewportKeepsWholeNumberScaling covers the resizable window: any window
// size picks the largest whole multiple that fits and centres the leftover.
func TestViewportKeepsWholeNumberScaling(t *testing.T) {
	tests := []struct {
		name                    string
		width, height           int
		scale, originX, originY int
	}{
		{"預設視窗", 1280, 800, 4, 0, 0},
		{"截圖用的六倍", 1920, 1200, 6, 0, 0},
		{"replay 腳本的三倍", 960, 600, 3, 0, 0},
		{"拖成非整數倍就補邊", 1000, 640, 3, 20, 20},
		{"寬螢幕以高度為準", 1920, 800, 4, 320, 0},
		{"小於一倍仍是一倍", 200, 100, 1, 0, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scale, originX, originY := viewportFor(test.width, test.height)
			if scale != test.scale || originX != test.originX || originY != test.originY {
				t.Fatalf("viewportFor(%d, %d) = (%d, %d, %d), want (%d, %d, %d)",
					test.width, test.height, scale, originX, originY, test.scale, test.originX, test.originY)
			}
		})
	}
}

// A resized window has to move the mouse mapping with it, or the visible rows
// and the clickable ones drift apart.
func TestMouseMappingFollowsTheResizedWindow(t *testing.T) {
	a := app{state: game.State{Screen: game.ScreenGround, Position: game.Position{X: 3, Y: 2}}}
	a.setViewport(1000, 640)
	if a.scale != 3 || a.originX != 20 || a.originY != 20 {
		t.Fatalf("viewport = (%d, %d, %d), want (3, 20, 20)", a.scale, a.originX, a.originY)
	}
	// The middle of the visible 補給 row, in window pixels.
	x := (panelX+panelW/2)*a.scale + a.originX
	y := (groundRowY0+3*groundRowStep+4)*a.scale + a.originY
	if got := a.mouseActionAtDisplay(x, y); got != game.ActionLoadSupplies {
		t.Fatalf("resized ground click = %q, want %q", got, game.ActionLoadSupplies)
	}
}

// TestHelpCardSwallowsInput covers the F1 card's contract: while it is up the
// game behind it must not move, or the key that closes it also acts on the
// game.
func TestHelpCardSwallowsInput(t *testing.T) {
	a := app{state: game.State{Screen: game.ScreenGround}}
	if a.showHelp {
		t.Fatal("the help card starts closed")
	}
	for _, key := range separatelyPolledKeys {
		if key == ebiten.KeyF1 {
			return
		}
	}
	t.Fatal("F1 is handled in Update but missing from separatelyPolledKeys")
}

// Quitting writes the writable slot, but only once there is a game to write.
// A title-screen state would otherwise overwrite a real save with an empty one.
func TestAutosaveSkipsTheMenusAndWritesAGameInProgress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "slot.json")

	menu := app{state: game.State{Screen: game.ScreenTitle}, savePath: path}
	menu.autosave()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("quitting from the title wrote %s", path)
	}

	state := game.NewState()
	state.Screen = game.ScreenGround
	state.Character = "測試"
	playing := app{state: state, savePath: path}
	playing.autosave()
	loaded, err := storage.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Screen != game.ScreenGround || loaded.Character != "測試" {
		t.Fatalf("autosave stored %q on %q, want 測試 on %q", loaded.Character, loaded.Screen, game.ScreenGround)
	}
	// Quitting is an action and every action clears the flag, so a naive
	// autosave writes a file that claims it was never saved — and it lands on
	// top of whatever the S key wrote a moment earlier.
	if !loaded.Saved {
		t.Fatal("autosave wrote a state that reads as unsaved")
	}
}

func TestResumeStateClearsPersistedQuitRequest(t *testing.T) {
	state := game.NewState()
	state.Screen = game.ScreenShipboard
	state.MessageKey = "ui.message.quit"
	state.QuitRequested = true
	state.Saved = true

	loaded := resumeState(state)
	if loaded.QuitRequested {
		t.Fatal("resumed state still requests process termination")
	}
	if loaded.MessageKey != "ui.message.loaded" || loaded.Saved {
		t.Fatalf("resume markers = message %q, saved %v", loaded.MessageKey, loaded.Saved)
	}
	if loaded.Screen != game.ScreenShipboard {
		t.Fatalf("resume changed screen to %q", loaded.Screen)
	}
}

// F10 is the documented way out, so it has to reach the same action the Q key
// reaches and it has to be a key Update actually polls.
func TestF10IsPolledAndQuits(t *testing.T) {
	found := false
	for _, key := range separatelyPolledKeys {
		if key == ebiten.KeyF10 {
			found = true
		}
	}
	if !found {
		t.Fatal("F10 is handled in Update but missing from separatelyPolledKeys")
	}
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := game.NewState()
	state.Screen = game.ScreenGround
	next, err := reducer.Step(state, game.ActionQuit)
	if err != nil {
		t.Fatal(err)
	}
	if !next.QuitRequested {
		t.Fatal("ActionQuit did not request a quit")
	}
}

// TestPresentationCycleCoversEveryPresetAndNothingElse: F8 走得完每一個預設，
// 而且只會停在預設上。三軸各自獨立的時代結束於 2026-09-03 的量測——Atari ST 版面下
// 配色與圖塊兩軸畫不出差別，而其中一組會把重製版地圖浮貼在原版滿版城市上。
func TestPresentationCycleCoversEveryPresetAndNothingElse(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := game.NewState()
	state.Screen = game.ScreenGround

	presets := theme.Presentations()
	seen := map[theme.PresentationID]struct{}{}
	cursor := state
	for step := 0; step < len(presets); step++ {
		if _, ok := reducer.GroupOf(cursor); !ok {
			t.Fatalf("第 %d 步畫不出來", step)
		}
		current := game.PresentationOf(cursor)
		if current.Chrome != cursor.Theme || current.Tileset != cursor.Tileset || current.Layout != cursor.Layout {
			t.Fatalf("第 %d 步停在不是預設的三元組：%q %q %q", step, cursor.Theme, cursor.Tileset, cursor.Layout)
		}
		seen[current.ID] = struct{}{}
		cursor, err = reducer.Step(cursor, game.ActionCyclePresentation)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != len(presets) {
		t.Fatalf("F8 走到 %d 個預設，共有 %d 個", len(seen), len(presets))
	}
	if game.PresentationOf(cursor).ID != game.PresentationOf(state).ID {
		t.Fatalf("走完一圈沒有回到起點：%q", game.PresentationOf(cursor).ID)
	}
}

// TestQuitKeyStaysOutOfTextEntry covers the name-entry screen: every keystroke
// there is a letter of the name, so Q must not also quit. F10 is the way out.
func TestQuitKeyStaysOutOfTextEntry(t *testing.T) {
	typing := game.State{Screen: game.ScreenCharacterCreation}
	if action, ok := keyAction(ebiten.KeyQ, typing); ok {
		t.Fatalf("Q on the name-entry screen mapped to %q, want no action", action)
	}
	// Everything else keeps the key: the replay harness and the failure
	// terminal's own footer both document Q as the way out.
	for _, screen := range []game.Screen{
		game.ScreenTitle,
		game.ScreenCharacterLibrary,
		game.ScreenShipboard,
		game.ScreenGround,
		game.ScreenTrade,
		game.ScreenNavigation,
		game.ScreenTactical,
		game.ScreenEngineering,
		game.ScreenCargo,
		game.ScreenOutcome,
	} {
		action, ok := keyAction(ebiten.KeyQ, game.State{Screen: screen})
		if !ok || action != game.ActionQuit {
			t.Fatalf("Q on %q mapped to (%q, %v), want quit", screen, action, ok)
		}
	}
}

// The name-entry screen must not turn any other letter into a command either,
// or that letter is unusable in a name. Enter and Space confirm; the arrows and
// backspace edit; nothing else may act.
func TestNameEntryLeavesLettersToTheName(t *testing.T) {
	typing := game.State{Screen: game.ScreenCharacterCreation}
	for key := ebiten.KeyA; key <= ebiten.KeyZ; key++ {
		if action, ok := keyAction(key, typing); ok {
			t.Fatalf("%v on the name-entry screen mapped to %q, want no action", key, action)
		}
	}
	// Space is a character a name may contain, so it cannot also be the button
	// that ends the typing. ENTER alone confirms.
	if action, ok := keyAction(ebiten.KeySpace, typing); ok {
		t.Fatalf("space on the name-entry screen mapped to %q, want no action", action)
	}
	if action, ok := keyAction(ebiten.KeyEnter, typing); !ok || action != game.ActionConfirmCharacterName {
		t.Fatalf("ENTER on the name-entry screen mapped to (%q, %v), want confirm", action, ok)
	}
}

// TestMenuRowsClearTheMessageRow is the panel rule applied to the menu screens,
// which carry their own message row inside the panel rather than using the
// shared status strip. The character library is the one with enough rows to
// reach it: its last key row used to sit three pixels into the message.
func TestMenuRowsClearTheMessageRow(t *testing.T) {
	// The library draws a title, four rows of slots in two columns, and the
	// key row under them; the message row comes after all of that.
	if 44+9 > librarySlotsY {
		t.Fatalf("library title ends at %d, the slot list starts at %d", 44+9, librarySlotsY)
	}
	lastSlot := librarySlotsY + (librarySlotRows-1)*librarySlotStep
	if lastSlot+9 > libraryKeysY {
		t.Fatalf("the last slot row ends at %d, the key row starts at %d", lastSlot+9, libraryKeysY)
	}
	if libraryKeysY+9 > menuMessageY {
		t.Fatalf("library key row ends at %d, the message row starts at %d", libraryKeysY+9, menuMessageY)
	}
	// The panel itself is 28..150, so the message has to fit inside it too.
	if menuMessageY+9 > 150 {
		t.Fatalf("the message row ends at %d, past the menu panel", menuMessageY+9)
	}
}

// The rows a menu screen draws and the bands it accepts clicks in have to stay
// together: moving one without the other leaves the visible row and the
// clickable row pointing at different things.
func TestMenuHotspotsSitOnTheDrawnRows(t *testing.T) {
	// The middle of a row's ink, which is where a player aims.
	const inkMiddle = 4
	tests := []struct {
		name   string
		screen game.Screen
		x, y   int
		want   game.Action
	}{
		{"標題開始遊玩", game.ScreenTitle, 100, 72 + inkMiddle, game.ActionPlay},
		{"標題角色保存庫", game.ScreenTitle, 100, 88 + inkMiddle, game.ActionSelectGame},
		{"標題載入", game.ScreenTitle, 100, 104 + inkMiddle, game.ActionLoadGame},
		// The library's five keys share one row now (libraryKeysY); the slot
		// list above it is picked by clicking a slot, not by a menu band.
		{"保存庫新建", game.ScreenCharacterLibrary, 40, libraryKeysY + inkMiddle, game.ActionNewGame},
		{"保存庫建庫", game.ScreenCharacterLibrary, 100, libraryKeysY + inkMiddle, game.ActionMakeLibrary},
		{"保存庫儲存", game.ScreenCharacterLibrary, 160, libraryKeysY + inkMiddle, game.ActionStoreGame},
		{"保存庫載入", game.ScreenCharacterLibrary, 210, libraryKeysY + inkMiddle, game.ActionLoadGame},
		{"保存庫刪除", game.ScreenCharacterLibrary, 270, libraryKeysY + inkMiddle, game.ActionDeleteGame},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := remakePanelMouseAction(test.screen, test.x, test.y)
			if !ok || got != test.want {
				t.Fatalf("點 (%d, %d) 得到 (%q, %v)，應該是 %q", test.x, test.y, got, ok, test.want)
			}
		})
	}
}

// A panel's clickable bands and the rows it actually draws have to line up.
// The bands are hard-coded rectangles and the rows are computed from
// panelRowY0/panelRowStep, so the two drift apart the moment a row is added or
// removed — and nothing about that shows up in a compile or in a reducer test.
//
// The contract: the actions are always the last rows of a panel, so the bands
// must cover exactly the last N drawn rows, one row each, in the same order.
func TestPanelBandsCoverTheDrawnActionRows(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	labels, face := auditCatalogAndFace(t)

	// rowAt gives where a panel row is drawn, matching drawScreenPanel. The x
	// matters as much as the y now: the readout rows are drawn in the floating
	// window over the viewport, and a window row can share a y with a button
	// row in the left box without being the same row.
	rowAt := func(screen game.Screen, index int) (int, int) {
		switch screen {
		case game.ScreenGround:
			return panelTextX, groundRowY0 + index*groundRowStep
		case game.ScreenNavigation:
			return panelTextX, routeRowY + index*routeRowStep
		default:
			y, ok := panelRowYFor(screen, index)
			if !ok {
				// 讀數列畫在浮動視窗裡，沒有點擊帶；給一個框外的 x 讓它不會撞上。
				return floatX + floatW + 100, 0
			}
			return panelTextX, y
		}
	}

	byScreen := map[game.Screen][]mouseHotspot{}
	for _, spot := range panelMouseHotspots() {
		byScreen[spot.screen] = append(byScreen[spot.screen], spot)
	}
	for screen, spots := range byScreen {
		// The title and library screens draw their own layout rather than the
		// shared panel rows; TestMenuHotspotsSitOnTheDrawnRows covers those.
		if screen == game.ScreenTitle || screen == game.ScreenCharacterLibrary {
			continue
		}
		t.Run(string(screen), func(t *testing.T) {
			state := game.NewState()
			state.Screen = screen
			a := app{reducer: reducer, state: state, labels: labels, face: face}
			lines := a.screenPanelLines()

			// Bands that share a row band (a row split left/right) count once.
			bands := map[[4]int]bool{}
			for _, spot := range spots {
				if !a.panelActionAvailable(spot.action) {
					// A capacity row with no destination must also reject actual clicks.
					if got := a.mouseActionAt((spot.minX+spot.maxX)/2, (spot.minY+spot.maxY)/2); got != "" {
						t.Fatalf("沒有目的地的Tube列仍接受%q", got)
					}
					continue
				}
				// 導航星圖那塊是一整片目標區，不是一列，它沒有對應的文字列。
				if spot.maxY-spot.minY > 16 {
					continue
				}
				bands[[4]int{spot.minY, spot.maxY, spot.minX, spot.maxX}] = true
			}
			seen := map[[2]int]bool{}
			for band := range bands {
				seen[[2]int{band[0], band[1]}] = true
			}
			var covered []int
			for index := range lines {
				x, y := rowAt(screen, index)
				middle := y + 4 // a row is eight pixels of ink starting one below y
				for band := range bands {
					if middle >= band[0] && middle < band[1] && x >= band[2] && x < band[3] {
						covered = append(covered, index)
						break
					}
				}
			}
			if len(lines) == 0 {
				// The outcome screen draws its own layout rather than panel rows.
				return
			}
			want := len(seen)
			if len(covered) != want {
				t.Fatalf("%s: %d bands cover %d drawn rows %v (panel has %d rows)",
					screen, want, len(covered), covered, len(lines))
			}
			for offset, index := range covered {
				if index != covered[0]+offset {
					t.Fatalf("%s: bands cover rows %v, which are not consecutive", screen, covered)
				}
			}
		})
	}
}

func TestFlightRescueKeysAndMouse(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	labels, face := auditCatalogAndFace(t)
	s := game.NewState()
	s.Screen, s.FlightTargetID, s.Fuel = game.ScreenFlight, "shoothan", 0
	s.ShipControls.Subc = false
	s.FuelRescuePending, s.FuelRescueDue, s.MessageKey = true, 6546, "ui.message.refuel_eta"
	a := app{reducer: r, state: s, labels: labels, face: face}
	if !strings.Contains(a.messageLine(), "6546") {
		t.Fatal("pending ETA not displayed")
	}
	a.state.MessageKey = "ui.message.back"
	if !strings.Contains(a.screenPanelLines()[1], "6546") {
		t.Fatal("pending ETA disappeared after leaving a save overlay")
	}
	for key, want := range map[ebiten.Key]game.Action{ebiten.KeyD: game.ActionDistressCall, ebiten.KeyR: game.ActionFlightResume} {
		if got, ok := keyAction(key, s); !ok || got != want {
			t.Fatalf("flight key %v maps %s", key, got)
		}
	}
	old := panelLayoutST
	defer func() { panelLayoutST = old }()
	for _, st := range []bool{false, true} {
		panelLayoutST = st
		for _, action := range []game.Action{game.ActionDistressCall, game.ActionFlightResume} {
			found := false
			for _, h := range panelMouseHotspots() {
				if h.screen != game.ScreenFlight || h.action != action {
					continue
				}
				found = true
				if got := a.mouseActionAt((h.minX+h.maxX)/2, (h.minY+h.maxY)/2); got != action {
					t.Fatalf("layout%v rescue click=%s want%s", st, got, action)
				}
			}
			if !found {
				t.Fatalf("missing layout%v flight action%s", st, action)
			}
		}
	}
}

// A packaged build starts from whatever directory the launcher used, so every
// read-only asset the shell opens has to be retried next to the executable.
// appdir.Asset is what does that, and forgetting one flag is invisible until
// somebody runs the packaged build: the game still starts, it just quietly
// falls back. That is how the title cover and the kai title font shipped
// broken in the 2026-08-20 packages.
//
// This binds the two lists: every flag whose default names a file inside the
// repository's read-only asset trees must be passed through appdir.Asset.
func TestEveryReadOnlyAssetFlagIsResolvedForAPackagedBuild(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(source)

	declaration := regexp.MustCompile(`(\w+) := flag\.String\("[^"]+", "((?:assets|docs)/[^"]+)"`)
	declared := declaration.FindAllStringSubmatch(text, -1)
	if len(declared) == 0 {
		t.Fatal("found no asset flags at all, so this test proves nothing")
	}
	for _, match := range declared {
		variable, def := match[1], match[2]
		resolved := "appdir.Asset(*" + variable + ")"
		if !strings.Contains(text, resolved) {
			t.Errorf("flag %s defaults to %q but never goes through %s, so a packaged build cannot find it",
				variable, def, resolved)
		}
	}
}

// A missing catalog entry shows as [the.key]. That marker is a development aid,
// not something a player should ever see, so no screen's panel may contain one.
// The trade screen shipped with a raw source key on it — the market quote's key
// is atarist.cargo.fuel, a source key, and looking that up in the UI catalog
// found nothing.
func TestNoScreenPanelShowsARawCatalogKey(t *testing.T) {
	labels, face := auditCatalogAndFace(t)
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	base := game.NewState()
	marker := regexp.MustCompile(`\[[a-z0-9._-]+\]`)
	screens := []game.Screen{
		game.ScreenCharacterStatus, game.ScreenGround, game.ScreenTrade,
		game.ScreenParts, game.ScreenNavigation, game.ScreenUniteller,
		game.ScreenEngineering, game.ScreenCargo, game.ScreenDialog,
		game.ScreenInterior, game.ScreenSlots, game.ScreenArena,
		game.ScreenScore, game.ScreenShipboard, game.ScreenTactical,
		game.ScreenFlight, game.ScreenEquipment,
	}
	for _, screen := range screens {
		state := base
		state.Screen = screen
		shell := app{reducer: reducer, state: state, labels: labels, face: face}
		for _, line := range shell.screenPanelLines() {
			if found := marker.FindString(line); found != "" {
				t.Errorf("screen %q shows a missing-catalog marker %s in %q", screen, found, line)
			}
		}
	}
}

// The panel sits beside the scene, never on it.
//
// It used to sit exactly on top of it: the panel was drawn at (5,27)-(138,153)
// with an 82%-opaque fill and the city map at (6,28)-(134,124). The map showed
// through the panel and the panel's words sat on the map, so a player could
// read neither — that is what "the dialog box covers the whole screen" was.
func TestThePanelNeverCoversTheScene(t *testing.T) {
	sceneRight := sceneX + world.ViewportWidth()*tilePixels
	// 原版把左方框放在視埠左邊，量表框放右邊，所以「不重疊」現在是面板整個在視埠左側。
	if panelX+panelW > sceneX {
		t.Fatalf("the panel runs to x=%d but the scene starts at x=%d, so they overlap",
			panelX+panelW, sceneX)
	}
	if sceneRight > frameViewportX+frameViewportW {
		t.Fatalf("the scene runs to x=%d, past the viewport box at x=%d",
			sceneRight, frameViewportX+frameViewportW)
	}
	if panelX+panelW > logicalWidth {
		t.Fatalf("the panel runs to x=%d, past the %d-wide screen", panelX+panelW, logicalWidth)
	}
	sceneBottom := sceneY + world.ViewportHeight()*tilePixels
	if sceneBottom > statusStripY {
		t.Fatalf("the scene runs to y=%d, into the message strip at y=%d", sceneBottom, statusStripY)
	}
}

// Every click band belongs to the panel it is drawn in. Bands are derived from
// the panel rectangle now, so this catches a band that was written out by hand
// and left behind when the panel moved.
func TestEveryPanelClickBandSitsInsideThePanel(t *testing.T) {
	for _, spot := range panelMouseHotspots() {
		// The title, library and outcome screens are full-frame menus with
		// their own layout; they do not draw the shared panel.
		if isMenuScreen(spot.screen) || spot.screen == game.ScreenOutcome {
			continue
		}
		if spot.minX < panelX || spot.maxX > panelX+panelW {
			t.Errorf("screen %q action %q has a band at x=%d..%d, outside the panel at x=%d..%d",
				spot.screen, spot.action, spot.minX, spot.maxX, panelX, panelX+panelW)
		}
	}
}

func TestWalkerPatternFollowsTheOriginalDirectionTable(t *testing.T) {
	cases := []struct {
		heading game.Heading
		parity  int
		want    int
	}{
		{game.HeadingNorth, 0, 6},
		{game.HeadingSouth, 0, 6},
		{game.HeadingEast, 0, 8},
		{game.HeadingWest, 0, 8},
		{game.HeadingNorth, 1, 6},
		{game.HeadingEast, 1, 8},
	}
	for _, c := range cases {
		got := walkerPattern(c.heading, game.Position{X: c.parity, Y: 0})
		if got != c.want {
			t.Fatalf("heading %v parity %d picked pattern %d, want %d", c.heading, c.parity, got, c.want)
		}
	}
}

func TestDrawGroundWalkerNeedsAReadableDisk(t *testing.T) {
	application := &app{groundSpritesRead: true}
	if application.drawGroundWalker(nil, 0, 0) {
		t.Fatal("drew a walker with no sprite table")
	}
}

// The slot list is picked by clicking a cell, which is not a menu band, so it
// needs its own check that the cells sit where drawLibrary paints them.
func TestLibrarySlotCellsCoverTheDrawnList(t *testing.T) {
	app := &app{scale: 1}
	for index := range prototypeLibrarySlots {
		x := librarySlotColX + 4
		row := index
		if index >= librarySlotRows {
			x = librarySlotColX2 + 4
			row = index - librarySlotRows
		}
		y := librarySlotsY + row*librarySlotStep + 4
		got, ok := app.librarySlotAtDisplay(x, y)
		if !ok || got != index {
			t.Fatalf("點 (%d, %d) 得到 (%d, %v)，應該是槽位 %d", x, y, got, ok, index)
		}
	}
	// The key row below the list is not a slot.
	if _, ok := app.librarySlotAtDisplay(librarySlotColX+4, libraryKeysY+4); ok {
		t.Fatal("鍵位列被當成槽位")
	}
}
