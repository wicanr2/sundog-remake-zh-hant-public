package atarist

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedSineTableCoversAQuarterTurn(t *testing.T) {
	data, err := EmbeddedSineTableData()
	if err != nil {
		t.Fatalf("load embedded sine table: %v", err)
	}
	table := data.Table()
	if len(table) != data.TurnSteps()/8+1 {
		t.Fatalf("table has %d entries for a %d-step turn", len(table), data.TurnSteps())
	}
	if table[0] != 0 || table[len(table)-1] != data.Scale() {
		t.Fatalf("table runs %d..%d, want 0..%d", table[0], table[len(table)-1], data.Scale())
	}
}

// TestSineFoldsTheWholeTurn is the property the folding rule exists for: the
// magnitude has to come back symmetric about every half turn, because the
// original stores only a quarter.
func TestSineFoldsTheWholeTurn(t *testing.T) {
	data, err := EmbeddedSineTableData()
	if err != nil {
		t.Fatalf("load embedded sine table: %v", err)
	}
	turn := data.TurnSteps()
	for angle := 0; angle < turn; angle++ {
		got := data.Sine(angle)
		want := int(math.Round(float64(data.Scale()) * math.Abs(math.Sin(float64(angle)*2*math.Pi/float64(turn)))))
		// The table is stored at half the angle resolution, so an odd angle
		// reads the same entry as the even one below it.
		if angle%2 == 0 && got != want {
			t.Fatalf("angle %d gives %d, want %d", angle, got, want)
		}
		if got < 0 || got > data.Scale() {
			t.Fatalf("angle %d gives %d, outside 0..%d", angle, got, data.Scale())
		}
	}
	if data.Sine(-3) != data.Sine(turn-3) {
		t.Fatal("a negative angle does not fold onto its positive counterpart")
	}
}

func TestEmbeddedSineTableMatchesEvidenceDocument(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/atari-st/sine-table.json")
	if err != nil {
		t.Fatalf("read evidence document: %v", err)
	}
	if string(docs) != string(sineTableJSON) {
		t.Fatalf("embedded assets/sine-table.json differs from docs/atari-st/sine-table.json")
	}
}

func TestLoadSineTableDataRejectsBrokenDocuments(t *testing.T) {
	for name, document := range map[string]string{
		"wrong schema": `{"schema":2}`,
		"no turn":      `{"schema":1,"turnSteps":0,"scale":128,"table":[0]}`,
		"wrong length": `{"schema":1,"turnSteps":256,"scale":128,"table":[0,6]}`,
		"not a sine":   `{"schema":1,"turnSteps":8,"scale":128,"table":[0,100]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadSineTableData(strings.NewReader(document)); err == nil {
				t.Fatal("a broken document was accepted")
			}
		})
	}
}
