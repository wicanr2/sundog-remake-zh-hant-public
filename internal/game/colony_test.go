package game

import (
	"encoding/json"
	"fmt"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestColonySiteLifecycleAndReaders(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	before := s
	if s.ColonySite != (ColonySite{Pending: true, Position: Position{31, 7}}) || s.CombatRolls != 0 {
		t.Fatal("新局先抽了位置")
	}
	for _, d := range r.DestinationsOnPlanet(s, s.PlanetID) {
		_, _ = r.TubeFare(s, d.CityID)
	}
	if err := PrepareColonySite(&s, func(*State) (int, error) { t.Fatal("非世界入口抽選址"); return 0, nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("唯讀目的地改變新局")
	}
	s.Screen, s.GroundWorld = ScreenGround, true
	if err := PrepareColonySite(&s, func(*State) (int, error) { return 162, nil }); err != nil {
		t.Fatal(err)
	}
	if s.ColonySite != (ColonySite{Position: Position{2, 4}}) {
		t.Fatal(s.ColonySite)
	}
	d, ok := r.Destination(s, world.MissionDestinationID)
	if !ok || d.MapColumn != 2 || d.MapRow != 4 || d.MapX != 11 || d.MapY != 18 {
		t.Fatal("導航座標未套保存城址", d)
	}
	disk, _ := r.World.Destination(world.MissionDestinationID)
	if disk.MapColumn != 31 || disk.MapRow != 7 {
		t.Fatal("覆寫來源表")
	}
	city, ok := GroundSurfaceCity(s, Position{2, 4})
	if !ok || city.CityID != world.MissionDestinationID {
		t.Fatal("新址缺城市")
	}
	if city, ok := GroundSurfaceCity(s, Position{31, 7}); ok && city.CityID == world.MissionDestinationID {
		t.Fatal("舊址還有城市")
	}
	here, _ := r.World.Destination(s.CityID)
	system, _ := r.World.System(s.SystemID)
	// 原版Tube沒有前往Banville的選項；可用的出口仍須讀保存的新城址。
	s.CityID = world.MissionDestinationID
	fare, ok := r.TubeFare(s, here.ID)
	if !ok || fare != tubeFareFactor*system.PriceLevel*isqrt((2-here.MapColumn)*(2-here.MapColumn)+(4-here.MapRow)*(4-here.MapRow)) {
		t.Fatal("票價未套保存城址", fare)
	}
	selected := s
	if err := PrepareColonySite(&s, func(*State) (int, error) { t.Fatal("重畫重新抽值"); return 0, nil }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, selected) {
		t.Fatal("已選位置被改")
	}
	for _, planet := range r.World.Planets() {
		if planet.ID == world.StartPlanetID {
			continue
		}
		s = NewState()
		s.PlanetID = planet.ID
		s.GroundWorld = true
		if err := PrepareColonySite(&s, func(*State) (int, error) { t.Fatal("其他行星抽值"); return 0, nil }); err != nil {
			t.Fatal(err)
		}
		if !s.ColonySite.Pending || s.CombatRolls != 0 {
			t.Fatal("其他行星清掉待選址")
		}
	}
}

func TestColonySiteErrorIsAtomic(t *testing.T) {
	for _, bad := range []int{-1, 32768} {
		s := NewState()
		s.Screen = ScreenGround
		before := s
		err := leaveGroundCity(&s, Position{-9, 0}, func(s *State) (int, error) { s.CombatRolls++; return bad, nil })
		if err == nil || !reflect.DeepEqual(s, before) {
			t.Fatal("錯誤抽值提交了部分出城", bad, err)
		}
	}
	s := NewState()
	s.Screen = ScreenGround
	before := s
	if err := leaveGroundCity(&s, Position{-9, 0}, func(*State) (int, error) { return 0, fmt.Errorf("controlled draw failure") }); err == nil || !reflect.DeepEqual(s, before) {
		t.Fatal("抽值錯誤未回報或提交了出城")
	}
}

func TestColonySiteRandomModuloAndReplay(t *testing.T) {
	for _, random := range []int{162, 1122, 31842} {
		s := NewState()
		s.GroundWorld = true
		if err := PrepareColonySite(&s, func(*State) (int, error) { return random, nil }); err != nil {
			t.Fatal(err)
		}
		if s.ColonySite.Position != (Position{2, 4}) {
			t.Fatal("RND未按960取餘數", random, s.ColonySite)
		}
	}
	a := NewState()
	a.GroundWorld = true
	a.CombatRolls = 78
	b := a
	if err := PrepareColonySite(&a, nil); err != nil {
		t.Fatal(err)
	}
	if err := PrepareColonySite(&b, nil); err != nil {
		t.Fatal(err)
	}
	if a.ColonySite != b.ColonySite || a.CombatRolls != b.CombatRolls || a.CombatRolls <= 78 {
		t.Fatal("正式保存亂數不可重播")
	}
}

// 正式選址與出城對原生相同候選序列；不以相同數字seed宣稱自然骰序一致。
func TestExportTalosColonySiteSelection(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("由原生玩家入口收據驅動")
	}
	data, err := os.ReadFile(filepath.Join(output, "native-banville-site-player-entry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Cells []struct {
			Index    int
			Eligible bool
		}
		Candidates  []struct{ Candidate, Selected, Draws, RedrawDraws int }
		SourceExits []struct {
			City, Before, After, PlayerColumn, PlayerRow, Draws int
			PendingBefore, PendingAfter                         bool
		}
	}
	if err := json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Cells) != 960 || len(native.Candidates) != 960 || len(native.SourceExits) != 4 {
		t.Fatal("原生覆蓋不足")
	}
	first := -1
	for _, cell := range native.Cells {
		if cell.Eligible {
			first = cell.Index
			break
		}
	}
	if first < 0 {
		t.Fatal("缺合法候選")
	}
	report := struct {
		Scope                          string
		Candidates, Exits, Differences int
	}{Scope: "正式typed來源→選址／出城，與原生同候選序列；不含全機原版GUI或自然RNG同序"}
	seen := [960]bool{}
	for _, row := range native.Candidates {
		if row.Candidate < 0 || row.Candidate >= 960 || seen[row.Candidate] {
			t.Fatal("候選重複或越界")
		}
		seen[row.Candidate] = true
		s := NewState()
		s.GroundWorld = true
		sequence := []int{row.Candidate, first}
		draws := 0
		draw := func(*State) (int, error) {
			if draws >= len(sequence) {
				return 0, fmt.Errorf("unexpected extra draw")
			}
			value := sequence[draws]
			draws++
			return value, nil
		}
		if err := PrepareColonySite(&s, draw); err != nil {
			t.Fatal(err)
		}
		if s.ColonySite.Pending || s.ColonySite.Position.Y*40+s.ColonySite.Position.X != row.Selected || draws != row.Draws {
			report.Differences++
		}
		before := s
		prior := draws
		if err := PrepareColonySite(&s, draw); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, s) || draws-prior != row.RedrawDraws {
			report.Differences++
		}
		report.Candidates++
	}
	for _, row := range native.SourceExits {
		s := NewState()
		s.Screen = ScreenGround
		s.ColonySite.Pending = row.PendingBefore
		s.ColonySite.Position = Position{row.Before & 63, (row.Before >> 6) & 31}
		origin := Position{-9, 0}
		if row.City == 60 {
			s.CityID = world.MissionDestinationID
			origin = Position{0, 3}
		} else if row.City != 1 {
			t.Fatal("未知原生城市")
		}
		before := s
		draws := 0
		if err := leaveGroundCity(&s, origin, func(*State) (int, error) {
			draws++
			if draws == 1 {
				return 123, nil
			}
			if draws == 2 {
				return first, nil
			}
			return 0, fmt.Errorf("unexpected exit draw")
		}); err != nil {
			t.Fatal(err)
		}
		if s.ColonySite.Pending != row.PendingAfter || s.ColonySite.Position != (Position{row.After & 63, (row.After >> 6) & 31}) || s.Position != (Position{row.PlayerColumn, row.PlayerRow}) || draws != row.Draws || s.ShipPosition != before.ShipPosition || s.CargoPosition != before.CargoPosition || s.Colony != before.Colony {
			report.Differences++
		}
		report.Exits++
	}
	if report.Differences != 0 {
		t.Fatalf("正式選址差異：%+v", report)
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-banville-site-selection.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS:%d candidates;%d exits;0 differences", report.Candidates, report.Exits)
}

