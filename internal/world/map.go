package world

import (
	"fmt"
	"sync"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// Terrain is the semantic tile the remake draws. It is deliberately separate
// from theme art: every presentation group renders the same semantic terrain.
type Terrain string

const (
	TerrainGrass Terrain = "grass"
	TerrainRoad  Terrain = "road"
	TerrainWater Terrain = "water"
	TerrainCity  Terrain = "city"
	TerrainShip  Terrain = "ship"
	TerrainWall  Terrain = "wall"
	TerrainCargo Terrain = "cargo"
)

// The map shape and every cell's code come from the original: 60 cities, each
// a 20 × 12 grid of 4-bit codes, decoded in docs/atari-st/city-maps.md.
//
// Three things about the codes are the original's, and the table below keeps
// them:
//
//   - Codes 0 and 1 are never drawn as a tile. They are the ground the rest
//     sits on, so they are the remake's open terrain too.
//   - Codes 5 and 15 are the two special tiles, and they live in the same strip
//     as the building sprites rather than with the terrain — so they are
//     structures, not ground.
//   - The remaining codes are 13 distinct terrains, each with two interchangeable
//     looks the original picks between with a die roll.
//
// What each of those 13 actually depicts is **not** from the original: it loads
// a different tile-image strip per city, so one code is different art in
// different cities, and the original's own art stays out of this repository.
// Which of them block is the remake's too — the original decides collision from
// the drawn pixel colours (its collision set is `01111100`, i.e. palette indices
// 1–5 block), which again needs its art.
var terrainByCode = [16]Terrain{
	// 0, 1: the original draws no tile here at all.
	TerrainGrass, TerrainGrass,
	// 2–4: three of the thirteen terrains.
	TerrainRoad, TerrainRoad, TerrainCargo,
	// 5: a special tile from the building strip.
	TerrainCity,
	// 6–10: five more of the thirteen.
	TerrainWall, TerrainWater, TerrainGrass, TerrainRoad, TerrainCargo,
	// 11: the original rolls between four looks for this one.
	TerrainGrass,
	// 12–14: the last three of the thirteen.
	TerrainWall, TerrainWater, TerrainWall,
	// 15: the other special tile from the building strip.
	TerrainCity,
}

var (
	cityMapsOnce  sync.Once
	cityMapsByID  map[string]atarist.CityMap
	cityMapsError error
)

// loadCityMaps indexes the embedded city maps by the same city ID the world
// tables use. Both tables are the original's own 60-entry city list in the same
// order, so the index is the join.
func loadCityMaps() (map[string]atarist.CityMap, error) {
	cityMapsOnce.Do(func() {
		maps, err := atarist.EmbeddedCityMapData()
		if err != nil {
			cityMapsError = fmt.Errorf("load Atari ST city maps: %w", err)
			return
		}
		data, err := atarist.EmbeddedWorldData()
		if err != nil {
			cityMapsError = fmt.Errorf("load Atari ST world data: %w", err)
			return
		}
		// The city table keeps its own record index, and the map table is that
		// same 60-entry list in the same order. The world registry drops the two
		// placeholder records, so the join is by record index rather than by
		// position in the filtered list.
		cities := data.Cities()
		indexed := make(map[string]atarist.CityMap, len(cities))
		for _, city := range cities {
			cityMap, ok := maps.City(city.Index)
			if !ok {
				cityMapsError = fmt.Errorf("city %q (record %d) has no map", city.ID, city.Index)
				return
			}
			indexed[city.ID] = cityMap
		}
		cityMapsByID = indexed
	})
	return cityMapsByID, cityMapsError
}

// CityMap returns a city's decoded ground map.
func CityMap(cityID string) (atarist.CityMap, bool) {
	maps, err := loadCityMaps()
	if err != nil {
		return atarist.CityMap{}, false
	}
	cityMap, ok := maps[cityID]
	return cityMap, ok
}

// TerrainAt returns the semantic terrain of a cell on a city's ground map. A
// cell that holds one of the city's buildings reads as a building whatever the
// terrain code under it is: the original draws the building sprite over that
// cell, so the code beneath it is never seen.
// TerrainCode is the raw four-bit code the source map stores for a cell, and
// the city's own one-based number. A renderer drawing the original's tiles
// needs both: the code picks the tile and the number picks the city's strips.
func TerrainCode(cityID string, x, y int) (code, cityNumber int, ok bool) {
	cityMap, found := CityMap(cityID)
	if !found {
		return 0, 0, false
	}
	if x < 0 || y < 0 || x >= MapWidth() || y >= MapHeight() {
		return 0, 0, false
	}
	value, cellOK := cityMap.Terrain(x, y)
	if !cellOK {
		return 0, 0, false
	}
	return value, cityMap.Number, true
}

func TerrainAt(cityID string, x, y int) (Terrain, bool) {
	cityMap, ok := CityMap(cityID)
	if !ok {
		return "", false
	}
	code, ok := cityMap.Terrain(x, y)
	if !ok {
		return "", false
	}
	for _, building := range cityMap.Buildings() {
		if building.Row == x && building.Column == y {
			return TerrainCity, true
		}
	}
	return terrainByCode[code], true
}

// MapWidth and MapHeight are the original map's dimensions. The shells show a
// smaller window onto it; see ViewportWidth/ViewportHeight.
// The map's long axis (20 cells, the source package's "row") runs across the
// screen: the original addresses the map as map[column−10][row−6] with a
// stride of twelve cells per column, and its ground screen shows the
// twenty-cell axis horizontally (hatari-live-run.md round twelve: the Drahew
// snapshot only matches the remake's tiles with the axes this way round).
// So x walks the twenty-cell axis and y the twelve-cell one.
func MapWidth() int  { return atarist.CityMapRows }
func MapHeight() int { return atarist.CityMapColumns }

// 視埠是原版介面框中央那一塊：165×116 的空間，以 16 像素的格子排得下 10×7。
// 座標量在 docs/ui/original-layout.json。
const (
	viewportWidth  = 10
	viewportHeight = 7
)

// ViewportWidth and ViewportHeight are how much of the map a shell draws at
// once. The original scrolls a window too; this size is the remake's, chosen to
// fit the panel it already had.
func ViewportWidth() int  { return viewportWidth }
func ViewportHeight() int { return viewportHeight }

// ViewportOrigin centres the window on a position and then clamps it inside the
// map, so the player is always visible and the window never shows empty space.
func ViewportOrigin(x, y int) (int, int) {
	originX := x - viewportWidth/2
	originY := y - viewportHeight/2
	if originX > MapWidth()-viewportWidth {
		originX = MapWidth() - viewportWidth
	}
	if originY > MapHeight()-viewportHeight {
		originY = MapHeight() - viewportHeight
	}
	if originX < 0 {
		originX = 0
	}
	if originY < 0 {
		originY = 0
	}
	return originX, originY
}

// IsPassable applies the remake-owned collision contract: water and walls
// block, everything else is walkable. The original decides this from drawn
// pixel colours, which needs its tile art; this is not that claim.
func IsPassable(terrain Terrain) bool {
	switch terrain {
	case TerrainWater, TerrainWall:
		return false
	default:
		return terrain != ""
	}
}

// CanEnter verifies both the city identity and the terrain cell.
func (r Registry) CanEnter(cityID string, position TilePosition) bool {
	knownCity := false
	for _, destination := range r.destinations {
		if destination.CityID == cityID {
			knownCity = true
			break
		}
	}
	if !knownCity {
		return false
	}
	terrain, ok := TerrainAt(cityID, position.X, position.Y)
	return ok && IsPassable(terrain)
}

// BuildingFacilityAt reports which of the city's decoded buildings occupies a
// cell, translated into the remake's facility vocabulary. The building and its
// position are the original's; which remake facility each one maps to is the
// remake's reading of the original's own dispatch (tube station → spaceport,
// depot → warehouse, exchange → exchange).
func BuildingFacilityAt(cityID string, position TilePosition) (Facility, bool) {
	cityMap, ok := CityMap(cityID)
	if !ok {
		return "", false
	}
	for _, building := range cityMap.Buildings() {
		if building.Row != position.X || building.Column != position.Y {
			continue
		}
		switch building.Facility {
		case atarist.FacilityTubeStation:
			return FacilitySpaceport, true
		case atarist.FacilityDepot:
			return FacilityWarehouse, true
		case atarist.FacilityExchange:
			return FacilityExchange, true
		}
	}
	return "", false
}

// mapColumnBase and mapRowBase are the original's map coordinate origins:
// columns count from 10 and rows from 6 (tile.map-addressing).
const (
	mapColumnBase = 10
	mapRowBase    = 6
)

// landingSquareOf is the planet record's landing square for a city, in the
// map's own zero-based cells, and whether the record carries one.
func landingSquareOf(cityID string) (TilePosition, bool) {
	data, err := atarist.EmbeddedWorldData()
	if err != nil {
		return TilePosition{}, false
	}
	planets := data.Planets()
	for _, city := range data.Cities() {
		if city.ID != cityID || city.PlanetIndex < 0 || city.PlanetIndex >= len(planets) {
			continue
		}
		planet := planets[city.PlanetIndex]
		if planet.LandingColumn == 0 && planet.LandingRow == 0 {
			return TilePosition{}, false
		}
		return TilePosition{X: planet.LandingColumn - mapColumnBase, Y: planet.LandingRow - mapRowBase}, true
	}
	return TilePosition{}, false
}

// LandingPosition is where the player stands after arriving: the planet
// record's landing square (planet.landing-square), which is where the
// original puts the walker — Drahew's is (1,0), the cell the live run
// started on. A city whose planet carries none lands on its exchange, the
// one building slot the original fills everywhere, or the first walkable
// cell.
//
// The square is taken as it is, even when the remake's own cell rules call
// that cell a wall or water. The original decides passability by pixel
// colour, so a planet's square can be a wall tile here and still be where
// the walker stands: three of the 58 cities are like that (Dramming and
// Aylika land on a wall, Tram-El on water), and all three have walkable
// neighbours, so the walker steps off without trouble. The path finder only
// tests the cells it moves *into*, which is what makes that work.
func LandingPosition(cityID string) TilePosition {
	if landing, ok := landingSquareOf(cityID); ok {
		return landing
	}
	cityMap, ok := CityMap(cityID)
	if ok {
		for _, building := range cityMap.Buildings() {
			if building.Facility == atarist.FacilityExchange {
				return TilePosition{X: building.Row, Y: building.Column}
			}
		}
	}
	for y := 0; y < MapHeight(); y++ {
		for x := 0; x < MapWidth(); x++ {
			if terrain, ok := TerrainAt(cityID, x, y); ok && IsPassable(terrain) {
				return TilePosition{X: x, Y: y}
			}
		}
	}
	return TilePosition{}
}
