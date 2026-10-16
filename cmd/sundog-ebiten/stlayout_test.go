package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/prefs"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

func TestRemakePlanetMapKeepsAllSitesInsidePanel(t *testing.T) {
	check := func(d world.Destination) {
		t.Helper()
		x, y := remakePlanetMapPoint(d)
		if x-4 < routeMapX || x+4 >= routeMapX+routeMapWidth || y-4 < routeMapY || y+4 >= routeMapY+routeMapHeight {
			t.Fatalf("來源座標(%d,%d)的標記(%v,%v)超出導航面板", d.MapX, d.MapY, x, y)
		}
	}
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range r.World.Destinations() {
		check(d)
	}
	for row := 0; row < 24; row++ {
		for column := 0; column < 40; column++ {
			// 原始change_xy投影，包含首末列；不只抽樣現有城市。
			x := 7*((column+1)/2) + 4
			if column%2 != 0 {
				x += 3
			}
			check(world.Destination{MapX: x, MapY: 4*row + 2})
		}
	}
	s := game.NewState()
	s.ColonySite = game.ColonySite{Position: game.Position{X: 37, Y: 9}}
	d, ok := r.Destination(s, world.MissionDestinationID)
	if !ok || d.MapX != 140 || d.MapY != 38 {
		t.Fatal("保存的新城址未接導航", d)
	}
	check(d)
	x, y := remakePlanetMapPoint(d)
	old, _ := r.World.Destination(world.MissionDestinationID)
	ox, oy := remakePlanetMapPoint(old)
	if x == ox && y == oy {
		t.Fatal("新城址標記仍在原始位置")
	}
}

func TestColonySiteInvalidatesOnlySurfaceScene(t *testing.T) {
	for _, surface := range []bool{false, true} {
		a := newOptionsTestApp(t)
		a.state.GroundWorld = surface
		scene := &stGroundScene{world: surface, cityID: a.state.CityID, colonySite: a.state.ColonySite, shipCity: a.state.ShipCityID, shipPosition: a.state.ShipPosition}
		a.stGroundScene = scene
		if got := a.stGroundSceneFor(a.state.CityID, 0, 0); got != scene {
			t.Fatal("相同城址重畫")
		}
		a.state.ColonySite = game.ColonySite{Position: game.Position{X: 3, Y: 3}}
		before := a.state
		got := a.stGroundSceneFor(a.state.CityID, 0, 0)
		if surface && got == scene {
			t.Fatal("新城址沿用旧地表")
		}
		if !surface && got != scene {
			t.Fatal("行星城址使城市內部重畫")
		}
		if a.state.ColonySite != before.ColonySite || a.state.CombatRolls != before.CombatRolls {
			t.Fatal("繪圖重新選址")
		}
	}
}

// 明示受控鏡頭前綴，驗ST滑鼠／鍵盤共用的捲動入口，不冒稱正常出城路線。
func TestSTScrollExitSelectsOnceAndLeavesPlayerAtOldCity(t *testing.T) {
	for _, cityID := range []string{world.StartDestinationID, world.MissionDestinationID} {
		a := newOptionsTestApp(t)
		a.state.Screen = game.ScreenGround
		a.state.CityID = cityID
		d, ok := a.reducer.Destination(a.state, cityID)
		if !ok {
			t.Fatal("缺出城城市")
		}
		left := 1
		if cityID == world.StartDestinationID {
			left = -8
		}
		a.groundWalker = stGroundWalker{ready: true, cityID: cityID, scrollX: left, scrollY: 3, x: 39, y: 100}
		before := a.state
		a.scrollSTGround()
		if !a.state.GroundWorld || a.state.ColonySite.Pending || a.state.Position != (game.Position{X: d.MapColumn, Y: d.MapRow}) {
			t.Fatal("ST捲動未按舊址出城再選址", cityID, a.state.Position, a.state.ColonySite)
		}
		if a.state.Colony != before.Colony || a.state.ShipPosition != before.ShipPosition || a.state.CargoPosition != before.CargoPosition {
			t.Fatal("選址搬動船、貨艙或進度")
		}
		selected, rolls := a.state.ColonySite, a.state.CombatRolls
		a.groundWalker.x = 39
		a.scrollSTGround()
		if a.state.ColonySite != selected || a.state.CombatRolls != rolls {
			t.Fatal("後續地表捲動重抽位置")
		}
	}
}

