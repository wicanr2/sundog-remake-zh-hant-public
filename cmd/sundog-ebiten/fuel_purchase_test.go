package main

// 完整燃料驗收入口：docs/atari-st/ship-rules.md，Issue #8。
// 以下為正式介面元件測試，不代替正常滑鼠玩家路徑或原版全機對拍。
import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/storage"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// 原版門檻逐筆答案由Talos生成；正式入口讀相同模式／偏移，不能用獨立公式代替。
func TestExportTalosFuelGroundGate(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("requires native ground gate receipt")
	}
	var native struct {
		Marks []struct {
			Foot       bool
			SpriteKind int
		}
		Board []struct {
			Foot       bool
			Kind, X, Y int
			Accepted   bool
		}
		Depot []struct {
			Foot     bool
			X, Y     int
			Accepted bool
		}
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-fuel-ground-gate.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Marks) != 2 || len(native.Board) != 2310 || len(native.Depot) != 612 {
		t.Fatal("incomplete native gate inputs")
	}
	report := struct {
		MarksCases, MarksDifferences, BoardCases, BoardDifferences, DepotCases, DepotDifferences int
		Samples                                                                                  []string
	}{}
	setup := func(foot bool) *app {
		a := newOptionsTestApp(t)
		a.state.Screen = game.ScreenGround
		if !foot {
			a.state.GroundMode = game.GroundInPod
		}
		return a
	}
	for _, row := range native.Marks {
		a := setup(row.Foot)
		a.resetSTGroundWalker()
		report.MarksCases++
		if a.stGroundModePattern() != row.SpriteKind {
			report.MarksDifferences++
		}
	}
	for _, row := range native.Board {
		a := setup(row.Foot)
		a.groundWalker = stGroundWalker{ready: true, pattern: row.Kind, x: row.X - originalmedia.CollideAnchorOffset, y: row.Y - originalmedia.CollideAnchorOffset}
		a.checkSTGroundExit()
		report.BoardCases++
		if (a.state.Screen == game.ScreenShipboard) != row.Accepted {
			report.BoardDifferences++
		}
	}
	for _, row := range native.Depot {
		a := setup(row.Foot)
		a.state.Position = game.Position{X: 3, Y: 0}
		a.groundWalker = stGroundWalker{ready: true, pattern: 2, x: row.X - originalmedia.CollideAnchorOffset, y: row.Y - originalmedia.CollideAnchorOffset}
		a.checkSTGroundExit()
		report.DepotCases++
		if (a.state.Screen == game.ScreenDepot) != row.Accepted {
			report.DepotDifferences++
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-fuel-ground-gate.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.MarksDifferences+report.BoardDifferences+report.DepotDifferences != 0 {
		t.Fatalf("formal gate differences: marks=%d board=%d depot=%d", report.MarksDifferences, report.BoardDifferences, report.DepotDifferences)
	}
}

func TestCargoPodExitStartsAtTraversablePointFromPlayerDisk(t *testing.T) {
	diskPath := os.Getenv("SUNDOG_PARITY_DISK")
	if diskPath == "" {
		t.Skip("requires player's original disk for actual background collision")
	}
	disk, err := originalmedia.Open(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	a := newOptionsTestApp(t)
	a.originalDisk, a.originalTiles = disk, map[int]*originalTileCache{}
	a.state.Screen = game.ScreenCargo
	a.step(game.ActionExitCargoPod)
	a.resetSTGroundWalker()
	x, y := a.groundWalker.anchor()
	scene := a.stGroundSceneFor(a.state.CityID, a.groundWalker.scrollX, a.groundWalker.scrollY)
	patterns, ok := a.collidePatterns()
	if !ok || scene == nil {
		t.Fatal("original collision background missing")
	}
	if !scene.canStand(patterns, 2, x, y) {
		t.Fatalf("pod starts inside a background obstacle at %d,%d", x, y)
	}
	// 起點必須有正常離船路徑；某一方向可否走由實際方向圖樣與像素決定。
	// 舊測試固定要求向南，前提來自誤接的圖樣1，沒有原版證據。
	escaped := false
	for _, action := range []game.Action{game.ActionMoveNorth, game.ActionMoveSouth, game.ActionMoveWest, game.ActionMoveEast} {
		trial := *a
		before := trial.state.GroundView.Anchor
		trial.nudgeSTGroundWalker(action)
		if trial.state.Screen != game.ScreenGround || trial.state.GroundView.Anchor == before {
			continue
		}
		if trial.state.CargoPosition != trial.state.Position || trial.state.CargoAttached {
			t.Fatal("moving pod lost its detached location")
		}
		escaped = true
	}
	if !escaped {
		t.Fatal("pod exit has no normal directional escape")
	}
}

// 真實Drahew地圖的depot底圖為0，建築槽35正規化成19。
// 此測試覆蓋入口consumer與含邊界門檻，不代替正常玩家步行。
func TestDepotDoorReadsBuildingOverlayBeforeTerrain(t *testing.T) {
	for _, tc := range []struct {
		name string
		x, y int
		want game.Screen
	}{
		{"門中心", 15, 16, game.ScreenDepot},
		{"含邊界", 11, 20, game.ScreenDepot},
		{"門外", 10, 20, game.ScreenGround},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newOptionsTestApp(t)
			a.state.Screen = game.ScreenGround
			depot, ok := a.reducer.World.FacilityPosition(a.state.CityID, world.FacilityDepot)
			if !ok || depot.X != 3 || depot.Y != 0 {
				t.Fatal("Drahew source depot changed")
			}
			code, _, ok := world.TerrainCode(a.state.CityID, depot.X, depot.Y)
			if !ok || code != 0 {
				t.Fatal("test no longer exposes the building overlay gap")
			}
			a.state.Position = game.Position{X: depot.X, Y: depot.Y}
			a.groundWalker = stGroundWalker{ready: true,
				x: tc.x - originalmedia.CollideAnchorOffset,
				y: tc.y - originalmedia.CollideAnchorOffset}
			a.checkSTGroundExit()
			if a.state.Screen != tc.want {
				t.Fatalf("screen=%s want=%s", a.state.Screen, tc.want)
			}
			if tc.want == game.ScreenDepot {
				a.step(game.ActionRefuel)
				if a.state.Screen != game.ScreenFuelPurchase || a.state.FuelPurchase.UnitPrice != 157 {
					t.Fatalf("depot child lost: %+v", a.state.FuelPurchase)
				}
			}
		})
	}
}

