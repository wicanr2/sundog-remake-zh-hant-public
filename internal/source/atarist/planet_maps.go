package atarist

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

const PlanetMapWidth, PlanetMapHeight = 40, 24

// PlanetSurface 保留解碼後地形狀態；外觀亂數及城市覆蓋不寫進來源資料。
type PlanetSurface struct {
	PlanetIndex int     `json:"planetIndex"`
	RawOffset   int     `json:"rawOffset"`
	Header      []int   `json:"header"`
	States      [][]int `json:"states"`
}

//go:embed assets/planet-maps.json
var planetMapsJSON []byte

var planetMapsOnce sync.Once
var planetMaps []PlanetSurface
var planetMapsErr error

func loadPlanetSurfaces(data []byte) ([]PlanetSurface, error) {
	var doc struct {
		Schema     int             `json:"schema"`
		DiskSHA256 string          `json:"diskSHA256"`
		Maps       []PlanetSurface `json:"maps"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Schema != 1 || doc.DiskSHA256 != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6" || len(doc.Maps) != 18 {
		return nil, fmt.Errorf("行星地表資料版本或筆數不符")
	}
	for i, m := range doc.Maps {
		if m.PlanetIndex != i || m.RawOffset != 0xaa00+i*256 || len(m.Header) != 16 || len(m.States) != PlanetMapHeight {
			return nil, fmt.Errorf("行星 %d 的資料邊界不符", i)
		}
		for _, b := range m.Header {
			if b < 0 || b > 255 {
				return nil, fmt.Errorf("行星 %d 的 header 超界", i)
			}
		}
		for _, row := range m.States {
			if len(row) != PlanetMapWidth {
				return nil, fmt.Errorf("行星 %d 的列寬不符", i)
			}
			for _, state := range row {
				if state < 0 || state > 13 {
					return nil, fmt.Errorf("行星 %d 的地形狀態超界", i)
				}
			}
		}
	}
	return doc.Maps, nil
}

func PlanetSurfaceAt(index int) (PlanetSurface, error) {
	planetMapsOnce.Do(func() { planetMaps, planetMapsErr = loadPlanetSurfaces(planetMapsJSON) })
	if planetMapsErr != nil {
		return PlanetSurface{}, planetMapsErr
	}
	if index < 0 || index >= len(planetMaps) {
		return PlanetSurface{}, fmt.Errorf("行星索引 %d 超界", index)
	}
	m := planetMaps[index]
	m.Header = append([]int(nil), m.Header...)
	m.States = append([][]int(nil), m.States...)
	for i := range m.States {
		m.States[i] = append([]int(nil), m.States[i]...)
	}
	return m, nil
}
