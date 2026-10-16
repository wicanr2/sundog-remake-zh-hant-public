package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// MAINLIB:0x09 isqrt rounds down.
func TestIsqrtRoundsDown(t *testing.T) {
	for _, test := range []struct{ in, want int }{
		{0, 0}, {1, 1}, {2, 1}, {3, 1}, {4, 2}, {8, 2}, {9, 3}, {15, 3}, {16, 4}, {224, 14}, {225, 15},
	} {
		if got := isqrt(test.in); got != test.want {
			t.Fatalf("isqrt(%d) = %d, want %d", test.in, got, test.want)
		}
	}
	if got := isqrt(-4); got != 0 {
		t.Fatalf("isqrt(-4) = %d, want 0", got)
	}
}

// tube.fare: 2 × price level × isqrt(Δcolumn² + Δrow²), measured in the packed
// map fields rather than in pixels.
func TestTubeFareFollowsTheOriginalsFormula(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	here, ok := reducer.World.Destination(state.CityID)
	if !ok {
		t.Fatalf("the start city %q is missing", state.CityID)
	}
	system, ok := reducer.World.System(state.SystemID)
	if !ok {
		t.Fatal("the start system is missing")
	}
	checked := 0
	for _, there := range reducer.World.Destinations() {
		if there.PlanetID != here.PlanetID || there.ID == here.ID {
			continue
		}
		if there.Hidden {
			if _, ok := reducer.TubeFare(state, there.ID); ok {
				t.Fatal("城市區間外的Banville成為Tube目的地")
			}
			continue
		}
		dx := there.MapColumn - here.MapColumn
		dy := there.MapRow - here.MapRow
		want := tubeFareFactor * system.PriceLevel * isqrt(dx*dx+dy*dy)
		got, ok := reducer.TubeFare(state, there.ID)
		if !ok {
			t.Fatalf("the tube refuses %q, which shares the planet", there.ID)
		}
		if got != want {
			t.Fatalf("fare to %q = %d, want %d", there.ID, got, want)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("the start planet has no other city, so this proves nothing")
	}
}

// The tube does not leave the planet, and does not sell a ticket to where you
// already are.
func TestTubeOnlyRunsWithinOnePlanet(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	here, _ := reducer.World.Destination(state.CityID)
	if _, ok := reducer.TubeFare(state, state.CityID); ok {
		t.Fatal("the tube sold a ticket to the platform it leaves from")
	}
	offPlanet := ""
	for _, there := range reducer.World.Destinations() {
		if there.PlanetID != here.PlanetID {
			offPlanet = there.ID
			break
		}
	}
	if offPlanet == "" {
		t.Fatal("every city is on one planet, so this proves nothing")
	}
	if _, ok := reducer.TubeFare(state, offPlanet); ok {
		t.Fatalf("the tube offered to reach %q, which is on another planet", offPlanet)
	}
}

// Riding takes the fare and puts the player in the other city, with no flight
// and no fuel spent.
func TestRidingTheTubeMovesTheePlayerWithoutFlying(t *testing.T) {
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
	state.Screen = ScreenTubeStation
	state.TubePage, state.TubeTargetID = TubePageConfirm, target
	// 車資從銀行扣，不是從現金——原版比的與扣的都是那個星系的存款。
	state.Cash, state.Bank = 100000, 100000
	fare, ok := reducer.TubeFare(state, target)
	if !ok {
		t.Fatalf("no fare to %q", target)
	}
	next, err := reducer.Step(state, ActionRideTube)
	if err != nil {
		t.Fatal(err)
	}
	if next.CityID != target {
		t.Fatalf("the ride ended at %q, want %q", next.CityID, target)
	}
	if next.Cash != state.Cash {
		t.Fatalf("cash = %d, want %d（車資不該碰現金）", next.Cash, state.Cash)
	}
	if next.Bank != state.Bank-fare {
		t.Fatalf("bank = %d, want %d", next.Bank, state.Bank-fare)
	}
	if next.Fuel != state.Fuel {
		t.Fatalf("the tube burned %d fuel", state.Fuel-next.Fuel)
	}
	if next.Screen == ScreenFlight {
		t.Fatal("the tube went through the flight screen")
	}
}

func TestRidingTheTubeNeedsTheFare(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	here, _ := reducer.World.Destination(state.CityID)
	for _, there := range reducer.World.Destinations() {
		if there.PlanetID == here.PlanetID && there.ID != here.ID {
			state.NavigationTargetID = there.ID
			break
		}
	}
	state.Screen = ScreenTubeStation
	state.TubePage, state.TubeTargetID = TubePageConfirm, state.NavigationTargetID
	state.Cash = 0
	state.Bank = 0
	next, err := reducer.Step(state, ActionRideTube)
	if err != nil {
		t.Fatal(err)
	}
	if next.CityID != state.CityID || next.MessageKey != "ui.message.tube_insufficient" {
		t.Fatalf("a broke player still rode: %+v", next)
	}
}

// do_tube_station 的清單是同一顆行星的其他城市，每一條帶自己的車資。
func TestTubeStationListsThePlanetsOtherCities(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	here, ok := reducer.World.Destination(state.CityID)
	if !ok {
		t.Fatalf("起始城市 %q 不在世界裡", state.CityID)
	}
	want := map[string]int{}
	for _, there := range reducer.World.Destinations() {
		if there.PlanetID != here.PlanetID || there.CityID == here.CityID || there.Hidden {
			continue
		}
		fare, ok := reducer.TubeFare(state, there.ID)
		if !ok {
			t.Fatalf("同行星的 %q 卻沒有車資", there.ID)
		}
		want[there.ID] = fare
	}
	list := reducer.TubeDestinations(state)
	if len(list) != len(want) {
		t.Fatalf("清單有 %d 條，應該有 %d 條", len(list), len(want))
	}
	for _, destination := range list {
		fare, ok := want[destination.ID]
		if !ok {
			t.Fatalf("清單裡混進了 %q", destination.ID)
		}
		if destination.Fare != fare {
			t.Fatalf("%q 的車資是 %d，應該是 %d", destination.ID, destination.Fare, fare)
		}
		if destination.Name == "" {
			t.Fatalf("%q 沒有城市名可以畫在框上", destination.ID)
		}
	}
}

func TestTubeRejectsDestinationsOutsideOriginalPicker(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen, s.TubePage, s.TubeTargetID = ScreenTubeStation, TubePageConfirm, "banville"
	if _, ok := r.TubeSelected(s); ok {
		t.Fatal("載入的舊錯誤目標繞過原版區間")
	}
	next, err := r.Step(s, ActionRideTube)
	if err != nil || next.CityID != s.CityID || next.Position != s.Position || next.Bank != s.Bank || next.Cash != s.Cash || next.CombatRolls != s.CombatRolls {
		t.Fatal("不合法Tube目標扣錢或移動", err)
	}
	s.CityID, s.TubeTargetID = "banville", ""
	list := r.TubeDestinations(s)
	if len(list) != 4 {
		t.Fatal("第60城沒有沿原版count-1限制", list)
	}
	for _, d := range r.World.Destinations() {
		if d.SourceIndex == 5 {
			if _, ok := r.TubeFare(s, d.ID); ok {
				t.Fatal("第60城可選到第五個普通城市")
			}
		}
	}
}

// 一顆行星最多六座城市，所以清單最長五條——pick_location 的框數永遠夠用，
// 那個給第六條用的 OTHERS 寬框不會出現。
func TestTubeStationListNeverOverflowsPickLocation(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	for _, destination := range reducer.World.Destinations() {
		state.CityID = destination.CityID
		state.SystemID = destination.SystemID
		if got := len(reducer.TubeDestinations(state)); got > 5 {
			t.Fatalf("%q 的電車站要畫 %d 個框，pick_location 只有五個窄框", destination.CityID, got)
		}
	}
}

// 選一個框就上車：扣車資、換城市，而且是從電車站畫面按的。
func TestTubeStationPickRidesThatCity(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenTubeStation
	state.Cash, state.Bank = 100000, 100000
	list := reducer.TubeDestinations(state)
	if len(list) == 0 {
		t.Skip("起始行星上沒有第二座城市")
	}
	next, err := reducer.Step(state, ActionTubePick1)
	if err != nil {
		t.Fatal(err)
	}
	// 第一個框只是把確認頁叫出來——原版也是兩頁。
	if next.TubePage != TubePageConfirm || next.TubeTargetID != list[0].ID {
		t.Fatalf("第一個框選到 page=%d target=%q，應該是確認頁的 %q", next.TubePage, next.TubeTargetID, list[0].ID)
	}
	if next.CityID != state.CityID {
		t.Fatalf("還沒按搭乘就把人送到 %q", next.CityID)
	}
	selection, ok := reducer.TubeSelected(next)
	if !ok || selection.Destination.Fare != list[0].Fare || !selection.Affordable {
		t.Fatalf("確認頁的內容不對：%+v", selection)
	}
	after, err := reducer.Step(next, ActionRideTube)
	if err != nil {
		t.Fatal(err)
	}
	if after.CityID != list[0].ID {
		t.Fatalf("搭乘之後在 %q，應該是 %q", after.CityID, list[0].ID)
	}
	if after.Bank != state.Bank-list[0].Fare {
		t.Fatalf("存款剩 %d，應該是 %d", after.Bank, state.Bank-list[0].Fare)
	}
	if after.Cash != state.Cash {
		t.Fatalf("現金動了：%d → %d", state.Cash, after.Cash)
	}
	if after.MessageKey != "ui.message.tube_arrived" {
		t.Fatalf("訊息是 %q", after.MessageKey)
	}
	if after.TubePage != TubePageList || after.TubeTargetID != "" {
		t.Fatal("下車之後確認頁還開著")
	}
}

// 錢不夠時原版不畫 TRANSPORT，改在第 7 列說 Insufficient funds。
func TestTubeStationRefusesARideItCannotPayFor(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenTubeStation
	list := reducer.TubeDestinations(state)
	if len(list) == 0 {
		t.Skip("起始行星上沒有第二座城市")
	}
	state.Bank = list[0].Fare - 1
	state.TubePage, state.TubeTargetID = TubePageConfirm, list[0].ID
	selection, ok := reducer.TubeSelected(state)
	if !ok || selection.Affordable {
		t.Fatalf("車資 %d、存款 %d，不該付得起", list[0].Fare, state.Bank)
	}
	next, err := reducer.Step(state, ActionRideTube)
	if err != nil {
		t.Fatal(err)
	}
	if next.CityID != state.CityID || next.MessageKey != "ui.message.tube_insufficient" {
		t.Fatalf("錢不夠還是上車了：%+v", next)
	}
	// 反過來也要成立：手上現金很多但存款不足，一樣上不了車。
	state.Cash = 100000
	if selection, ok := reducer.TubeSelected(state); !ok || selection.Affordable {
		t.Fatal("現金多就付得起了——原版比的是存款")
	}
	next, _ = reducer.Step(state, ActionRideTube)
	if next.CityID != state.CityID {
		t.Fatal("現金補上了存款的缺口")
	}
}

// 右鍵從確認頁回到清單，不是離開整個站。
func TestBackingOutOfTheConfirmationReturnsToTheList(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenTubeStation
	state.TubePage, state.TubeTargetID = TubePageConfirm, "somewhere"
	next, err := reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if next.Screen != ScreenTubeStation || next.TubePage != TubePageList || next.TubeTargetID != "" {
		t.Fatalf("右鍵之後 screen=%s page=%d target=%q", next.Screen, next.TubePage, next.TubeTargetID)
	}
}

// 框外的編號不會憑空生出一趟車。
func TestTubeStationIgnoresABoxThatIsNotOnTheList(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenTubeStation
	state.Cash = 100000
	next, err := reducer.Step(state, ActionTubePick6)
	if err != nil {
		t.Fatal(err)
	}
	if next.CityID != state.CityID {
		t.Fatalf("第六個框把人送到 %q", next.CityID)
	}
	if next.MessageKey != "ui.message.tube_no_destination" {
		t.Fatalf("訊息是 %q", next.MessageKey)
	}
}

// 電車站是自己的建築（碼 18），不是 Uniteller 櫃檯的一個選項。
func TestTubeStationOpensOnItsOwnBuilding(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenGround
	if _, err := reducer.Step(state, ActionRideTube); err == nil {
		t.Fatal("站在街上就搭到車了")
	}
	state.Screen = ScreenUniteller
	if _, err := reducer.Step(state, ActionRideTube); err == nil {
		t.Fatal("Uniteller 櫃檯還是賣車票")
	}
}

func TestExportTalosTubeDestinations(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("由原生Tube選城收據驅動")
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-tube-destination-range.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Rows []struct {
			City   int
			Labels []int
		}
		Distances []struct{ From, To, Column, Row, DistanceSquared, Multiplier int }
	}
	if err := json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Rows) != 6 {
		t.Fatal("原生六案不足")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	bySource := map[int]string{}
	for _, d := range r.World.Destinations() {
		bySource[d.SourceIndex] = d.ID
	}
	type row struct {
		City             int
		Expected, Actual []int
	}
	report := struct {
		Scope                              string
		Cases, Differences                 int
		DistanceCases, DistanceDifferences int
		Rows                               []row
	}{Scope: "六案真實原版選城前綴／label輸出對正式TubeDestinations；不含原版GUI／搭乘／自然RNG"}
	for _, n := range native.Rows {
		id, ok := bySource[n.City]
		if !ok {
			t.Fatalf("缺來源城市%d", n.City)
		}
		s := NewState()
		s.CityID = id
		d, _ := r.World.Destination(id)
		s.PlanetID = d.PlanetID
		s.SystemID = d.SystemID
		actual := []int{}
		for _, entry := range r.TubeDestinations(s) {
			d, _ := r.World.Destination(entry.ID)
			actual = append(actual, d.SourceIndex)
		}
		report.Cases++
		if fmt.Sprint(n.Labels) != fmt.Sprint(actual) {
			report.Differences++
		}
		report.Rows = append(report.Rows, row{n.City, n.Labels, actual})
	}
	if len(native.Distances) != 32 {
		t.Fatal("原生車資讀取32案不足")
	}
	for _, n := range native.Distances {
		s := NewState()
		s.CityID = bySource[n.From]
		h, _ := r.World.Destination(s.CityID)
		s.SystemID = h.SystemID
		s.PlanetID = h.PlanetID
		if n.From == 60 && (n.Column != 31 || n.Row != 7) {
			s.ColonySite = ColonySite{Position: Position{n.Column, n.Row}}
		}
		here, _ := r.Destination(s, s.CityID)
		there, _ := r.Destination(s, bySource[n.To])
		system, _ := r.World.System(s.SystemID)
		dx, dy := there.MapColumn-here.MapColumn, there.MapRow-here.MapRow
		fare, ok := r.TubeFare(s, there.ID)
		report.DistanceCases++
		if !ok || dx*dx+dy*dy != n.DistanceSquared || 2*system.PriceLevel != n.Multiplier || fare != n.Multiplier*isqrt(n.DistanceSquared) {
			report.DistanceDifferences++
		}
	}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "remake-tube-destination-range.json"), append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 || report.DistanceDifferences != 0 {
		t.Fatalf("Tube清單差異%d、32車資讀取差異%d", report.Differences, report.DistanceDifferences)
	}
}