func TestDepotMenuKeysAndButtons(t *testing.T) {
	for _, city := range []string{"drahew", "banville"} {
		a := newOptionsTestApp(t)
		a.state.CityID = city
		a.state.Screen = game.ScreenDepot
		setPanelLayout(theme.AtariSTLayout)
		count := 3
		if city == "banville" {
			count = 4
		}
		if len(a.depotPanelLines()) != count {
			t.Fatal("depot option count", city)
		}
		for i, action := range []game.Action{game.ActionRefuel, game.ActionRepairHull, game.ActionLoadAuxFuel, game.ActionOpenFTLBoost} {
			key := []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4}[i]
			got, ok := keyAction(key, a.state)
			band := panelBand(game.ScreenDepot, i, action)
			mouse := a.mouseActionAt((band.minX+band.maxX)/2, (band.minY+band.maxY)/2)
			if i < count {
				if !ok || got != action || mouse != action {
					t.Fatal("depot key/mouse", city, i, got, mouse)
				}
			} else if ok || mouse != "" {
				t.Fatal("engineering visible outside Banville", got, mouse)
			}
		}
	}
}

func TestFuelPurchaseKeysAndButtonsPerformTheSameTransaction(t *testing.T) {
	for _, layout := range []theme.LayoutID{theme.AtariSTLayout, theme.RemakeLayout} {
		for _, tc := range []struct {
			key    ebiten.Key
			row    int
			action game.Action
		}{
			{ebiten.Key1, 4, game.ActionFuelBuyFull},
			{ebiten.Key2, 5, game.ActionFuelBuyOne},
			{ebiten.Key3, 6, game.ActionFuelCancel},
		} {
			a := newOptionsTestApp(t)
			a.state.Screen, a.state.Layout = game.ScreenGround, layout
			depot, ok := a.reducer.World.FacilityPosition(a.state.CityID, world.FacilityDepot)
			if !ok {
				t.Fatal("source depot missing")
			}
			a.state.Position = game.Position{X: depot.X, Y: depot.Y}
			a.state.Fuel, a.state.Bank, a.state.Cash = 0, 0, 200
			a.step(game.ActionRefuel)
			if a.state.Screen != game.ScreenFuelPurchase {
				t.Fatal("depot did not open")
			}
			setPanelLayout(layout)
			if action, ok := keyAction(tc.key, a.state); !ok || action != tc.action {
				t.Fatalf("key %s: %s", tc.key, action)
			}
			band := panelBand(game.ScreenFuelPurchase, tc.row, tc.action)
			if action := a.mouseActionAt((band.minX+band.maxX)/2, (band.minY+band.maxY)/2); action != tc.action {
				t.Fatalf("mouse row %d: %s", tc.row, action)
			}
			byKey, err := a.reducer.Step(a.state, tc.action)
			if err != nil {
				t.Fatal(err)
			}
			a.step(tc.action)
			if a.state.FuelPurchase != byKey.FuelPurchase || a.state.Fuel != byKey.Fuel || a.state.Cash != byKey.Cash || a.state.Screen != byKey.Screen {
				t.Fatal("key and mouse transactions differ")
			}
		}
	}
}

