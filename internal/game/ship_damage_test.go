package game

import "testing"

// TestNewGameShipCarriesRandomDamage pins XSTARTUP:0x31: a new game's six
// systems are not all intact.
func TestNewGameShipCarriesRandomDamage(t *testing.T) {
	damaged := 0
	for seed := int64(1); seed <= 40; seed++ {
		for _, system := range defaultShipSystems(seed) {
			if system.Status != 16 {
				damaged++
			}
		}
	}
	if damaged == 0 {
		t.Fatal("40 seeds produced no damage at all")
	}
	// Every system intact is (2/3)^4 = 19.8%, so over 240 systems almost all of
	// them should carry something.
	if damaged < 150 {
		t.Errorf("only %d of 240 systems were damaged, expected roughly 190", damaged)
	}
}

// TestNewGameShipIsReproducibleFromItsSeed keeps the property the remake had
// before the damage went in: the same seed gives the same ship, so fixtures and
// same-state comparisons stay stable.
func TestNewGameShipIsReproducibleFromItsSeed(t *testing.T) {
	first := defaultShipSystems(20260905)
	second := defaultShipSystems(20260905)
	for i := range first {
		if first[i].Status != second[i].Status {
			t.Fatalf("system %d differs between runs: %d vs %d",
				i, first[i].Status, second[i].Status)
		}
		for row := range first[i].Rows {
			for column := range first[i].Rows[row] {
				if first[i].Rows[row][column] != second[i].Rows[row][column] {
					t.Fatalf("system %d row %d column %d differs", i, row, column)
				}
			}
		}
	}
	if same := defaultShipSystems(20260906); same[0].Status == first[0].Status &&
		same[1].Status == first[1].Status && same[2].Status == first[2].Status &&
		same[3].Status == first[3].Status && same[4].Status == first[4].Status &&
		same[5].Status == first[5].Status {
		t.Error("a different seed produced an identical ship")
	}
}

// TestRowStatusDistributionMatchesTheMeasuredOriginal checks the shape against
// the original rather than against the code that produced it. Three independent
// new games on the port oracle gave 72 row statuses: 49 intact (4), 8 at 3 and
// 15 at 0 — see docs/atari-st/secondary-gamestate.md. The damaged share is what
// `random() mod 3` predicts; the split between the two damaged shapes is what
// this test keeps honest.
func TestRowStatusDistributionMatchesTheMeasuredOriginal(t *testing.T) {
	counts := map[int]int{}
	rows := 0
	for seed := int64(1); seed <= 3000; seed++ {
		for _, system := range defaultShipSystems(seed) {
			source, ok := sourceShipSystem(system.ID)
			if !ok {
				t.Fatalf("no source for %s", system.ID)
			}
			for _, row := range system.Rows {
				counts[source.RowStatus(row)]++
				rows++
			}
		}
	}
	share := func(status int) float64 { return float64(counts[status]) / float64(rows) }

	// Measured: 49/72 = 68.1%, 8/72 = 11.1%, 15/72 = 20.8%. The sample is small,
	// so the bands are wide enough to hold it without being vacuous.
	for _, want := range []struct {
		status    int
		low, high float64
		label     string
	}{
		{4, 0.60, 0.74, "intact"},
		{3, 0.07, 0.16, "alternate part"},
		{0, 0.15, 0.28, "dead or empty"},
	} {
		if got := share(want.status); got < want.low || got > want.high {
			t.Errorf("row status %d (%s) = %.1f%%, want between %.0f%% and %.0f%%",
				want.status, want.label, got*100, want.low*100, want.high*100)
		}
	}
	if counts[4]+counts[3]+counts[0] != rows {
		t.Errorf("unexpected row statuses beyond 4/3/0: %v", counts)
	}
}

// 兩種壞法的比例現在有解了：`(欄 == 0) | (random() & 1)` 為假才寫替代零件，
// 也就是壞掉的列裡 3/4 × 1/2 = 3/8 是狀態 3，其餘 5/8 平分成空槽與死零件。
// 這條測試比 TestRowStatusDistributionMatchesTheMeasuredOriginal 窄：那一條拿
// 72 個實測樣本當寬帶，這一條拿解出來的比例當窄帶。
func TestDamagedRowsSplitThreeEighthsToTheAlternatePart(t *testing.T) {
	damaged, alternate, dead, empty := 0, 0, 0, 0
	for seed := int64(1); seed <= 4000; seed++ {
		for _, system := range defaultShipSystems(seed) {
			source, ok := sourceShipSystem(system.ID)
			if !ok {
				t.Fatalf("%s 沒有來源資料", system.ID)
			}
			for _, row := range system.Rows {
				if source.RowStatus(row) == 4 {
					continue
				}
				damaged++
				for _, item := range row {
					switch item {
					case newGameAlternatePart:
						alternate++
					case newGameDeadPart:
						dead++
					case newGameEmptySlot:
						empty++
					}
				}
			}
		}
	}
	if damaged == 0 {
		t.Fatal("四千個種子裡一列都沒壞")
	}
	share := func(n int) float64 { return float64(n) / float64(damaged) }
	for _, want := range []struct {
		got   float64
		low   float64
		high  float64
		label string
	}{
		{share(alternate), 0.355, 0.395, "替代零件（3/8）"},
		{share(dead), 0.29, 0.335, "死零件（5/16）"},
		{share(empty), 0.29, 0.335, "空槽（5/16）"},
	} {
		if want.got < want.low || want.got > want.high {
			t.Errorf("%s 佔壞掉的列 %.1f%%，應該落在 %.1f%%–%.1f%%",
				want.label, want.got*100, want.low*100, want.high*100)
		}
	}
}
