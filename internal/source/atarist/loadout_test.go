package atarist

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedLoadoutDataCarriesTheNewGameKit(t *testing.T) {
	data, err := EmbeddedLoadoutData()
	if err != nil {
		t.Fatalf("load embedded loadout data: %v", err)
	}
	if got := data.ShuntCount(); got != 5 {
		t.Fatalf("a new game carries %d shunts, want 5", got)
	}
	stores := data.Stores(20)
	counts := map[string]int{}
	for _, raw := range stores {
		if raw == 0 {
			continue
		}
		for _, write := range data.Writes() {
			if write.Kind == LoadoutItem && write.RawValue == raw {
				counts[write.ItemName]++
				break
			}
		}
	}
	for name, want := range map[string]int{"rapidheal": 5, "burger": 5, "shield": 1, "stinger": 1} {
		if counts[name] != want {
			t.Fatalf("stores hold %d × %s, want %d (%v)", counts[name], name, want, counts)
		}
	}
	if !data.Complete() {
		t.Fatal("the loadout still carries an undecoded write")
	}
	// The hull is rolled rather than fixed, so the data gives a range and lets
	// the consumer decide.
	low, high, ok := data.HullRange()
	if !ok || low != 13 || high != 16 {
		t.Fatalf("hull range = %d..%d (%t), want 13..16", low, high, ok)
	}
}

func TestEmbeddedLoadoutDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/new-game-loadout.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(loadoutJSON) {
		t.Fatalf("embedded assets/new-game-loadout.json differs from docs/atari-st/new-game-loadout.json")
	}
}

func TestLoadLoadoutDataRejectsBrokenDocuments(t *testing.T) {
	for name, document := range map[string]string{
		"wrong schema": `{"schema":2,"writes":[]}`,
		"no writes":    `{"schema":1,"writes":[]}`,
		"unknown kind": `{"schema":1,"writes":[{"kind":"poke","stateOffset":"0x10","count":1,"rawValue":"0x08","itemIndex":"0x08","itemName":"burger"}]}`,
		"code mismatch": `{"schema":1,"writes":[{"kind":"store","stateOffset":"0x10","count":1,"rawValue":"0xeb",` +
			`"itemIndex":"0x0c","itemName":"stinger"}]}`,
	} {
		if _, err := LoadLoadoutData(strings.NewReader(document)); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}
