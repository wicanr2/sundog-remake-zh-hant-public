package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// The supply and demand a city trades under come from logical block 7, not
// from a die: eighteen records, one a planet, thirty three-bit classes each.
func TestEveryCityGetsItsTradeClassesFromTheDisk(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]int{}
	for _, destination := range reducer.World.Destinations() {
		classes, ok := world.TradeClassesFor(destination.CityID, NewState().TradeClassShift)
		if !ok {
			t.Fatalf("城市 %q 沒有交易類別", destination.CityID)
		}
		if len(classes.Supply) != atarist.TradeClassGoods || len(classes.Demand) != atarist.TradeClassGoods {
			t.Fatalf("城市 %q 的類別長度是 %d/%d", destination.CityID, len(classes.Supply), len(classes.Demand))
		}
		for goods := range classes.Supply {
			if classes.Supply[goods] < 0 || classes.Supply[goods] > atarist.TradeClassMax ||
				classes.Demand[goods] < 0 || classes.Demand[goods] > atarist.TradeClassMax {
				t.Fatalf("城市 %q 商品 %d 的類別超出三個位元", destination.CityID, goods+1)
			}
		}
		// get_data gives the planet's own produce supply 7 and demand 0.
		if classes.HomeGoods > 0 {
			index := classes.HomeGoods - 1
			if classes.Supply[index] != atarist.TradeClassMax || classes.Demand[index] != 0 {
				t.Fatalf("城市 %q 的本地產物 %d 沒有被拉滿供給／歸零需求", destination.CityID, classes.HomeGoods)
			}
			seen[classes.HomeGoods]++
		}
	}
	// The table is not one record repeated: several different goods have to
	// come up as somebody's produce.
	if len(seen) < 5 {
		t.Fatalf("只有 %d 種商品被當成本地產物，表看起來沒有讀對", len(seen))
	}
}

// init_sd seeds inside [0, class]; adjust climbs back to the class and stops.
func TestTheMarketSeedsAndAgesWithinItsClass(t *testing.T) {
	state := NewState()
	for class := 0; class <= atarist.TradeClassMax; class++ {
		for i := 0; i < 200; i++ {
			value := seedFromClass(&state, class)
			if value < 0 || value > class {
				t.Fatalf("類別 %d 種出 %d，應該落在 [0, %d]", class, value, class)
			}
		}
		if got := ageUpTo(0, class, 1); got != minInt(1, class) {
			t.Fatalf("類別 %d 老化一期得到 %d", class, got)
		}
		if got := ageUpTo(class, class, 10); got != class {
			t.Fatalf("已經在類別上還被推到 %d", got)
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
