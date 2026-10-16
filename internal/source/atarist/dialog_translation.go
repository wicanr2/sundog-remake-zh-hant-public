package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// placeholderPattern is how a reference is written in a translated string:
// {2b} for stock phrase 0x2b. Translators may move a placeholder to suit
// Chinese word order, but adding or dropping one changes what the sentence
// says, so the translation validator rejects that before it reaches here.
const (
	placeholderOpen   = '{'
	placeholderClose  = '}'
	placeholderDigits = 2
)

type translationEntry struct {
	Source      string `json:"source"`
	Translation string `json:"translation"`
}

type translationPhrase struct {
	Alternatives []translationEntry `json:"alternatives"`
}

type translationBlock struct {
	Phrases map[string]translationPhrase `json:"phrases"`
	Lines   []translationEntry           `json:"lines"`
}

type translationDocument struct {
	Schema       int                          `json:"schema"`
	Locale       string                       `json:"locale"`
	StockPhrases map[string]translationPhrase `json:"stockPhrases"`
	Blocks       map[string]translationBlock  `json:"blocks"`
}

// ParsePlaceholderText turns a translated string into tokens. The dynamic set
// tells it which references the running game fills in, so a translated line
// keeps the same dynamic markers the source line had.
func ParsePlaceholderText(text string, dynamic map[int]bool) ([]Token, error) {
	tokens := make([]Token, 0, 4)
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			tokens = append(tokens, Token{Kind: TokenText, Text: literal.String()})
			literal.Reset()
		}
	}
	runes := []rune(text)
	for index := 0; index < len(runes); index++ {
		if runes[index] != placeholderOpen {
			literal.WriteRune(runes[index])
			continue
		}
		end := index + placeholderDigits + 1
		if end >= len(runes) || runes[end] != placeholderClose {
			return nil, fmt.Errorf("unterminated placeholder at %d in %q", index, text)
		}
		value, err := strconv.ParseInt(string(runes[index+1:end]), 16, 32)
		if err != nil || value < 0 || value >= referenceLimit {
			return nil, fmt.Errorf("invalid placeholder %q in %q", string(runes[index:end+1]), text)
		}
		flush()
		tokens = append(tokens, Token{
			Kind:      TokenReference,
			Reference: int(value),
			Dynamic:   dynamic[int(value)],
		})
		index = end
	}
	flush()
	return tokens, nil
}

// Translate returns a copy of the corpus with every string replaced by its
// translation. Indices, raw offsets, alternative counts and dynamic markers all
// carry over, so an expanded line has the same shape in either language and the
// same code path renders both.
func (d DialogData) Translate(reader io.Reader) (DialogData, error) {
	var document translationDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return DialogData{}, fmt.Errorf("decode dialog translation: %w", err)
	}
	if document.Schema != 1 {
		return DialogData{}, fmt.Errorf("unsupported dialog translation schema %d", document.Schema)
	}
	if document.Locale == "" {
		return DialogData{}, fmt.Errorf("dialog translation has no locale")
	}

	dynamic := d.dynamicReferences()

	translated := DialogData{
		stock:         make([]Phrase, len(d.stock)),
		blocks:        make([]Block, len(d.blocks)),
		blockByNumber: make(map[int]int, len(d.blockByNumber)),
	}

	for index, phrase := range d.stock {
		translated.stock[index] = phrase
		// A phrase with no text is one the running game fills in; the
		// translation file has no entry for it and none is needed.
		if !hasContent(phrase) {
			continue
		}
		key := fmt.Sprintf("0x%02x", phrase.Index)
		record, ok := document.StockPhrases[key]
		if !ok {
			return DialogData{}, fmt.Errorf("dialog translation is missing stock phrase %s", key)
		}
		alternatives, err := translateAlternatives(phrase.Alternatives, record.Alternatives, dynamic, "stock phrase "+key)
		if err != nil {
			return DialogData{}, err
		}
		translated.stock[index].Alternatives = alternatives
	}

	for index, block := range d.blocks {
		key := fmt.Sprintf("0x%02x", block.Number)
		record, ok := document.Blocks[key]
		if !ok {
			return DialogData{}, fmt.Errorf("dialog translation is missing block %s", key)
		}
		next := Block{Number: block.Number, RawOffset: block.RawOffset}

		next.Phrases = make([]Phrase, len(block.Phrases))
		for slot, phrase := range block.Phrases {
			next.Phrases[slot] = phrase
			phraseKey := fmt.Sprintf("0x%02x", phrase.Index)
			phraseRecord, ok := record.Phrases[phraseKey]
			if !ok {
				return DialogData{}, fmt.Errorf("dialog translation is missing block %s phrase %s", key, phraseKey)
			}
			alternatives, err := translateAlternatives(phrase.Alternatives, phraseRecord.Alternatives, dynamic, fmt.Sprintf("block %s phrase %s", key, phraseKey))
			if err != nil {
				return DialogData{}, err
			}
			next.Phrases[slot].Alternatives = alternatives
		}

		if len(record.Lines) != len(block.Lines) {
			return DialogData{}, fmt.Errorf("dialog translation has %d lines for block %s, want %d", len(record.Lines), key, len(block.Lines))
		}
		next.Lines = make([]Line, len(block.Lines))
		for slot, line := range block.Lines {
			next.Lines[slot] = line
			tokens, err := translateOne(record.Lines[slot], dynamic, fmt.Sprintf("block %s line %d", key, slot))
			if err != nil {
				return DialogData{}, err
			}
			next.Lines[slot].Tokens = tokens
		}

		translated.blocks[index] = next
		translated.blockByNumber[block.Number] = index
	}

	if err := translated.validateReferences(); err != nil {
		return DialogData{}, fmt.Errorf("translated corpus: %w", err)
	}
	return translated, nil
}

