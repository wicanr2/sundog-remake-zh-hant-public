package atarist

import (
	"encoding/json"
	"fmt"
	"io"
)

// 60 座城市各有一張自己的地表地圖。版面來自原版的 `XMOVEONG:0x22 draw_city_map`：
// 20 列 × 12 格的 4-bit 地形，加上六個建築位置。走上哪一格會進哪個設施也是原版的
// 分派：建築槽位 0、1、2 分別是接駁站、倉庫與交易所。
//
// **地形每一格代表什麼地物不在這裡宣稱**。原版的地形圖塊是每座城市各自載入的三組
// 影像，而通行判定又是以繪出來的像素顏色決定的（碰撞色集合 `01111100`，也就是
// 調色盤索引 1–5 擋路）。兩者都要先解出影像串流才能重現，因此 remake 的外觀與通行
// 規則仍是自有的；這個 consumer 只提供**版面、地形碼與設施位置**這三件已證實的事。

const (
	// CityMapRows／CityMapColumns 是原版地圖的尺寸，不是 remake 畫面的尺寸。
	CityMapRows    = 20
	CityMapColumns = 12
	// 建築槽位在原版畫面緩衝區裡的圖塊碼是 0x22–0x27；槽位 3–5 只出現在兩筆
	// dummy 城市記錄裡，真城市用不到。
	CityBuildingSlots = 6
)

// CityFacility 是走上某個建築槽位會進到的設施。名稱取自原版程序自己的名字。
type CityFacility string

const (
	FacilityTubeStation CityFacility = "tube-station"
	FacilityDepot       CityFacility = "depot"
	FacilityExchange    CityFacility = "exchange"
	FacilityNone        CityFacility = ""
)

// facilityBySlot 是原版 `XMOVEONG:0x1e check_building` 的分派：槽位 0 的碼 18 走
// do_tube_station、槽位 1 的碼 19 走 do_depot、槽位 2 的碼 20 走 do_exchange。
// 槽位 3–5 不在分派表裡。
var facilityBySlot = map[int]CityFacility{
	0: FacilityTubeStation,
	1: FacilityDepot,
	2: FacilityExchange,
}

// CityBuilding 是一座城市地圖上的一棟建築。
type CityBuilding struct {
	Slot     int
	Code     int
	Row      int
	Column   int
	Facility CityFacility
}

// CityMap 是一座城市的地表地圖。
type CityMap struct {
	Index     int
	Number    int
	Name      string
	RawOffset string
	// Header is the four-byte source map header. It has no established
	// identity with MAINLIB_G10b; see ground-route-spec.md and the class
	// startup writer in banville-restoration-spec.md.
	Header    []int
	grid      [][]int
	buildings []CityBuilding
}

// HeaderField is one byte of the city header, or −1 when the record is short.
func (m CityMap) HeaderField(index int) int {
	if index < 0 || index >= len(m.Header) {
		return -1
	}
	return m.Header[index]
}

// Terrain 回傳某一格的地形碼（0–15）。超出範圍回 false。
func (m CityMap) Terrain(row, column int) (int, bool) {
	if row < 0 || row >= len(m.grid) || column < 0 || column >= len(m.grid[row]) {
		return 0, false
	}
	return m.grid[row][column], true
}

// Buildings 回傳這座城市的建築，順序即槽位順序。
func (m CityMap) Buildings() []CityBuilding {
	out := make([]CityBuilding, len(m.buildings))
	copy(out, m.buildings)
	return out
}

// CityMapData 是驗證過的 60 張地圖。
type CityMapData struct {
	cities []CityMap
}

// Count 回傳地圖數量。
func (d CityMapData) Count() int { return len(d.cities) }

// City 以 0-based 索引取一張地圖。
func (d CityMapData) City(index int) (CityMap, bool) {
	if index < 0 || index >= len(d.cities) {
		return CityMap{}, false
	}
	return d.cities[index], true
}

// CityByName 以世界表裡的城市名稱取一張地圖。兩張表都是 60 筆且共用同一個索引，
// 名稱因此是可用的鍵；重名時取第一筆。
func (d CityMapData) CityByName(name string) (CityMap, bool) {
	for _, city := range d.cities {
		if city.Name == name {
			return city, true
		}
	}
	return CityMap{}, false
}

