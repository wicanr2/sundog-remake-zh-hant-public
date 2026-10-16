package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// stockPhraseCount is the size of the global phrase table.
	stockPhraseCount = 64
	// blockPhraseBase is the reference index of a block's first local phrase.
	blockPhraseBase = 0x34
	// endOfTextReference terminates a line. It is a marker, not a phrase.
	endOfTextReference = 0x3F
	// referenceLimit is one past the last reference value.
	referenceLimit = 0x40
	// expansionDepthLimit stops a malformed corpus from recursing forever.
	// Phrases legitimately nest two or three deep; ten is generous.
	expansionDepthLimit = 10
)

// TokenKind distinguishes the parts of a decoded dialog string.
type TokenKind string

const (
	TokenText      TokenKind = "text"
	TokenReference TokenKind = "reference"
)

// Token is one element of a dialog string. A reference points either at the
// global phrase table or, from 0x34 upwards, at the owning block's own phrases.
type Token struct {
	Kind      TokenKind
	Text      string
	Reference int
	// Dynamic marks a reference the running game fills in, such as a money
	// amount or an item name. The source table has no text for these.
	Dynamic bool
}

// Phrase is one stock phrase with its alternatives. The original picks an
// alternative at random unless the script asks for a specific one.
type Phrase struct {
	Index        int
	RawOffset    int
	Alternatives [][]Token
	Dynamic      bool
}

// Line is one dialog line: a token sequence that expands into a sentence.
type Line struct {
	Index     int
	RawOffset int
	Tokens    []Token
}

// Block is one dialog block, addressed by its source block number.
type Block struct {
	Number    int
	RawOffset int
	Phrases   []Phrase
	Lines     []Line
}

// DialogData is the validated view of the Atari ST dialog corpus.
type DialogData struct {
	stock         []Phrase
	blocks        []Block
	blockByNumber map[int]int
}

type rawToken struct {
	Kind    string `json:"kind"`
	Text    string `json:"text"`
	Index   string `json:"index"`
	Byte    string `json:"byte"`
	Dynamic bool   `json:"dynamic"`
}

type rawDialogString struct {
	Index          int          `json:"index"`
	RawOffset      string       `json:"rawOffset"`
	Role           string       `json:"role"`
	ReferenceIndex string       `json:"referenceIndex"`
	Alternatives   [][]rawToken `json:"alternatives"`
	Tokens         []rawToken   `json:"tokens"`
}

type rawStockPhrase struct {
	Index        string       `json:"index"`
	RawOffset    string       `json:"rawOffset"`
	Alternatives [][]rawToken `json:"alternatives"`
	Dynamic      bool         `json:"dynamic"`
}

type rawDialogBlock struct {
	RawOffset                string            `json:"rawOffset"`
	BlockNumber              string            `json:"blockNumber"`
	DeclaredBlockPhraseCount int               `json:"declaredBlockPhraseCount"`
	Strings                  []rawDialogString `json:"strings"`
}

type dialogDocument struct {
	Schema       int `json:"schema"`
	StockPhrases struct {
		Records []rawStockPhrase `json:"records"`
	} `json:"stockPhrases"`
	DialogBlocks []rawDialogBlock `json:"dialogBlocks"`
	Totals       struct {
		StockPhrases int `json:"stockPhrases"`
		DialogBlocks int `json:"dialogBlocks"`
		BlockPhrases int `json:"blockPhrases"`
		Lines        int `json:"lines"`
	} `json:"totals"`
}

