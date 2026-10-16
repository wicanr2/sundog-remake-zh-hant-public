package world

import (
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// Facility, fuel, and market data are remake-owned prototype data. The Atari
// ST manual proves that these kinds of places and operations exist, but it
// does not prove the original binary's complete city coordinates, fuel price,
// or supply/demand table.

// minimumCargoPrice and remakeCargoSpread are remake-owned. The source supplies
// a per-cargo value; it does not say what a city charges above or below it.
const (
	minimumCargoPrice = 5
	remakeCargoSpread = 12
)

type Facility string

const (
	FacilitySpaceport Facility = "spaceport"
	FacilityWarehouse Facility = "warehouse"
	FacilityUniteller Facility = "uniteller"
	FacilityExchange  Facility = "exchange"
	FacilityColony    Facility = "colony"
	FacilityDepot     Facility = "depot"
	// FacilityTubeStation is the city's own tube station building, decoded
	// from the city map (the original's door code 18). The remake also keeps
	// FacilitySpaceport on that cell, which is its own reading rather than
	// the original's dispatch; see BuildingFacilityAt.
	FacilityTubeStation Facility = "tube-station"
)

type TilePosition struct {
	X int
	Y int
}

// MarketQuote keeps the item identity beside its local buy/sell values. The
// item key is the validated Apple II source identity used by the first typed
// consumer; the prices are intentionally not marked as original-exact.
type MarketQuote struct {
	ItemKey   string
	BuyPrice  int
	SellPrice int
}

// CargoKind retains goods1..31 from the Atari ST cargo table. BaseValue is
// raw record byte15, read unconditionally by XDOTRADI:12 (0x0927..0x0933).
// Exchange prices use internal/game/exchange.go; MarketQuote below remains
// prototype pricing and does not establish original parity.
type CargoKind struct {
	ID        string
	Key       string
	Name      string
	BaseValue int
	// Code is the record's index in the item table: what a pod lot, a quote
	// and the exchange's supply tables carry (XDOTRADI's five-bit goods).
	Code int
}

func sourceCargoKinds() ([]CargoKind, error) {
	data, err := atarist.EmbeddedItemData()
	if err != nil {
		return nil, fmt.Errorf("load Atari ST item data: %w", err)
	}
	kinds := make([]CargoKind, 0, 32)
	for _, entry := range data.Cargo() {
		if entry.Index == 0 {
			continue
		}
		kinds = append(kinds, CargoKind{
			ID:        entry.ID,
			Key:       "atarist.cargo." + entry.ID,
			Name:      entry.Name,
			BaseValue: entry.RawTailByte,
			Code:      entry.Index,
		})
	}
	if len(kinds) == 0 {
		return nil, fmt.Errorf("Atari ST cargo table produced no tradable kinds")
	}
	return kinds, nil
}

const (
	cityMapRows       = 20
	cityMapColumns    = 12
	tellerTerrainCode = 15
)

var facilityOrder = []Facility{
	FacilitySpaceport,
	FacilityWarehouse,
	FacilityUniteller,
	FacilityExchange,
	FacilityColony,
	FacilityTubeStation,
	FacilityDepot,
}

func prototypeFacilities(destinations []Destination) map[string]map[TilePosition]map[Facility]struct{} {
	result := make(map[string]map[TilePosition]map[Facility]struct{}, len(destinations))
	for _, destination := range destinations {
		byPosition := make(map[TilePosition]map[Facility]struct{})
		addFacility := func(position TilePosition, facilities ...Facility) {
			set := byPosition[position]
			if set == nil {
				set = make(map[Facility]struct{})
				byPosition[position] = set
			}
			for _, facility := range facilities {
				set[facility] = struct{}{}
			}
		}
		// The landing cell is the city's exchange building, which every one of
		// the 60 decoded maps has. The remake keeps its whole facility cluster
		// there so a city is usable the moment the player arrives; the original
		// spreads these across separate buildings and a teller tile, and only
		// the exchange's position is decoded.
		landing := LandingPosition(destination.CityID)
		addFacility(landing, FacilitySpaceport, FacilityWarehouse, FacilityUniteller, FacilityExchange)
		// Where the city's own depot and tube station are decoded, those cells
		// work too — those two positions are the original's.
		if cityMap, ok := CityMap(destination.CityID); ok {
			for _, building := range cityMap.Buildings() {
				position := TilePosition{X: building.Row, Y: building.Column}
				switch building.Facility {
				case atarist.FacilityDepot:
					addFacility(position, FacilityWarehouse, FacilityDepot)
				case atarist.FacilityTubeStation:
					addFacility(position, FacilitySpaceport, FacilityTubeStation)
				}
			}
			// The original's teller is terrain code 15 (drawn as screen code
			// 32, dispatched as city.facility-teller); every such cell is a
			// Uniteller (parity-playtest-report.md, second batch).
			for row := 0; row < cityMapRows; row++ {
				for column := 0; column < cityMapColumns; column++ {
					if code, ok := cityMap.Terrain(row, column); ok && code == tellerTerrainCode {
						addFacility(TilePosition{X: row, Y: column}, FacilityUniteller)
					}
				}
			}
		}
		if destination.ID == MissionDestinationID {
			addFacility(landing, FacilityColony)
		}
		result[destination.CityID] = byPosition
	}
	return result
}

// sourceMarkets gives every city one cargo from the source table. Which cargo a
// city offers, and the spread between its buy and sell price, are remake-owned:
// the original's supply model lives in XDOTRADI (set_demand, update_market,
// current_price), which has not been read. The price itself starts from the
// source record's own value rather than an invented number.
func sourceMarkets(destinations []Destination, kinds []CargoKind) map[string]MarketQuote {
	result := make(map[string]MarketQuote, len(destinations))
	for index, destination := range destinations {
		kind := kinds[index%len(kinds)]
		buy := kind.BaseValue
		if buy < minimumCargoPrice {
			buy = minimumCargoPrice
		}
		sell := buy - remakeCargoSpread
		if sell < minimumCargoPrice {
			sell = minimumCargoPrice
		}
		result[destination.CityID] = MarketQuote{
			ItemKey:   kind.Key,
			BuyPrice:  buy,
			SellPrice: sell,
		}
	}
	return result
}

func prototypeFuelPrices(destinations []Destination) map[string]int {
	result := make(map[string]int, len(destinations))
	for index, destination := range destinations {
		// Prototype price per fuel unit. The manual proves the refuelling
		// operation, not this value or the original transaction formula.
		result[destination.CityID] = 2 + index%3
	}
	return result
}

// FacilitiesAt returns a stable, display-friendly list at one city tile.
func (r Registry) FacilitiesAt(cityID string, position TilePosition) []Facility {
	byPosition := r.facilities[cityID]
	set := byPosition[position]
	result := make([]Facility, 0, len(set))
	for _, facility := range facilityOrder {
		if _, ok := set[facility]; ok {
			result = append(result, facility)
		}
	}
	return result
}

// FacilityPosition is the lowest-numbered cell of a city that has the
// facility. Nothing in play needs it — the player walks to the door — but the
// diagnostic entry points have to put the walker on one without walking. The
// lowest cell rather than any cell, so the answer does not move between runs.
func (r Registry) FacilityPosition(cityID string, facility Facility) (TilePosition, bool) {
	best, found := TilePosition{}, false
	for position, set := range r.facilities[cityID] {
		if _, ok := set[facility]; !ok {
			continue
		}
		if !found || position.X < best.X || (position.X == best.X && position.Y < best.Y) {
			best, found = position, true
		}
	}
	return best, found
}

func (r Registry) HasFacility(cityID string, x, y int, facility Facility) bool {
	for _, candidate := range r.FacilitiesAt(cityID, TilePosition{X: x, Y: y}) {
		if candidate == facility {
			return true
		}
	}
	return false
}

func (r Registry) Market(cityID string) (MarketQuote, bool) {
	quote, ok := r.markets[cityID]
	return quote, ok
}

func (r Registry) FuelPrice(cityID string) (int, bool) {
	d, ok := r.Destination(cityID)
	if !ok {
		return 0, false
	}
	s, ok := r.System(d.SystemID)
	if !ok {
		return 0, false
	}
	return 100 * (s.EncounterRating + s.PriceLevel + 16 - s.StreetDealRating) / 14, true
}
