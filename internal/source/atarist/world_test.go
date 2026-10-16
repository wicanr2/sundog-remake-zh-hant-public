package atarist

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTradeClassSourceRejectsMissingResolvedFields(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/trade-classes.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(docs, tradeClassesJSON) {
		t.Fatal("分類來源文件與embedded資料不同")
	}
	for _, key := range []string{"startupCounter", "supplyHomeGoods", "demandHomeGoods"} {
		for _, value := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("-1"), json.RawMessage("256")} {
			var document map[string]json.RawMessage
			if err = json.Unmarshal(docs, &document); err != nil {
				t.Fatal(err)
			}
			if key == "startupCounter" {
				var source map[string]json.RawMessage
				_ = json.Unmarshal(document["source"], &source)
				delete(source, key)
				if value != nil {
					source[key] = value
				}
				document["source"], _ = json.Marshal(source)
			} else {
				var rows []map[string]json.RawMessage
				_ = json.Unmarshal(document["records"], &rows)
				delete(rows[0], key)
				if value != nil {
					rows[0][key] = value
				}
				document["records"], _ = json.Marshal(rows)
			}
			bad, _ := json.Marshal(document)
			if _, err = LoadTradeClassData(bytes.NewReader(bad)); err == nil {
				t.Fatalf("分類來源缺項或越界被接受:%s %s", key, value)
			}
		}
	}
}

func TestEmbeddedWorldDataMatchesSourceTables(t *testing.T) {
	data, err := EmbeddedWorldData()
	if err != nil {
		t.Fatalf("load embedded world data: %v", err)
	}
	if got := len(data.Systems()); got != systemRecordCount {
		t.Fatalf("systems = %d, want %d", got, systemRecordCount)
	}
	if got := len(data.Planets()); got != planetRecordCount {
		t.Fatalf("planets = %d, want %d", got, planetRecordCount)
	}
	// 60 source records minus the two reserved filler slots.
	if got := len(data.Cities()); got != cityRecordCount-2 {
		t.Fatalf("cities = %d, want %d", got, cityRecordCount-2)
	}

	systems := data.Systems()
	if systems[0].Name != "Jondd" {
		t.Fatalf("first system = %q, want Jondd", systems[0].Name)
	}
	planets := data.Planets()
	if planets[0].Name != "Jondd" || planets[0].SystemIndex != 0 {
		t.Fatalf("first planet = %q (system %d), want Jondd in system 0", planets[0].Name, planets[0].SystemIndex)
	}
	cities := data.Cities()
	if cities[0].Name != "Drahew" || cities[0].PlanetIndex != 0 {
		t.Fatalf("first city = %q (planet %d), want Drahew on planet 0", cities[0].Name, cities[0].PlanetIndex)
	}
}

func TestEmbeddedWorldDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/world-data.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(worldDataJSON) {
		t.Fatalf("embedded assets/world-data.json differs from docs/atari-st/world-data.json")
	}
}

func TestEmbeddedWorldDataKeepsHiddenColonyOutsideRuns(t *testing.T) {
	data, err := EmbeddedWorldData()
	if err != nil {
		t.Fatalf("load embedded world data: %v", err)
	}
	hidden := make([]City, 0, 1)
	for _, city := range data.Cities() {
		if city.Hidden {
			hidden = append(hidden, city)
		}
	}
	if len(hidden) != 1 {
		t.Fatalf("hidden cities = %d, want 1", len(hidden))
	}
	if hidden[0].Name != "Banville" {
		t.Fatalf("hidden city = %q, want Banville", hidden[0].Name)
	}
	planets := data.Planets()
	if planets[hidden[0].PlanetIndex].Name != "Jondd" {
		t.Fatalf("hidden city planet = %q, want Jondd", planets[hidden[0].PlanetIndex].Name)
	}
}

func TestEmbeddedWorldDataPreservesRawEvidence(t *testing.T) {
	data, err := EmbeddedWorldData()
	if err != nil {
		t.Fatalf("load embedded world data: %v", err)
	}
	for _, system := range data.Systems() {
		if len(system.TailHex) != (systemRecordSize-nameFieldSize)*2 {
			t.Fatalf("system %q tail = %q", system.Name, system.TailHex)
		}
		if system.RawOffset < 0x05800 {
			t.Fatalf("system %q raw offset = 0x%05x", system.Name, system.RawOffset)
		}
	}
	for _, planet := range data.Planets() {
		if len(planet.TailHex) != (planetRecordSize-nameFieldSize)*2 {
			t.Fatalf("planet %q tail = %q", planet.Name, planet.TailHex)
		}
	}
	for _, city := range data.Cities() {
		if len(city.TailHex) != (cityRecordSize-nameFieldSize)*2 {
			t.Fatalf("city %q tail = %q", city.Name, city.TailHex)
		}
	}
}

func TestSlugKeepsCityIdentitiesUnique(t *testing.T) {
	data, err := EmbeddedWorldData()
	if err != nil {
		t.Fatalf("load embedded world data: %v", err)
	}
	seen := make(map[string]string)
	for _, city := range data.Cities() {
		if city.ID == "" {
			t.Fatalf("city %q has an empty identity", city.Name)
		}
		if previous, exists := seen[city.ID]; exists {
			t.Fatalf("cities %q and %q share identity %q", previous, city.Name, city.ID)
		}
		seen[city.ID] = city.Name
	}
	if got := slug("Ft. Lyon"); got != "ft-lyon" {
		t.Fatalf("slug(Ft. Lyon) = %q", got)
	}
	if got := slug("W'orrad"); got != "w-orrad" {
		t.Fatalf("slug(W'orrad) = %q", got)
	}
}

const minimalWorldDocument = `{
  "schema": 1,
  "tables": {
    "systems": {"rawOffset": "0x05800", "recordSize": 20, "recordCount": 12, "records": []},
    "planets": {"rawOffset": "0x058f0", "recordSize": 16, "recordCount": 18, "records": []},
    "cities": {"rawOffset": "0x05a10", "recordSize": 12, "recordCount": 60, "records": []}
  },
  "grouping": {"contiguousCityCount": 57, "fillerCityCount": 2, "trailingCityCount": 1}
}`

func TestLoadWorldDataRejectsRecordCountDrift(t *testing.T) {
	if _, err := LoadWorldData(strings.NewReader(minimalWorldDocument)); err == nil {
		t.Fatalf("expected an error when a table declares more records than it carries")
	}
}

func TestLoadWorldDataRejectsUnsupportedSchema(t *testing.T) {
	document := strings.Replace(minimalWorldDocument, `"schema": 1`, `"schema": 2`, 1)
	if _, err := LoadWorldData(strings.NewReader(document)); err == nil {
		t.Fatalf("expected an error for an unsupported schema")
	}
}
