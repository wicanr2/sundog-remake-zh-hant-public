package atarist

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ColonyPhase 保留原始八格順序；零碼是空欄，其餘重複碼代表多筆需求。
type ColonyPhase struct {
	Stage          int
	Goods          [8]int
	RequiredCounts [32]int
}

//go:embed assets/colony-phases.json
var colonyPhasesJSON []byte

// LoadColonyPhases 驗證 XSTARTUP writer 與 XDOTRADI reader 共同的九階段格式。
// 這是需求資料介面，不自行推定交付、成長或舊存檔遷移規則。
func LoadColonyPhases(data []byte) ([9]ColonyPhase, error) {
	var result [9]ColonyPhase
	var doc struct {
		Schema             int    `json:"schema"`
		SourceSHA256       string `json:"sourceSHA256"`
		Segment            string `json:"segment"`
		SegmentRawBase     string `json:"segmentRawBase"`
		ConstantPoolOffset string `json:"constantPoolOffset"`
		Phases             []struct {
			Stage          int    `json:"stage"`
			RawOffset      string `json:"rawOffset"`
			SourceHex      string `json:"sourceHex"`
			Goods          []int  `json:"goods"`
			RequiredCounts []int  `json:"requiredCounts"`
		} `json:"phases"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return result, err
	}
	if doc.Schema != 1 || doc.SourceSHA256 != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6" || doc.Segment != "XSTARTUP" || doc.SegmentRawBase != "0x39c00" || doc.ConstantPoolOffset != "0x2e20" || len(doc.Phases) != 9 {
		return result, fmt.Errorf("殖民地階段來源或版面不符")
	}
	for i, row := range doc.Phases {
		raw, err := hex.DecodeString(row.SourceHex)
		if err != nil || len(raw) != 9 || raw[0] != 8 || row.Stage != i || row.RawOffset != fmt.Sprintf("0x%x", 0x3cfd8+i*10) || len(row.Goods) != 8 || len(row.RequiredCounts) != 32 {
			return [9]ColonyPhase{}, fmt.Errorf("殖民地階段 %d 邊界不符", i)
		}
		phase := ColonyPhase{Stage: i}
		for j, g := range row.Goods {
			if g < 0 || g > 31 || int(raw[j+1]) != g+64 {
				return [9]ColonyPhase{}, fmt.Errorf("殖民地階段 %d 貨物來源不符", i)
			}
			phase.Goods[j] = g
			if g != 0 {
				phase.RequiredCounts[g]++
			}
		}
		for g, n := range row.RequiredCounts {
			if n != phase.RequiredCounts[g] {
				return [9]ColonyPhase{}, fmt.Errorf("殖民地階段 %d 需求計數不符", i)
			}
		}
		result[i] = phase
	}
	return result, nil
}

// EmbeddedColonyPhases 回傳值型別，呼叫端修改需求不會污染來源或其他存檔。
func EmbeddedColonyPhases() ([9]ColonyPhase, error) {
	return LoadColonyPhases(colonyPhasesJSON)
}
