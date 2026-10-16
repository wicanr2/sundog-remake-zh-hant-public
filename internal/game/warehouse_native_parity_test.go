package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 原始Store／Fetch六槽狀態比較，Swap缺入口獨立列未支援。
func TestExportTalosWarehouseTransfer(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("需要Docker原生倉庫收據")
	}
	var native struct {
		Rows []struct {
			Mode, SelectedWarehouse, SelectedPod int
			Before, After                        [6][2]uint16
		}
	}
	b, err := os.ReadFile(filepath.Join(out, "native-warehouse-transfer.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &native); err != nil {
		t.Fatal(err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		StoreCases, FetchCases, Differences, SwapUnsupported int
		StoreDifferences, FetchDifferences                   int
	}
	decode := func(w [2]uint16) Lot {
		return Lot{Goods: int(w[0] & 31), Quality: int(w[0] >> 5 & 7), SourceCode: uint8(w[0] >> 8), Price: int(w[1])}
	}
	encode := func(l Lot) [2]uint16 {
		return [2]uint16{uint16(l.Goods | l.Quality<<5 | int(l.SourceCode)<<8), uint16(l.Price)}
	}
	for _, n := range native.Rows {
		if n.Mode == 3 {
			result.SwapUnsupported++
			continue
		}
		s := NewState()
		s.CityID = "tuie"
		s.Holds = [2]Hold{}
		s.Warehouses = map[string][]Lot{s.CityID: make([]Lot, 4)}
		for i, w := range n.Before {
			l := decode(w)
			if i < 4 {
				s.Warehouses[s.CityID][i] = l
				continue
			}
			if l.Goods > 0 {
				kind, ok := r.World.CargoKindByCode(l.Goods)
				if !ok {
					t.Fatal("missing cargo")
				}
				s.Holds[i-4] = goodsHold(l, kind.Key)
			}
		}
		bank, cash := s.Bank, s.Cash
		if n.Mode == 1 {
			if n.SelectedPod > 0 {
				r.storeLot(&s, n.SelectedPod-1)
			}
			result.StoreCases++
		}
		if n.Mode == 2 {
			if n.SelectedWarehouse > 0 {
				r.fetchLot(&s, n.SelectedWarehouse-1)
			}
			result.FetchCases++
		}
		var after [6][2]uint16
		for i := 0; i < 4; i++ {
			after[i] = encode(s.Warehouses[s.CityID][i])
		}
		for i := 0; i < 2; i++ {
			after[i+4] = encode(s.Holds[i].Lot())
		}
		if after != n.After || s.Bank != bank || s.Cash != cash {
			result.Differences++
			if n.Mode == 1 {
				result.StoreDifferences++
			} else {
				result.FetchDifferences++
			}
			if result.Differences == 1 {
				t.Logf("first difference mode=%d pod=%d warehouse=%d before=%v native=%v remake=%v", n.Mode, n.SelectedPod, n.SelectedWarehouse, n.Before, n.After, after)
			}
		}
	}
	b, err = json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-warehouse-transfer.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("store%d fetch%d differences%d swapUnsupported%d", result.StoreCases, result.FetchCases, result.Differences, result.SwapUnsupported)
}
