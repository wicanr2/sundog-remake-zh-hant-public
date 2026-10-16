package game

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Every frame the remake opens has to render something. Before dialogFrameOpenings
// existed, four of these passed a frame number in where a block number was
// expected: the corpus had no such block, DialogLine returned an error, and the
// shell drew a conversation with no words in it.
func TestEveryDialogFrameTheRemakeOpensRenders(t *testing.T) {
	if !DialogAvailable() {
		t.Skip("the dialog corpus is not built into this binary")
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	frames := map[string]int{
		"hail":              hailDialogFrame,
		"bar booth":         boothDialogFrame,
		"bar counter":       0x10,
		"equipment counter": 0x50,
		"parts counter":     0x51,
		"hotel desk":        0x60,
		"street mugger":     streetMuggerFrame,
		"street beggar":     streetBeggarFrame,
		"street dealer":     streetDealerFrame,
		"developer credit":  streetEasterEggFrame,
	}
	for name, frame := range frames {
		opening, ok := dialogOpeningForFrame(frame)
		if !ok {
			t.Errorf("%s (frame 0x%02x) has no opening", name, frame)
			continue
		}
		state := NewState()
		state.Screen = ScreenDialog
		state.DialogBlock = opening.Block
		state.DialogLine = opening.Line
		line, err := reducer.DialogLine(state)
		if err != nil {
			t.Errorf("%s (frame 0x%02x) does not render: %v", name, frame, err)
			continue
		}
		if line == "" {
			t.Errorf("%s (frame 0x%02x) renders an empty line", name, frame)
		}
	}
}

// The table is copied out of the rendered corpus, so it can go stale. This
// re-derives every entry from docs/atari-st/dialog-rendered.json: a frame's
// opening is the index, among its block's lines, of the line at the raw offset
// the rendered document gives it. A chain frame carries no lines of its own, so
// it takes the opening of the frame it chains to.
func TestDialogFrameOpeningsMatchTheRenderedCorpus(t *testing.T) {
	var rendered struct {
		Frames []struct {
			ID       string   `json:"id"`
			Block    string   `json:"block"`
			ChainsTo []string `json:"chainsTo"`
			Lines    []struct {
				RawOffset string `json:"rawOffset"`
			} `json:"lines"`
		} `json:"frames"`
	}
	readJSON(t, "../../docs/atari-st/dialog-rendered.json", &rendered)

	var corpus struct {
		DialogBlocks []struct {
			BlockNumber string `json:"blockNumber"`
			Strings     []struct {
				Role      string `json:"role"`
				RawOffset string `json:"rawOffset"`
			} `json:"strings"`
		} `json:"dialogBlocks"`
	}
	readJSON(t, "../../internal/source/atarist/assets/dialog-text.json", &corpus)

	lineIndex := map[int]map[string]int{}
	for _, block := range corpus.DialogBlocks {
		number := parseHex(t, block.BlockNumber)
		offsets := map[string]int{}
		next := 0
		for _, entry := range block.Strings {
			if entry.Role != "line" {
				continue
			}
			offsets[entry.RawOffset] = next
			next++
		}
		lineIndex[number] = offsets
	}

	byID := map[int]int{}       // frame -> block
	firstOf := map[int]string{} // frame -> raw offset of its first line
	chains := map[int]int{}
	for _, frame := range rendered.Frames {
		id := parseHex(t, frame.ID)
		byID[id] = parseHex(t, frame.Block)
		if len(frame.Lines) > 0 {
			firstOf[id] = frame.Lines[0].RawOffset
			continue
		}
		if len(frame.ChainsTo) > 0 {
			chains[id] = parseHex(t, frame.ChainsTo[0])
		}
	}

	resolve := func(id int) (int, int, bool) {
		for hop := 0; hop < 4; hop++ {
			if offset, ok := firstOf[id]; ok {
				return byID[id], lineIndex[byID[id]][offset], true
			}
			target, ok := chains[id]
			if !ok {
				return 0, 0, false
			}
			id = target
		}
		return 0, 0, false
	}

	for frame, want := range dialogFrameOpenings {
		block, line, ok := resolve(frame)
		if !ok {
			t.Errorf("frame 0x%02x: the rendered corpus gives it no line", frame)
			continue
		}
		if block != want.Block || line != want.Line {
			t.Errorf("frame 0x%02x: table says block 0x%02x line %d, the corpus says block 0x%02x line %d",
				frame, want.Block, want.Line, block, line)
		}
	}
}

func readJSON(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("evidence document unavailable: %v", err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func parseHex(t *testing.T, value string) int {
	t.Helper()
	number, err := strconv.ParseInt(strings.TrimPrefix(value, "0x"), 16, 32)
	if err != nil {
		t.Fatalf("cannot read %q as a hex number: %v", value, err)
	}
	return int(number)
}
