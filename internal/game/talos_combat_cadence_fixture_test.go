package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// Actual schema45 parent, including both initial readiness states. Original
// display requests and original RNG words remain outside the typed State.
func TestExportTalosCombatContinuousParent(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("continuous combat research not enabled")
	}
	initialShield := 255
	if value := os.Getenv("SUNDOG_COMBAT_INITIAL_SHIELD"); value != "" {
		var err error
		initialShield, err = strconv.Atoi(value)
		if err != nil || (initialShield != 0 && initialShield != 1 && initialShield != 7 && initialShield != 255) {
			t.Fatal("unsupported initial shield", value)
		}
	}
	type input struct {
		Enemy, Mode, Phase, Fuel int
		Ready                    bool
	}
	type snapshot struct {
		Phase, Fuel, Weapon, Shield, Setting, Hull                         int
		Cooldown, Range, Bearing, Pitch, BearingDrift, PitchDrift, Counter int
		Timer, Closing, Aux8, Aux9                                         int
		Ready, CanPrepareFire                                              bool
		Active                                                             bool
		Words                                                              [2]uint16
		Systems                                                            [6][18]int
	}
	type row struct {
		Input          input
		InitialSystems [6][18]int
		Steps          []snapshot
	}
	captureSystems := func(s State) (systems [6][18]int) {
		for system, record := range s.ShipSystems {
			for row, parts := range record.Rows {
				for col, part := range parts {
					systems[system][row*4+col] = part
				}
			}
			systems[system][16], systems[system][17] = record.Status, record.LastHitSlot
		}
		return systems
	}
	rows := []row{}
	if EnemyShipCount() != 7 {
		t.Fatal("seven validated source types required")
	}
	for enemy := 0; enemy < EnemyShipCount(); enemy++ {
		for mode := 0; mode < 8; mode++ {
			for phase := 0; phase < 3; phase++ {
				for _, fuel := range []int{0, 1, 255} {
					for _, ready := range []bool{false, true} {
						s := consumerState(true, [2]int{1, 1})
						s.Screen = ScreenTactical
						s.ShipControls = ShipControls{Warp: true, Subc: true, Guns: true, Tactical: true, Pilotage: true}
						s.ShieldsActive = true
						s.ShipSystems = IntactShipSystems()
						for n := range s.ShipSystems {
							if n == 2 || n == 4 {
								s.ShipSystems[n].Rows[2] = make([]int, 4)
								s.ShipSystems[n].Rows[3] = make([]int, 4)
							}
							s.ShipSystems[n].LastHitSlot = 0
						}
						normalizeShipSystems(&s)
						if shipSystemStatusByID(s, ShipSystemSubcEngine) != 16 || shipSystemStatusByID(s, ShipSystemShipGuns) != 8 || shipSystemStatusByID(s, ShipSystemForceShields) != 8 {
							t.Fatal("initial part rows/status mismatch")
						}
						s.Fuel, s.ShipPhase, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = fuel, phase, 0, initialShield, 255
						s.HullCondition = 16
						s.FlightTicks = 1000
						s.FlightFromAngle, s.FlightFromRadius, s.FlightAngle, s.FlightRadius = 250, 20, 250, 20
						s.FlightDeltaAngle, s.FlightDeltaRadius = 0, 0
						s.EnemyType, s.EnemyCourseMode, s.EnemyFireCountdown = enemy, mode, 1
						s.CombatRange, s.EnemyHull, s.EnemyMaxHull = 70, 200, 200
						s.EnemyBearing, s.EnemyPitch, s.EnemyDriftBearing, s.EnemyDriftPitch = 512, 512, 3, -3
						s.EnemyClosing = -1
						s.CombatActive = true
						s.CombatRolls = 0
						s.EnemyCourse = EnemyCourseState{Ticks: 1, FireReady: ready, CanPrepareFire: true, Maneuver: [2]int{2, 1}}
						r := row{Input: input{enemy, mode, phase, fuel, ready}, InitialSystems: captureSystems(s)}
						for step := 0; step < 12; step++ {
							advanceShipClock(&s)
							words, _ := consumerWords(s)
							x := snapshot{Phase: s.ShipPhase, Fuel: s.Fuel, Weapon: s.WeaponCharge, Shield: s.ShieldLevel, Setting: s.ShieldSetting,
								Hull: s.HullCondition, Cooldown: s.EnemyFireCountdown, Range: s.CombatRange, Bearing: s.EnemyBearing, Pitch: s.EnemyPitch,
								BearingDrift: s.EnemyDriftBearing, PitchDrift: s.EnemyDriftPitch, Counter: s.CombatRolls, Active: s.CombatActive, Words: words}
							x.Timer, x.Ready, x.Closing, x.Aux8, x.Aux9 = s.EnemyCourse.Ticks, s.EnemyCourse.FireReady, s.EnemyClosing, s.EnemyCourse.Maneuver[0], s.EnemyCourse.Maneuver[1]
							x.CanPrepareFire = s.EnemyCourse.CanPrepareFire
							x.Systems = captureSystems(s)
							r.Steps = append(r.Steps, x)
						}
						rows = append(rows, r)
					}
				}
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-combat-continuous.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("schema45 actual parent: %d inputs x 12 updates; display flags and original RNG remain excluded", len(rows))
}

// Legacy fire projection retained for mode0; it is not the modes1..7 parent.
func TestExportTalosCombatCadence(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定原版排程研究輸出")
	}
	type input struct {
		Enemy, Mode, Timer, Range, Phase, Drift, Seed int
		Flag44, Flag12, Ready                         bool
	}
	type row struct {
		Input          input
		Timer, Counter int
	}
	rows := []row{}
	for enemy := 0; enemy < 7; enemy++ {
		for mode := 0; mode < 8; mode++ {
			for _, timer := range []int{-1, 1, 10} {
				for _, distance := range []int{39, 69, 79, 80, 100} {
					for phase := 0; phase < 3; phase++ {
						for _, seed := range []int{0, 4} {
							in := input{enemy, mode, timer, distance, phase, []int{-4, -3, 3, 4}[(mode+phase+seed)%4], seed, true, false, false}
							s := State{EnemyType: enemy, EnemyCourseMode: mode, EnemyFireCountdown: timer,
								CombatRange: distance, EnemyDriftPitch: in.Drift, CombatRolls: seed,
								EnemyHull: 200, EnemyMaxHull: 200, EnemyBearing: aimCentre, EnemyPitch: aimCentre,
								ShieldLevel: 300}
							resolveEnemyFire(&s)
							rows = append(rows, row{in, s.EnemyFireCountdown, s.CombatRolls})
						}
					}
				}
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-cadence.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("匯出現行正式開火入口 %d 案；僅投影冷卻及 counter，不冒充完整同狀態", len(rows))
}

// 兩個既有正式helper的隔離組合，僅投影可比的漂移／接近率與抽值。
// 不宣稱這個呼叫順序就是現行或原版的正常玩家更新順序。
func TestExportTalosCombatCourse(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定原版航向研究輸出")
	}
	type input struct {
		Mode, Range, BearingDrift, PitchDrift, Aux8, Aux9, Closing, Phase, Hull, Seed int
		HelperLocal2, HelperLocal3                                                    int
		Flag12                                                                        bool
	}
	type row struct {
		Input                                      input
		BearingDrift, PitchDrift, Closing, Counter int
	}
	configs := [][5]int{{0, 0, 0, 0, 0}, {4, 4, 1, 1, -3}, {-4, -4, -1, -1, 3}, {5, -5, 2, 1, 1}, {-5, 5, 1, -1, -1}, {3, -3, 10, 0, 0}}
	rows := []row{}
	for mode := 0; mode < 8; mode++ {
		for _, distance := range []int{10, 39, 40, 49, 50, 59, 60, 69, 70, 79, 80, 89, 90, 99, 100, 101, 128} {
			for ci, c := range configs {
				for _, seed := range []int{0, 1, 17, 41} {
					in := input{mode, distance, c[0], c[1], c[2], c[3], c[4], 1 + (mode+distance+ci+seed)%2, 200, seed, 45 + ci, 3 + seed%2, false}
					s := State{EnemyCourseMode: mode, CombatRange: distance, EnemyDriftBearing: c[0], EnemyDriftPitch: c[1], CombatRolls: seed, EnemyHull: 200}
					setEnemyDrift(&s)
					closing := setEnemyCourse(&s)
					rows = append(rows, row{in, s.EnemyDriftBearing, s.EnemyDriftPitch, closing, s.CombatRolls})
				}
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-course.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("隔離正式航向helper投影 %d 案；不是正常玩家路徑", len(rows))
}

func TestExportTalosCombatCourseBounds(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定原版接近率驗證輸出")
	}
	type row struct{ Mode, Range, Seed, Closing, Counter int }
	rows := []row{}
	for _, mode := range []int{4, 5} {
		distances := []int{40, 60, 80}
		if mode == 5 {
			distances = []int{40, 60, 70}
		}
		for _, distance := range distances {
			for seed := 0; seed < 32; seed++ {
				s := State{EnemyCourseMode: mode, CombatRange: distance, CombatRolls: seed}
				closing := setEnemyCourse(&s)
				rows = append(rows, row{mode, distance, seed, closing, s.CombatRolls})
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-course-bounds.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式接近率helper %d 案，等待原版獨立比較", len(rows))
}

func TestTalosCourseBoundsReferenceVectors(t *testing.T) {
	// 原生 before-r2 的實際答案；包含舊[-2,-1]漏掉的遠離／不變結果。
	for _, r := range []struct{ mode, distance, seed, closing int }{{4, 40, 3, 1}, {5, 70, 3, 1}, {4, 60, 5, 0}, {5, 60, 5, 0}} {
		s := State{EnemyCourseMode: r.mode, CombatRange: r.distance, CombatRolls: r.seed}
		if got := setEnemyCourse(&s); got != r.closing || s.CombatRolls != r.seed+1 {
			t.Fatalf("原版接近率向量 %+v：closing=%d counter=%d", r, got, s.CombatRolls)
		}
	}
}

func TestExportTalosCombatInit(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定遭遇初始化研究輸出")
	}
	type input struct{ System, Rating, Phase, BearingDrift, PitchDrift, Aux8, Aux9, Seed int }
	type row struct {
		Input                                                                                 input
		Enemy, Mode, Range, Hull, Bearing, Pitch, BearingDrift, PitchDrift, Cooldown, Counter int
	}
	rows := []row{}
	// 固定十二個來源星系設定；Phase/Aux僅供原版比較，現行State尚未有完整對應。
	for system, rating := range []int{12, 8, 1, 9, 10, 14, 15, 6, 4, 1, 12, 13} {
		for phase := 0; phase < 3; phase++ {
			for _, config := range [][4]int{{0, 0, 0, 0}, {3, -3, 2, 1}} {
				for seed := 0; seed < 32; seed++ {
					in := input{system, rating, phase, config[0], config[1], config[2], config[3], seed}
					s := State{EnemyDriftBearing: in.BearingDrift, EnemyDriftPitch: in.PitchDrift, CombatRolls: seed}
					beginEncounter(&s, rating)
					rows = append(rows, row{in, s.EnemyType, s.EnemyCourseMode, s.CombatRange, s.EnemyHull, s.EnemyBearing, s.EnemyPitch, s.EnemyDriftBearing, s.EnemyDriftPitch, s.EnemyFireCountdown, s.CombatRolls})
				}
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-init.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式遭遇初始化投影 %d 案；不包含原版遭遇閘門或全機時間", len(rows))
	type positionRow struct{ Enemy, Seed, Bearing, Pitch, Hull, Counter int }
	positions := []positionRow{}
	for enemy := 0; enemy < 7; enemy++ {
		for seed := 0; seed < 32; seed++ {
			s := State{EnemyType: enemy, CombatRolls: seed}
			initializeEnemyPositionAndHull(&s)
			positions = append(positions, positionRow{enemy, seed, s.EnemyBearing, s.EnemyPitch, s.EnemyHull, s.CombatRolls})
		}
	}
	raw, err = json.MarshalIndent(positions, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-init-position.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestEncounterInitialCooldownMatchesOriginalRule(t *testing.T) {
	data, err := atarist.EmbeddedRuleData()
	if err != nil {
		t.Fatal(err)
	}
	value, ok := data.RuleValue("combat.fire-countdown-initial")
	if !ok || value != initialEnemyFireCountdown {
		t.Fatalf("原始typed規則初值=%d/%t，正式初值=%d", value, ok, initialEnemyFireCountdown)
	}
}

func TestExportTalosCombatMode5Drift(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定模式5漂移對拍輸出")
	}
	type row struct{ Bearing, Pitch, Seed, ResultBearing, ResultPitch, Counter int }
	rows := []row{}
	for b := -4; b <= 4; b++ {
		for p := -4; p <= 4; p++ {
			for seed := 0; seed < 32; seed++ {
				s := State{EnemyCourseMode: 5, EnemyDriftBearing: b, EnemyDriftPitch: p, CombatRolls: seed}
				setEnemyDrift(&s)
				rows = append(rows, row{b, p, seed, s.EnemyDriftBearing, s.EnemyDriftPitch, s.CombatRolls})
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-mode5-drift.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

// 只匯出現行船艙補油helper的共有輸入，不能據此宣稱附件或排程已整合。
func TestExportTalosCombatAuxPod(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定備用油對拍輸出")
	}
	type row struct {
		Fuel        int
		Kinds       [2]int
		ResultFuel  int
		ResultKinds [2]int
		Consumed    bool
	}
	rows := []row{}
	for _, fuel := range []int{0, 1, 4, 5, 6, 64, 100} {
		for _, first := range []int{0, 1, 2} {
			for _, second := range []int{0, 1, 2} {
				s := State{Fuel: fuel}
				kinds := [2]int{first, second}
				for i, k := range kinds {
					switch k {
					case 1:
						s.Holds[i] = Hold{Kind: HoldAuxFuel}
					case 2:
						s.Holds[i] = Hold{Kind: HoldGoods, Goods: 2}
					}
				}
				consumed := drainAuxFuelTankBelow(&s, auxPodRefillThreshold)
				result := [2]int{}
				for i, h := range s.Holds {
					switch h.Kind {
					case HoldAuxFuel:
						result[i] = 1
					case HoldGoods:
						result[i] = 2
					}
				}
				rows = append(rows, row{fuel, kinds, s.Fuel, result, consumed})
			}
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-aux-pod.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExportTalosCombatLevelsShield(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定護盾子規則對拍輸出")
	}
	type row struct {
		Level, Setting, Status, Fuel, ResultLevel, ResultSetting, ResultFuel int
		Active                                                               bool
		ResultActive                                                         bool
	}
	rows := []row{}
	for _, level := range []int{0, 1, 62, 63, 64, 65, 98, 99, 100} {
		for _, setting := range []int{0, 1, 62, 63, 64, 65, 98, 99, 100} {
			for _, status := range []int{0, 1, 8, 16} {
				for _, fuel := range []int{0, 1, 2, 5, 64, 100} {
					for _, active := range []bool{false, true} {
						s := State{ShieldLevel: level, ShieldSetting: setting, Fuel: fuel, ShieldsActive: active,
							ShipSystems: []ShipSystem{{ID: ShipSystemForceShields, Status: status}}}
						rechargeShields(&s)
						closeShieldsAfterFuelExhaustion(&s, fuel)
						rows = append(rows, row{level, setting, status, fuel, s.ShieldLevel, s.ShieldSetting, s.Fuel, active, s.ShieldsActive})
					}
				}
			}
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-levels-shield.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

// Compare each saved initial phase. Active energy systems are zero to isolate
// mode2 movement. Intact status makes every movement draw pass, so RNG values
// are immaterial, but its range/count remain observable.
func TestExportTalosCombatMovementFuel(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定航行估油研究輸出")
	}
	type row struct {
		Ticks, Phase, FuelCost                                                 int
		ActualFuelCost, ActualPhase, ActualTick, Updates, Draws, Angle, Radius int
		Travel                                                                 bool
	}
	rows := []row{}
	for _, ticks := range []int{1, 5, 9, 13, 17, 33} {
		for _, phase := range []int{0, 1, 2} {
			s := State{Fuel: 255, ShipPhase: phase, CargoAttached: true, CombatRolls: 17,
				ShipSystems:  []ShipSystem{{ID: ShipSystemSubcEngine, Status: 16}},
				ShipControls: ShipControls{Subc: true}, FlightTicks: ticks,
				FlightFromAngle: 250, FlightFromRadius: 20, FlightDeltaAngle: 6, FlightDeltaRadius: 9,
				FlightAngle: 250, FlightRadius: 20}
			r := row{Ticks: ticks, Phase: phase, FuelCost: routeFuelCost(s, routeLegs{Ticks: ticks, ArcTicks: ticks})}
			for !FlightArrived(s) && r.Updates < ticks*3 {
				advanceInSystemFlight(&s)
				r.Updates++
			}
			if !FlightArrived(s) {
				t.Fatal("intact mode2 trip did not terminate")
			}
			r.ActualFuelCost, r.ActualPhase, r.ActualTick = 255-s.Fuel, s.ShipPhase, s.FlightTick
			r.Draws, r.Angle, r.Radius, r.Travel = int(s.CombatRolls-17), s.FlightAngle, s.FlightRadius, s.ShipControls.Subc
			rows = append(rows, r)
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-movement-fuel.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}

// 比較明示啟用的艦砲一次回充；排除護盾／航行／備用油箱耗能。
func TestExportTalosCombatLevelsWeapon(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定武器能量子規則對拍輸出")
	}
	type row struct{ Charge, Status, Fuel, ResultCharge, ResultFuel int }
	rows := []row{}
	for _, charge := range []int{0, 1, 7, 8, 9, 15, 16, 191, 192, 193, 254, 255} {
		for status := 0; status <= 16; status++ {
			for _, fuel := range []int{0, 1, 2, 5, 64, 100} {
				s := State{WeaponCharge: charge, Fuel: fuel, ShipSystems: []ShipSystem{{ID: ShipSystemShipGuns, Status: status}}}
				rechargeWeapon(&s)
				rows = append(rows, row{charge, status, fuel, s.WeaponCharge, s.Fuel})
			}
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-levels-weapon.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
}