func TestBanvilleGrowthInvalidatesGroundScene(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.CityID = world.MissionDestinationID
	scene := &stGroundScene{cityID: a.state.CityID, colonyPhase: 0,
		shipCity: a.state.ShipCityID, shipPosition: a.state.ShipPosition}
	a.stGroundScene = scene
	if got := a.stGroundSceneFor(a.state.CityID, 0, 0); got != scene {
		t.Fatal("相同階段不應重畫")
	}
	a.state.Colony.Phase = 1
	if got := a.stGroundSceneFor(a.state.CityID, 0, 0); got == scene {
		t.Fatal("換期沿用舊城市圖")
	}
	// 沒有原版磁碟時可安全fallback；測試不偽造原版圖塊。
	if a.state.Colony.Phase != 1 {
		t.Fatal("重画改變進度")
	}
}

// TestLayoutDefaultsToAtariST: 新局與空設定檔落在 Atari ST 版面；舊存檔拒絕。
// 設定檔記過的預設會贏。版面不再是獨立的一軸，它跟著預設走。
func TestLayoutDefaultsToAtariST(t *testing.T) {
	if got := game.NewState().Layout; got != theme.AtariSTLayout {
		t.Fatalf("新局的版面是 %q", got)
	}
	old := game.NewState()
	old.Schema, old.Layout = 22, ""
	if _, err := game.Migrate(old); err == nil {
		t.Fatal("舊存檔應被拒絕")
	}
	a := newOptionsTestApp(t)
	a.state.Layout = ""
	applyPreferences(a, prefs.Preferences{})
	if a.state.Layout != theme.AtariSTLayout {
		t.Fatalf("沒有偏好時給了 %q", a.state.Layout)
	}
	applyPreferences(a, prefs.Preferences{Presentation: string(theme.AppleIIPresentation)})
	if a.state.Layout != theme.RemakeLayout {
		t.Fatalf("Apple II的版面是 %q", a.state.Layout)
	}
	applyPreferences(a, prefs.Preferences{Presentation: "no-such-look", Layout: "no-such-layout"})
	if a.state.Layout != theme.AtariSTLayout {
		t.Fatalf("不認得的名字給了 %q", a.state.Layout)
	}
}

// TestPanelRowsFollowTheLayout: under the Atari ST layout the button rows are
// the original's menu rows (21 + 13n) and the click bands are built for it;
// the remake layout keeps its own rows. Both tables stay reachable.
func TestPanelRowsFollowTheLayout(t *testing.T) {
	defer setPanelLayout(theme.RemakeLayout)
	setPanelLayout(theme.AtariSTLayout)
	y, ok := panelRowYFor(game.ScreenGround, 0)
	if !ok || y != stMenuRowY(0) {
		t.Fatalf("ST ground row 0 at %d, want %d", y, stMenuRowY(0))
	}
	if y2, _ := panelRowYFor(game.ScreenTactical, 5); y2 != stMenuRowY(0) {
		t.Fatalf("ST tactical first button at %d, want %d", y2, stMenuRowY(0))
	}
	st := panelMouseHotspots()
	setPanelLayout(theme.RemakeLayout)
	remake := panelMouseHotspots()
	if y3, _ := panelRowYFor(game.ScreenGround, 0); y3 != panelRowY0 {
		t.Fatalf("remake ground row 0 at %d, want %d", y3, panelRowY0)
	}
	var stBand, remakeBand *mouseHotspot
	for i := range st {
		if st[i].screen == game.ScreenGround && st[i].action == game.ActionOpenDepot {
			stBand = &st[i]
		}
	}
	for i := range remake {
		if remake[i].screen == game.ScreenGround && remake[i].action == game.ActionOpenDepot {
			remakeBand = &remake[i]
		}
	}
	if stBand == nil || remakeBand == nil || stBand.minY == remakeBand.minY {
		t.Fatalf("bands did not follow the layout: %v vs %v", stBand, remakeBand)
	}
	if stBand.minY != stMenuRowY(0) || stBand.maxY-stBand.minY != stMenuPitch {
		t.Fatalf("ST band %v does not cover its drawn row", stBand)
	}
}

