package atarist

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedCityMapsCoverEveryCity(t *testing.T) {
	data, err := EmbeddedCityMapData()
	if err != nil {
		t.Fatalf("load embedded city maps: %v", err)
	}
	if data.Count() != 60 {
		t.Fatalf("%d city maps, want 60", data.Count())
	}
	exchanges := 0
	for index := 0; index < data.Count(); index++ {
		city, ok := data.City(index)
		if !ok {
			t.Fatalf("city %d is missing", index)
		}
		if city.Number != index+1 {
			t.Fatalf("city %d is numbered %d", index, city.Number)
		}
		for row := 0; row < CityMapRows; row++ {
			for column := 0; column < CityMapColumns; column++ {
				if _, ok := city.Terrain(row, column); !ok {
					t.Fatalf("city %d cell (%d,%d) is missing", index, row, column)
				}
			}
		}
		if _, ok := city.Terrain(CityMapRows, 0); ok {
			t.Fatalf("city %d returned a cell past the last row", index)
		}
		for _, building := range city.Buildings() {
			if building.Code != 0x22+building.Slot {
				t.Fatalf("city %d slot %d has code 0x%02x", index, building.Slot, building.Code)
			}
			if building.Facility == FacilityExchange {
				exchanges++
			}
		}
	}
	// The exchange is the one building the original places in every city, and
	// the remake lands the player on it, so a map without one would strand a
	// city. All 60 have to have exactly one.
	if exchanges != 60 {
		t.Fatalf("%d cities have an exchange building, want 60", exchanges)
	}
}

func TestEmbeddedCityMapsMatchEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/city-maps.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(cityMapsJSON) {
		t.Fatal("embedded assets/city-maps.json differs from docs/atari-st/city-maps.json")
	}
}

func TestCityMapDataRejectsAGridThatIsNotTheOriginalShape(t *testing.T) {
	const document = `{"schema":1,"layout":{"gridRows":10,"gridColumns":12,"buildingSlots":6},"cities":[]}`
	if _, err := LoadCityMapData(strings.NewReader(document)); err == nil {
		t.Fatal("a grid of the wrong shape was accepted")
	}
}

func TestCityMapDataRejectsABuildingOutsideTheMap(t *testing.T) {
	document := `{"schema":1,"layout":{"gridRows":20,"gridColumns":12,"buildingSlots":6},"cities":[
		{"index":0,"cityNumber":1,"buildings":[{"slot":0,"code":34,"row":20,"column":0}],"grid":` +
		emptyGridJSON() + `}]}`
	if _, err := LoadCityMapData(strings.NewReader(document)); err == nil {
		t.Fatal("a building outside the map was accepted")
	}
}

func TestCityMapDataRejectsACellThatIsNotFourBits(t *testing.T) {
	document := `{"schema":1,"layout":{"gridRows":20,"gridColumns":12,"buildingSlots":6},"cities":[
		{"index":0,"cityNumber":1,"buildings":[],"grid":` + gridWithFirstCell(16) + `}]}`
	if _, err := LoadCityMapData(strings.NewReader(document)); err == nil {
		t.Fatal("a cell wider than four bits was accepted")
	}
}

func TestCityMapDataAcceptsAWellFormedDocument(t *testing.T) {
	document := `{"schema":1,"layout":{"gridRows":20,"gridColumns":12,"buildingSlots":6},"cities":[
		{"index":0,"cityNumber":1,"buildings":[{"slot":2,"code":36,"row":3,"column":4}],"grid":` +
		gridWithFirstCell(9) + `}]}`
	data, err := LoadCityMapData(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	city, ok := data.City(0)
	if !ok {
		t.Fatal("the decoded city is missing")
	}
	if code, ok := city.Terrain(0, 0); !ok || code != 9 {
		t.Fatalf("first cell = %d (%t), want 9", code, ok)
	}
	buildings := city.Buildings()
	if len(buildings) != 1 || buildings[0].Facility != FacilityExchange {
		t.Fatalf("buildings = %+v, want one exchange", buildings)
	}
}

func emptyGridJSON() string {
	return gridWithFirstCell(0)
}

func gridWithFirstCell(value int) string {
	rows := make([]string, CityMapRows)
	for row := range rows {
		cells := make([]string, CityMapColumns)
		for column := range cells {
			cells[column] = "0"
		}
		if row == 0 {
			cells[0] = fmt.Sprintf("%d", value)
		}
		rows[row] = "[" + strings.Join(cells, ",") + "]"
	}
	return "[" + strings.Join(rows, ",") + "]"
}

// TestEveryCityMapCarriesItsName locks the declaration against the data. The
// decoder declares a cityName and CityByName looks it up, but the generator
// only fills that field when it is handed the world table — regenerate without
// it and every lookup silently finds nothing.
func TestEveryCityMapCarriesItsName(t *testing.T) {
	data, err := EmbeddedCityMapData()
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < data.Count(); index++ {
		city, ok := data.City(index)
		if !ok {
			t.Fatalf("city %d is missing", index)
		}
		if city.Name == "" {
			t.Fatalf("city %d has no name", index)
		}
	}
	if city, ok := data.CityByName("Drahew"); !ok || city.Index != 0 {
		t.Fatalf("CityByName(Drahew) = %+v, %v", city, ok)
	}
}