// LoadDialogData validates the corpus before it can reach a player-facing
// screen. An unrecognised byte, a reference with no target, or a count that
// disagrees with the records is an error rather than a line that silently
// renders as a placeholder in front of the player.
func LoadDialogData(reader io.Reader) (DialogData, error) {
	var document dialogDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return DialogData{}, fmt.Errorf("decode Atari ST dialog data: %w", err)
	}
	if document.Schema != 1 {
		return DialogData{}, fmt.Errorf("unsupported Atari ST dialog schema %d", document.Schema)
	}
	if len(document.StockPhrases.Records) != stockPhraseCount {
		return DialogData{}, fmt.Errorf("dialog data has %d stock phrases, want %d", len(document.StockPhrases.Records), stockPhraseCount)
	}

	stock := make([]Phrase, 0, stockPhraseCount)
	for index, record := range document.StockPhrases.Records {
		declared, err := parseHex(record.Index)
		if err != nil || declared != index {
			return DialogData{}, fmt.Errorf("stock phrase %d declares index %q", index, record.Index)
		}
		offset, err := parseHex(record.RawOffset)
		if err != nil {
			return DialogData{}, fmt.Errorf("stock phrase %d has an invalid raw offset %q", index, record.RawOffset)
		}
		alternatives, err := convertAlternatives(record.Alternatives, fmt.Sprintf("stock phrase 0x%02x", index))
		if err != nil {
			return DialogData{}, err
		}
		stock = append(stock, Phrase{
			Index:        index,
			RawOffset:    offset,
			Alternatives: alternatives,
			Dynamic:      record.Dynamic,
		})
	}

	blocks := make([]Block, 0, len(document.DialogBlocks))
	byNumber := make(map[int]int, len(document.DialogBlocks))
	totalPhrases := 0
	totalLines := 0
	for _, record := range document.DialogBlocks {
		number, err := parseHex(record.BlockNumber)
		if err != nil {
			return DialogData{}, fmt.Errorf("dialog block has an invalid number %q", record.BlockNumber)
		}
		offset, err := parseHex(record.RawOffset)
		if err != nil {
			return DialogData{}, fmt.Errorf("dialog block 0x%02x has an invalid raw offset %q", number, record.RawOffset)
		}
		if _, exists := byNumber[number]; exists {
			return DialogData{}, fmt.Errorf("duplicate dialog block 0x%02x", number)
		}

		block := Block{Number: number, RawOffset: offset}
		for _, entry := range record.Strings {
			entryOffset, err := parseHex(entry.RawOffset)
			if err != nil {
				return DialogData{}, fmt.Errorf("dialog block 0x%02x string %d has an invalid raw offset %q", number, entry.Index, entry.RawOffset)
			}
			switch entry.Role {
			case "block-phrase":
				expected := blockPhraseBase + len(block.Phrases)
				declared, err := parseHex(entry.ReferenceIndex)
				if err != nil || declared != expected {
					return DialogData{}, fmt.Errorf("dialog block 0x%02x phrase %d declares reference %q, want 0x%02x", number, entry.Index, entry.ReferenceIndex, expected)
				}
				alternatives, err := convertAlternatives(entry.Alternatives, fmt.Sprintf("block 0x%02x phrase 0x%02x", number, expected))
				if err != nil {
					return DialogData{}, err
				}
				block.Phrases = append(block.Phrases, Phrase{
					Index:        expected,
					RawOffset:    entryOffset,
					Alternatives: alternatives,
				})
			case "line":
				tokens, err := convertTokens(entry.Tokens, fmt.Sprintf("block 0x%02x line %d", number, entry.Index))
				if err != nil {
					return DialogData{}, err
				}
				block.Lines = append(block.Lines, Line{
					Index:     len(block.Lines),
					RawOffset: entryOffset,
					Tokens:    tokens,
				})
			default:
				return DialogData{}, fmt.Errorf("dialog block 0x%02x string %d has unknown role %q", number, entry.Index, entry.Role)
			}
		}
		if len(block.Phrases) != record.DeclaredBlockPhraseCount {
			return DialogData{}, fmt.Errorf("dialog block 0x%02x declares %d phrases and carries %d", number, record.DeclaredBlockPhraseCount, len(block.Phrases))
		}
		if len(block.Lines) == 0 {
			return DialogData{}, fmt.Errorf("dialog block 0x%02x has no lines", number)
		}
		byNumber[number] = len(blocks)
		blocks = append(blocks, block)
		totalPhrases += len(block.Phrases)
		totalLines += len(block.Lines)
	}

	if document.Totals.DialogBlocks != len(blocks) || document.Totals.BlockPhrases != totalPhrases || document.Totals.Lines != totalLines {
		return DialogData{}, fmt.Errorf("dialog totals disagree with the records: %d/%d/%d versus %d/%d/%d",
			document.Totals.DialogBlocks, document.Totals.BlockPhrases, document.Totals.Lines,
			len(blocks), totalPhrases, totalLines)
	}

	data := DialogData{stock: stock, blocks: blocks, blockByNumber: byNumber}
	if err := data.validateReferences(); err != nil {
		return DialogData{}, err
	}
	return data, nil
}

