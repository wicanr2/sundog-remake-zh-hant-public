package world

import "github.com/wicanr2/sundog-remake/internal/source/atarist"

// The exchange's supply and demand start from a table on the disk, not from a
// die: logical block 7 holds one record a planet, and XDOTRADI:0x07 get_data
// picks the record using the saved game's shift as
//
//	((planet + shift) mod 18)
//
// then applies both planet-specific goods overrides from that planet's own
// record. The city map header is unrelated. See banville-restoration-spec.md.

// TradeClasses is the supply and demand class of each goods for a city,
// indexed by goods − 1, and the goods the planet produces (0 for none).
type TradeClasses struct {
	Supply      []int
	Demand      []int
	HomeGoods   int
	DemandGoods int
}

// TradeClassesFor is the classes a city trades under.
func TradeClassesFor(cityID string, shift int) (TradeClasses, bool) {
	if shift < 0 || shift >= atarist.TradeClassRecords {
		return TradeClasses{}, false
	}
	table, err := atarist.EmbeddedTradeClassData()
	if err != nil {
		return TradeClasses{}, false
	}
	planet, ok := planetIndexOf(cityID)
	if !ok {
		return TradeClasses{}, false
	}
	record, ok := table.Record(((planet + shift) % atarist.TradeClassRecords))
	if !ok {
		return TradeClasses{}, false
	}
	classes := TradeClasses{
		Supply: append([]int(nil), record.Supply...),
		Demand: append([]int(nil), record.Demand...),
	}
	// The produce comes from the planet's own record, not the shifted one.
	if home, found := table.Record(planet); found && home.HomeGoods > 0 {
		index := home.HomeGoods - 1
		if index < len(classes.Supply) {
			classes.Supply[index] = atarist.TradeClassMax
			classes.Demand[index] = 0
			classes.HomeGoods = home.HomeGoods
		}
	}
	if home, found := table.Record(planet); found && home.DemandGoods > 0 {
		index := home.DemandGoods - 1
		classes.Supply[index] = 0
		classes.Demand[index] = atarist.TradeClassMax
		classes.DemandGoods = home.DemandGoods
	}
	return classes, true
}

// planetIndexOf is the planet a city sits on, zero based.
func planetIndexOf(cityID string) (int, bool) {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		return 0, false
	}
	for _, city := range data.Cities() {
		if city.ID == cityID {
			return city.PlanetIndex, true
		}
	}
	return 0, false
}
