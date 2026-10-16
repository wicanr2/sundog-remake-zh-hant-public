package main

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParseAuxiliaryMapPreservesPairsTriplesAndPhase(t *testing.T) {
	archive := makeMapArchive(t, map[string]string{
		mapPlanetsHTML: `<img src="Sundog%20Maps_fichiers/NewShoot_NewShoot.gif"><img src="Sundog%20Maps_fichiers/Star_Map.gif">`,
		mapCitiesHTML:  `<strong>New Shoot/New Shoot/New Temol (Phase 9)</strong><strong><img src="ignored.gif"></strong>`,
	})
	catalog, err := parseAuxiliaryMap(archive)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.PlanetPairCount != 1 || catalog.PlanetPairs[0] != (mapPlanetPair{System: "NewShoot", Planet: "NewShoot"}) {
		t.Fatalf("planet pairs = %#v", catalog.PlanetPairs)
	}
	if catalog.CityTripleCount != 1 {
		t.Fatalf("city triples = %#v", catalog.CityTriples)
	}
	triple := catalog.CityTriples[0]
	if triple.System != "New Shoot" || triple.Planet != "New Shoot" || triple.City != "New Temol" || triple.Phase != "Phase 9" {
		t.Fatalf("city triple = %#v", triple)
	}
}

func TestCompareNamesUsesDeclaredNormalizationWithoutGuessing(t *testing.T) {
	coverage := compareNames([]nameRecord{
		{Name: "New Shoot"},
		{Name: "W'orrad"},
		{Name: "D'helro"},
	}, []string{"NewShoot", "Worrad", "Vahevela"})
	if coverage.MatchCount != 2 || len(coverage.CandidateOnlyNames) != 1 || coverage.CandidateOnlyNames[0] != "D'helro" {
		t.Fatalf("unexpected candidate coverage: %#v", coverage)
	}
	if len(coverage.MapOnlyNames) != 1 || coverage.MapOnlyNames[0] != "Vahevela" {
		t.Fatalf("unexpected map-only coverage: %#v", coverage)
	}
}

func TestParseDiskCandidatesStopsPlanetTableBeforeCityBlock(t *testing.T) {
	disk := make([]byte, 0x3010+20*12)
	for index := 0; index < 13; index++ {
		putFixedName(disk, 0x2100+index*20, 20, "System")
	}
	for index := 0; index < 17; index++ {
		putFixedName(disk, 0x2f00+index*16, 16, "Planet")
	}
	putFixedName(disk, 0x3010, 12, "Drahew")
	for index := 1; index < 20; index++ {
		putFixedName(disk, 0x3010+index*12, 12, "City")
	}
	candidates, err := parseDiskCandidates(disk)
	if err != nil {
		t.Fatal(err)
	}
	if candidates.Planets.RecordCount != 17 || len(candidates.Planets.Entries) != 17 {
		t.Fatalf("planet table count regressed: %#v", candidates.Planets)
	}
	if candidates.Cities.Entries[0].Name != "Drahew" {
		t.Fatalf("city block was not preserved: %#v", candidates.Cities.Entries[0])
	}
}

func makeMapArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, contents := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func putFixedName(data []byte, offset, recordSize int, name string) {
	if len(name) >= recordSize {
		panic("test fixture name does not fit record")
	}
	data[offset] = byte(len(name))
	copy(data[offset+1:offset+recordSize], name)
}
