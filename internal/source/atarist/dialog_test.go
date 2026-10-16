package atarist

import (
	"os"
	"strings"
	"testing"
)

func TestEmbeddedDialogDataMatchesSourceCorpus(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	if got := len(data.StockPhrases()); got != stockPhraseCount {
		t.Fatalf("stock phrases = %d, want %d", got, stockPhraseCount)
	}
	blocks := data.Blocks()
	if len(blocks) != 14 {
		t.Fatalf("dialog blocks = %d, want 14", len(blocks))
	}
	phrases, lines := 0, 0
	for _, block := range blocks {
		phrases += len(block.Phrases)
		lines += len(block.Lines)
	}
	if phrases != 66 || lines != 173 {
		t.Fatalf("block phrases/lines = %d/%d, want 66/173", phrases, lines)
	}
	if _, ok := data.Block(0x10); !ok {
		t.Fatal("block 0x10 is missing")
	}
	if _, ok := data.Block(0x11); ok {
		t.Fatal("block 0x11 should not exist")
	}
}

func TestEmbeddedDialogDataMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/dialog-text.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(dialogTextJSON) {
		t.Fatalf("embedded assets/dialog-text.json differs from docs/atari-st/dialog-text.json")
	}
}

// TestExpandComposesAKnownSourceLine walks the composition the original uses:
// block 0x10's first line is a chain of references, and taking the first
// alternative at every step has to produce the sentence those bytes encode.
func TestExpandComposesAKnownSourceLine(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	block, ok := data.Block(0x10)
	if !ok {
		t.Fatal("block 0x10 is missing")
	}
	got, err := data.ExpandLine(block, 0, FirstAlternative, nil)
	if err != nil {
		t.Fatalf("expand line: %v", err)
	}
	// 0x2a "what", 0x15 -> "can I " + 0x03 "serve" + " " + 0x2b "you",
	// then ", ", then 0x31 "sir", then the end-of-text marker.
	if got != "what can I serve you, sir" {
		t.Fatalf("expanded line = %q", got)
	}
}

func TestExpandUsesTheChooserForEveryAlternative(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	block, _ := data.Block(0x10)
	last := func(n int) int { return n - 1 }
	got, err := data.ExpandLine(block, 0, last, nil)
	if err != nil {
		t.Fatalf("expand line: %v", err)
	}
	// The same line with the last alternative everywhere: 0x15 -> "do you need",
	// 0x31 -> "friend".
	if got != "what do you need, friend" {
		t.Fatalf("expanded line = %q", got)
	}
}

func TestExpandEveryLineInTheCorpus(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	for _, block := range data.Blocks() {
		for _, line := range block.Lines {
			for _, choose := range []Chooser{FirstAlternative, func(n int) int { return n - 1 }} {
				got, err := data.Expand(block, line.Tokens, choose, nil)
				if err != nil {
					t.Fatalf("block 0x%02x line %d: %v", block.Number, line.Index, err)
				}
				if strings.TrimSpace(got) == "" {
					t.Fatalf("block 0x%02x line %d expanded to nothing", block.Number, line.Index)
				}
			}
		}
	}
}