// TestPresentationCycleRemembersTheLayout: 預設換過去，版面跟著換，而且立刻落進設定檔。
func TestPresentationCycleRemembersTheLayout(t *testing.T) {
	a := newOptionsTestApp(t)
	a.lookChosen = true
	a.state = game.WithPresentation(a.state, theme.DefaultPresentation())
	a.cyclePresentation(1)
	if a.state.Layout != theme.AtariSTLayout || a.state.Tileset != theme.AtariSTHDTiles {
		t.Fatalf("換一個預設之後版面是 %q", a.state.Layout)
	}
	saved, err := prefs.Load(a.settingsPath)
	if err != nil || saved.Layout != string(theme.AtariSTLayout) || saved.Presentation != string(theme.AtariSTHDPresentation) {
		t.Fatalf("設定檔記的版面是 %q（%v）", saved.Layout, err)
	}
	for range theme.Presentations()[1:] {
		a.cyclePresentation(1)
	}
	if a.state.Layout != theme.AtariSTLayout {
		t.Fatalf("繞一圈之後版面是 %q", a.state.Layout)
	}
}

// TestSTMenuRowsFitTheLeftBox: eight rows of 13 from 21 end inside the box.
func TestSTMenuRowsFitTheLeftBox(t *testing.T) {
	if last := stMenuY0 + (stMenuRows-1)*stMenuPitch + stMenuH; last > frameLeftY+frameLeftH {
		t.Fatalf("row 8 ends at %d, box ends at %d", last, frameLeftY+frameLeftH)
	}
}

// TestSTStationsAreTheWindowTable: the rectangles are windows 65–77 as the table
// has them, minus 67 — get_location's loop skips 0x43 outright. A click inside
// lands on the station, one outside on nothing, and the three that lead
// somewhere other than repairs are named, including Stores selector6.
func TestSTStationsAreTheWindowTable(t *testing.T) {
	if len(stStations) != 12 || stStations[0].window != 65 || stStations[11].window != 77 {
		t.Fatalf("stations are not windows 65–77 without 67: %+v", stStations)
	}
	for _, station := range stStations {
		if station.window == 67 {
			t.Fatal("window 67 is in the table; get_location skips it")
		}
	}
	if station, ok := stStationAt(74, 113); !ok || station.action != game.ActionOpenGround {
		t.Fatalf("the airlock (68) gives %+v %v", station, ok)
	}
	if station, ok := stStationAt(62, 95); !ok || station.action != game.ActionOpenNavigation {
		t.Fatalf("the chair (65) gives %+v %v", station, ok)
	}
	if _, ok := stStationAt(0, 0); ok {
		t.Fatal("the hull is a station")
	}
	named := 0
	for _, station := range stStations {
		if station.labelKey != "" {
			named++
		}
	}
	if named != 4 {
		t.Fatalf("%d stations named, want the four with their own screens", named)
	}
	if station, ok := stStationAt(74, 72); !ok || station.action != game.ActionOpenStores {
		t.Fatal("selector6 station still opens repair")
	}
}

// TestSTShipboardNeedsThePicture: without the disk the shipboard stays framed.
func TestSTShipboardNeedsThePicture(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.Screen = game.ScreenShipboard
	if a.usesSTShipboard() {
		t.Fatal("full-screen shipboard without a disk")
	}
}

// TestSTGroundScrollIsTheOriginals: the walker is the cell four in and two
// down, unclamped — Drahew's origin (7,4) for a walker at (11,6), Heavy's
// (16,10) for the landing square (20,12).
func TestSTGroundScrollIsTheOriginals(t *testing.T) {
	if x, y := stGroundOrigin(11, 6); x != 7 || y != 4 {
		t.Fatalf("Drahew origin (%d,%d), want (7,4)", x, y)
	}
	if x, y := stGroundOrigin(20, 12); x != 16 || y != 10 {
		t.Fatalf("Heavy origin (%d,%d), want (16,10)", x, y)
	}
	if x, y := stGroundOrigin(1, 0); x != -3 || y != -2 {
		t.Fatalf("an origin off the map was clamped: (%d,%d)", x, y)
	}
	if stGroundColumns != 8 || stGroundRows != 5 {
		t.Fatalf("the screen holds %d×%d cells, want 8×5", stGroundColumns, stGroundRows)
	}
}

