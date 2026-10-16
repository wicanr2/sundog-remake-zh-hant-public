package atarist

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedShipSystemDataDescribesSixSystems(t *testing.T) {
	data, err := EmbeddedShipSystemData()
	if err != nil {
		t.Fatalf("load embedded ship system data: %v", err)
	}
	systems := data.Systems()
	if len(systems) != 6 {
		t.Fatalf("ship system data describes %d systems, want 6", len(systems))
	}
	for _, system := range systems {
		if system.Columns[0].ItemName != "control node" {
			t.Fatalf("system %d column 0 = %q, want the control node", system.Index, system.Columns[0].ItemName)
		}
	}
	// Two systems have no second-choice part and four do; a change either way
	// is a change in the decoded call sites, not a detail to absorb quietly.
	alternates := 0
	for _, system := range systems {
		if system.Alternate != nil {
			alternates++
			if system.Alternate.Column == 0 {
				t.Fatalf("system %d offers an alternate for the control node", system.Index)
			}
		}
	}
	if alternates != 4 {
		t.Fatalf("%d systems carry an alternate part, want 4", alternates)
	}
}

func TestEmbeddedShipSystemDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/ship-systems.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(shipSystemsJSON) {
		t.Fatalf("embedded assets/ship-systems.json differs from docs/atari-st/ship-systems.json")
	}
}

func TestEveryShipSystemPartExistsInTheItemTable(t *testing.T) {
	data, err := EmbeddedShipSystemData()
	if err != nil {
		t.Fatal(err)
	}
	items, err := EmbeddedItemData()
	if err != nil {
		t.Fatal(err)
	}
	table := items.Items()
	check := func(index int, name string) {
		t.Helper()
		if index < 0 || index >= len(table) {
			t.Fatalf("part 0x%02x is outside the item table", index)
		}
		if table[index].Name != name {
			t.Fatalf("part 0x%02x is %q in the item table and %q in the system table", index, table[index].Name, name)
		}
	}
	for _, system := range data.Systems() {
		for _, column := range system.Columns {
			check(column.ItemIndex, column.ItemName)
		}
		if system.Alternate != nil {
			check(system.Alternate.ItemIndex, system.Alternate.ItemName)
		}
	}
}

func TestRowStatusFollowsTheOriginalRules(t *testing.T) {
	data, err := EmbeddedShipSystemData()
	if err != nil {
		t.Fatal(err)
	}
	// System 2 is the one whose second column takes either its own part or the
	// alternate, which makes it the useful case for every rule at once.
	system, ok := data.System(2)
	if !ok {
		t.Fatal("ship system 2 is missing")
	}
	intact := make([]int, len(system.Columns))
	for index, column := range system.Columns {
		intact[index] = column.ItemIndex
	}
	if got := system.RowStatus(intact); got != 4 {
		t.Fatalf("an intact row has status %d, want 4", got)
	}

	shunted := append([]int(nil), intact...)
	shunted[1] = ShuntItemIndex()
	if got := system.RowStatus(shunted); got != 3 {
		t.Fatalf("a row with one shunt has status %d, want 3", got)
	}

	shuntedControl := append([]int(nil), intact...)
	shuntedControl[0] = ShuntItemIndex()
	if got := system.RowStatus(shuntedControl); got != 0 {
		t.Fatalf("a shunt in the control node column left status %d, want 0", got)
	}

	dead := append([]int(nil), intact...)
	dead[2] = DeadPartItemIndex()
	if got := system.RowStatus(dead); got != 0 {
		t.Fatalf("a row with a dead part has status %d, want 0", got)
	}

	if system.Alternate == nil {
		t.Fatal("ship system 2 lost its alternate part")
	}
	alternate := append([]int(nil), intact...)
	alternate[system.Alternate.Column] = system.Alternate.ItemIndex
	if got := system.RowStatus(alternate); got != 4 {
		t.Fatalf("a row using the alternate part has status %d, want 4", got)
	}
	// The alternate belongs to one column only.
	misplaced := append([]int(nil), intact...)
	other := 1
	if system.Alternate.Column == 1 {
		other = 2
	}
	misplaced[other] = system.Alternate.ItemIndex
	if got := system.RowStatus(misplaced); got != 0 {
		t.Fatalf("the alternate part worked in column %d as well, status %d", other, got)
	}
}

func TestLoadShipSystemDataRejectsBrokenDocuments(t *testing.T) {
	for name, document := range map[string]string{
		"wrong schema":  `{"schema":2,"systems":[]}`,
		"missing rows":  `{"schema":1,"systems":[]}`,
		"bad column":    `{"schema":1,"systems":[{"index":0,"rawOffset":"0x1","columns":[{"column":1,"itemIndex":"0x1f","itemName":"control node"}]}]}`,
		"shunt as part": `{"schema":1,"systems":[{"index":0,"rawOffset":"0x1","columns":[{"column":0,"itemIndex":"0x17","itemName":"shunt"}]}]}`,
	} {
		if _, err := LoadShipSystemData(strings.NewReader(document)); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}
