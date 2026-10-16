package atarist

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedItemDataDecodesBothTables(t *testing.T) {
	data, err := EmbeddedItemData()
	if err != nil {
		t.Fatalf("load embedded item data: %v", err)
	}
	items := data.Items()
	cargo := data.Cargo()
	if len(items) != 32 || len(cargo) != 32 {
		t.Fatalf("tables have %d items and %d cargo, want 32 each", len(items), len(cargo))
	}
	if items[0].Name != "(nothing)" {
		t.Fatalf("first item = %q, want the empty-slot record", items[0].Name)
	}
	if cargo[0].Name != "nothing" {
		t.Fatalf("first cargo = %q, want the empty-slot record", cargo[0].Name)
	}
	// Preserve absence of a separate value while retaining the byte the exchange reads.
	found := false
	for _, entry := range cargo {
		if entry.Name != "pharmaceuticals" {
			continue
		}
		found = true
		if entry.HasValue {
			t.Fatalf("pharmaceuticals claims a value it has no room for: %+v", entry)
		}
		if entry.RawTailByte != 115 {
			t.Fatalf("wrong physical tail byte: %+v", entry)
		}
	}
	if !found {
		t.Fatal("cargo table lost pharmaceuticals")
	}
}

func TestItemDataRejectsMissingAndContradictoryTailByte(t *testing.T) {
	for _, test := range []struct {
		name   string
		index  int
		remove bool
		value  any
	}{
		{"missing", 25, true, nil}, {"null", 25, false, nil}, {"negative", 25, false, -1}, {"overflow", 25, false, 256},
		{"full-name-mismatch", 25, false, 114}, {"value-mismatch", 31, false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var doc map[string]any
			if err := json.Unmarshal(itemDataJSON, &doc); err != nil {
				t.Fatal(err)
			}
			row := doc["cargo"].(map[string]any)["records"].([]any)[test.index].(map[string]any)
			if test.remove {
				delete(row, "rawTailByte")
			} else {
				row["rawTailByte"] = test.value
			}
			encoded, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = LoadItemData(strings.NewReader(string(encoded))); err == nil {
				t.Fatal("invalid physical tail byte accepted")
			}
		})
	}
}

func TestEmbeddedItemDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/item-data.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(itemDataJSON) {
		t.Fatalf("embedded assets/item-data.json differs from docs/atari-st/item-data.json")
	}
}

func TestItemIdentitiesAreStableAndUnique(t *testing.T) {
	data, err := EmbeddedItemData()
	if err != nil {
		t.Fatalf("load embedded item data: %v", err)
	}
	seen := make(map[string]string)
	for _, group := range [][]Entry{data.Items(), data.Cargo()} {
		for _, entry := range group {
			if entry.ID == "" {
				t.Fatalf("record %q produced an empty identity", entry.Name)
			}
			if strings.ContainsAny(entry.ID, " /'()") {
				t.Fatalf("identity %q kept a character that does not belong in a key", entry.ID)
			}
		}
		for _, entry := range group {
			if previous, exists := seen[entry.ID]; exists && previous != entry.Name {
				t.Fatalf("identity %q is shared by %q and %q", entry.ID, previous, entry.Name)
			}
			seen[entry.ID] = entry.Name
		}
		clear(seen)
	}
}