func TestOldLibraryShowsExplicitMessageAndDoesNotLoadOtherSave(t *testing.T) {
	a := newOptionsTestApp(t)
	a.libraryPath = filepath.Join(t.TempDir(), "old-library.json")
	a.librarySlot = "slot-1"
	if err := os.WriteFile(a.libraryPath, []byte(`{"format":"sundog-remake-prototype-library-v1","slots":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.savePath = filepath.Join(t.TempDir(), "current-save.json")
	other := game.NewState()
	other.Character = "Other save"
	if err := storage.Save(a.savePath, other); err != nil {
		t.Fatal(err)
	}
	before := a.state.Character
	a.loadLibraryGame()
	if a.state.MessageKey != "ui.message.old_save_unsupported" || a.state.Character != before {
		t.Fatal("old library was hidden by loading another game")
	}
	for _, action := range []game.Action{game.ActionMakeLibrary, game.ActionStoreGame, game.ActionDeleteGame} {
		a.step(action)
		if a.state.MessageKey != "ui.message.old_save_unsupported" {
			t.Fatalf("%s hid old-library rejection", action)
		}
	}
}

func TestCargoPodExitAndGroundPatternRemainIndependentOfTankContents(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.Screen = game.ScreenCargo
	a.state.Holds[0] = game.Hold{Kind: game.HoldAuxFuel, SourceCode: 0xab}
	if action, ok := keyAction(ebiten.KeyE, a.state); !ok || action != game.ActionExitCargoPod {
		t.Fatal("cargo pod exit key missing")
	}
	a.step(game.ActionExitCargoPod)
	if a.state.Screen != game.ScreenGround || a.state.GroundMode != game.GroundInPod || a.state.CargoAttached || a.stGroundModePattern() != 2 {
		t.Fatal("pod exit did not detach or change ground mode")
	}
	a.state.Position.X++
	game.SyncCargoGroundPosition(&a.state)
	parked := a.state.CargoPosition
	a.state.GroundMode = game.GroundOnFoot
	a.state.Position.X++
	game.SyncCargoGroundPosition(&a.state)
	if a.state.CargoPosition != parked || a.stGroundModePattern() != 4 || a.state.Holds[0].SourceCode != 0xab {
		t.Fatal("walking moved pod or contents changed mode")
	}
}
