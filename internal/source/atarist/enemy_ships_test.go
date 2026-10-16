package atarist

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedEnemyShipDataCarriesEveryType(t *testing.T) {
	data, err := EmbeddedEnemyShipData()
	if err != nil {
		t.Fatalf("load embedded enemy ships: %v", err)
	}
	if data.Count() < 2 {
		t.Fatalf("only %d enemy ship types are decoded", data.Count())
	}
	for _, ship := range data.Ships() {
		if ship.Name == "" || ship.Armour <= 0 || ship.FireRate <= 0 || len(ship.Outline) == 0 {
			t.Fatalf("ship %d is incomplete: %+v", ship.Index, ship)
		}
	}
	if _, ok := data.Ship(data.Count()); ok {
		t.Fatal("an index past the end was accepted")
	}
}

// TestEmbeddedEnemyShipsSpreadTheirStats is what tells the table apart from a
// misread: seven identical rows would decode without complaint but would mean
// the record size is wrong.
func TestEmbeddedEnemyShipsSpreadTheirStats(t *testing.T) {
	data, err := EmbeddedEnemyShipData()
	if err != nil {
		t.Fatalf("load embedded enemy ships: %v", err)
	}
	seen := map[[4]int]bool{}
	for _, ship := range data.Ships() {
		seen[[4]int{ship.WeaponPower, ship.Armour, ship.Accuracy, ship.FireRate}] = true
	}
	if len(seen) != data.Count() {
		t.Fatalf("%d ship types share only %d distinct stat rows", data.Count(), len(seen))
	}
}

func TestEmbeddedEnemyShipDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/enemy-ships.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(enemyShipsJSON) {
		t.Fatalf("embedded assets/enemy-ships.json differs from docs/atari-st/enemy-ships.json")
	}
}

func TestLoadEnemyShipDataRejectsBrokenDocuments(t *testing.T) {
	for name, document := range map[string]string{
		"wrong schema": `{"schema":2,"ships":[]}`,
		"no ships":     `{"schema":1,"ships":[],"outlines":[]}`,
		"partial coverage": `{"schema":1,"region":{"size":1024},"coverage":{"total":1000},
			"ships":[{"index":0,"name":"a","armour":1,"fireRate":1}],
			"outlines":[{"index":0,"points":[[1,2]]}]}`,
		"zero armour": `{"schema":1,"region":{"size":4},"coverage":{"total":4},
			"ships":[{"index":0,"name":"a","armour":0,"fireRate":1}],
			"outlines":[{"index":0,"points":[[1,2]]}]}`,
		"zero fire rate": `{"schema":1,"region":{"size":4},"coverage":{"total":4},
			"ships":[{"index":0,"name":"a","armour":1,"fireRate":0}],
			"outlines":[{"index":0,"points":[[1,2]]}]}`,
		"missing outline": `{"schema":1,"region":{"size":4},"coverage":{"total":4},
			"ships":[{"index":0,"name":"a","armour":1,"fireRate":1}],
			"outlines":[{"index":1,"points":[[1,2]]}]}`,
		"out of order": `{"schema":1,"region":{"size":4},"coverage":{"total":4},
			"ships":[{"index":1,"name":"a","armour":1,"fireRate":1}],
			"outlines":[{"index":1,"points":[[1,2]]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadEnemyShipData(strings.NewReader(document)); err == nil {
				t.Fatal("a broken document was accepted")
			}
		})
	}
}
