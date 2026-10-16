package game

import (
	"fmt"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// GroundView 是 remake 自己的可攜座標；Anchor 為人物碰撞錨點，不是原版 RAM。
type GroundView struct {
	Valid  bool     `json:"valid"`
	Origin Position `json:"origin"`
	Anchor Position `json:"anchor"`
}

// SyncCargoGroundPosition follows the pod's own movement. Walking leaves the
// detached pod in place; its attachment is never inferred from its contents.
func SyncCargoGroundPosition(state *State) {
	if state.GroundMode == GroundInPod && state.Screen == ScreenGround {
		state.CargoCityID, state.CargoPosition = state.CityID, state.Position
	}
}

func GroundPitch(state State) int {
	if state.GroundWorld {
		return 32
	}
	return 40
}

func groundMod(value, size int) int { return (value%size + size) % size }

func GroundWrap(state State, p Position) Position {
	if state.GroundWorld {
		return Position{groundMod(p.X, 40), groundMod(p.Y, 24)}
	}
	return Position{groundMod(p.X+10, 40) - 10, groundMod(p.Y+6, 24) - 6}
}

func GroundPositionValid(state State, p Position) bool {
	return GroundWrap(state, p) == p
}

func PrimaryCity(cityID string) bool {
	here, ok := geographyRegistry.Destination(cityID)
	if !ok {
		return false
	}
	for _, d := range geographyRegistry.Destinations() {
		if d.PlanetID == here.PlanetID && !d.Hidden {
			return d.CityID == cityID
		}
	}
	return false
}

func GroundCitySize(state State, cityID string) (int, int) {
	if cityID == world.MissionDestinationID {
		w, h, err := state.Colony.CityExtent()
		if err != nil {
			return 0, 0
		}
		return w, h
	}
	return 9, 5
}

// XMOVEONG 的地形／建築 writer 使用嚴格內部矩形，城市格原點為10、6。
// READY 與原生2640案：docs/atari-st/banville-restoration-spec.md。
func GroundCityCellVisible(state State, cityID string, x, y int) bool {
	if cityID != world.MissionDestinationID {
		return true
	}
	w, h, err := state.Colony.CityExtent()
	return err == nil && x > 8-w && x < 11+w && y > 4-h && y < 7+h
}

func GroundTerrainCode(state State, cityID string, x, y int) (int, int, bool) {
	code, number, ok := world.TerrainCode(cityID, x, y)
	if ok && !GroundCityCellVisible(state, cityID, x, y) {
		code = 0
	}
	return code, number, ok
}

func GroundTerrainAt(state State, cityID string, x, y int) (world.Terrain, bool) {
	terrain, ok := world.TerrainAt(cityID, x, y)
	if ok && !GroundCityCellVisible(state, cityID, x, y) {
		terrain = world.TerrainGrass
	}
	return terrain, ok
}

func GroundCityBuildings(state State, cityID string) []atarist.CityBuilding {
	m, ok := world.CityMap(cityID)
	if !ok {
		return nil
	}
	out := m.Buildings()
	visible := out[:0]
	for _, b := range out {
		if GroundCityCellVisible(state, cityID, b.Row, b.Column) {
			visible = append(visible, b)
		}
	}
	return visible
}

func GroundBuildingFacilityAt(state State, position world.TilePosition) (world.Facility, bool) {
	if !GroundCityCellVisible(state, state.CityID, position.X, position.Y) {
		return "", false
	}
	return world.BuildingFacilityAt(state.CityID, position)
}

func (r Reducer) GroundFacilitiesAt(state State, position world.TilePosition) []world.Facility {
	if !GroundCityCellVisible(state, state.CityID, position.X, position.Y) {
		return nil
	}
	list := r.World.FacilitiesAt(state.CityID, position)
	// 原始交易所格不依賴原型設施群。Banville另保留其來源設施與成長遮罩。
	if f, ok := GroundBuildingFacilityAt(state, position); ok && (state.CityID == world.MissionDestinationID || f == world.FacilityExchange) {
		found := false
		for _, existing := range list {
			found = found || existing == f
		}
		if !found {
			list = append(list, f)
		}
		if f == world.FacilityExchange && state.CityID == world.MissionDestinationID {
			// Colony只提供remake交付提示；交付與付款由交易確認處理。
			list = append(list, world.FacilityColony)
		}
	}
	return list
}

func (r Reducer) GroundHasFacility(state State, facility world.Facility) bool {
	for _, f := range r.GroundFacilitiesAt(state, world.TilePosition{X: state.Position.X, Y: state.Position.Y}) {
		if f == facility {
			return true
		}
	}
	return false
}

// GroundLeavesCity 收到的是準備捲動到的新鏡頭原點。
func GroundLeavesCity(state State, origin Position) bool {
	if state.GroundWorld {
		return false
	}
	w, h := GroundCitySize(state, state.CityID)
	return origin.X < 1-w || origin.X > 11+w || origin.Y < -h || origin.Y > 7+h
}

func GroundPlanetIndex(state State) (int, bool) {
	for _, p := range geographyRegistry.Planets() {
		if p.ID == state.PlanetID {
			return p.Index, true
		}
	}
	return 0, false
}

func GroundSurfaceCity(state State, position Position) (world.Destination, bool) {
	for _, d := range geographyRegistry.Destinations() {
		d = ResolveDestination(state, d)
		if d.PlanetID == state.PlanetID && d.MapColumn == position.X && d.MapRow == position.Y {
			return d, true
		}
	}
	return world.Destination{}, false
}

// GroundSurface 回傳來源解碼資料；繪圖外觀及城市覆蓋由呼叫端分開處理。
func GroundSurface(state State) (atarist.PlanetSurface, bool) {
	i, ok := GroundPlanetIndex(state)
	if !ok {
		return atarist.PlanetSurface{}, false
	}
	m, err := atarist.PlanetSurfaceAt(i)
	return m, err == nil
}

func ResetGroundView(state *State, offset Position) {
	left, top := 4, 2
	if state.GroundWorld {
		left, top = 5, 3
	}
	pitch := GroundPitch(*state)
	state.GroundView = GroundView{Valid: true,
		Origin: GroundWrap(*state, Position{state.Position.X - left, state.Position.Y - top}),
		Anchor: Position{left*pitch + offset.X, top*pitch + offset.Y}}
}

// LeaveGroundCity 原版以城市在行星上的位置換圖，船留在原城市。
func LeaveGroundCity(state *State, origin Position) bool {
	left, _ := LeaveGroundCityChecked(state, origin)
	return left
}

// LeaveGroundCityChecked 讓正式輸入端分辨未出界與世界圖準備失敗。
func LeaveGroundCityChecked(state *State, origin Position) (bool, error) {
	if !GroundLeavesCity(*state, origin) {
		return false, nil
	}
	if err := leaveGroundCity(state, origin, nil); err != nil {
		return false, err
	}
	return true, nil
}

func leaveGroundCity(state *State, origin Position, draw func(*State) (int, error)) error {
	if !GroundLeavesCity(*state, origin) {
		return fmt.Errorf("not leaving a city")
	}
	d, ok := geographyRegistry.Destination(state.CityID)
	if !ok {
		return fmt.Errorf("unknown exit city")
	}
	d = ResolveDestination(*state, d)
	original := state
	next := *state
	state = &next
	w, h := GroundCitySize(*state, state.CityID)
	x, y := 171, 116
	if origin.X+10 < 12-w {
		x = 163
	} else if origin.X+10 > 17+w {
		x = 187
	}
	if origin.Y+6 < 7-h {
		y = 107
		if PrimaryCity(state.CityID) {
			y = 102
		}
	} else if origin.Y+6 > 11+h {
		y = 123
		if PrimaryCity(state.CityID) {
			y = 128
		}
	}
	if x == 171 && y == 116 {
		x = 163
	}
	state.GroundWorld = true
	state.Position = Position{d.MapColumn, d.MapRow}
	state.GroundView = GroundView{Valid: true,
		Origin: GroundWrap(*state, Position{state.Position.X - 5, state.Position.Y - 3}), Anchor: Position{x, y}}
	// 原版 y=128 可位於城市格的下一列；保存必須跟實際人物所在格一致。
	state.Position = GroundWrap(*state, Position{state.GroundView.Origin.X + x/32, state.GroundView.Origin.Y + y/32})
	state.MessageKey = "ui.message.left_city"
	if err := PrepareColonySite(state, draw); err != nil {
		return err
	}
	*original = next
	return nil
}

func EnterGroundCity(state *State, offset Position) bool {
	if !state.GroundWorld {
		return false
	}
	d, ok := GroundSurfaceCity(*state, state.Position)
	if !ok {
		return false
	}
	w, h := GroundCitySize(*state, d.CityID)
	left, right, top, bottom := 18-w, 21+w, 10-h, 13+h
	x := left + (right-left+1)*offset.X/32
	y := top + (bottom-top+1)*offset.Y/32
	switch {
	case offset.X < 12:
		x = left
	case offset.Y < 12:
		y = top
	case offset.X > 20:
		x = right
	default:
		y = bottom
	}
	state.CityID = d.CityID
	state.Location = d.Label
	state.GroundWorld = false
	state.Position = Position{x - 10, y - 6}
	ResetGroundView(state, Position{0, 28})
	if !contains(state.VisitedDestinations, d.ID) {
		state.VisitedDestinations = append(append([]string(nil), state.VisitedDestinations...), d.ID)
	}
	state.MessageKey = "ui.message.entered_city"
	return true
}

func BoardGroundShip(state *State, offset Position, podHorizontal bool) bool {
	if state.GroundWorld || state.CityID != state.ShipCityID {
		return false
	}
	// XMOVEONG re_enter_ship：只有載具分支加6，且要求水平圖樣。
	// READY：docs/atari-st/ship-rules.md，下船／登船／門位勘誤。
	centerX, toleranceY := 17, 4
	if state.GroundMode == GroundInPod {
		if !podHorizontal {
			return false
		}
		centerX, toleranceY = 23, 3
	}
	dx := (state.Position.X-state.ShipPosition.X)*40 + offset.X - centerX
	dy := (state.Position.Y-state.ShipPosition.Y)*40 + offset.Y - 20
	if absInt(dx) > 4 || absInt(dy) > toleranceY {
		return false
	}
	if state.GroundMode == GroundInPod {
		state.CargoAttached = true
		state.CargoCityID, state.CargoPosition = state.ShipCityID, state.ShipPosition
		drainAuxFuelTankBelow(state, auxPodRefillThreshold)
	}
	state.GroundMode = GroundOnFoot
	state.SleepTheftProtected = true
	state.Screen = ScreenShipboard
	state.Position = state.ShipPosition
	state.GroundView = GroundView{}
	state.MessageKey = "ui.message.boarded"
	return true
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// GroundScannerFitted 對應 XREADMAP:0x01 讀取的 G53[21]。
func GroundScannerFitted(state State) bool {
	return alternatePartFitted(state, ShipSystemPilotage)
}

// groundLandingPosition 對應 XREADMAP:0x10；首城使用行星記錄的停船格，
// 其他城市在四邊選落點，Banville 固定為原版 raw (19,12)。
func groundLandingPosition(state *State, cityID string) Position {
	if PrimaryCity(cityID) {
		return landingPosition(cityID)
	}
	if cityID == world.MissionDestinationID {
		return Position{9, 6}
	}
	switch combatRoll(state, 4) {
	case 0:
		return Position{rollBetween(state, 10, 30) - 10, -1}
	case 1:
		return Position{21, rollBetween(state, 6, 17) - 6}
	case 2:
		return Position{rollBetween(state, 10, 30) - 10, 12}
	default:
		return Position{-1, rollBetween(state, 6, 17) - 6}
	}
}