// source→typed資格與原生全行星外觀／城市覆蓋比對，不代替正式選址或玩家驗收。
func TestExportTalosColonySiteEligibility(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("只由原生來源選址收據驅動")
	}
	data, err := os.ReadFile(filepath.Join(output, "native-banville-site-player-entry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Cells []struct {
			Index, Base, Code  int
			Occupied, Eligible bool
		}
		FirstCity, CityCount, EligibleCount int
	}
	if err := json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Cells) != 960 || native.FirstCity != 1 || native.CityCount != 5 || native.EligibleCount != 318 {
		t.Fatal("缺少第一行星完整原生資格覆蓋")
	}
	surface, err := atarist.PlanetSurfaceAt(0)
	if err != nil {
		t.Fatal(err)
	}
	occupied := [960]bool{}
	for _, d := range geographyRegistry.Destinations() {
		if d.PlanetID == world.StartPlanetID && d.CityID != world.MissionDestinationID {
			occupied[d.MapRow*40+d.MapColumn] = true
		}
	}
	report := struct {
		Scope                                  string
		Cases, Eligible, Occupied, Differences int
	}{Scope: "第一行星已解析typed地形及普通城市佔位→ColonyTerrainAllowsSite，逐格比對真實原版外觀writer／城市覆蓋後的資格；不含正式選址觸發、保存或GUI"}
	seen := [960]bool{}
	for _, row := range native.Cells {
		if row.Index < 0 || row.Index >= 960 || seen[row.Index] {
			t.Fatal("原生格索引重複或越界")
		}
		seen[row.Index] = true
		x, y := row.Index%40, row.Index/40
		base := surface.States[y][x]
		eligible := ColonyTerrainAllowsSite(base, surface.Header[4]*256+surface.Header[5] != 0, occupied[row.Index])
		if base != row.Base || occupied[row.Index] != row.Occupied || eligible != row.Eligible {
			report.Differences++
		}
		if eligible {
			report.Eligible++
		}
		if occupied[row.Index] {
			report.Occupied++
		}
		report.Cases++
	}
	if report.Differences != 0 || report.Eligible != 318 || report.Occupied != 5 {
		t.Fatalf("typed資格與原版不符：%+v", report)
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-banville-site-eligibility.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("PASS:960 typed source cells;318 eligible;5 ordinary cities;0 differences")
}