// TestSTGroundClickWalksTowardTheCell: a click right of the walker walks east.
func TestSTGroundClickWalksTowardTheCell(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.Position = game.Position{X: 11, Y: 6}
	if got := a.stGroundActionAt(5*stGroundPitch+1, 2*stGroundPitch+1); got != game.ActionMoveEast {
		t.Fatalf("a click one cell east gives %q", got)
	}
	if got := a.stGroundActionAt(4*stGroundPitch+1, 1*stGroundPitch+1); got != game.ActionMoveNorth {
		t.Fatalf("a click one cell north gives %q", got)
	}
}

// TestSTOnGroundScreens: the windows over the city are the ground's own
// screens, not the ship's.
func TestSTOnGroundScreens(t *testing.T) {
	for _, screen := range []game.Screen{game.ScreenTrade, game.ScreenUniteller, game.ScreenDialog, game.ScreenInterior} {
		if !stOnGround(screen) {
			t.Fatalf("%s is not over the city", screen)
		}
	}
	for _, screen := range []game.Screen{game.ScreenGround, game.ScreenShipboard, game.ScreenNavigation, game.ScreenTactical, game.ScreenCargo} {
		if stOnGround(screen) {
			t.Fatalf("%s is over the city", screen)
		}
	}
}

// TestSTCharacterStatusKeepsItsContext: the same card can be opened aboard
// ship or on the ground; its saved return screen also selects the backdrop.
func TestSTCharacterStatusKeepsItsContext(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.Screen = game.ScreenCharacterStatus
	if stOnGround(game.ScreenCharacterStatus) {
		t.Fatal("character status was classified as unconditionally on ground")
	}
	a.state.StatusReturn = game.ScreenShipboard
	if a.usesSTGroundBackdrop() {
		t.Fatal("shipboard character status selected the city backdrop")
	}
	a.state.StatusReturn = game.ScreenGround
	if a.usesSTShipboardBackdrop() {
		t.Fatal("ground character status selected the shipboard backdrop")
	}
}

// TestSTZoomIsTheDepotWindow: a window with nothing to show and three
// choices is the Drahew depot's, edge to edge (hatari-live-run.md, round
// twenty), and a click on a band runs the panel row's action.
func TestSTZoomIsTheDepotWindow(t *testing.T) {
	window := stZoomFor(game.ScreenUniteller, 0, 3, 0)
	if window.x0 != 72 || window.x1 != 231 || window.y0 != 24 || window.y1 != 123 {
		t.Fatalf("depot window %d..%d × %d..%d, want 72..231 × 24..123", window.x0, window.x1, window.y0, window.y1)
	}
	for i, want := range []int{81, 94, 107} {
		if got := window.bandY(i); got != want {
			t.Fatalf("band %d at y %d, want %d", i, got, want)
		}
	}
	if content := window.content(); content.y != 38 || content.h != 43 || content.x != 74 || content.w != 156 {
		t.Fatalf("depot fill %+v, want x 74 w 156 y 38 h 43", content)
	}
	if band, ok := window.bandAt(150, 100); !ok || band != 1 {
		t.Fatalf("(150,100) is band %d %v, want 1", band, ok)
	}
	if _, ok := window.bandAt(150, 60); ok {
		t.Fatal("the fill is not a band")
	}
	// make_zoom at the walker: every window is 160 wide and, with the
	// depot's few rows, 100 tall, at the corner its screen clamps to.
	for screen, want := range map[game.Screen][2]int{
		game.ScreenTrade: {152, 80}, game.ScreenCharacterStatus: {152, 72}, game.ScreenArena: {144, 72},
		game.ScreenInterior: {160, 88}, game.ScreenScore: {72, 24},
	} {
		w := stZoomFor(screen, 0, 3, 0)
		if w.x0 != want[0] || w.y0 != want[1] || w.x1-w.x0 != 159 || w.y1-w.y0 != 99 {
			t.Fatalf("%s window %d,%d %dx%d, want %v 160x100", screen, w.x0, w.y0, w.x1-w.x0+1, w.y1-w.y0+1, want)
		}
	}
	a := statusApp(t)
	a.state.Layout = theme.AtariSTLayout
	a.state.Screen = game.ScreenTrade
	zoom, _, readouts := a.stZoomOf()
	if readouts != 5 || zoom.bands < 2 {
		t.Fatalf("trade window has %d readouts and %d bands", readouts, zoom.bands)
	}
	if action, ok := a.stZoomActionAt(zoom.x0+80, zoom.bandY(1)+3); !ok || action != game.ActionSellGems {
		t.Fatalf("trade band 1 runs %q %v, want sell", action, ok)
	}
	if zoom.y1 > 199 || zoom.x1 > 319 {
		t.Fatalf("trade window runs off the screen: %d,%d", zoom.x1, zoom.y1)
	}
}

