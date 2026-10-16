package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// Controlled original inputs are fixed before execution. The comparison reads
// native receipts and runs the production initializer, not a second model.
func TestExportTalosCryoInitialization(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("需要Docker原生冷凍人初始化收據")
	}
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]int{}
	for _, city := range data.Cities() {
		byID[city.ID] = city.Index + 1
	}
	type coldLot struct{ City, Slot, Goods, Quality, Source, Price int }
	var result struct {
		Cases, Records, Differences, FetchCases, FetchDifferences int
		Rows                                                      []struct{ Initial, Random, RandomCalls int }
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for random := 0; random < 4; random++ {
		name := "native-cryo-source.json"
		if random > 0 {
			name = "native-cryo-source-rnd-" + itoa(random) + ".json"
		}
		var native struct {
			Rows []struct {
				Initial, RandomCalls int
				Lots                 []coldLot
			}
		}
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(b, &native); err != nil {
			t.Fatal(err)
		}
		if len(native.Rows) != 57 {
			t.Fatal("missing original initialization inputs")
		}
		for _, n := range native.Rows {
			calls := 0
			warehouses, err := initialCryoWarehouses(data.Systems(), data.Planets(), data.Cities(), n.Initial, func() int { calls++; return random })
			if err != nil {
				t.Fatal(err)
			}
			expected := map[int]coldLot{}
			for _, l := range n.Lots {
				expected[l.City] = l
			}
			different := calls != n.RandomCalls
			for city, slots := range warehouses {
				for i, l := range slots {
					e := expected[byID[city]]
					if i > 0 {
						e = coldLot{}
					}
					if l != (Lot{Goods: e.Goods, Quality: e.Quality, Price: e.Price, SourceCode: uint8(e.Source)}) {
						different = true
					}
				}
			}
			if different {
				result.Differences++
			}
			result.Cases++
			result.Records += len(n.Lots)
			result.Rows = append(result.Rows, struct{ Initial, Random, RandomCalls int }{n.Initial, random, calls})
			// The original roundtrip receipt uses the first zero-RND case.
			if random == 0 && n.Initial == 1 {
				for _, e := range n.Lots {
					s := NewState()
					s.Warehouses, err = initialCryoWarehouses(data.Systems(), data.Planets(), data.Cities(), 1, func() int { return 0 })
					if err != nil {
						t.Fatal(err)
					}
					for city, source := range byID {
						if source == e.City {
							s.CityID = city
						}
					}
					before := map[string][]Lot{}
					for city, slots := range s.Warehouses {
						before[city] = append([]Lot(nil), slots...)
					}
					before[s.CityID][0] = Lot{}
					bank, cash := s.Bank, s.Cash
					_, ok := r.fetchLot(&s, 0)
					if !ok || s.Holds[0].Lot() != (Lot{Goods: e.Goods, Quality: e.Quality, Price: e.Price, SourceCode: uint8(e.Source)}) || s.Holds[1] != (Hold{}) || !reflect.DeepEqual(s.Warehouses, before) || s.Bank != bank || s.Cash != cash {
						result.FetchDifferences++
					}
					result.FetchCases++
				}
			}
		}
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-cryo-source.json"), append(b, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if result.Cases != 228 || result.Records != 3876 || result.FetchCases != 17 || result.Differences != 0 || result.FetchDifferences != 0 {
		t.Fatalf("cryo comparison failed: %+v", result)
	}
}