func convertAlternatives(raw [][]rawToken, where string) ([][]Token, error) {
	result := make([][]Token, 0, len(raw))
	for index, alternative := range raw {
		tokens, err := convertTokens(alternative, fmt.Sprintf("%s alternative %d", where, index))
		if err != nil {
			return nil, err
		}
		result = append(result, tokens)
	}
	return result, nil
}

func convertTokens(raw []rawToken, where string) ([]Token, error) {
	tokens := make([]Token, 0, len(raw))
	for _, token := range raw {
		switch token.Kind {
		case "text":
			tokens = append(tokens, Token{Kind: TokenText, Text: token.Text})
		case "reference":
			index, err := parseHex(token.Index)
			if err != nil || index < 0 || index >= referenceLimit {
				return nil, fmt.Errorf("%s has an invalid reference %q", where, token.Index)
			}
			tokens = append(tokens, Token{Kind: TokenReference, Reference: index, Dynamic: token.Dynamic})
		case "alternative":
			return nil, fmt.Errorf("%s carries an unsplit alternative separator", where)
		case "unknown":
			return nil, fmt.Errorf("%s carries the unrecognised source byte %s", where, token.Byte)
		default:
			return nil, fmt.Errorf("%s has a token of unknown kind %q", where, token.Kind)
		}
	}
	return tokens, nil
}

