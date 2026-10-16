package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

func TestExportTalosTradePurchase(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生收據")
	}
	read := func(name string, v any) {
		b, e := os.ReadFile(filepath.Join(output, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal(b, v); e != nil {
			t.Fatal(e)
		}
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := func(pod bool) State {
		s := NewState()
		s.Screen = ScreenTrade
		s.CityID = world.MissionDestinationID
		s.TradePage = TradePageBuy
		s.GroundMode = GroundOnFoot
		if pod {
			s.GroundMode = GroundInPod
		}
		s.Cash, s.Bank = 9000, 0
		s.Quotes = make([]Lot, QuoteRing)
		s.Quotes[2] = Lot{Goods: 11, Quality: 6, Price: 20}
		s.QuoteIndex = 2
		s.Holds = [2]Hold{}
		beginTradeSession(&s)
		return s
	}
	decode := func(w [2]uint16) Lot {
		return Lot{Goods: int(w[0] & 31), Quality: int(w[0]>>5) & 7, SourceCode: uint8(w[0] >> 8), Price: int(w[1])}
	}
	encode := func(l Lot) [2]uint16 {
		return [2]uint16{uint16(l.Goods | l.Quality<<5 | int(l.SourceCode)<<8), uint16(l.Price)}
	}
	setLots := func(s *State, before [6][2]uint16) {
		s.Warehouses = map[string][]Lot{s.CityID: make([]Lot, 4)}
		for i, w := range before {
			l := decode(w)
			if i < 4 {
				s.Warehouses[s.CityID][i] = l
			} else {
				h := goodsHold(l, "fixture")
				if l.Goods == 0 {
					h.Kind = HoldEmpty
					h.ItemKey = ""
				}
				s.Holds[i-4] = h
			}
		}
	}
	getLots := func(s State) (out [6][2]uint16) {
		for i, l := range WarehouseLotsOf(s, s.CityID) {
			out[i] = encode(l)
		}
		for i, h := range s.Holds {
			out[i+4] = encode(h.Lot())
		}
		return
	}
	report := struct {
		CapacityCases, CapacityDifferences, PickerCases, PickerDifferences, CommitCases, CommitDifferences int
		Scope                                                                                              string
	}{Scope: "正式128容量分類、256批准後選槽與36受控Buy／Pass／取消的付款／lot結果。前兩項由typed容量與放貨核心比較；36項經Reducer.Step。原版選單輸入、stock／market primitive為明示邊界；不比較完整RND、庫存內容或正常GUI。"}
	var gates struct {
		Gates []struct {
			PodMode                         bool
			WarehouseMask, PodMask, Allowed int
		}
	}
	read("native-trade-purchase-slots.json", &gates)
	for _, n := range gates.Gates {
		s := state(n.PodMode)
		var before [6][2]uint16
		for i := range before {
			mask, bit := n.WarehouseMask, i
			if i >= 4 {
				mask, bit = n.PodMask, i-4
			}
			before[i] = [2]uint16{0xab00, 100}
			if mask&(1<<bit) != 0 {
				before[i][0] |= 12
			}
		}
		setLots(&s, before)
		allowed := 0
		if TradePurchaseFits(s) {
			allowed = 1
		}
		if allowed != n.Allowed {
			report.CapacityDifferences++
		}
		report.CapacityCases++
	}
	var picker struct {
		Rows []struct {
			PodMode                     bool
			Choice, Selected, Container int
			Before, After               [6][2]uint16
		}
	}
	read("native-trade-purchase-picker.json", &picker)
	for _, n := range picker.Rows {
		s := state(n.PodMode)
		setLots(&s, n.Before)
		lot := decode([2]uint16{0x3ccb, 20})
		s.TradePurchaseLot = &lot
		_, ok := r.finishTradePurchase(&s, n.PodMode && n.Choice != 1)
		if getLots(s) != n.After || ok != (n.Selected > 0) {
			report.PickerDifferences++
		}
		report.PickerCases++
	}
	var commit struct {
		Rows []struct {
			Choice, Destination, Cost, Abort, FundsAfter, Selected int
			Lots                                                   [6][2]uint16
		}
	}
	read("native-trade-purchase-commit.json", &commit)
	for _, n := range commit.Rows {
		s := state(true)
		s.Quotes[2].Price = n.Cost / CreditsPerPriceUnit
		action := ActionBack
		if n.Cost > 0 && n.Abort == 0 {
			if n.Choice == 1 {
				action = ActionBuyGems
			}
			if n.Choice == 2 {
				action = ActionTradePass
			}
		}
		s, err = r.Step(s, action)
		if err != nil {
			t.Fatal(err)
		}
		if n.Selected > 0 {
			if s.TradePage != TradePageBuyDestination || s.TradePurchaseLot == nil || TradeFunds(s) != n.FundsAfter || s.Holds != ([2]Hold{}) {
				t.Fatal("批准後應先扣款並保持待放貨")
			}
			action = ActionBack
			if n.Destination == 1 {
				action = ActionTradePick1
			}
			if n.Destination == 2 {
				action = ActionTradePick2
			}
			s, err = r.Step(s, action)
			if err != nil {
				t.Fatal(err)
			}
		}
		if TradeFunds(s) != n.FundsAfter || getLots(s) != n.Lots || s.TradePurchaseLot != nil {
			report.CommitDifferences++
		}
		report.CommitCases++
	}
	b, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(output, "remake-trade-purchase.json"), append(b, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if report.CapacityDifferences+report.PickerDifferences+report.CommitDifferences != 0 {
		t.Fatalf("購買差異 %+v", report)
	}
}

// 只比較售出後的槽位清空，不比較前置成交價格或完整玩家選單。
func TestExportTalosTradeSlotClear(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生收據")
	}
	var native struct {
		Native []struct {
			Slot          int
			Before, After [6][2]uint16
		}
	}
	data, err := os.ReadFile(filepath.Join(output, "native-trade-slot-clear.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		Cases, Differences, WarehouseUnsupported int
		SourceCases, SourceDifferences           int
		Scope                                    string
	}{Scope: "正式六槽來源讀取與售出後兩word清空，按原始selector1..4倉庫及5/6 pod映射。原生切片僅涵蓋已成交後，直接比較共用clearTradeSaleSlot；不把任意pattern的燃料／冷凍人當成普通可售貨物。來源選單另經Reducer.Step。"}
	encode := func(h Hold) [2]uint16 {
		return [2]uint16{uint16(h.Goods | h.Quality<<5 | int(h.SourceCode)<<8), uint16(h.Price)}
	}
	for _, n := range native.Native {
		s := NewState()
		s.Screen = ScreenTrade
		s.Quotes = make([]Lot, QuoteRing)
		beginTradeSession(&s)
		lots := make([]Lot, WarehouseLots)
		for i := 0; i < WarehouseLots; i++ {
			w := n.Before[i]
			lots[i] = Lot{Goods: int(w[0] & 31), Quality: int(w[0]>>5) & 7, SourceCode: uint8(w[0] >> 8), Price: int(w[1])}
		}
		s.Warehouses = map[string][]Lot{s.CityID: lots}
		for i := 0; i < 2; i++ {
			w := n.Before[i+4]
			s.Holds[i] = goodsHold(Lot{Goods: int(w[0] & 31), Quality: int(w[0]>>5) & 7, SourceCode: uint8(w[0] >> 8), Price: int(w[1])}, "fixture")
		}
		index := n.Slot - 5
		clearTradeSaleSlot(&s, n.Slot)
		var after [6][2]uint16
		for i, l := range WarehouseLotsOf(s, s.CityID) {
			after[i] = encode(goodsHold(l, ""))
		}
		for i, h := range s.Holds {
			after[i+4] = encode(h)
		}
		if after != n.After {
			report.Differences++
		}
		if n.Slot > 4 && s.Holds[index] != (Hold{}) {
			t.Fatal("售出pod仍有衍生資料")
		}
		report.Cases++
	}
	var sources struct {
		Native []struct{ Param, Choice, SourceFlag, Pod int }
	}
	data, err = os.ReadFile(filepath.Join(output, "native-trade-sale-source.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &sources); err != nil {
		t.Fatal(err)
	}
	for _, n := range sources.Native {
		s := NewState()
		s.Screen = ScreenTrade
		s.TradePage = TradePageMain
		s.GroundMode = GroundOnFoot
		if n.SourceFlag != 0 {
			s.GroundMode = GroundInPod
		}
		s, err = r.Step(s, ActionTradeSellPage)
		if err != nil {
			t.Fatal(err)
		}
		if n.SourceFlag != 0 {
			if s.TradePage != TradePageSellSource {
				report.SourceDifferences++
			}
			action := ActionBack
			if n.Choice == 1 {
				action = ActionTradePick1
			}
			if n.Choice == 2 {
				action = ActionTradePick2
			}
			s, err = r.Step(s, action)
			if err != nil {
				t.Fatal(err)
			}
		}
		want := TradePageSellWarehouse
		if n.Pod != 0 {
			want = TradePageSell
		}
		if n.SourceFlag != 0 && n.Choice < 0 {
			want = TradePageMain
		}
		if s.TradePage != want {
			report.SourceDifferences++
		}
		report.SourceCases++
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-trade-slot-clear.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("formal slot clear cases%d differences%d warehouse unsupported%d", report.Cases, report.Differences, report.WarehouseUnsupported)
}

// 控制分類位移與原版相同；不把remake的新局骰序宣稱為原版骰序。
func TestExportTalosTradeMarketRange(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生收據")
	}
	var native struct {
		Native []struct{ Steps, Goods31Before, Goods31After int }
	}
	data, err := os.ReadFile(filepath.Join(output, "native-trade-market-range.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range native.Native {
		s := NewState()
		s.CityID = world.StartDestinationID
		s.TimeMinutes = n.Steps * marketAgeDays * 24 * 60
		m := CityMarket{Supply: make([]int, 32), Demand: make([]int, 32), LastSeen: 0}
		m.Supply[31] = n.Goods31Before
		m.Demand[31] = n.Goods31Before
		s.Markets = map[string]CityMarket{s.CityID: m}
		got := r.cityMarket(&s, s.CityID)
		if got.Supply[31] != n.Goods31After || got.Demand[31] != n.Goods31After {
			t.Fatal("正式市場修改原版未老化的第31格")
		}
	}
	s := NewState()
	fresh := r.cityMarket(&s, s.CityID)
	if fresh.Supply[31] != 0 || fresh.Demand[31] != 0 {
		t.Fatal("正式新市場生成第31格供需")
	}
	report := struct {
		Cases, Differences int
		Scope              string
	}{len(native.Native), 0, "同第31格初態及週期，驗正式市場保持；原版研究步進0是直接helper，正式0週是caller跳過。1..30各自分類數值不是本比較範圍。另驗新市場第31格0/0。"}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-trade-market-range.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExportTalosTradeClasses(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生收據")
	}
	var classes struct {
		Native []struct {
			Planet, Shift, SupplyGoods, DemandGoods int
			Supply, Demand                          []int
		}
	}
	data, err := os.ReadFile(filepath.Join(output, "native-trade-classes.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &classes); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range classes.Native {
		var cityID string
		for _, city := range r.World.Destinations() {
			p, found := r.World.Planet(city.PlanetID)
			if found && p.Index+1 == n.Planet {
				cityID = city.CityID
				break
			}
		}
		if cityID == "" {
			t.Fatalf("無行星%d的正式入口", n.Planet)
		}
		c, ok := world.TradeClassesFor(cityID, n.Shift)
		if !ok || !reflect.DeepEqual(c.Supply, n.Supply) || !reflect.DeepEqual(c.Demand, n.Demand) || c.HomeGoods != n.SupplyGoods || c.DemandGoods != n.DemandGoods {
			t.Fatalf("分類不同 planet%d shift%d: %+v != %+v", n.Planet, n.Shift, c, n)
		}
		// 老化到上限，驗證正式市場consumer用保存位移與兩項覆寫。
		s := NewState()
		s.CityID = cityID
		s.TradeClassShift = n.Shift
		s.Markets = map[string]CityMarket{cityID: {Supply: make([]int, goodsCodes), Demand: make([]int, goodsCodes), LastSeen: 0}}
		s.TimeMinutes = 100 * 24 * 60
		m := r.cityMarket(&s, cityID)
		for goods := 1; goods <= 30; goods++ {
			if m.Supply[goods] != n.Supply[goods-1] || m.Demand[goods] != n.Demand[goods-1] {
				t.Fatalf("正式市場未接分類 planet%d goods%d", n.Planet, goods)
			}
		}
	}
	var shift struct {
		Native []struct{ Counter, Random, NextCounter, Shift int }
	}
	data, err = os.ReadFile(filepath.Join(output, "native-trade-class-shift.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &shift); err != nil {
		t.Fatal(err)
	}
	for _, n := range shift.Native {
		next, got, err := tradeClassShift(n.Counter, n.Random)
		if err != nil || next != n.NextCounter || got != n.Shift {
			t.Fatalf("初始化不同 %+v: %d/%d %v", n, next, got, err)
		}
	}
	for _, input := range [][2]int{{-1, 0}, {256, 0}, {4, -1}, {4, 32768}} {
		if _, _, err := tradeClassShift(input[0], input[1]); err == nil {
			t.Fatal("非法初始化輸入被接受")
		}
	}
	for _, invalid := range []int{-1, 18} {
		if _, ok := world.TradeClassesFor(world.StartDestinationID, invalid); ok {
			t.Fatal("非法分類位移被接受")
		}
	}
	report := struct {
		Cases, StartupCases, Differences int
		Scope                            string
	}{len(classes.Native), len(shift.Native), 0, "同原版typed行星／保存位移與兩項分類覆寫；另驗正式市場老化consumer。初始化使用明示受控RND，不含跨新局磁碟計數生命週期或自然骰序。"}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-trade-classes.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS:%d class profiles and formal market consumers;%d startup cases", report.Cases, report.StartupCases)
}

// 入口：docs/atari-st/banville-restoration-spec.md。原生資料僅作測試oracle。
func TestExportTalosTradeRecords(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生收據")
	}
	var native struct {
		Records []struct {
			City, Goods, Quality, Price int
			Purchased, Stored, Fetched  [2]uint16
		}
		Locations []struct{ City, Planet, System int }
	}
	data, err := os.ReadFile(filepath.Join(output, "native-trade-records.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		Cases, Unsupported, Locations, WriterOnlyCases int
		Scope                                          string
	}{Scope: "有效正價研究報價走正式pod購買批准後選貨艙；零價writer pattern只作typed已持有貨物初態，不冒充可買報價。再驗pod→warehouse→pod兩word欄位及來源城市定位。Store／Fetch consumer勘誤：原生歷史copy不涵蓋原槽清空，真實:10搬運另由倉庫矩陣比較。這不是完整正常GUI驗收。"}
	for _, n := range native.Records {
		var cityID string
		for _, city := range r.World.Destinations() {
			if city.SourceIndex == n.City {
				cityID = city.ID
				break
			}
		}
		if _, ok := r.World.CargoKindByCode(n.Goods); !ok || cityID == "" {
			report.Unsupported++
			continue
		}
		dst, _ := r.World.Destination(cityID)
		s := NewState()
		s.Screen = ScreenTrade
		s.CityID = cityID
		s.SystemID = dst.SystemID
		s.PlanetID = dst.PlanetID
		s.GroundMode = GroundInPod
		s.Cash = Credits(n.Price) + 100
		s.Bank = 0
		s.Quotes = make([]Lot, QuoteRing)
		s.Quotes[0] = Lot{Goods: n.Goods, Quality: n.Quality, Price: n.Price}
		beginTradeSession(&s)
		if n.Price > 0 {
			key, ok := r.buyQuote(&s)
			if !ok {
				t.Fatalf("buy%+v:%s", n, key)
			}
			if _, ok = r.finishTradePurchase(&s, true); !ok {
				t.Fatal("approved purchase pod destination")
			}
		} else {
			kind, _ := r.World.CargoKindByCode(n.Goods)
			s.Holds[0] = goodsHold(Lot{Goods: n.Goods, Quality: n.Quality, Price: n.Price, SourceCode: uint8(n.City)}, kind.Key)
			report.WriterOnlyCases++
		}
		encode := func(l Lot) [2]uint16 {
			return [2]uint16{uint16(l.Goods | l.Quality<<5 | int(l.SourceCode)<<8), uint16(l.Price)}
		}
		if got := encode(s.Holds[0].Lot()); got != n.Purchased {
			t.Fatalf("purchase%+v:%v!=%v", n, got, n.Purchased)
		}
		if _, ok := r.storeLot(&s, 0); !ok {
			t.Fatal("store")
		}
		if got := encode(s.Warehouses[cityID][0]); got != n.Stored {
			t.Fatalf("stored%+v:%v", n, got)
		}
		if _, ok := r.fetchLot(&s, 0); !ok {
			t.Fatal("fetch")
		}
		if got := encode(s.Holds[0].Lot()); got != n.Fetched {
			t.Fatalf("fetched%+v:%v", n, got)
		}
		report.Cases++
	}
	for _, n := range native.Locations {
		p, known := r.World.CargoOrigin(uint8(n.City))
		if n.Planet == 19 && n.System == 13 {
			if known {
				t.Fatalf("表外來源被提升為有效位置:%+v", n)
			}
		} else {
			if !known || p.Index+1 != n.Planet {
				t.Fatalf("來源行星%+v:%+v", n, p)
			}
			if r.World.Systems()[n.System-1].ID != p.SystemID {
				t.Fatalf("來源星系%+v:%+v", n, p)
			}
		}
		report.Locations++
	}
	if report.Cases != 163 || report.Unsupported != 2 || report.Locations != 256 {
		t.Fatalf("覆蓋%+v", report)
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-trade-records.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("PASS:163 formal purchased/store/fetch records;2 no-city inputs;256 origin locations")
}

func TestExportTalosTradeFunds(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生收據")
	}
	var native struct {
		Native []struct{ BankBefore, CashBefore, Cost, Income, AvailableBefore, AvailableAfterCost, AvailableAfterIncome, BankAfter, CashAfter int }
	}
	data, err := os.ReadFile(filepath.Join(output, "native-trade-funds.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	for _, n := range native.Native {
		s := NewState()
		s.Screen = ScreenTrade
		s.Bank = n.BankBefore
		s.Cash = n.CashBefore
		s.Quotes = make([]Lot, QuoteRing)
		beginTradeSession(&s)
		if TradeFunds(s) != n.AvailableBefore {
			t.Fatalf("入口%+v:%+v", n, s.TradeSession)
		}
		if !spendTradeFunds(&s, n.Cost) || TradeFunds(s) != n.AvailableAfterCost {
			t.Fatalf("付款%+v", n)
		}
		creditTradeFunds(&s, n.Income)
		if TradeFunds(s) != n.AvailableAfterIncome || s.Cash != n.CashBefore {
			t.Fatalf("賣款%+v", n)
		}
		// 用真正JSON序列化重建typed session，避免只在同一物件完成結算。
		encoded, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var loaded State
		if err = json.Unmarshal(encoded, &loaded); err != nil {
			t.Fatal(err)
		}
		if err = ValidateTradeState(loaded); err != nil {
			t.Fatal(err)
		}
		finishTradeSession(&loaded)
		if loaded.Bank != n.BankAfter || loaded.Cash != n.CashAfter || loaded.TradeSession.Active {
			t.Fatalf("結算%+v:Cash%d Bank%d", n, loaded.Cash, loaded.Bank)
		}
	}
	if len(native.Native) != 117 {
		t.Fatal("資金覆蓋改變")
	}
	if err = os.WriteFile(filepath.Join(output, "remake-trade-funds.json"), []byte("{\"cases\":117,\"differences\":0,\"jsonReentry\":true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("PASS:117 formal buy/sell/JSON-reentry/settlement sequences")
}

// 原始 confirmation 止於核准/取消分派；正式結果投影是否成交，負分支驗完整無副作用。
// caller 的正值是受控输入，不宣稱完整報價或正常商品取得。
func TestExportTalosDeliveryIntegration(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("需原生收據")
	}
	read := func(name string, target any) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		Cases, Differences, ConfirmCases, ConfirmDifferences, EligibilityCases, EligibilityDifferences int
		Scope                                                                                          string
	}{Scope: "72原始已核准交付完整需求/score/U/funds/current/next。已核定caller價格為明示typed輸入；synthetic需求與current/next沿原生初態，不用作可達保存或正常GUI。32確認案在兩種來源各跑一次，共64案，只比較批准投影與拒絕無副作用；62原始goods gate另比。不比較quote refresh或自然RNG。"}
	var donations struct {
		Native []struct {
			Goods, Quality, Next, DemandBefore, AccumulatorBefore, Price, FundsAfter, ScoreAfter, AccumulatorAfter, CurrentAfter, NextAfter, Stop int
			Remaining                                                                                                                             [32]int
		}
	}
	read("native-banville-donation.json", &donations)
	for _, n := range donations.Native {
		s := NewState()
		s.CityID = world.MissionDestinationID
		s.Screen = ScreenTrade
		s.Cash, s.Bank = 0, 43210
		beginTradeSession(&s)
		s.Quotes = make([]Lot, QuoteRing)
		s.BanvilleCargo = 17
		s.Colony = ColonyProgress{Initialized: true, Phase: 0, NextPhase: n.Next, Accumulated: n.AccumulatorBefore}
		s.Colony.Remaining[n.Goods] = n.DemandBefore
		if !r.settleApprovedLot(&s, Lot{Goods: n.Goods, Quality: n.Quality}, n.Price) {
			t.Fatal("approved donation refused")
		}
		if s.Colony.Remaining != n.Remaining || s.Colony.Accumulated != n.AccumulatorAfter || TradeFunds(s) != n.FundsAfter || s.BanvilleCargo != n.ScoreAfter ||
			s.Colony.Phase != n.CurrentAfter || s.Colony.NextPhase != n.NextAfter || s.MissionComplete != (n.Stop == 0x11e2) {
			report.Differences++
		}
		report.Cases++
	}
	var confirmations struct {
		Native []struct {
			Needed, Selection         int
			Abort, Approved, Banville bool
		}
	}
	read("native-banville-confirm.json", &confirmations)
	for _, n := range confirmations.Native {
		for _, slot := range []int{1, 5} {
			s := NewState()
			s.Screen = ScreenTrade
			s.Cash, s.Bank = 0, 43210
			if n.Banville {
				s.CityID = world.MissionDestinationID
				s.Colony = ColonyProgress{Initialized: true, Phase: 0, NextPhase: 1}
				s.Colony.Remaining[8] = n.Needed
			}
			beginTradeSession(&s)
			s.Quotes = make([]Lot, QuoteRing)
			goods := 8
			if !n.Banville && n.Needed == 0 {
				goods = 1
			}
			lot := Lot{Goods: goods, Quality: 3}
			if slot == 1 {
				s.Warehouses = map[string][]Lot{s.CityID: {lot, {}, {}, {}}}
			} else {
				s.Holds[0] = goodsHold(lot, "fixture")
			}
			s.TradeSaleSlot, s.TradeSalePrice, s.TradePage = slot, n.Needed, TradePageSellConfirm
			before := s
			action := ActionTradePick2
			if n.Selection == 1 {
				action = ActionTradePick1
			}
			if n.Selection < 0 || n.Abort {
				action = ActionBack
			}
			s, err = r.Step(s, action)
			if err != nil {
				t.Fatal(err)
			}
			approved := s.LastSalePrice > 0
			if approved != n.Approved {
				report.ConfirmDifferences++
			}
			if !n.Approved && (!reflect.DeepEqual(s.Holds, before.Holds) || !reflect.DeepEqual(s.Warehouses, before.Warehouses) || TradeFunds(s) != TradeFunds(before) || s.Colony != before.Colony || s.BanvilleCargo != before.BanvilleCargo) {
				report.ConfirmDifferences++
			}
			report.ConfirmCases++
		}
	}
	var eligibility struct {
		Native []struct {
			Goods, OfferAfter int
			Banville, Allowed bool
		}
	}
	read("native-trade-sale-eligibility.json", &eligibility)
	for _, n := range eligibility.Native {
		s := NewState()
		if n.Banville {
			s.CityID = world.MissionDestinationID
		}
		if tradeGoodsAccepted(s, n.Goods) != n.Allowed {
			report.EligibilityDifferences++
		}
		report.EligibilityCases++
	}
	if report.Cases != 72 || report.ConfirmCases != 64 || report.EligibilityCases != 62 || report.Differences != 0 || report.ConfirmDifferences != 0 || report.EligibilityDifferences != 0 {
		t.Fatalf("delivery parity %+v", report)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-banville-delivery.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