// hasContent reports whether a phrase carries any text at all. An empty stock
// phrase still decodes to one empty alternative, so counting alternatives is
// not enough to tell "no text" from "one blank choice".
func hasContent(phrase Phrase) bool {
	for _, alternative := range phrase.Alternatives {
		if len(alternative) > 0 {
			return true
		}
	}
	return false
}

func translateAlternatives(source [][]Token, entries []translationEntry, dynamic map[int]bool, where string) ([][]Token, error) {
	// The translation file lists only the alternatives that carry text, which
	// is how the scaffold writes it; blank ones stay as they are.
	withText := make([]int, 0, len(source))
	for index, alternative := range source {
		if len(alternative) > 0 {
			withText = append(withText, index)
		}
	}
	if len(entries) == len(withText) && len(withText) != len(source) {
		result := make([][]Token, len(source))
		copy(result, source)
		for slot, index := range withText {
			tokens, err := translateOne(entries[slot], dynamic, fmt.Sprintf("%s alternative %d", where, index))
			if err != nil {
				return nil, err
			}
			result[index] = tokens
		}
		return result, nil
	}
	if len(entries) != len(source) {
		return nil, fmt.Errorf("%s has %d translated alternatives, want %d", where, len(entries), len(source))
	}
	result := make([][]Token, len(source))
	for index := range source {
		tokens, err := translateOne(entries[index], dynamic, fmt.Sprintf("%s alternative %d", where, index))
		if err != nil {
			return nil, err
		}
		result[index] = tokens
	}
	return result, nil
}

func translateOne(entry translationEntry, dynamic map[int]bool, where string) ([]Token, error) {
	if entry.Translation == "" {
		return nil, fmt.Errorf("%s has no translation", where)
	}
	tokens, err := ParsePlaceholderText(entry.Translation, dynamic)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", where, err)
	}
	return tokens, nil
}

// dynamicReferences collects every reference the source corpus marks as filled
// in by the running game. A translated line has to keep the same markers, or a
// money amount would render as a literal placeholder in front of the player.
func (d DialogData) dynamicReferences() map[int]bool {
	dynamic := make(map[int]bool, 8)
	visit := func(tokens []Token) {
		for _, token := range tokens {
			if token.Kind == TokenReference && token.Dynamic {
				dynamic[token.Reference] = true
			}
		}
	}
	for _, phrase := range d.stock {
		if phrase.Dynamic {
			dynamic[phrase.Index] = true
		}
		for _, alternative := range phrase.Alternatives {
			visit(alternative)
		}
	}
	for _, block := range d.blocks {
		for _, phrase := range block.Phrases {
			for _, alternative := range phrase.Alternatives {
				visit(alternative)
			}
		}
		for _, line := range block.Lines {
			visit(line.Tokens)
		}
	}
	return dynamic
}