// validateReferences checks that every reference has a target before any of it
// reaches a player. A block-local reference is only valid inside its own block.
func (d DialogData) validateReferences() error {
	check := func(tokens []Token, block *Block, where string) error {
		for _, token := range tokens {
			if token.Kind != TokenReference || token.Dynamic {
				continue
			}
			if token.Reference < blockPhraseBase {
				if token.Reference >= len(d.stock) {
					return fmt.Errorf("%s references global phrase 0x%02x beyond the table", where, token.Reference)
				}
				continue
			}
			if block == nil {
				return fmt.Errorf("%s references block phrase 0x%02x outside a block", where, token.Reference)
			}
			slot := token.Reference - blockPhraseBase
			if slot >= len(block.Phrases) {
				return fmt.Errorf("%s references block phrase 0x%02x but block 0x%02x has %d", where, token.Reference, block.Number, len(block.Phrases))
			}
		}
		return nil
	}

	for _, phrase := range d.stock {
		for index, alternative := range phrase.Alternatives {
			if err := check(alternative, nil, fmt.Sprintf("stock phrase 0x%02x alternative %d", phrase.Index, index)); err != nil {
				return err
			}
		}
	}
	for blockIndex := range d.blocks {
		block := &d.blocks[blockIndex]
		for _, phrase := range block.Phrases {
			for index, alternative := range phrase.Alternatives {
				if err := check(alternative, block, fmt.Sprintf("block 0x%02x phrase 0x%02x alternative %d", block.Number, phrase.Index, index)); err != nil {
					return err
				}
			}
		}
		for _, line := range block.Lines {
			if err := check(line.Tokens, block, fmt.Sprintf("block 0x%02x line %d", block.Number, line.Index)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Blocks returns the dialog blocks in source order.
func (d DialogData) Blocks() []Block {
	result := make([]Block, len(d.blocks))
	copy(result, d.blocks)
	return result
}

// Block returns one dialog block by its source block number.
func (d DialogData) Block(number int) (Block, bool) {
	index, ok := d.blockByNumber[number]
	if !ok {
		return Block{}, false
	}
	return d.blocks[index], true
}

// StockPhrases returns the 64 global phrases in table order.
func (d DialogData) StockPhrases() []Phrase {
	result := make([]Phrase, len(d.stock))
	copy(result, d.stock)
	return result
}

// Chooser picks one of n alternatives. It is injected so a test or a replay can
// reproduce an exact line; the original picks at random.
type Chooser func(n int) int

// FirstAlternative always takes the first alternative, which makes expansion
// deterministic for tests and screenshots.
func FirstAlternative(int) int { return 0 }

// Resolver supplies the text for a reference the running game fills in, such as
// a money amount or an item name. Returning false leaves the reference visible
// as a bracketed marker rather than silently dropping it.
type Resolver func(reference int) (string, bool)

// Expand renders a token sequence into a sentence, resolving references and
// picking alternatives with the supplied chooser. The end-of-text marker stops
// expansion, matching the source corpus where it always closes a line.
func (d DialogData) Expand(block Block, tokens []Token, choose Chooser, resolve Resolver) (string, error) {
	if choose == nil {
		choose = FirstAlternative
	}
	var builder strings.Builder
	if err := d.expandInto(&builder, block, tokens, choose, resolve, 0); err != nil {
		return "", err
	}
	return strings.TrimSpace(builder.String()), nil
}

// ExpandLine renders one line of a block.
func (d DialogData) ExpandLine(block Block, lineIndex int, choose Chooser, resolve Resolver) (string, error) {
	if lineIndex < 0 || lineIndex >= len(block.Lines) {
		return "", fmt.Errorf("dialog block 0x%02x has no line %d", block.Number, lineIndex)
	}
	return d.Expand(block, block.Lines[lineIndex].Tokens, choose, resolve)
}

func (d DialogData) expandInto(builder *strings.Builder, block Block, tokens []Token, choose Chooser, resolve Resolver, depth int) error {
	if depth > expansionDepthLimit {
		return fmt.Errorf("dialog expansion exceeded depth %d; the corpus may reference itself", expansionDepthLimit)
	}
	for _, token := range tokens {
		if token.Kind == TokenText {
			builder.WriteString(token.Text)
			continue
		}
		if token.Reference == endOfTextReference {
			return nil
		}
		if token.Dynamic {
			if resolve != nil {
				if value, ok := resolve(token.Reference); ok {
					builder.WriteString(value)
					continue
				}
			}
			fmt.Fprintf(builder, "[%02x]", token.Reference)
			continue
		}
		phrase, err := d.phrase(block, token.Reference)
		if err != nil {
			return err
		}
		if len(phrase.Alternatives) == 0 {
			continue
		}
		pick := choose(len(phrase.Alternatives))
		if pick < 0 || pick >= len(phrase.Alternatives) {
			return fmt.Errorf("chooser returned %d for phrase 0x%02x with %d alternatives", pick, token.Reference, len(phrase.Alternatives))
		}
		if err := d.expandInto(builder, block, phrase.Alternatives[pick], choose, resolve, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (d DialogData) phrase(block Block, reference int) (Phrase, error) {
	if reference < blockPhraseBase {
		if reference >= len(d.stock) {
			return Phrase{}, fmt.Errorf("global phrase 0x%02x is beyond the table", reference)
		}
		return d.stock[reference], nil
	}
	slot := reference - blockPhraseBase
	if slot >= len(block.Phrases) {
		return Phrase{}, fmt.Errorf("block 0x%02x has no phrase 0x%02x", block.Number, reference)
	}
	return block.Phrases[slot], nil
}

func parseHex(value string) (int, error) {
	parsed, err := strconv.ParseInt(value, 0, 32)
	if err != nil {
		return 0, err
	}
	return int(parsed), nil
}
