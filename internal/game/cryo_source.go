package game

import (
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// XSTARTUP:2d, raw disk base 0x39c00, body 0x1872..0x1962.
// READY: docs/atari-st/banville-restoration-spec.md, 冷凍人新局倉庫內容.
// Source order is significant: the third system's first planet supplies the
// excluded city interval and its last city receives the fixed batch.
func initialCryoWarehouses(systems []atarist.System, planets []atarist.Planet, cities []atarist.City, initial int, random func() int) (map[string][]Lot, error) {
	const citySpan = 57
	if len(systems) < 3 || systems[2].Index != 2 || initial < 1 || initial > citySpan || random == nil {
		return nil, fmt.Errorf("invalid cryogen initialization input")
	}
	planetIndex := systems[2].PlanetStart
	if planetIndex < 0 || planetIndex >= len(planets) {
		return nil, fmt.Errorf("missing cryogen source planet")
	}
	planet := planets[planetIndex]
	first, last := planet.CityStart+1, planet.CityStart+planet.CityCount
	if planet.Index != planetIndex || planet.SystemIndex != 2 || planet.CityCount < 1 || first < 1 || last > citySpan {
		return nil, fmt.Errorf("invalid cryogen source city interval")
	}
	// The range walker must have sixteen distinct eligible cities in addition
	// to the fixed batch. Invalid source data must not trap it in the loop.
	if citySpan-planet.CityCount < 16 {
		return nil, fmt.Errorf("insufficient cryogen destination cities")
	}
	warehouses := make(map[string][]Lot, len(cities))
	bySource := make(map[int]string, len(cities))
	for _, city := range cities {
		if city.ID == "" || city.Index < 0 || bySource[city.Index+1] != "" || warehouses[city.ID] != nil {
			return nil, fmt.Errorf("invalid cryogen city mapping")
		}
		bySource[city.Index+1] = city.ID
		warehouses[city.ID] = make([]Lot, WarehouseLots)
	}
	for source := 1; source <= citySpan; source++ {
		if bySource[source] == "" {
			return nil, fmt.Errorf("missing cryogen city %d", source)
		}
	}
	if citiesSource := bySource[last]; citiesSource == "" {
		return nil, fmt.Errorf("missing fixed cryogen city")
	}
	put := func(source int) {
		warehouses[bySource[source]][0] = Lot{Goods: 31, Quality: 7, SourceCode: uint8(source)}
	}
	put(last)
	current := initial
	for count := 1; count < 17; {
		value := random()
		if value < 0 || value > 32767 {
			return nil, fmt.Errorf("invalid cryogen random input %d", value)
		}
		current = (current+value%4+1)%citySpan + 1
		if current >= first && current <= last || warehouses[bySource[current]][0].Goods != 0 {
			continue
		}
		put(current)
		count++
	}
	return warehouses, nil
}

// Only NewState calls this. Loading a save never allocates or replenishes lots.
// The stream is remake-owned; controlled oracle comparisons supply explicit
// inputs rather than claiming the original's natural RNG sequence.
func newGameWarehouses(seed int64) map[string][]Lot {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		panic(err)
	}
	roll := newGameRoller(seed)
	warehouses, err := initialCryoWarehouses(data.Systems(), data.Planets(), data.Cities(), 1+roll(57), func() int { return roll(32768) })
	if err != nil {
		panic(err)
	}
	return warehouses
}
