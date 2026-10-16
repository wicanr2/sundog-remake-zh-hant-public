package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// TestNewGameParksTheShipWhereThePlayerStands: 新局的船停在落地格上，玩家也站在
// 那一格——原版的 put_ship 與走路人一開始就在同一格（Heavy 快照的 (20,12)）。
func TestNewGameParksTheShipOnTheLandingSquare(t *testing.T) {
	state := NewState()
	if state.ShipCityID != state.CityID {
		t.Fatalf("新局的船停在 %q，玩家在 %q", state.ShipCityID, state.CityID)
	}
	if state.ShipPosition != state.Position {
		t.Fatalf("船在 %v，玩家在 %v", state.ShipPosition, state.Position)
	}
}

// TestWalkingOntoTheShipBoardsIt: 走進船那一格就上船（XMOVEONG:0x17 re_enter_ship）；
// 走開不會。
func TestWalkingOntoTheShipBoardsIt(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenGround

	away, err := reducer.Step(state, ActionMoveEast)
	if err != nil {
		t.Fatal(err)
	}
	if away.Screen != ScreenGround {
		t.Fatalf("走離開船卻進了 %s", away.Screen)
	}
	if away.Position == away.ShipPosition {
		t.Fatalf("往東走了卻還在船那一格 %v", away.Position)
	}

	back, err := reducer.Step(away, ActionMoveWest)
	if err != nil {
		t.Fatal(err)
	}
	if back.Screen != ScreenShipboard {
		t.Fatalf("走回船那一格之後在 %s，訊息 %q", back.Screen, back.MessageKey)
	}
	if back.MessageKey != "ui.message.boarded" {
		t.Fatalf("上船的訊息是 %q", back.MessageKey)
	}
}

// TestGroundBackDoesNotBoard: 地面是根畫面，`ActionBack`（ESC）不再回船，只留一句
// 提示——原版沒有「按一個鍵回船」這回事。
func TestGroundBackDoesNotBoard(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenGround
	next, err := reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if next.Screen != ScreenGround {
		t.Fatalf("地面按返回跑到 %s", next.Screen)
	}
	if next.MessageKey != "ui.message.walk_to_ship" {
		t.Fatalf("地面按返回的訊息是 %q", next.MessageKey)
	}
	if next.Landing.Phase != LandingIdle {
		t.Fatalf("地面按返回啟動了起飛過場：%q", next.Landing.Phase)
	}
}

// TestTubeLeavesTheShipBehind: 電車只送人。船留在原來的城市，人站在目的地的街上，
// 而目的地的落地格有 Uniteller，所以搭得回來。
func TestTubeLeavesTheShipBehind(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	here, _ := reducer.World.Destination(state.CityID)
	target := ""
	for _, there := range reducer.World.Destinations() {
		if there.PlanetID == here.PlanetID && there.ID != here.ID {
			target = there.ID
			break
		}
	}
	if target == "" {
		t.Skip("這顆行星上沒有第二座城市")
	}
	state.Screen = ScreenTubeStation
	state.TubePage, state.TubeTargetID = TubePageConfirm, target
	state.Cash = 100000

	next, err := reducer.Step(state, ActionRideTube)
	if err != nil {
		t.Fatal(err)
	}
	if next.CityID != target {
		t.Fatalf("電車把人送到 %q", next.CityID)
	}
	if next.ShipCityID != state.CityID {
		t.Fatalf("船跟著電車走到 %q 了", next.ShipCityID)
	}
	if next.Screen != ScreenGround {
		t.Fatalf("下電車之後在 %s，應該是站在街上", next.Screen)
	}
	// 搭得回來：目的地的落地格有 Uniteller。
	if !reducer.World.HasFacility(next.CityID, next.Position.X, next.Position.Y, world.FacilityUniteller) {
		t.Fatal("電車的終點沒有 Uniteller，回不去了")
	}
	// 船不在這座城市，所以這裡不畫船、也走不上船。
	moved, err := reducer.Step(next, ActionMoveEast)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Screen == ScreenShipboard {
		t.Fatal("在沒有船的城市走一步就上船了")
	}
}

// TestFlyingTakesTheShipAlong: 飛行是船帶著人走，所以到站之後船跟人同格、人在船艙裡。
func TestFlyingTakesTheShipAlong(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := newGameWithFuel()
	// 此案例測航程／任務，先滿足原版非首城降落所需的掃描器。
	fitGroundScannerForTest(t, &state)
	state.Screen = ScreenShipboard
	state, err = reducer.Step(state, ActionOpenNavigation)
	if err != nil {
		t.Fatal(err)
	}
	state, err = reducer.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if state.Screen != ScreenShipboard {
		t.Fatalf("飛完之後在 %s", state.Screen)
	}
	if state.ShipCityID != state.CityID || state.ShipPosition != state.Position {
		t.Fatalf("飛完之後船在 %q%v，人在 %q%v",
			state.ShipCityID, state.ShipPosition, state.CityID, state.Position)
	}
}

// 舊存檔缺船與燃料排程位置，拒絕讀取，不猜補目前落地格。
func TestSchema26SaveWithoutShipPlaceIsRejected(t *testing.T) {
	old := NewState()
	old.Schema = 26
	old.ShipCityID = ""
	old.ShipPosition = Position{}
	if _, err := Migrate(old); err == nil {
		t.Fatal("舊保存被推測為新保存")
	}
}