func TestColonyAllNinePhasesAndSaveRoundTrip(t *testing.T) {
	phases, err := atarist.EmbeddedColonyPhases()
	if err != nil {
		t.Fatal(err)
	}
	var c ColonyProgress
	for phase := 0; phase < 9; phase++ {
		changed, err := c.AdvanceAtExchange()
		if err != nil || !changed || c.Phase != phase || c.Remaining != phases[phase].RequiredCounts {
			t.Fatalf("階段 %d: %+v %v", phase, c, err)
		}
		if changed, err := c.AdvanceAtExchange(); err != nil || changed {
			t.Fatal("需求未完成卻跳階段")
		}
		for _, goods := range phases[phase].Goods {
			if goods == 0 {
				continue
			}
			if !c.DeliverLot(goods) {
				t.Fatalf("階段 %d 貨物 %d 不被接受", phase, goods)
			}
			raw, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			var loaded ColonyProgress
			if err := json.Unmarshal(raw, &loaded); err != nil || loaded != c {
				t.Fatal("部分交付保存不一致")
			}
			c = loaded
		}
	}
	if changed, err := c.AdvanceAtExchange(); err != nil || !changed || !c.Complete() {
		t.Fatalf("第九期後未完成: %+v", c)
	}
	before := c
	if c.DeliverLot(31) {
		t.Fatal("完成後仍扣需求")
	}
	if changed, err := c.AdvanceAtExchange(); err != nil || changed || c != before {
		t.Fatal("完成後越過末階段")
	}
}