// TestSTLightsAreBayLights: the colours SHIPLIB:0x11 bay_lights picks, for
// a row's status byte and for the hull bar's height, and the parking places
// park_pods draws through -- the six vehicles the Drahew snapshot shows all
// sit on entries of the ordinary table at the cell's origin plus (7, 1).
func TestSTLightsAreBayLights(t *testing.T) {
	for status, want := range map[int]int{0: 0, 1: stColourRed, 2: stColourRed, 3: stColourYellow, 4: stColourGreen} {
		if got := stLightColour(status); got != want {
			t.Fatalf("status %d lights colour %d, want %d", status, got, want)
		}
	}
	for hull, want := range map[int]int{16: stColourGreen, 15: stColourYellow, 6: stColourYellow, 5: stColourRed} {
		if got := stHullColour(hull); got != want {
			t.Fatalf("hull %d bar colour %d, want %d", hull, got, want)
		}
	}
	seen := map[[2]int]bool{}
	for _, place := range stPodPlaces {
		seen[[2]int{place[0] + stPodOriginX, place[1] + stPodOriginY}] = true
	}
	for _, snapshot := range [][2]int{{17, 8}, {9, 20}, {33, 20}, {9, 32}, {17, 20}} {
		if !seen[snapshot] {
			t.Fatalf("the snapshot's vehicle at %v is on no parking place", snapshot)
		}
	}
	if len(stPodPlacesEnlie) != 6 || len(stPodPlaces) != 9 {
		t.Fatalf("%d and %d parking places, want 9 and 6", len(stPodPlaces), len(stPodPlacesEnlie))
	}
	if stPodParked(1, 2, 3, 0) || !stPodParked(1, 2, 3, stPodDieSides) {
		t.Fatal("the die ignores the density")
	}
}

// TestSTZoomShowsAFreshMessageThenTheMenu: a shop's answer takes the window
// for a moment (the bands stop answering clicks) and the menu is back after
// the delay; opening the screen never counts as an answer.
func TestSTZoomShowsAFreshMessageThenTheMenu(t *testing.T) {
	a := statusApp(t)
	a.state.Layout = theme.AtariSTLayout
	a.state.Screen = game.ScreenGround
	a.state.MessageKey = "ui.message.landing"
	a.frames = 10
	a.step(game.ActionOpenUniteller)
	if a.state.Screen != game.ScreenUniteller {
		t.Fatalf("screen %q after opening the bank", a.state.Screen)
	}
	if a.stZoomMessageFresh() {
		t.Fatal("opening a screen counted as an answer")
	}
	a.frames = 20
	a.step(game.ActionWithdrawCash)
	if !a.stZoomMessageFresh() {
		t.Fatalf("the bank's answer %q is not shown in the window", a.state.MessageKey)
	}
	a.frames = 20 + stZoomMessageFrames
	if a.stZoomMessageFresh() {
		t.Fatal("the answer outlived the delay")
	}
}

