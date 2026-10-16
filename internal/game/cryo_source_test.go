package game

import (
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

func TestNewGameCryoWarehouses(t *testing.T) {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]int{}
	for _, city := range data.Cities() {
		byID[city.ID] = city.Index + 1
	}
	for seed := int64(0); seed < 256; seed++ {
		lots := newGameWarehouses(seed)
		if !reflect.DeepEqual(lots, newGameWarehouses(seed)) {
			t.Fatalf("seed %d does not replay", seed)
		}
		count := 0
		for city, slots := range lots {
			if len(slots) != WarehouseLots {
				t.Fatal("invalid warehouse slot count")
			}
			for i, lot := range slots {
				if lot.Goods == 0 {
					if lot != (Lot{}) {
						t.Fatal("new empty slot has residue")
					}
					continue
				}
				if i != 0 || lot != (Lot{Goods: 31, Quality: 7, SourceCode: uint8(byID[city])}) || byID[city] > 57 || byID[city] == 16 {
					t.Fatalf("invalid cold lot %s %v", city, lot)
				}
				count++
			}
		}
		if count != 17 || lots["tuie"][0].Goods != 31 {
			t.Fatalf("seed %d has %d batches", seed, count)
		}
	}
	if !reflect.DeepEqual(NewState().Warehouses, newGameWarehouses(NewGameSeed())) {
		t.Fatal("NewState did not use the initializer")
	}
}

func TestCryoInitializerRejectsInvalidSourceAndRandom(t *testing.T) {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []string{"systems", "planet", "interval", "city", "duplicate", "initial", "negative-random", "high-random", "nil-random"} {
		t.Run(test, func(t *testing.T) {
			systems, planets, cities := data.Systems(), data.Planets(), data.Cities()
			initial := 1
			random := func() int { return 0 }
			switch test {
			case "systems":
				systems = systems[:2]
			case "planet":
				systems[2].PlanetStart = len(planets)
			case "interval":
				planets[systems[2].PlanetStart].CityCount = 0
			case "city":
				cities = cities[1:]
			case "duplicate":
				cities[1].ID = cities[0].ID
			case "initial":
				initial = 58
			case "negative-random":
				random = func() int { return -1 }
			case "high-random":
				random = func() int { return 32768 }
			case "nil-random":
				random = nil
			}
			if lots, err := initialCryoWarehouses(systems, planets, cities, initial, random); err == nil || lots != nil {
				t.Fatal("invalid initializer returned usable warehouses")
			}
		})
	}
}
