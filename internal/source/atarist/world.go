// Package atarist exposes typed, evidence-preserving views of Atari ST source
// data. The records keep their raw disk offsets and undecoded tail bytes so a
// remake consumer can never silently promote a guess into a rule.
package atarist

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	// systemRecordSize .. cityRecordSize mirror the record boundaries proven by
	// tools/atari_st_world_data.py against the read-only source disk.
	nameFieldSize     = 10
	systemRecordSize  = 20
	planetRecordSize  = 16
	cityRecordSize    = 12
	systemRecordCount = 12
	planetRecordCount = 18
	cityRecordCount   = 60

	// fillerCityName is the reserved name used by two unused city slots. They
	// are source records, not places, and never reach a player-facing list.
	fillerCityName = "dummy"
)

type rawRecord struct {
	Index            int    `json:"index"`
	RawOffset        string `json:"rawOffset"`
	Name             string `json:"name"`
	TailHex          string `json:"tailHex"`
	Coordinates      []int  `json:"coordinates"`
	EncounterRating  int    `json:"encounterRating"`
	PriceLevel       int    `json:"priceLevel"`
	StreetDealRating int    `json:"streetDealRating"`
	Orbit            *struct {
		Radius int `json:"radius"`
		Angle  int `json:"angle"`
	} `json:"orbit"`
	OrbitRate     int `json:"orbitRate"`
	LandingSquare *struct {
		Column int `json:"column"`
		Row    int `json:"row"`
	} `json:"landingSquare"`
	Position *struct {
		Column int `json:"column"`
		Row    int `json:"row"`
	} `json:"position"`
	PlanetMapPixel *struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"planetMapPixel"`
}

type rawTable struct {
	RawOffset   string      `json:"rawOffset"`
	RecordSize  int         `json:"recordSize"`
	RecordCount int         `json:"recordCount"`
	Records     []rawRecord `json:"records"`
}

type rawRun struct {
	System string `json:"system"`
	Planet string `json:"planet"`
	Start  int    `json:"start"`
	Count  int    `json:"count"`
}

type rawTrailingCity struct {
	Index  int    `json:"index"`
	City   string `json:"city"`
	Planet string `json:"planet"`
	System string `json:"system"`
}

type rawGrouping struct {
	ContiguousCityCount int               `json:"contiguousCityCount"`
	FillerCityCount     int               `json:"fillerCityCount"`
	TrailingCityCount   int               `json:"trailingCityCount"`
	PlanetCityRuns      []rawRun          `json:"planetCityRuns"`
	TrailingCities      []rawTrailingCity `json:"trailingCities"`
	SystemPlanetRuns    []rawRun          `json:"systemPlanetRuns"`
}

type worldDocument struct {
	Schema int `json:"schema"`
	Tables struct {
		Systems rawTable `json:"systems"`
		Planets rawTable `json:"planets"`
		Cities  rawTable `json:"cities"`
	} `json:"tables"`
	Grouping rawGrouping `json:"grouping"`
}

// System is one entry of the 12-record star-system table.
type System struct {
	Index int
	ID    string
	Name  string
	// Coordinates are the three 4-bit fields the original packs into the
	// record's eighth word. SHIPLIB:0x0a dist reads exactly these to measure a
	// jump; see docs/atari-st/ship-rules.md.
	Coordinates [3]int
	// PriceLevel is the first 4-bit field of the record's tenth word. It is the
	// numerator of the price multiplier every shop in the system applies.
	PriceLevel int
	// StreetDealRating is the record's sixth 4-bit field, the second one of the
	// tenth word. Its only decoded consumer is XMOVEONG:0x36 confirm, which
	// cancels a street encounter of kind 22 -- the one that opens with "can I
	// interest you in a deal" -- when random() mod 20 falls under it, so a
	// higher value means fewer of them. XDOINTER:0x24 initialize reads it too;
	// what that side does with it is unresolved.
	StreetDealRating int
	// EncounterRating is the record's fourth 4-bit field. The jump distance
	// never reads it; SHIPLIB:0x1f update_movement does, to decide whether an
	// enemy appears in this system and which class it is. A higher value means
	// fewer encounters.
	EncounterRating int
	RawOffset       int
	TailHex         string
	PlanetStart     int
	PlanetCount     int
}

// Planet is one entry of the 18-record planet table.
// OrbitTurnSteps is one full turn of a planet's orbit. The original's angle is
// a byte, so a turn is 256 steps — the same scale the sine table projects with.
const OrbitTurnSteps = 0x100

// Orbit is a planet's place in its star system: an angle on a 256-step turn and
// a radius the projection multiplies by. It is the record's seventh word, which
// XSTARTUP:0x2f init_location copies straight into the position the flight code
// then interpolates.
type Orbit struct {
	Radius int
	Angle  int
}

type Planet struct {
	Index       int
	ID          string
	Name        string
	SystemIndex int
	RawOffset   int
	TailHex     string
	Orbit       Orbit
	// OrbitRate is how far the planet moves each day. XMOVEONS:0x07
	// check_planets advances angle by rate × elapsed days and wraps it into a
	// byte; six of the eighteen planets have rate 0 and never move.
	OrbitRate int
	// LandingColumn and LandingRow are where a new game puts the walker on
	// the planet's city (rule planet.landing-square): the record's low
	// byte, column = 10 + byte mod 20, row = 6 + byte div 20, in the
	// original's 10- and 6-based map coordinates. Zero when the record
	// carries none.
	LandingColumn int
	LandingRow    int
	CityStart     int
	CityCount     int
}

// City is one entry of the 60-record city table. Filler records keep their slot
// so the table round-trips, but they are excluded from Cities().
type City struct {
	Index       int
	ID          string
	Name        string
	PlanetIndex int
	RawOffset   int
	TailHex     string
	// Hidden marks a city stored outside the contiguous per-planet runs. In the
	// source table this is the colony the player has to locate.
	Hidden bool
	// MapPosition is where the city sits on its planet's map. The two packed
	// fields and the pixel the original draws them at both come from the source
	// table; see docs/atari-st/ship-rules.md rules map.city-position and
	// map.city-pixel.
	MapPosition CityMapPosition
}

// CityMapPosition is a city's place on the planet map. Column and Row are the
// packed fields; X and Y are what the original's change_xy turns them into.
type CityMapPosition struct {
	Column int
	Row    int
	X      int
	Y      int
}

// WorldData is the validated view of the systems/planets/cities tables.
type WorldData struct {
	systems []System
	planets []Planet
	cities  []City
}

// LoadWorldData validates every record boundary, name, tail length and grouping
// run before the data can reach a player-facing registry. Any drift between the
// declared counts and the actual records is an error, not a silent truncation.
func LoadWorldData(reader io.Reader) (WorldData, error) {
	var document worldDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return WorldData{}, fmt.Errorf("decode Atari ST world data: %w", err)
	}
	if document.Schema != 1 {
		return WorldData{}, fmt.Errorf("unsupported Atari ST world data schema %d", document.Schema)
	}

	systemRecords, err := validateTable(document.Tables.Systems, "system", systemRecordSize, systemRecordCount)
	if err != nil {
		return WorldData{}, err
	}
	planetRecords, err := validateTable(document.Tables.Planets, "planet", planetRecordSize, planetRecordCount)
	if err != nil {
		return WorldData{}, err
	}
	cityRecords, err := validateTable(document.Tables.Cities, "city", cityRecordSize, cityRecordCount)
	if err != nil {
		return WorldData{}, err
	}

	grouping := document.Grouping
	if len(grouping.PlanetCityRuns) != planetRecordCount {
		return WorldData{}, fmt.Errorf("world data has %d planet city runs, want %d", len(grouping.PlanetCityRuns), planetRecordCount)
	}
	if len(grouping.SystemPlanetRuns) != systemRecordCount {
		return WorldData{}, fmt.Errorf("world data has %d system planet runs, want %d", len(grouping.SystemPlanetRuns), systemRecordCount)
	}

	systems := make([]System, 0, systemRecordCount)
	planetCursor := 0
	for index, run := range grouping.SystemPlanetRuns {
		record := systemRecords[index]
		if run.System != record.Name {
			return WorldData{}, fmt.Errorf("system run %d names %q, table names %q", index, run.System, record.Name)
		}
		if run.Start != planetCursor {
			return WorldData{}, fmt.Errorf("system %q starts at planet %d, want %d", run.System, run.Start, planetCursor)
		}
		if run.Count <= 0 {
			return WorldData{}, fmt.Errorf("system %q owns %d planets", run.System, run.Count)
		}
		if len(record.Coordinates) != systemCoordinateCount {
			return WorldData{}, fmt.Errorf("system %q carries %d coordinates, want %d", record.Name, len(record.Coordinates), systemCoordinateCount)
		}
		// A price level of zero would make every shop in the system give its
		// stock away, so it is a decoding failure rather than a cheap system.
		if record.PriceLevel <= 0 || record.PriceLevel > systemCoordinateMax {
			return WorldData{}, fmt.Errorf("system %q has price level %d, outside a usable 4-bit field",
				record.Name, record.PriceLevel)
		}
		var coordinates [systemCoordinateCount]int
		for axis, value := range record.Coordinates {
			if value < 0 || value > systemCoordinateMax {
				return WorldData{}, fmt.Errorf("system %q coordinate %d is %d, outside a 4-bit field", record.Name, axis, value)
			}
			coordinates[axis] = value
		}
		systems = append(systems, System{
			Index:            index,
			ID:               slug(record.Name),
			Name:             record.Name,
			Coordinates:      coordinates,
			EncounterRating:  record.EncounterRating,
			PriceLevel:       record.PriceLevel,
			StreetDealRating: record.StreetDealRating,
			RawOffset:        record.offset,
			TailHex:          record.TailHex,
			PlanetStart:      run.Start,
			PlanetCount:      run.Count,
		})
		planetCursor += run.Count
	}
	if planetCursor != planetRecordCount {
		return WorldData{}, fmt.Errorf("system runs cover %d planets, want %d", planetCursor, planetRecordCount)
	}

	planetSystem := make([]int, planetRecordCount)
	for systemIndex, system := range systems {
		for offset := 0; offset < system.PlanetCount; offset++ {
			planetSystem[system.PlanetStart+offset] = systemIndex
		}
	}

	planets := make([]Planet, 0, planetRecordCount)
	cityCursor := 0
	for index, run := range grouping.PlanetCityRuns {
		record := planetRecords[index]
		if run.Planet != record.Name {
			return WorldData{}, fmt.Errorf("planet run %d names %q, table names %q", index, run.Planet, record.Name)
		}
		// A planet with no orbit, or one sitting on its own star, would make the
		// in-system projection meaningless rather than merely odd.
		if record.Orbit == nil || record.Orbit.Radius <= 0 {
			return WorldData{}, fmt.Errorf("planet %q has no usable orbit radius", record.Name)
		}
		if record.Orbit.Angle < 0 || record.Orbit.Angle > 0xFF {
			return WorldData{}, fmt.Errorf("planet %q has orbit angle %d, outside a byte", record.Name, record.Orbit.Angle)
		}
		// The decoded rates run 0..8. A rate outside that is a decoder fault,
		// not a planet that happens to move quickly.
		if record.OrbitRate < 0 || record.OrbitRate > 0xFF {
			return WorldData{}, fmt.Errorf("planet %q has orbit rate %d, outside a byte", record.Name, record.OrbitRate)
		}
		orbit := Orbit{Radius: record.Orbit.Radius, Angle: record.Orbit.Angle}
		if run.Start != cityCursor {
			return WorldData{}, fmt.Errorf("planet %q starts at city %d, want %d", run.Planet, run.Start, cityCursor)
		}
		if run.Count <= 0 {
			return WorldData{}, fmt.Errorf("planet %q owns %d cities", run.Planet, run.Count)
		}
		systemIndex := planetSystem[index]
		if run.System != systems[systemIndex].Name {
			return WorldData{}, fmt.Errorf("planet %q maps to system %q, table groups it under %q", run.Planet, run.System, systems[systemIndex].Name)
		}
		planets = append(planets, Planet{
			Index:       index,
			ID:          slug(record.Name),
			Name:        record.Name,
			SystemIndex: systemIndex,
			RawOffset:   record.offset,
			TailHex:     record.TailHex,
			Orbit:       orbit,
			OrbitRate:   record.OrbitRate,
			CityStart:   run.Start,
			CityCount:   run.Count,
		})
		if record.LandingSquare != nil {
			planets[len(planets)-1].LandingColumn = record.LandingSquare.Column
			planets[len(planets)-1].LandingRow = record.LandingSquare.Row
		}
		cityCursor += run.Count
	}
	if cityCursor != grouping.ContiguousCityCount {
		return WorldData{}, fmt.Errorf("planet runs cover %d cities, declared %d", cityCursor, grouping.ContiguousCityCount)
	}

	planetByName := make(map[string]int, len(planets))
	for index, planet := range planets {
		planetByName[planet.Name] = index
	}

	cities := make([]City, 0, cityRecordCount)
	for _, planet := range planets {
		for offset := 0; offset < planet.CityCount; offset++ {
			record := cityRecords[planet.CityStart+offset]
			if record.Name == fillerCityName {
				return WorldData{}, fmt.Errorf("filler record %d appears inside a planet run", record.Index)
			}
			position, err := cityPosition(record.rawRecord)
			if err != nil {
				return WorldData{}, err
			}
			cities = append(cities, City{
				Index:       record.Index,
				ID:          slug(record.Name),
				Name:        record.Name,
				PlanetIndex: planet.Index,
				RawOffset:   record.offset,
				TailHex:     record.TailHex,
				MapPosition: position,
			})
		}
	}

	if len(grouping.TrailingCities) != grouping.TrailingCityCount {
		return WorldData{}, fmt.Errorf("world data declares %d trailing cities and lists %d", grouping.TrailingCityCount, len(grouping.TrailingCities))
	}
	trailingByIndex := make(map[int]rawTrailingCity, len(grouping.TrailingCities))
	for _, entry := range grouping.TrailingCities {
		trailingByIndex[entry.Index] = entry
	}

	fillerSeen := 0
	for _, record := range cityRecords[cityCursor:] {
		if record.Name == fillerCityName {
			fillerSeen++
			continue
		}
		entry, ok := trailingByIndex[record.Index]
		if !ok {
			return WorldData{}, fmt.Errorf("trailing city %q at index %d is not declared in the grouping", record.Name, record.Index)
		}
		if entry.City != record.Name {
			return WorldData{}, fmt.Errorf("trailing city %d names %q, table names %q", record.Index, entry.City, record.Name)
		}
		planetIndex, ok := planetByName[entry.Planet]
		if !ok {
			return WorldData{}, fmt.Errorf("trailing city %q has unknown planet %q", record.Name, entry.Planet)
		}
		trailingPosition, err := cityPosition(record.rawRecord)
		if err != nil {
			return WorldData{}, err
		}
		cities = append(cities, City{
			Index:       record.Index,
			ID:          slug(record.Name),
			Name:        record.Name,
			MapPosition: trailingPosition,
			PlanetIndex: planetIndex,
			RawOffset:   record.offset,
			TailHex:     record.TailHex,
			Hidden:      true,
		})
	}
	if fillerSeen != grouping.FillerCityCount {
		return WorldData{}, fmt.Errorf("found %d filler city records, declared %d", fillerSeen, grouping.FillerCityCount)
	}
	if got := len(cities) - grouping.ContiguousCityCount; got != grouping.TrailingCityCount {
		return WorldData{}, fmt.Errorf("found %d trailing cities, declared %d", got, grouping.TrailingCityCount)
	}

	seen := make(map[string]struct{}, len(cities))
	for _, city := range cities {
		if _, exists := seen[city.ID]; exists {
			return WorldData{}, fmt.Errorf("duplicate city identity %q", city.ID)
		}
		seen[city.ID] = struct{}{}
	}

	return WorldData{systems: systems, planets: planets, cities: cities}, nil
}

// validatedRecord carries the parsed offset beside the decoded JSON record.
type validatedRecord struct {
	rawRecord
	offset int
}

func validateTable(table rawTable, kind string, recordSize, recordCount int) ([]validatedRecord, error) {
	if table.RecordSize != recordSize {
		return nil, fmt.Errorf("%s table declares record size %d, want %d", kind, table.RecordSize, recordSize)
	}
	if table.RecordCount != recordCount || len(table.Records) != recordCount {
		return nil, fmt.Errorf("%s table declares %d records and carries %d, want %d", kind, table.RecordCount, len(table.Records), recordCount)
	}
	tailBytes := recordSize - nameFieldSize
	records := make([]validatedRecord, 0, recordCount)
	for index, record := range table.Records {
		if record.Index != index {
			return nil, fmt.Errorf("%s record %d declares index %d", kind, index, record.Index)
		}
		if record.Name == "" {
			return nil, fmt.Errorf("%s record %d has an empty name", kind, index)
		}
		if len(record.Name) > nameFieldSize-1 {
			return nil, fmt.Errorf("%s record %d name %q exceeds the %d-byte field", kind, index, record.Name, nameFieldSize-1)
		}
		for _, r := range record.Name {
			if r < 0x20 || r > 0x7E {
				return nil, fmt.Errorf("%s record %d name %q has a non-printable byte", kind, index, record.Name)
			}
		}
		if len(record.TailHex) != tailBytes*2 {
			return nil, fmt.Errorf("%s record %d tail is %d hex chars, want %d", kind, index, len(record.TailHex), tailBytes*2)
		}
		if _, err := hex.DecodeString(record.TailHex); err != nil {
			return nil, fmt.Errorf("%s record %d has an undecodable tail: %w", kind, index, err)
		}
		offset, err := strconv.ParseInt(record.RawOffset, 0, 64)
		if err != nil || offset < 0 {
			return nil, fmt.Errorf("%s record %d has an invalid raw offset %q", kind, index, record.RawOffset)
		}
		records = append(records, validatedRecord{rawRecord: record, offset: int(offset)})
	}
	return records, nil
}

// Systems returns the star systems in source-table order.
func (w WorldData) Systems() []System {
	result := make([]System, len(w.systems))
	copy(result, w.systems)
	return result
}

// Planets returns the planets in source-table order.
func (w WorldData) Planets() []Planet {
	result := make([]Planet, len(w.planets))
	copy(result, w.planets)
	return result
}

// Cities returns the playable cities in source-table order, filler slots
// excluded and the hidden colony last.
func (w WorldData) Cities() []City {
	result := make([]City, len(w.cities))
	copy(result, w.cities)
	return result
}

// slug builds a stable, lower-case identity from a source name. It only affects
// remake-owned identifiers; the source name itself is preserved verbatim.
// cityPosition validates the decoded planet-map position. The pixel is checked
// against the original's own formula rather than trusted: an evidence document
// that drifted from the routine it claims to describe would otherwise reach the
// renderer unnoticed.
func cityPosition(record rawRecord) (CityMapPosition, error) {
	if record.Position == nil || record.PlanetMapPixel == nil {
		return CityMapPosition{}, fmt.Errorf("city %q has no decoded planet-map position", record.Name)
	}
	column, row := record.Position.Column, record.Position.Row
	if column < 0 || column > 0x3F {
		return CityMapPosition{}, fmt.Errorf("city %q has column %d, outside six bits", record.Name, column)
	}
	if row < 0 || row > 0x1F {
		return CityMapPosition{}, fmt.Errorf("city %q has row %d, outside five bits", record.Name, row)
	}
	// change_xy adds 3 "if column" — and the p-machine tests only the low
	// bit, so that is "if column is odd". Checked on the running original.
	x := 7*((column+1)/2) + 4
	if column&1 == 1 {
		x += 3
	}
	y := 4*row + 2
	if record.PlanetMapPixel.X != x || record.PlanetMapPixel.Y != y {
		return CityMapPosition{}, fmt.Errorf(
			"city %q pixel (%d,%d) does not match change_xy of (%d,%d), which gives (%d,%d)",
			record.Name, record.PlanetMapPixel.X, record.PlanetMapPixel.Y, column, row, x, y)
	}
	return CityMapPosition{Column: column, Row: row, X: x, Y: y}, nil
}

func slug(name string) string {
	var builder strings.Builder
	previousDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			previousDash = false
		default:
			if !previousDash {
				builder.WriteByte('-')
				previousDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

// The jump rules below come from the original's own p-code; each is pinned by a
// byte signature in docs/atari-st/ship-rules.json.
const (
	systemCoordinateCount = 3
	systemCoordinateMax   = 0xF
	// distanceAdjustedSystem is the one system SHIPLIB:0x0a dist adds a point
	// of distance for, at either end of a jump. The original indexes systems
	// from one, so this is the third record.
	distanceAdjustedSystem = 2
)

// JumpDistance measures a jump between two systems the way SHIPLIB:0x0a dist
// does: the integer square root of the summed squared coordinate differences,
// plus one when either end is the adjusted system.
func JumpDistance(from, to System) int {
	sum := 0
	for axis := 0; axis < systemCoordinateCount; axis++ {
		delta := from.Coordinates[axis] - to.Coordinates[axis]
		sum += delta * delta
	}
	distance := isqrt(sum)
	if from.Index == distanceAdjustedSystem || to.Index == distanceAdjustedSystem {
		distance++
	}
	return distance
}

// JumpDuration is what a jump of a given distance costs in time: the original
// stores 3 * distance² + 10 as the warp charge.
func JumpDuration(distance int) int { return 3*distance*distance + 10 }

// isqrt is the integer square root the p-machine's own routine computes.
func isqrt(value int) int {
	if value <= 0 {
		return 0
	}
	root := 0
	for (root+1)*(root+1) <= value {
		root++
	}
	return root
}