// TestMenuTableReproducesTheDepotBands: the depot's three bands measured on
// the original (round thirteen: frames at y 81, 94, 107 on a window at (72,
// 24), x 74..229) are style 6's boxes 1–3 through the table.
func TestMenuTableReproducesTheDepotBands(t *testing.T) {
	window := stZoomWindow{x0: 72, y0: 24, x1: 231, y1: 123}
	boxes := stMenuBoxes(6)
	want := []int{109, 96, 83}
	for i, top := range want {
		x0, y0, x1, _ := window.menuBoxRect(stMenuBaseFill, boxes[i])
		if y0 != top || x0-stBoxFrame != 74 || x1+stBoxFrame != 229 {
			t.Fatalf("box %d: inner top %d x %d..%d, want top %d x 74..229", i, y0, x0-stBoxFrame, x1+stBoxFrame, top)
		}
	}
	// The depot's answer starts at (77, 41): column 0, row 0 of the fill.
	if x, y := window.textCell(0, 0); x != 77 || y != 41 {
		t.Fatalf("text cell (0,0) = (%d,%d), want (77,41)", x, y)
	}
}

// TestMenuTableMatchesTheDecodedEvidence keeps the in-code menu table equal
// to docs/atari-st/window-menu-tables.json, the tool's decoding of the disk.
func TestMenuTableMatchesTheDecodedEvidence(t *testing.T) {
	raw, err := os.ReadFile("../../docs/atari-st/window-menu-tables.json")
	if err != nil {
		t.Skip("evidence file not beside the tests:", err)
	}
	var doc struct {
		MenuDirectory struct {
			Menus []struct{ Menu, Start, Count int } `json:"menus"`
		} `json:"menuDirectory"`
		MenuItems struct {
			Items []struct{ Item, X0, X1, Y0, Y1 int } `json:"items"`
		} `json:"menuItems"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, menu := range doc.MenuDirectory.Menus {
		boxes := stMenuBoxes(menu.Menu)
		if len(boxes) != menu.Count {
			t.Fatalf("menu %d has %d boxes in code, %d in the evidence", menu.Menu, len(boxes), menu.Count)
		}
		for i, box := range boxes {
			item := doc.MenuItems.Items[menu.Start+i]
			if box != (stMenuBox{item.X0, item.X1, item.Y0, item.Y1}) {
				t.Fatalf("menu %d box %d = %+v, evidence item %d = %+v", menu.Menu, i, box, item.Item, item)
			}
		}
	}
}

// TestGroundShipSitsOnTheLandingSquare: put_ship draws the SunDog on the cell
// the player lands on, and only while that cell is inside the eight-by-five
// view — the original's own two guards (XMOVEONG:0x07: the ship's city, then
// diff(...) < 8 and < 5).
func TestGroundShipSitsOnTheLandingSquare(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.CityID = world.StartDestinationID
	landing := world.LandingPosition(a.state.CityID)

	// 站在落地格上：走路人固定在畫面 (4,2)，船就畫在同一格。
	a.state.Position = game.Position{X: landing.X, Y: landing.Y}
	originX, originY := stGroundOrigin(a.state.Position.X, a.state.Position.Y)
	column, row, ok := a.groundShipCell(originX, originY, stGroundColumns, stGroundRows)
	if !ok || column != stGroundOriginLeft || row != stGroundOriginTop {
		t.Fatalf("船畫在畫面格 (%d,%d) ok=%v，應該是 (%d,%d)",
			column, row, ok, stGroundOriginLeft, stGroundOriginTop)
	}

	// 走遠到落地格離開視窗就不畫，不能畫在邊界上。
	a.state.Position = game.Position{X: landing.X + stGroundColumns, Y: landing.Y + stGroundRows}
	originX, originY = stGroundOrigin(a.state.Position.X, a.state.Position.Y)
	if _, _, ok := a.groundShipCell(originX, originY, stGroundColumns, stGroundRows); ok {
		t.Fatal("落地格已經捲出畫面，船還是畫了")
	}
}

// TestGroundShipIsTwoCharactersCentredInTheCell: the pair covers twelve by
// eight pixels, so the anchors put that box in the middle of the cell.
func TestGroundShipIsTwoCharactersCentredInTheCell(t *testing.T) {
	if stCharShipGroundTail-stCharShipGroundNose != 1 || stGroundShipGap != 6 {
		t.Fatalf("字元對是 %d/%d、間距 %d，原版是相鄰兩字元、間距 6",
			stCharShipGroundNose, stCharShipGroundTail, stGroundShipGap)
	}
	// drawSTChar 把字元錨在 (x-3, y-4)，所以字元對佔 x-3..x+8、y-4..y+3。
	x := 0 + (stGroundPitch-stGroundShipWidth)/2 + 3
	y := 0 + (stGroundPitch-stGroundShipHeight)/2 + 4
	left, right := x-3, x+stGroundShipGap+2
	top, bottom := y-4, y+3
	if left != (stGroundPitch-stGroundShipWidth)/2 || right != left+stGroundShipWidth-1 {
		t.Fatalf("字元對橫向佔 %d..%d，格寬 %d", left, right, stGroundPitch)
	}
	if top != (stGroundPitch-stGroundShipHeight)/2 || bottom != top+stGroundShipHeight-1 {
		t.Fatalf("字元對縱向佔 %d..%d，格高 %d", top, bottom, stGroundPitch)
	}
}

// TestSystemMapShipUsesItsOwnPolarPosition pins what the tenth round of the UX
// ledger could not check from a screenshot: the ship on the system map is drawn
// at its own polar position, not at the planet's.
//
// The two are equal while parked — state.go fills a zero flight radius in from
// the planet — so a parked capture cannot tell the two implementations apart.
// This drives them apart the way flight does.
func TestSystemMapShipUsesItsOwnPolarPosition(t *testing.T) {
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := game.NewState()
	planets := reducer.World.PlanetsInSystem(state.SystemID)
	if len(planets) == 0 {
		t.Fatal("the start system has no planets")
	}
	here := reducer.PlanetOrbit(state, planets[0])

	// In flight the ship is somewhere else on the disc.
	state.PlanetID = planets[0].ID
	state.FlightAngle = (here.Angle + 90) % 360
	state.FlightRadius = here.Radius/2 + 3

	shipX, shipY := systemMapShipOffset(state)
	planetX, planetY := game.PolarToRect(here.Angle, here.Radius)
	if shipX == planetX && shipY == planetY {
		t.Fatalf("ship drawn on the planet: ship (%d,%d), planet (%d,%d)",
			shipX, shipY, planetX, planetY)
	}
	wantX, wantY := game.PolarToRect(state.FlightAngle, state.FlightRadius)
	if shipX != wantX || shipY != wantY {
		t.Fatalf("ship at (%d,%d), want its own polar position (%d,%d)",
			shipX, shipY, wantX, wantY)
	}

	// Parked, the two agree — which is why the capture could not tell.
	state.FlightAngle, state.FlightRadius = here.Angle, here.Radius
	if x, y := systemMapShipOffset(state); x != planetX || y != planetY {
		t.Errorf("parked ship at (%d,%d), want the planet's (%d,%d)", x, y, planetX, planetY)
	}
}

// TestMenuTableReproducesTheTubeStationBoxes：原版電車站的四個框（2026-09-06 的
// probe 擷取，視窗邊在 (88,72)）是 style 9 的前四個框，上排內框上緣 144、
// 下排 157，左欄 90..169、右欄 167..245（含 2 像素外框）。
func TestMenuTableReproducesTheTubeStationBoxes(t *testing.T) {
	window := stZoomWindow{x0: 88, y0: 72, x1: 247, y1: 171}
	boxes := stMenuBoxes(9)
	if got := stTellerPickStyle(4); got != 9 {
		t.Fatalf("四個目的地挑到 style %d，原版量到的是 style 9", got)
	}
	for i, want := range []struct{ top, left, right int }{
		{157, 90, 169}, // 下排左
		{157, 167, 245},
		{144, 90, 169}, // 上排左
		{144, 167, 245},
	} {
		x0, y0, x1, _ := window.menuBoxRect(stMenuBaseFill, boxes[i])
		if y0 != want.top || x0-stBoxFrame != want.left || x1+stBoxFrame != want.right {
			t.Fatalf("框 %d：內框上緣 %d、x %d..%d，原版是上緣 %d、x %d..%d",
				i, y0, x0-stBoxFrame, x1+stBoxFrame, want.top, want.left, want.right)
		}
	}
	// 提示的兩行在 fill 的第 0、1 列，都在框上方。
	_, promptTop := window.textCell(0, 0)
	_, promptBottom := window.textCell(0, 1)
	_, firstBoxTop, _, _ := window.menuBoxRect(stMenuBaseFill, boxes[3])
	if promptBottom+stTextCellH > firstBoxTop-stBoxFrame {
		t.Fatalf("提示第二行（%d..%d）碰到最上面那個框（%d）", promptBottom, promptBottom+stTextCellH, firstBoxTop-stBoxFrame)
	}
	if promptTop != 89 {
		t.Fatalf("提示第一行在 y=%d，fill 起點加 17 應該是 89", promptTop)
	}
}

// TestTheTellerBalanceIsNotCoveredByABox：Uniteller 的第四個框會蓋住
// `In bank:` 那一列，所以它只能放 PAY FINE——同時出現的框最多三個。
func TestTheTellerBalanceIsNotCoveredByABox(t *testing.T) {
	window := stZoomWindow{x0: 72, y0: 24, x1: 231, y1: 123}
	_, balanceTop := window.textCell(1, 3)
	_, fourthTop, _, _ := window.menuBoxRect(stMenuBaseFill, stMenuBoxes(6)[3])
	if fourthTop-stBoxFrame >= balanceTop+stTextCellH {
		t.Fatal("第四個框不再與餘額那一列重疊——這條測試的前提沒了，重寫它")
	}
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	labels, face := auditCatalogAndFace(t)
	state := game.NewState()
	state.Screen = game.ScreenUniteller
	state.Cash = 500
	state.Bank = 36306
	state.PodParkedAt = 0
	state.TimeMinutes = 10000 // 罰款一定跑起來了
	a := &app{reducer: reducer, state: state, labels: labels, face: face}
	if a.reducer.ParkingFine(a.state) <= 0 {
		t.Fatal("這個狀態沒有欠罰款，測不到第四個框")
	}
	boxes := a.stTellerBoxes()
	if _, ok := boxes[3]; !ok {
		t.Fatal("欠罰款時第四個框應該是 PAY FINE")
	}
	if _, ok := boxes[1]; ok {
		t.Fatal("PAY FINE 出現時不該同時有 WITHDRAW CASH")
	}
	state.PodParkedAt = -1
	a.state = state
	if _, ok := a.stTellerBoxes()[3]; ok {
		t.Fatal("沒有欠罰款時第四個框應該空著，否則會蓋住餘額")
	}
}

// 有效確認只接受menu2兩項；無交易只接受menu0項2，其餘原style0框不得隱形生效。
func TestSTTradeConfirmationOnlyVisibleActions(t *testing.T) {
	for _, banville := range []bool{false, true} {
		for _, available := range []bool{false, true} {
			a := &app{state: game.NewState()}
			a.state.Screen, a.state.TradePage = game.ScreenTrade, game.TradePageSellConfirm
			a.state.TradeSaleSlot = 5
			goods := 8
			if !available && !banville {
				goods = 1
			}
			a.state.Holds[0] = game.Hold{Kind: game.HoldGoods, Goods: goods}
			if banville {
				a.state.CityID = "banville"
				a.state.Colony = game.ColonyProgress{Initialized: true, Phase: 0, NextPhase: 1}
				if available {
					a.state.Colony.Remaining[goods] = 1
				}
			}
			w := a.stTradeWindow()
			if available {
				for i, box := range stMenuBoxes(2) {
					x0, y0, x1, y1 := w.menuBoxRect(stMenuBaseInside, box)
					got, ok := a.stTradeActionAt((x0+x1)/2, (y0+y1)/2)
					want := []game.Action{game.ActionTradePick1, game.ActionTradePick2}[i]
					if !ok || got != want {
						t.Fatalf("confirmation %v/%v item%d: %s", banville, available, i, got)
					}
				}
			} else {
				for i, box := range stMenuBoxes(0) {
					x0, y0, x1, y1 := w.menuBoxRect(stMenuBaseInside, box)
					got, ok := a.stTradeActionAt((x0+x1)/2, (y0+y1)/2)
					if i == 1 {
						if !ok || got != game.ActionTradePick2 {
							t.Fatal("Exit missing")
						}
					} else if ok {
						t.Fatal("invisible disabled action")
					}
				}
			}
		}
	}
}
