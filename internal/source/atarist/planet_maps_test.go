package atarist

import (
	"encoding/json"
	"testing"
)

func TestPlanetSurfacesValidateAllRecords(t *testing.T) {
	for i := 0; i < 18; i++ {
		m, err := PlanetSurfaceAt(i)
		if err != nil {
			t.Fatal(err)
		}
		if m.RawOffset != 0xaa00+i*256 || len(m.States) != 24 || len(m.States[23]) != 40 {
			t.Fatalf("行星 %d 邊界不符", i)
		}
		m.States[0][0] = 99
		m.Header[0] = 999
		again, err := PlanetSurfaceAt(i)
		if err != nil || again.States[0][0] == 99 || again.Header[0] == 999 {
			t.Fatal("呼叫端污染來源")
		}
	}
	for _, i := range []int{-1, 18} {
		if _, err := PlanetSurfaceAt(i); err == nil {
			t.Fatal("接受越界行星")
		}
	}
}

func TestPlanetSurfacesRejectCorruptShape(t *testing.T) {
	for _, field := range []string{"index", "offset", "header", "rows", "columns", "state"} {
		t.Run(field, func(t *testing.T) {
			var doc struct {
				Schema     int             `json:"schema"`
				DiskSHA256 string          `json:"diskSHA256"`
				Maps       []PlanetSurface `json:"maps"`
			}
			if err := json.Unmarshal(planetMapsJSON, &doc); err != nil {
				t.Fatal(err)
			}
			m := &doc.Maps[17]
			switch field {
			case "index":
				m.PlanetIndex = 0
			case "offset":
				m.RawOffset++
			case "header":
				m.Header = m.Header[:15]
			case "rows":
				m.States = m.States[:23]
			case "columns":
				m.States[23] = m.States[23][:39]
			case "state":
				m.States[23][39] = 14
			}
			data, _ := json.Marshal(doc)
			if _, err := loadPlanetSurfaces(data); err == nil {
				t.Fatal("損壞資料未被拒絕")
			}
		})
	}
}