func TestFormalColonyDeliveryAndTerminalLot(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.CityID = world.MissionDestinationID
	s.Screen = ScreenTrade
	s.Cash, s.Bank = 0, 43210
	r.openExchange(&s)
	if !s.Colony.Initialized || s.Colony.Phase != 0 || s.Colony.NextPhase != 1 || s.Colony.Accumulated != 0 {
		t.Fatal("first phase not initialized")
	}
	phases, _ := atarist.EmbeddedColonyPhases()
	for phase := 0; phase < 9; phase++ {
		if s.Colony.Phase != phase || s.Colony.Remaining != phases[phase].RequiredCounts {
			t.Fatalf("phase %d", phase)
		}
		for _, goods := range phases[phase].Goods {
			if goods == 0 {
				continue
			}
			s.Holds[0] = goodsHold(Lot{Goods: goods, Quality: 3}, "fixture")
			s.TradeSaleSlot, s.TradeSalePrice, s.TradePage = 5, 100, TradePageSellConfirm
			beforeFunds, beforeU := TradeFunds(s), s.Colony.Accumulated
			s, err = r.Step(s, ActionTradePick1)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateTradeState(s); err != nil {
				t.Fatal(err)
			}
			if s.MissionComplete {
				if phase != 8 || s.Screen != ScreenScore || !s.Holds[0].Occupied() || TradeFunds(s) != beforeFunds || s.Colony.Accumulated != beforeU+50 {
					t.Fatal("terminal path paid or cleared lot")
				}
			} else {
				if s.Holds[0] != (Hold{}) {
					t.Fatal("approved lot not cleared")
				}
				if s.Colony.Phase == phase && (TradeFunds(s) != beforeFunds || s.Colony.Accumulated != beforeU+50) {
					t.Fatal("premature payout")
				}
				if s.Colony.Phase != phase && (TradeFunds(s) != beforeFunds+Credits(beforeU+50) || s.Colony.Accumulated != 0) {
					t.Fatal("phase payout mismatch")
				}
			}
		}
	}
	if !s.Colony.Complete() || !s.MissionComplete {
		t.Fatal("nine typed phases incomplete")
	}
}

func TestColonyRejectsUnneededAndInvalidLots(t *testing.T) {
	var c ColonyProgress
	if c.DeliverLot(8) {
		t.Fatal("未初始化便接受交付")
	}
	if _, err := c.AdvanceAtExchange(); err != nil {
		t.Fatal(err)
	}
	before := c
	for _, goods := range []int{-1, 0, 1, 31, 32, 100} {
		if c.DeliverLot(goods) {
			t.Fatalf("接受無需求／越界貨物 %d", goods)
		}
	}
	if c != before {
		t.Fatal("無效交付改變狀態")
	}
	bad := ColonyProgress{Initialized: true, Phase: 10}
	if _, err := bad.AdvanceAtExchange(); err == nil {
		t.Fatal("接受越界階段")
	}
}

func TestColonyCityExtentAcrossDeliveredPhases(t *testing.T) {
	// 期望值來自 XMOVEONG 的階段映射與邊界算術，包含第 8 期跳至完整尺寸。
	want := [10][2]int{{0, 0}, {1, 1}, {2, 1}, {3, 2}, {4, 2}, {5, 3}, {6, 3}, {7, 4}, {9, 5}, {9, 5}}
	var c ColonyProgress
	for phase, size := range want {
		if _, err := c.AdvanceAtExchange(); err != nil {
			t.Fatal(err)
		}
		before := c
		w, h, err := c.CityExtent()
		if err != nil || c.Phase != phase || w != size[0] || h != size[1] || c != before {
			t.Fatalf("階段 %d: 尺寸 %d,%d 狀態 %+v 錯誤 %v", phase, w, h, c, err)
		}
		for goods, count := range c.Remaining {
			for n := 0; n < count; n++ {
				if !c.DeliverLot(goods) {
					t.Fatal("無法完成當期交付")
				}
			}
		}
	}
	for _, phase := range []int{-1, 10, 255} {
		if _, _, err := (ColonyProgress{Phase: phase}).CityExtent(); err == nil {
			t.Fatalf("接受無效階段 %d", phase)
		}
	}
}

func TestColonySiteMatchesOriginalSurfaceConversion(t *testing.T) {
	// 以原始圖塊轉換及 Talos 集合為對照，不以 predicate 自己產生期望值。
	for terrain := 0; terrain <= 13; terrain++ {
		for _, blank := range []bool{false, true} {
			for _, city := range []int{0, 32, 33} {
				for roll := 0; roll < 17; roll++ {
					for _, variant := range []int{0, 16} {
						code := terrain
						switch {
						case terrain == 0 && blank:
							code = 0
						case terrain == 0 || terrain == 13:
							switch {
							case roll < 4:
								code = 13
							case roll < 8:
								code = 29
							default:
								code = roll + 26
							}
						default:
							code += variant
						}
						if city != 0 {
							code = city
						}
						want := code == 13 || code == 29 || code >= 34 && code <= 42
						if got := ColonyTerrainAllowsSite(terrain, blank, city != 0); got != want {
							t.Fatalf("地形 %d 空白 %v 城市 %d 亂數 %d 外觀 %d", terrain, blank, city, roll, variant)
						}
					}
				}
			}
		}
	}
	for _, terrain := range []int{-1, 14, 29, 34, 42, 255} {
		if ColonyTerrainAllowsSite(terrain, false, false) {
			t.Fatalf("把非法原始地形／已轉換圖塊 %d 當成可建地", terrain)
		}
	}
}

