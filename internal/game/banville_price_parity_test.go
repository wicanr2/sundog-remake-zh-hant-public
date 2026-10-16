package game

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

func TestExportTalosBanvilleFixedPrice(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生研究收據")
	}
	type input struct {
		Shift, Goods, Supply, Demand int
		Selling                      bool
	}
	type observation struct {
		Input     input
		Price     int
		Supported bool
		Reason    string `json:",omitempty"`
	}
	var native struct {
		Native []struct {
			Input                           input
			ClassSupply, ClassDemand, Price int
		}
	}
	data, err := os.ReadFile(filepath.Join(output, "native-banville-fixed-price.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	registry, err := world.NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r := Reducer{World: registry}
	report := struct {
		Scope                           string
		Cases, Differences, Unsupported int
		Native                          []observation
	}{Scope: "正式CurrentLotPrice；同一Banville／保存分類位移，活躍市場刻意不同。只驗正式估值路徑，不宣稱九期交付或正常Banville保存完成。"}
	for _, n := range native.Native {
		in := n.Input
		row := observation{Input: in, Price: -1, Supported: true}
		if _, ok := registry.CargoKindByCode(in.Goods); !ok {
			row.Supported = false
			row.Reason = "正式交易沒有此商品入口"
			report.Unsupported++
			report.Native = append(report.Native, row)
			continue
		}
		state := NewState()
		state.CityID = world.MissionDestinationID
		state.TradeClassShift = in.Shift
		market := CityMarket{Supply: make([]int, 32), Demand: make([]int, 32), LastSeen: state.TimeMinutes / (24 * 60)}
		for goods := 1; goods < 32; goods++ {
			market.Supply[goods] = in.Supply
			market.Demand[goods] = in.Demand
		}
		state.Markets = map[string]CityMarket{state.CityID: market}
		marketBefore, err := json.Marshal(state.Markets)
		if err != nil {
			t.Fatal(err)
		}
		rollsBefore := state.TradeRolls
		var ok bool
		row.Price, ok = r.CurrentLotPrice(&state, Lot{Goods: in.Goods, Quality: 3, Price: 100}, in.Selling)
		if !ok {
			t.Fatal("正式來源不完整")
		}
		marketAfter, err := json.Marshal(state.Markets)
		if err != nil || !bytes.Equal(marketBefore, marketAfter) || state.TradeRolls != rollsBefore {
			t.Fatal("固定分類估值改變市場或交易亂數")
		}
		report.Cases++
		if row.Price != n.Price {
			report.Differences++
		}
		report.Native = append(report.Native, row)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-banville-fixed-price.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("formal fixed price cases%d differences%d unsupported%d", report.Cases, report.Differences, report.Unsupported)
}

// 入口：docs/atari-st/banville-restoration-spec.md；原版輸出只作測試oracle，不進runtime。
func TestExportTalosBanvillePrice(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("需Docker原生研究收據")
	}
	type input struct {
		Goods, Quality, Supply, Demand, City, Origin, StoredPrice int
		Selling                                                   bool
	}
	type observation struct {
		Input     input
		Price     int
		Supported bool
		Reason    string `json:",omitempty"`
	}
	var native struct {
		Native []struct {
			Input input
			Price int
		}
		PriceLevel, StreetDealRating int
	}
	data, err := os.ReadFile(filepath.Join(output, "native-banville-price.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	registry, err := world.NewSourceRegistry()
	if err != nil {
		t.Fatal(err)
	}
	system, ok := registry.System(world.StartSystemID)
	if !ok || system.PriceLevel != native.PriceLevel || system.StreetDealRating != native.StreetDealRating {
		t.Fatal("系統來源欄位不可比")
	}
	reducer := Reducer{World: registry}
	report := struct {
		Scope                           string
		Cases, Differences, Unsupported int
		Native                          []observation
	}{
		Scope: "同供需、商品、品質、來源城市／購入價、Jondd系統與城市；一般城市驗正式CurrentLotPrice，Banville任意供需初態只驗共用估值算術helper。正式Banville分類選擇由TestExportTalosBanvilleFixedPrice另驗。缺商品入口明示unsupported；不當作完整Banville交易／保存驗收。",
	}
	for _, n := range native.Native {
		in := n.Input
		row := observation{Input: in, Price: -1, Supported: true}
		if _, exists := registry.CargoKindByCode(in.Goods); !exists {
			row.Supported = false
			row.Reason = "正式交易沒有此商品入口"
		}
		if !row.Supported {
			report.Unsupported++
			report.Native = append(report.Native, row)
			continue
		}
		state := NewState()
		state.CityID = map[int]string{1: "drahew", 2: "obburg", 60: world.MissionDestinationID}[in.City]
		market := CityMarket{Supply: make([]int, 32), Demand: make([]int, 32), LastSeen: state.TimeMinutes / (24 * 60)}
		for goods := 1; goods < 32; goods++ {
			market.Supply[goods] = in.Supply
			market.Demand[goods] = in.Demand
		}
		state.Markets = map[string]CityMarket{state.CityID: market}
		lot := Lot{Goods: in.Goods, Quality: in.Quality, Price: in.StoredPrice, SourceCode: uint8(in.Origin)}
		if in.City == 60 {
			row.Price, ok = reducer.currentLotPriceForSupplyDemand(&state, lot, in.Selling, in.Supply, in.Demand)
		} else {
			row.Price, ok = reducer.CurrentLotPrice(&state, lot, in.Selling)
		}
		if !ok {
			t.Fatalf("正式來源缺少貨物 %+v", in)
		}
		report.Cases++
		if row.Price != n.Price {
			report.Differences++
		}
		report.Native = append(report.Native, row)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-banville-price.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("formal price cases%d differences%d unsupported%d", report.Cases, report.Differences, report.Unsupported)
}