type cityMapDocument struct {
	Schema int `json:"schema"`
	Layout struct {
		GridRows      int `json:"gridRows"`
		GridColumns   int `json:"gridColumns"`
		BuildingSlots int `json:"buildingSlots"`
	} `json:"layout"`
	Cities []struct {
		Index      int    `json:"index"`
		CityNumber int    `json:"cityNumber"`
		CityName   string `json:"cityName"`
		RawOffset  string `json:"rawOffset"`
		Header     struct {
			Fields []int `json:"fields"`
		} `json:"header"`
		Buildings []struct {
			Slot   int `json:"slot"`
			Code   int `json:"code"`
			Row    int `json:"row"`
			Column int `json:"column"`
		} `json:"buildings"`
		Grid [][]int `json:"grid"`
	} `json:"cities"`
}

// LoadCityMapData validates the document. The checks that earn their keep are
// the ones a wrong layout would fail: every grid has to be exactly 20 × 12 with
// 4-bit cells, and every building has to sit inside it. That is the same range
// argument the decoder makes against the disk.
func LoadCityMapData(reader io.Reader) (CityMapData, error) {
	var document cityMapDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return CityMapData{}, fmt.Errorf("decode Atari ST city maps: %w", err)
	}
	if document.Schema != 1 {
		return CityMapData{}, fmt.Errorf("unsupported Atari ST city map schema %d", document.Schema)
	}
	if document.Layout.GridRows != CityMapRows || document.Layout.GridColumns != CityMapColumns {
		return CityMapData{}, fmt.Errorf("city maps declare a %d × %d grid, want %d × %d",
			document.Layout.GridRows, document.Layout.GridColumns, CityMapRows, CityMapColumns)
	}
	if document.Layout.BuildingSlots != CityBuildingSlots {
		return CityMapData{}, fmt.Errorf("city maps declare %d building slots, want %d",
			document.Layout.BuildingSlots, CityBuildingSlots)
	}
	if len(document.Cities) == 0 {
		return CityMapData{}, fmt.Errorf("city maps contain no cities")
	}

	cities := make([]CityMap, 0, len(document.Cities))
	for position, entry := range document.Cities {
		if entry.Index != position {
			return CityMapData{}, fmt.Errorf("city %d declares index %d", position, entry.Index)
		}
		if len(entry.Grid) != CityMapRows {
			return CityMapData{}, fmt.Errorf("city %d has %d rows", position, len(entry.Grid))
		}
		grid := make([][]int, CityMapRows)
		for row, cells := range entry.Grid {
			if len(cells) != CityMapColumns {
				return CityMapData{}, fmt.Errorf("city %d row %d has %d cells", position, row, len(cells))
			}
			grid[row] = make([]int, CityMapColumns)
			for column, cell := range cells {
				if cell < 0 || cell > 0xF {
					return CityMapData{}, fmt.Errorf("city %d cell (%d,%d) is %d, not a 4-bit value",
						position, row, column, cell)
				}
				grid[row][column] = cell
			}
		}
		buildings := make([]CityBuilding, 0, len(entry.Buildings))
		for _, building := range entry.Buildings {
			if building.Slot < 0 || building.Slot >= CityBuildingSlots {
				return CityMapData{}, fmt.Errorf("city %d has building slot %d", position, building.Slot)
			}
			if building.Row < 0 || building.Row >= CityMapRows ||
				building.Column < 0 || building.Column >= CityMapColumns {
				return CityMapData{}, fmt.Errorf("city %d building %d sits at (%d,%d), outside the map",
					position, building.Slot, building.Row, building.Column)
			}
			buildings = append(buildings, CityBuilding{
				Slot:     building.Slot,
				Code:     building.Code,
				Row:      building.Row,
				Column:   building.Column,
				Facility: facilityBySlot[building.Slot],
			})
		}
		cities = append(cities, CityMap{
			Index:     entry.Index,
			Number:    entry.CityNumber,
			Name:      entry.CityName,
			RawOffset: entry.RawOffset,
			Header:    append([]int(nil), entry.Header.Fields...),
			grid:      grid,
			buildings: buildings,
		})
	}
	return CityMapData{cities: cities}, nil
}
