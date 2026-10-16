package atarist

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedRuleDataLocatesEveryRule(t *testing.T) {
	data, err := EmbeddedRuleData()
	if err != nil {
		t.Fatalf("load embedded ship rules: %v", err)
	}
	ids := data.RuleIDs()
	if len(ids) < 12 {
		t.Fatalf("only %d rules are located", len(ids))
	}
	// The one rule whose signature carries a constant has to hand it over,
	// because the consumer reads the number from here rather than repeating it.
	value, ok := data.RuleValue("flight.initial-jump-range")
	if !ok || value <= 0 {
		t.Fatalf("initial jump range = %d (%t), want a positive value", value, ok)
	}
	for _, id := range ids {
		rule, ok := data.Rule(id)
		if !ok || rule.Summary == "" || rule.RawOffset <= 0 {
			t.Fatalf("rule %q is incomplete: %+v", id, rule)
		}
	}
}

func TestEmbeddedRuleDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/ship-rules.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(shipRulesJSON) {
		t.Fatalf("embedded assets/ship-rules.json differs from docs/atari-st/ship-rules.json")
	}
}

// TestLoadRuleDataRejectsAnAddressThatDoesNotLineUp is the check that makes the
// document worth having: a rule whose raw offset is not its segment base plus
// its cited address means one of the two is wrong.
func TestLoadRuleDataRejectsAnAddressThatDoesNotLineUp(t *testing.T) {
	const document = `{"schema":1,"segmentBases":{"SHIPLIB":"0x31e00"},"rules":[
		{"id":"x","procedure":"SHIPLIB:0x21 update_attack","segmentOffset":"0x0e37",
		 "rawOffset":"0x32c38","signature":"00","summary":"s"}]}`
	if _, err := LoadRuleData(strings.NewReader(document)); err == nil {
		t.Fatal("a rule whose address does not line up was accepted")
	}
}

func TestLoadRuleDataRejectsBrokenDocuments(t *testing.T) {
	for name, document := range map[string]string{
		"wrong schema": `{"schema":2,"rules":[]}`,
		"no rules":     `{"schema":1,"segmentBases":{},"rules":[]}`,
		"unknown segment": `{"schema":1,"segmentBases":{},"rules":[
			{"id":"x","procedure":"SHIPLIB:0x21 a","segmentOffset":"0x1","rawOffset":"0x2","signature":"00","summary":"s"}]}`,
	} {
		if _, err := LoadRuleData(strings.NewReader(document)); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}
