package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// The ship rules are control flow, not a data table, so this document does not
// carry a decoded structure — it carries, for each rule, the byte signature
// that pins it to a place on the disk, and for a few of them the constant that
// signature contains. A consumer reads those constants from here rather than
// writing them out again; see tools/atari_st_ship_rules.py.

// Rule is one located rule.
type Rule struct {
	ID          string
	Procedure   string
	RawOffset   int
	Summary     string
	Value       int
	HasValue    bool
	SegmentBase int
}

// RuleData is the validated view of the located rules.
type RuleData struct {
	rules map[string]Rule
	order []string
}

type ruleDocument struct {
	Schema       int               `json:"schema"`
	SegmentBases map[string]string `json:"segmentBases"`
	Rules        []struct {
		ID            string `json:"id"`
		Procedure     string `json:"procedure"`
		SegmentOffset string `json:"segmentOffset"`
		RawOffset     string `json:"rawOffset"`
		Signature     string `json:"signature"`
		Summary       string `json:"summary"`
		Value         *int   `json:"value"`
	} `json:"rules"`
}

// LoadRuleData validates the document. A rule whose raw offset does not line up
// with its own segment base is a decoding failure: the whole point of the
// document is that the located bytes and the cited addresses agree.
func LoadRuleData(reader io.Reader) (RuleData, error) {
	var document ruleDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return RuleData{}, fmt.Errorf("decode Atari ST ship rules: %w", err)
	}
	if document.Schema != 1 {
		return RuleData{}, fmt.Errorf("unsupported Atari ST ship rule schema %d", document.Schema)
	}
	if len(document.Rules) == 0 {
		return RuleData{}, fmt.Errorf("ship rules document locates nothing")
	}

	bases := make(map[string]int, len(document.SegmentBases))
	for segment, value := range document.SegmentBases {
		base, err := strconv.ParseInt(value, 0, 64)
		if err != nil || base < 0 {
			return RuleData{}, fmt.Errorf("segment %q has an invalid base %q", segment, value)
		}
		bases[segment] = int(base)
	}

	data := RuleData{rules: make(map[string]Rule, len(document.Rules))}
	for _, entry := range document.Rules {
		if entry.ID == "" || entry.Summary == "" || entry.Signature == "" {
			return RuleData{}, fmt.Errorf("rule %q is missing an identity, a summary or a signature", entry.ID)
		}
		if _, exists := data.rules[entry.ID]; exists {
			return RuleData{}, fmt.Errorf("rule %q appears twice", entry.ID)
		}
		raw, err := strconv.ParseInt(entry.RawOffset, 0, 64)
		if err != nil || raw < 0 {
			return RuleData{}, fmt.Errorf("rule %q has an invalid raw offset %q", entry.ID, entry.RawOffset)
		}
		segmentOffset, err := strconv.ParseInt(entry.SegmentOffset, 0, 64)
		if err != nil || segmentOffset < 0 {
			return RuleData{}, fmt.Errorf("rule %q has an invalid segment offset %q", entry.ID, entry.SegmentOffset)
		}
		segment := entry.Procedure
		for index, r := range segment {
			if r == ':' || r == ' ' {
				segment = segment[:index]
				break
			}
		}
		base, known := bases[segment]
		if !known {
			return RuleData{}, fmt.Errorf("rule %q names segment %q, which has no base", entry.ID, segment)
		}
		if int(raw)-int(segmentOffset) != base {
			return RuleData{}, fmt.Errorf("rule %q sits at 0x%05x, which is not %s base 0x%05x plus 0x%04x",
				entry.ID, raw, segment, base, segmentOffset)
		}

		rule := Rule{
			ID:          entry.ID,
			Procedure:   entry.Procedure,
			RawOffset:   int(raw),
			Summary:     entry.Summary,
			SegmentBase: base,
		}
		if entry.Value != nil {
			if *entry.Value < 0 || *entry.Value > 0xFF {
				return RuleData{}, fmt.Errorf("rule %q carries value %d, outside a byte", entry.ID, *entry.Value)
			}
			rule.Value = *entry.Value
			rule.HasValue = true
		}
		data.rules[entry.ID] = rule
		data.order = append(data.order, entry.ID)
	}
	return data, nil
}

// Rule returns one located rule by identity.
func (d RuleData) Rule(id string) (Rule, bool) {
	rule, ok := d.rules[id]
	return rule, ok
}

// RuleValue returns the constant a rule's signature carries.
func (d RuleData) RuleValue(id string) (int, bool) {
	rule, ok := d.rules[id]
	if !ok || !rule.HasValue {
		return 0, false
	}
	return rule.Value, true
}

// RuleIDs returns the located rules in document order.
func (d RuleData) RuleIDs() []string {
	result := make([]string, len(d.order))
	copy(result, d.order)
	return result
}
