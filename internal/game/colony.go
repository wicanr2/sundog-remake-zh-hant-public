package game

import (
	"fmt"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// ColonySite 保存行星格位置；城市內部落點與殖民進度各自獨立。
// XMOVEONG:093d..0980、1d9c..1e4d 的 READY 契約見 banville-restoration-spec.md。
type ColonySite struct {
	Pending  bool     `json:"pending"`
	Position Position `json:"position"`
}

func initialColonySite() ColonySite {
	d, ok := geographyRegistry.Destination(world.MissionDestinationID)
	if !ok {
		panic("missing source colony destination")
	}
	return ColonySite{Pending: true, Position: Position{d.MapColumn, d.MapRow}}
}

func colonySiteEligibility() ([960]bool, error) {
	var eligible [960]bool
	surface, err := atarist.PlanetSurfaceAt(0)
	if err != nil {
		return eligible, err
	}
	occupied := [960]bool{}
	cities := 0
	for _, d := range geographyRegistry.Destinations() {
		if d.PlanetID != world.StartPlanetID || d.CityID == world.MissionDestinationID {
			continue
		}
		if d.MapColumn < 0 || d.MapColumn >= 40 || d.MapRow < 0 || d.MapRow >= 24 || occupied[d.MapRow*40+d.MapColumn] {
			return eligible, fmt.Errorf("invalid colony source city position")
		}
		occupied[d.MapRow*40+d.MapColumn] = true
		cities++
	}
	if cities != 5 {
		return eligible, fmt.Errorf("invalid colony source city count: %d", cities)
	}
	count := 0
	for i := range eligible {
		eligible[i] = ColonyTerrainAllowsSite(surface.States[i/40][i%40], surface.Header[4]*256+surface.Header[5] != 0, occupied[i])
		if eligible[i] {
			count++
		}
	}
	if count == 0 {
		return eligible, fmt.Errorf("colony source has no legal site")
	}
	return eligible, nil
}

// PrepareColonySite 只在城市出城後的第一行星世界圖準備時呼叫。
// draw 的受控回傳是原版 MAINLIB RND 的 0..32767，不是候選索引。
// nil 沿用正式已保存亂數；錯誤不提交位置、pending 或亂數狀態。
func PrepareColonySite(state *State, draw func(*State) (int, error)) error {
	if !state.GroundWorld || state.PlanetID != world.StartPlanetID || !state.ColonySite.Pending {
		return nil
	}
	eligible, err := colonySiteEligibility()
	if err != nil {
		return err
	}
	next := *state
	if draw == nil {
		draw = func(s *State) (int, error) { return combatRoll(s, 32768), nil }
	}
	for {
		random, err := draw(&next)
		if err != nil {
			return err
		}
		if random < 0 || random > 32767 {
			return fmt.Errorf("colony random value out of range: %d", random)
		}
		candidate := random % len(eligible)
		if eligible[candidate] {
			next.ColonySite = ColonySite{Position: Position{candidate % 40, candidate / 40}}
			*state = next
			return nil
		}
	}
}

func ValidateColonySiteState(state State) error {
	site := state.ColonySite
	if site.Pending {
		if site != initialColonySite() || state.PlanetID == world.StartPlanetID && state.GroundWorld {
			return fmt.Errorf("invalid pending colony site")
		}
		return nil
	}
	if site.Position.X < 0 || site.Position.X >= 40 || site.Position.Y < 0 || site.Position.Y >= 24 {
		return fmt.Errorf("colony site position out of range")
	}
	eligible, err := colonySiteEligibility()
	if err != nil {
		return err
	}
	if !eligible[site.Position.Y*40+site.Position.X] {
		return fmt.Errorf("illegal colony site")
	}
	return nil
}

// ResolveDestination 覆寫呈現座標，來源 registry 不變。
func ResolveDestination(state State, destination world.Destination) world.Destination {
	if destination.CityID == world.MissionDestinationID {
		p := state.ColonySite.Position
		destination.MapColumn, destination.MapRow = p.X, p.Y
		destination.MapX = 7*((p.X+1)/2) + 4
		if p.X%2 != 0 {
			destination.MapX += 3
		}
		destination.MapY = 4*p.Y + 2
	}
	return destination
}

func (r Reducer) Destination(state State, id string) (world.Destination, bool) {
	d, ok := r.World.Destination(id)
	return ResolveDestination(state, d), ok
}

func (r Reducer) DestinationsOnPlanet(state State, planetID string) []world.Destination {
	list := r.World.DestinationsOnPlanet(planetID)
	for i := range list {
		list[i] = ResolveDestination(state, list[i])
	}
	return list
}

// ColonyProgress 是 remake 的殖民任務狀態，不包含原版 RAM 或 p-system 欄位。
// XDOTRADI:0x06 當期需求清空才換階段；階段 9 走完成分支。
type ColonyProgress struct {
	Initialized bool    `json:"initialized"`
	Phase       int     `json:"phase"`
	NextPhase   int     `json:"nextPhase"`
	Accumulated int     `json:"accumulated"`
	Remaining   [32]int `json:"remaining"`
}

func (c ColonyProgress) Complete() bool { return c.Initialized && c.Phase == 9 }

// ColonyTerrainAllowsSite 與原版外觀轉換後的 13、29、34..42 集合等價。
// 呼叫端提供城市佔用，避免選址規則依賴繪圖器或消耗外觀亂數。
func ColonyTerrainAllowsSite(terrain int, preserveBlank, occupied bool) bool {
	return !occupied && (terrain == 13 || terrain == 0 && !preserveBlank)
}

// CityExtent 回傳原版城市矩形的水平／垂直擴張值，不是圖塊總數。
// XMOVEONG:0x1153 將階段 8 改 9，0x1185 以 (w+1)/2 算垂直擴張。
func (c ColonyProgress) CityExtent() (int, int, error) {
	if c.Phase < 0 || c.Phase > 9 {
		return 0, 0, fmt.Errorf("殖民地階段 %d 越界", c.Phase)
	}
	w := c.Phase
	if w == 8 {
		w = 9
	}
	return w, (w + 1) / 2, nil
}

// AdvanceAtExchange 對應進入交易所與成交後的當期需求檢查。
// 資料載入錯誤時不改狀態；不取亂數，也不依交付總數猜測阶段。
func (c *ColonyProgress) AdvanceAtExchange() (bool, error) {
	if c.Phase < 0 || c.Phase > 9 {
		return false, fmt.Errorf("殖民地階段 %d 越界", c.Phase)
	}
	if c.Complete() {
		return false, nil
	}
	for _, n := range c.Remaining {
		if n != 0 {
			return false, nil
		}
	}
	phases, err := atarist.EmbeddedColonyPhases()
	if err != nil {
		return false, err
	}
	next := 0
	if c.Initialized {
		next = c.NextPhase
	}
	if next < 0 || next > 9 {
		return false, fmt.Errorf("殖民地下一階段 %d 越界", next)
	}
	c.Initialized = true
	c.Phase = next
	c.NextPhase = next + 1
	c.Remaining = [32]int{}
	if next < 9 {
		c.Remaining = phases[next].RequiredCounts
	}
	return true, nil
}

// DeliverLot 只代表一筆已成功成交的貨物；付款與實體貨艙由交易路徑處理。
func (c *ColonyProgress) DeliverLot(goods int) bool {
	if !c.Initialized || c.Complete() || goods <= 0 || goods >= len(c.Remaining) || c.Remaining[goods] <= 0 {
		return false
	}
	c.Remaining[goods]--
	return true
}

func ValidateColonyState(s State) error {
	c := s.Colony
	if c.Accumulated < 0 || c.Accumulated > 32767 {
		return fmt.Errorf("invalid colony accumulator")
	}
	if !c.Initialized {
		if c != (ColonyProgress{}) || s.MissionComplete {
			return fmt.Errorf("invalid uninitialized colony")
		}
		return nil
	}
	if c.Phase < 0 || c.Phase > 9 || c.NextPhase != c.Phase+1 || s.MissionComplete != c.Complete() {
		return fmt.Errorf("invalid colony phase")
	}
	phases, err := atarist.EmbeddedColonyPhases()
	if err != nil {
		return err
	}
	// 原始 byte 基價、3-bit供需／品質與4-bit系統參數給出售價上界1794。
	// 用3000的保守整數界檢查可達新保存，不接受原版未核定的溢位累積。
	phaseForBound := min(c.Phase, 8)
	delivered := 0
	for goods, required := range phases[phaseForBound].RequiredCounts {
		delivered += required - c.Remaining[goods]
	}
	for goods, count := range c.Remaining {
		limit := 0
		if c.Phase < 9 {
			limit = phases[c.Phase].RequiredCounts[goods]
		}
		if count < 0 || count > limit {
			return fmt.Errorf("invalid colony demand %d", goods)
		}
	}
	if c.Accumulated > 1500*delivered {
		return fmt.Errorf("unreachable colony accumulator")
	}
	return nil
}

// XDOTRADI:06；呼叫端只在交易入口或已核准交付後呼叫。
func advanceColonyAtExchange(s *State) {
	changed, err := s.Colony.AdvanceAtExchange()
	if err != nil {
		panic(err)
	}
	if !changed {
		return
	}
	if s.Colony.Complete() {
		s.MissionComplete = true
		s.OutcomeKey, s.MessageKey = "ui.outcome.success", "ui.message.mission_complete"
		s.Screen = ScreenScore
		return
	}
	if s.Colony.Phase > 0 {
		creditTradeFunds(s, Credits(s.Colony.Accumulated))
		s.Colony.Accumulated = 0
	}
}
