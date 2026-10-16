package game

import "testing"

// The table is the disk's own (raw 0x401c); these checks bind the copy here to
// the structure the extractor enforces, so a typo in either place collides.
func TestSkylineHeightsMatchTheDiskTablesShape(t *testing.T) {
	if len(SkylineHeights) != skylineStyles {
		t.Fatalf("%d shapes, want %d", len(SkylineHeights), skylineStyles)
	}
	for shape, row := range SkylineHeights {
		for column, value := range row {
			if value < 0 || value > 6 {
				t.Fatalf("shape %d column %d height %d is outside 0..6", shape, column, value)
			}
		}
	}
	// Shape 6 is the pyramid — the one profile a transposition error cannot
	// leave intact.
	pyramid := [SkylineColumns]int{2, 3, 4, 5, 6, 5, 4}
	if SkylineHeights[6] != pyramid {
		t.Fatalf("shape 6 = %v, want %v", SkylineHeights[6], pyramid)
	}
}

func TestEverySkylineBuildingRollsAStyleTheTableHas(t *testing.T) {
	state := NewState()
	for _, building := range rollSkyline(&state) {
		if building.Style < 0 || building.Style >= skylineStyles {
			t.Fatalf("style %d is outside the table", building.Style)
		}
	}
}