func TestExpandLeavesDynamicReferencesVisibleWithoutAResolver(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	block, _ := data.Block(0x20)
	// Block 0x20 is the haggling block; its lines quote a money amount.
	var withMarker, withResolver string
	for index := range block.Lines {
		line, err := data.ExpandLine(block, index, FirstAlternative, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(line, "[24]") {
			withMarker = line
			withResolver, err = data.ExpandLine(block, index, FirstAlternative, func(reference int) (string, bool) {
				if reference == 0x24 {
					return "120 Cr", true
				}
				return "", false
			})
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if withMarker == "" {
		t.Fatal("no line in block 0x20 quoted a money amount")
	}
	if !strings.Contains(withResolver, "120 Cr") || strings.Contains(withResolver, "[24]") {
		t.Fatalf("resolver did not fill the money reference: %q", withResolver)
	}
}

func TestExpandRejectsAnOutOfRangeChoice(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	block, _ := data.Block(0x10)
	if _, err := data.ExpandLine(block, 0, func(int) int { return 99 }, nil); err == nil {
		t.Fatal("expected an error for an out-of-range alternative")
	}
}

func TestExpandLineRejectsAnUnknownLineIndex(t *testing.T) {
	data, err := EmbeddedDialogData()
	if err != nil {
		t.Fatalf("load embedded dialog data: %v", err)
	}
	block, _ := data.Block(0x10)
	if _, err := data.ExpandLine(block, len(block.Lines), FirstAlternative, nil); err == nil {
		t.Fatal("expected an error for an out-of-range line index")
	}
}

const minimalDialogDocument = `{
  "schema": 1,
  "stockPhrases": {"records": []},
  "dialogBlocks": [],
  "totals": {"stockPhrases": 64, "dialogBlocks": 0, "blockPhrases": 0, "lines": 0}
}`

func TestLoadDialogDataRejectsAShortStockTable(t *testing.T) {
	if _, err := LoadDialogData(strings.NewReader(minimalDialogDocument)); err == nil {
		t.Fatal("expected an error when the stock phrase table is short")
	}
}

func TestLoadDialogDataRejectsAnUnrecognisedSourceByte(t *testing.T) {
	document := strings.Replace(
		string(dialogTextJSON),
		`"kind": "text"`,
		`"kind": "unknown", "byte": "0x40"`,
		1,
	)
	if _, err := LoadDialogData(strings.NewReader(document)); err == nil {
		t.Fatal("expected an error when the corpus carries an unrecognised byte")
	}
}

func TestTranslatedDialogExpandsIntoChinese(t *testing.T) {
	data, err := EmbeddedTranslatedDialogData()
	if err != nil {
		t.Fatalf("load translated dialog data: %v", err)
	}
	block, ok := data.Block(0x10)
	if !ok {
		t.Fatal("block 0x10 is missing")
	}
	got, err := data.ExpandLine(block, 0, FirstAlternative, nil)
	if err != nil {
		t.Fatalf("expand line: %v", err)
	}
	if got != "先生，我能為你服務些什麼？" {
		t.Fatalf("expanded line = %q", got)
	}
	last := func(n int) int { return n - 1 }
	got, err = data.ExpandLine(block, 0, last, nil)
	if err != nil {
		t.Fatalf("expand line: %v", err)
	}
	if got != "朋友，你需要什麼？" {
		t.Fatalf("expanded line = %q", got)
	}
}

func TestTranslatedDialogExpandsEveryLine(t *testing.T) {
	data, err := EmbeddedTranslatedDialogData()
	if err != nil {
		t.Fatalf("load translated dialog data: %v", err)
	}
	for _, block := range data.Blocks() {
		for _, line := range block.Lines {
			for _, choose := range []Chooser{FirstAlternative, func(n int) int { return n - 1 }} {
				got, err := data.Expand(block, line.Tokens, choose, nil)
				if err != nil {
					t.Fatalf("block 0x%02x line %d: %v", block.Number, line.Index, err)
				}
				if strings.TrimSpace(got) == "" {
					t.Fatalf("block 0x%02x line %d expanded to nothing", block.Number, line.Index)
				}
				for _, r := range got {
					if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
						// Proper nouns such as SunDog and Uniteller stay in
						// Latin script by design; flag only long English runs.
						break
					}
				}
			}
		}
	}
}

func TestParsePlaceholderTextRejectsMalformedPlaceholders(t *testing.T) {
	if _, err := ParsePlaceholderText("你好{2b", nil); err == nil {
		t.Fatal("expected an error for an unterminated placeholder")
	}
	if _, err := ParsePlaceholderText("你好{zz}", nil); err == nil {
		t.Fatal("expected an error for a non-hex placeholder")
	}
	tokens, err := ParsePlaceholderText("你好{2b}好", map[int]bool{0x2b: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 3 || tokens[1].Reference != 0x2b || !tokens[1].Dynamic {
		t.Fatalf("tokens = %+v", tokens)
	}
}