// 保存的完成旗標不能讓普通城市交易跳過自己的售出清槽與訊息。
func TestOrdinarySaleUsesItsOwnTerminalTransition(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	s.Screen = ScreenTrade
	s.Cash, s.Bank = 0, 43210
	s.Colony = ColonyProgress{Initialized: true, Phase: 9, NextPhase: 10}
	s.MissionComplete = true
	s.Holds[0] = goodsHold(Lot{Goods: 8, Quality: 3}, "fixture")
	s.TradeSaleSlot, s.TradeSalePrice, s.TradePage = 5, 100, TradePageSellConfirm
	s.Quotes = make([]Lot, QuoteRing)
	beginTradeSession(&s)
	s, err = r.Step(s, ActionTradePick1)
	if err != nil || s.Screen != ScreenTrade || s.MessageKey != "ui.message.sold" || s.Holds[0] != (Hold{}) || TradeFunds(s) != 53210 {
		t.Fatalf("ordinary sale misused colony flag: %v", err)
	}
}

// 有界原生writer與正式可見格規則比較，Code是明示的外觀輸入。
func TestExportTalosColonyGrowth(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("Talos收據目錄未設定")
	}
	var native struct {
		Native []struct {
			Colony                                         bool
			Phase, X, Y, Code, TerrainAfter, BuildingAfter int
		}
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-banville-growth-mask.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	report := struct {
		Cases, Differences int
		Scope              string
	}{Scope: "正式GroundCityCellVisible與尺寸；受控外觀Code投影，不是自然RNG或正常九期"}
	for _, n := range native.Native {
		s := NewState()
		city := world.StartDestinationID
		if n.Colony {
			city = world.MissionDestinationID
		}
		s.Colony.Phase = n.Phase
		visible := GroundCityCellVisible(s, city, n.X, n.Y)
		terrain, building := 0, 0xbb
		if visible {
			terrain, building = n.Code, n.Code
		}
		report.Cases++
		if terrain != n.TerrainAfter || building != n.BuildingAfter {
			report.Differences++
			t.Errorf("phase%d colony%t cell%d,%d mismatch", n.Phase, n.Colony, n.X, n.Y)
		}
	}
	if report.Cases != 2640 {
		t.Fatal("原生覆蓋不同")
	}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-banville-growth-mask.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestColonyGrowthMapsAndMaskedEntrances(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	source, _ := world.CityMap(world.MissionDestinationID)
	for phase := 0; phase <= 9; phase++ {
		s := NewState()
		s.CityID = world.MissionDestinationID
		s.Colony.Phase = phase
		before := s.Colony
		rolls := s.CombatRolls
		visibleCells := 0
		for y := 0; y < 12; y++ {
			for x := 0; x < 20; x++ {
				visible := GroundCityCellVisible(s, s.CityID, x, y)
				code, number, ok := GroundTerrainCode(s, s.CityID, x, y)
				original, _ := source.Terrain(x, y)
				if !ok || number != 60 {
					t.Fatal("來源地圖消失")
				}
				if visible {
					visibleCells++
					if code != original {
						t.Fatal("改寫已建地形")
					}
				} else {
					if code != 0 {
						t.Fatal("未建地形沒有清零")
					}
					if terrain, ok := GroundTerrainAt(s, s.CityID, x, y); !ok || terrain != world.TerrainGrass {
						t.Fatal("未建區沒有空地")
					}
					if len(r.GroundFacilitiesAt(s, world.TilePosition{X: x, Y: y})) != 0 {
						t.Fatal("未建區仍有設施")
					}
				}
			}
		}
		if phase == 0 && visibleCells != 4 || phase >= 8 && visibleCells != 240 {
			t.Fatalf("phase%d cells%d", phase, visibleCells)
		}
		for _, b := range GroundCityBuildings(s, s.CityID) {
			if !GroundCityCellVisible(s, s.CityID, b.Row, b.Column) {
				t.Fatal("未建建築仍繪出")
			}
		}
		if s.Colony != before || s.CombatRolls != rolls {
			t.Fatal("讀地圖改變進度或RNG")
		}
	}
	s := NewState()
	s.CityID = world.MissionDestinationID
	s.Screen = ScreenGround
	for _, b := range source.Buildings() {
		s.Position = Position{b.Row, b.Column}
		if b.Facility == "exchange" {
			if !r.GroundHasFacility(s, world.FacilityExchange) {
				t.Fatal("初期真實交易所不可進入")
			}
			next, err := r.Step(s, ActionOpenTrade)
			if err != nil || next.Screen != ScreenTrade || !next.Colony.Initialized {
				t.Fatalf("交易入口: %v", err)
			}
		} else if !GroundCityCellVisible(s, s.CityID, b.Row, b.Column) {
			next, err := r.Step(s, ActionEnterBuilding)
			if err != nil || next.Screen != ScreenGround {
				t.Fatal("未建建築仍能進入")
			}
		}
	}
}

func TestColonyGrowthCitySurfaceRoundTrip(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	destination, ok := r.World.Destination(world.MissionDestinationID)
	if !ok {
		t.Fatal("缺少來源Banville目的地")
	}
	// get_clim已證實phase0=(0,0)、4=(4,2)、8=(9,5)。
	// 固定預期來自原始城市／行星座標契約，不從待測函式計算預期值。
	for _, sample := range []struct {
		phase   int
		exits   [4]Position
		entries [4]Position
	}{
		{0, [4]Position{{0, 3}, {12, 3}, {5, -1}, {5, 8}}, [4]Position{{8, 6}, {11, 6}, {10, 4}, {10, 7}}},
		{4, [4]Position{{-4, 3}, {16, 3}, {5, -3}, {5, 10}}, [4]Position{{4, 6}, {15, 6}, {10, 2}, {10, 9}}},
		{8, [4]Position{{-9, 3}, {21, 3}, {5, -6}, {5, 13}}, [4]Position{{-1, 6}, {20, 6}, {10, -1}, {10, 12}}},
	} {
		for direction := range sample.exits {
			s := NewState()
			s.Screen = ScreenGround
			s.CityID, s.PlanetID, s.SystemID = destination.CityID, destination.PlanetID, destination.SystemID
			s.ShipCityID, s.ShipPosition = destination.CityID, Position{9, 6}
			s.Position = s.ShipPosition
			// 成長往返驗的是已完成選址的城市；首次出城會留下舊址，另由選址測試驗證。
			s.GroundWorld = true
			if err := PrepareColonySite(&s, func(*State) (int, error) { return 162, nil }); err != nil {
				t.Fatal(err)
			}
			s.GroundWorld = false
			destination = ResolveDestination(s, destination)
			// 明示受控合法進度，不宣稱這是正常交付玩家鏈。
			for next := 0; next <= sample.phase; next++ {
				s.Colony.Remaining = [32]int{}
				if _, err := s.Colony.AdvanceAtExchange(); err != nil {
					t.Fatal(err)
				}
			}
			before := s
			if !LeaveGroundCity(&s, sample.exits[direction]) || !s.GroundWorld {
				t.Fatalf("phase%d direction%d未切到行星", sample.phase, direction)
			}
			anchors := [4]Position{{163, 116}, {187, 116}, {171, 107}, {171, 123}}
			if s.GroundView.Anchor != anchors[direction] || s.Position != (Position{destination.MapColumn, destination.MapRow}) {
				t.Fatalf("phase%d direction%d出城落點錯誤: %+v", sample.phase, direction, s.GroundView)
			}
			offsets := [4]Position{{0, 16}, {31, 16}, {16, 0}, {16, 31}}
			if !EnterGroundCity(&s, offsets[direction]) || s.GroundWorld || s.CityID != destination.CityID || s.Position != sample.entries[direction] {
				t.Fatalf("phase%d direction%d回城落點錯誤: %+v", sample.phase, direction, s.Position)
			}
			if s.GroundView.Anchor != (Position{160, 108}) || s.Screen != ScreenGround {
				t.Fatal("回城未建立地面人物視圖")
			}
			if s.Colony != before.Colony || s.CombatRolls != before.CombatRolls || s.Bank != before.Bank || s.Cash != before.Cash || s.ShipCityID != before.ShipCityID || s.ShipPosition != before.ShipPosition {
				t.Fatal("出入城市改變進度、亂數、資金或停船")
			}
		}
	}
}
