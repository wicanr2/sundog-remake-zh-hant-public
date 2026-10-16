package game

// 原版與正式比較入口：docs/atari-st/ship-rules.md，Issue #8。
// Packed words/flags exist only in this oracle comparison, never in State.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fuelEnergyInput struct {
	Group                                                            string
	Phase, Fuel, Current, Target, WarpStatus, Draw, Range, DueMinute int
	Tractor, Cloak, Pending, Battle, Jettison, Attached              bool
	Kinds                                                            [2]int
}

type fuelFlightParentInput struct {
	Phase, Fuel int
	Energy      [3]int
	Kinds       [2]int
	Attached    bool
}
type fuelFlightParentSnapshot struct {
	Phase, Tick, Fuel, Weapon, Shield, Setting, Angle, Radius int
	Words                                                     [2]uint16
	Flags                                                     []uint16
}
type fuelFlightParentRow struct {
	Input fuelFlightParentInput
	Steps []fuelFlightParentSnapshot
}

func TestExportTalosFuelFlightParent(t *testing.T) {
	out := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if out == "" {
		t.Skip("original fuel parent export not enabled")
	}
	exportTalosFuelFlightParent(t, out)
}

func TestFuelFlightParentNativeParity(t *testing.T) {
	raw, err := os.ReadFile("../../docs/atari-st/fuel-flight-parent-parity-20261016.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct{ Native []fuelFlightParentRow }
	if err = json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Native) != 432 {
		t.Fatal("missing native432 continuous fuel inputs")
	}
	out := t.TempDir()
	raw, err = json.Marshal(receipt.Native)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "native-fuel-flight-parent.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	exportTalosFuelFlightParent(t, out)
}

func exportTalosFuelFlightParent(t *testing.T, out string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(out, "native-fuel-flight-parent.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native []fuelFlightParentRow
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native) != 432 {
		t.Fatal("missing native432 fuel parent inputs")
	}
	actual := []fuelFlightParentRow{}
	differences := 0
	for _, n := range native {
		i := n.Input
		s := consumerState(i.Attached, i.Kinds)
		s.Screen = ScreenFlight
		s.Fuel, s.ShipPhase, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = i.Fuel, i.Phase, i.Energy[0], i.Energy[1], i.Energy[2]
		s.ShipControls = ShipControls{Warp: true, Subc: true, Guns: true, Tactical: true, Pilotage: true}
		s.ShieldsActive = true
		s.FlightTicks = 33
		s.FlightFromAngle, s.FlightFromRadius, s.FlightDeltaAngle, s.FlightDeltaRadius = 250, 20, 6, 10
		s.FlightAngle, s.FlightRadius = 250, 20
		for j := range s.ShipSystems {
			system := &s.ShipSystems[j]
			system.Rows = intactRows(system.ID)
			if system.ID == ShipSystemShipGuns || system.ID == ShipSystemForceShields {
				system.Rows[2], system.Rows[3] = make([]int, 4), make([]int, 4)
			}
		}
		normalizeShipSystems(&s)
		if shipSystemStatusByID(s, ShipSystemSubcEngine) != 16 || shipSystemStatusByID(s, ShipSystemShipGuns) != 8 || shipSystemStatusByID(s, ShipSystemForceShields) != 8 {
			t.Fatal("controlled system rows differ")
		}
		r := fuelFlightParentRow{Input: i}
		for step := 0; step < 9; step++ {
			advanceShipClock(&s)
			words, _ := consumerWords(s)
			x := fuelFlightParentSnapshot{Phase: s.ShipPhase, Tick: s.FlightTick, Fuel: s.Fuel, Weapon: s.WeaponCharge, Shield: s.ShieldLevel, Setting: s.ShieldSetting, Angle: s.FlightAngle, Radius: s.FlightRadius, Words: words, Flags: consumerFlags(s)}
			r.Steps = append(r.Steps, x)
			if len(n.Steps) != 9 {
				t.Fatal("missing nine original continuous updates")
			}
			if !reflect.DeepEqual(x, n.Steps[step]) {
				differences++
				if differences <= 5 {
					t.Logf("input%+v step%d original%+v remake%+v", i, step, n.Steps[step], x)
				}
			}
		}
		actual = append(actual, r)
	}
	raw, err = json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "remake-fuel-flight-parent.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if differences != 0 {
		t.Fatalf("continuous fuel parent differences %d/3888", differences)
	}
	t.Log("432 fuel inputs x 9 continuous formal ship callbacks conform")
}

// This comparison consumes the original answers, rather than a second copy
// of the rule contract. Packed fields are reconstructed only at this boundary.
type fuelConsumerInput struct {
	OldFuel, RawFuel, CurrentSpeed, TargetSpeed, Setting int
	Attached                                             bool
	Kinds                                                [2]int
}
type fuelConsumerRow struct {
	Input                      fuelConsumerInput
	Fuel, TargetSpeed, Setting int
	Kinds                      [2]int
	Words                      [2]uint16
	Flags                      []uint16
}
type auxConsumerInput struct {
	Fuel     int
	Attached bool
	Kinds    [2]int
}
type auxConsumerRow struct {
	Input             auxConsumerInput
	Fuel              int
	Words, PriceWords [2]uint16
	LabelEmpty        [2]bool
	Flags             []uint16
}
type levelsConsumerInput struct {
	Phase, Fuel, Weapon, GunStatus, Shield, Setting, ShieldStatus int
	GunsActive, ShieldsActive                                     bool
}
type levelsConsumerRow struct {
	Input                         levelsConsumerInput
	Fuel, Weapon, Shield, Setting int
	Flags                         []uint16
}

type mixedFuelInput struct {
	Levels   levelsConsumerInput
	Scenario int
	Attached bool
	Kinds    [2]int
}
type mixedFuelRow struct {
	Input                                                                  mixedFuelInput
	Fuel, Weapon, Shield, Setting, Current, Target, Draws, Closing, Course int
	Refresh                                                                bool
	Words                                                                  [2]uint16
	Flags                                                                  []uint16
}

func TestExportTalosFuelMixedEnergy(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定原版混合耗能输入")
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-fuel-mixed.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native struct{ Native []mixedFuelRow }
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	var wanted, actual []mixedFuelInput
	for phase := 0; phase < 3; phase++ {
		for _, fuel := range []int{0, 1, 4, 5, 254, 255} {
			for _, weapon := range []int{0, 254, 255} {
				for _, shield := range [][3]int{{0, 255, 8}, {63, 63, 16}, {255, 63, 8}, {0, 0, 0}} {
					for _, attached := range []bool{false, true} {
						for _, kinds := range [][2]int{{0, 0}, {1, 0}, {2, 1}, {1, 1}} {
							for scenario := 0; scenario < 7; scenario++ {
								wanted = append(wanted, mixedFuelInput{levelsConsumerInput{phase, fuel, weapon, 8, shield[0], shield[1], shield[2], true, true}, scenario, attached, kinds})
							}
						}
					}
				}
			}
		}
	}
	for _, row := range native.Native {
		actual = append(actual, row.Input)
	}
	requireConsumerInputs(t, actual, wanted)
	report := struct {
		Scope              string
		Cases, Differences int
		Native             []mixedFuelRow
	}{Scope: "活躍充能與兩種槽位／附件、速度、牽引／隱形、救援或拋艙的同次正式updateShipEnergy對原版完整update_levels。相位已選定，RND受控並核對抽次，非完整父排程或正常GUI。"}
	for index, expected := range native.Native {
		in := expected.Input
		lv := in.Levels
		s := consumerState(in.Attached, in.Kinds)
		s.Fuel, s.ShipPhase, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = lv.Fuel, lv.Phase, lv.Weapon, lv.Shield, lv.Setting
		s.ShipControls.Pilotage = true
		s.ShipControls.Secondary[5] = true
		s.CurrentSpeed, s.TargetSpeed = 0, 0
		s.TractorActive = in.Scenario == 3 || in.Scenario == 4
		s.ShipControls.Secondary[4] = s.TractorActive
		s.FuelRescuePending, s.FuelJettisonPending = in.Scenario == 5, in.Scenario == 6
		s.FuelRescueDue, s.TimeMinutes = 20*60+10, 20*60+10
		s.CombatActive = in.Scenario == 5
		s.CombatRange = 0
		if s.TractorActive {
			s.CombatRange = 255
		}
		draw := 0
		if in.Scenario == 1 || in.Scenario == 2 || in.Scenario == 4 {
			s.CurrentSpeed, s.TargetSpeed = 224, 255
		}
		if in.Scenario == 2 {
			draw = 15
		}
		if in.Scenario == 5 {
			draw = 1
		}
		for n := range s.ShipSystems {
			s.ShipSystems[n].Status = 0
			switch s.ShipSystems[n].ID {
			case ShipSystemWarpDrive:
				s.ShipSystems[n].Status = 8
			case ShipSystemShipGuns:
				s.ShipSystems[n].Status = lv.GunStatus
			case ShipSystemForceShields:
				s.ShipSystems[n].Status = lv.ShieldStatus
			}
		}
		draws := 0
		updateShipEnergy(&s, func(n int) int { draws++; return draw % n })
		words, _ := consumerWords(s)
		got := mixedFuelRow{Input: in, Fuel: s.Fuel, Weapon: s.WeaponCharge, Shield: s.ShieldLevel, Setting: s.ShieldSetting, Current: s.CurrentSpeed, Target: s.TargetSpeed, Draws: draws, Closing: s.EnemyClosing, Course: s.EnemyCourseMode, Refresh: s.ShipRefresh, Words: words, Flags: consumerFlags(s)}
		if !reflect.DeepEqual(expected, got) {
			report.Differences++
			if report.Differences <= 5 {
				t.Logf("case%d original=%+v remake=%+v", index, expected, got)
			}
		}
		report.Cases++
		report.Native = append(report.Native, got)
	}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-fuel-mixed.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 {
		t.Fatalf("正式混合耗能差異%d/%d", report.Differences, report.Cases)
	}
}

func consumerState(attached bool, kinds [2]int) State {
	s := NewState()
	s.CargoAttached = attached
	s.ShipControls = ShipControls{Warp: true, Subc: true, Guns: true, Tactical: true, Secondary: [6]bool{true, true, true, true, true, false}}
	s.ShieldsActive = true
	for i, kind := range kinds {
		s.Holds[i] = Hold{Quality: 1, SourceCode: 0xab, ItemKey: "oracle-label"}
		switch kind {
		case 0:
			s.Holds[i].Kind = HoldEmpty
		case 1:
			s.Holds[i].Kind = HoldAuxFuel
		default:
			s.Holds[i].Kind, s.Holds[i].Goods = HoldGoods, kind
		}
	}
	return s
}
func consumerFlags(s State) []uint16 {
	flags := make([]uint16, 5)
	flag := func(n int, on bool) {
		if on {
			flags[n/16] |= 1 << uint(n%16)
		}
	}
	for i, id := range []ShipSystemID{ShipSystemWarpDrive, ShipSystemSubcEngine, ShipSystemShipGuns, ShipSystemTacticalDisplay, ShipSystemForceShields, ShipSystemPilotage} {
		flag(i, shipSystemActive(s, id))
		flag(i+8, s.ShipControls.Secondary[i])
	}
	flag(7, s.CargoAttached)
	flag(25, s.FuelRescuePending)
	flag(37, s.TractorActive)
	flag(70, s.FuelJettisonPending)
	return flags
}
func consumerWords(s State) (words [2]uint16, kinds [2]int) {
	for i, h := range s.Holds {
		kind := h.Goods
		if h.Kind == HoldEmpty {
			kind = 0
		} else if h.Kind == HoldAuxFuel {
			kind = 1
		}
		kinds[i] = kind
		words[i] = uint16(kind) | uint16(h.Quality)<<5 | uint16(h.SourceCode)<<8
	}
	return
}

// Validate the complete Cartesian input set, not just the reported row count.
func requireConsumerInputs[I comparable](t *testing.T, actual, wanted []I) {
	t.Helper()
	remaining := make(map[I]bool, len(wanted))
	for _, in := range wanted {
		remaining[in] = true
	}
	if len(actual) != len(wanted) || len(remaining) != len(wanted) {
		t.Fatal("原版案例全集案數不同")
	}
	for _, in := range actual {
		if !remaining[in] {
			t.Fatalf("原版缺案、重複或額外輸入：%+v", in)
		}
		delete(remaining, in)
	}
	if len(remaining) != 0 {
		t.Fatal("原版輸入缺案")
	}
}

func TestExportTalosFuelConsumers(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定原版補油／充能輸入")
	}
	read := func(name string, target any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	var fuel struct{ Native []fuelConsumerRow }
	var aux struct {
		Native []struct {
			Input             auxConsumerInput
			Fuel              int
			Words, PriceWords [2]uint16
			Labels            [2][16]byte
			Flags             []uint16
		}
	}
	var levels struct{ CoreNative []levelsConsumerRow }
	read("native-fuel.json", &fuel)
	read("native-aux-pod.json", &aux)
	read("native-levels.json", &levels)
	var wantedFuel, actualFuel []fuelConsumerInput
	for _, old := range []int{0, 1, 5} {
		for _, raw := range []int{-6, -1, 0, 1, 4, 5} {
			for _, speed := range [][2]int{{0, 0}, {0, 1}, {1, 0}} {
				for _, attached := range []bool{false, true} {
					for _, a := range []int{0, 1, 2, 31} {
						for _, b := range []int{0, 1, 2, 31} {
							wantedFuel = append(wantedFuel, fuelConsumerInput{old, raw, speed[0], speed[1], 100, attached, [2]int{a, b}})
						}
					}
				}
			}
		}
	}
	for _, row := range fuel.Native {
		actualFuel = append(actualFuel, row.Input)
	}
	requireConsumerInputs(t, actualFuel, wantedFuel)
	var wantedAux, actualAux []auxConsumerInput
	for _, f := range []int{-6, -1, 0, 1, 4, 5, 6, 100, 254, 255} {
		for _, attached := range []bool{false, true} {
			for _, a := range []int{0, 1, 2, 31} {
				for _, b := range []int{0, 1, 2, 31} {
					wantedAux = append(wantedAux, auxConsumerInput{f, attached, [2]int{a, b}})
				}
			}
		}
	}
	for _, row := range aux.Native {
		actualAux = append(actualAux, row.Input)
	}
	requireConsumerInputs(t, actualAux, wantedAux)
	var wantedLevels, actualLevels []levelsConsumerInput
	for phase := 0; phase < 3; phase++ {
		for _, f := range []int{0, 1, 5, 255} {
			for _, gun := range []int{0, 1, 16} {
				for _, weapon := range []int{0, 1, 254, 255} {
					for _, active := range []bool{false, true} {
						for _, shield := range [][3]int{{0, 0, 0}, {0, 255, 16}, {255, 63, 8}, {63, 63, 0}} {
							wantedLevels = append(wantedLevels, levelsConsumerInput{phase, f, weapon, gun, shield[0], shield[1], shield[2], active, true})
						}
					}
				}
			}
		}
	}
	for _, row := range levels.CoreNative {
		actualLevels = append(actualLevels, row.Input)
	}
	requireConsumerInputs(t, actualLevels, wantedLevels)
	report := struct {
		Scope                                                                            string
		FuelCases, FuelDifferences, AuxCases, AuxDifferences, CoreCases, CoreDifferences int
		Fuel                                                                             []fuelConsumerRow
		Aux                                                                              []auxConsumerRow
		Core                                                                             []levelsConsumerRow
	}{Scope: "同原版輸入的兩種補油完整矩陣及活躍武器／護盾能量核心。Aux比較typed空標籤語意、完整metadata與價格，不把Pascal字串尾端bytes放進runtime。Core無備用油／特殊耗能／速度差；不是混合父排程或正常GUI。"}
	difference := func(group string, index int, expected, got any, count *int) {
		if !reflect.DeepEqual(expected, got) {
			(*count)++
			if *count <= 4 {
				t.Logf("%s case%d original=%+v remake=%+v", group, index, expected, got)
			}
		}
	}
	for i, expected := range fuel.Native {
		in := expected.Input
		s := consumerState(in.Attached, in.Kinds)
		s.Fuel, s.CurrentSpeed, s.TargetSpeed, s.ShieldSetting = in.RawFuel, in.CurrentSpeed, in.TargetSpeed, in.Setting
		checkShipFuel(&s)
		closeShipSystemsAfterFuelExhaustion(&s, in.OldFuel)
		words, kinds := consumerWords(s)
		got := fuelConsumerRow{in, s.Fuel, s.TargetSpeed, s.ShieldSetting, kinds, words, consumerFlags(s)}
		difference("fuel", i, expected, got, &report.FuelDifferences)
		report.Fuel = append(report.Fuel, got)
	}
	for i, native := range aux.Native {
		in := native.Input
		s := consumerState(in.Attached, in.Kinds)
		s.Fuel = in.Fuel
		s.Holds[0].Price, s.Holds[1].Price = 0x1357, 0x2468
		drainAuxFuelTankBelow(&s, auxPodRefillThreshold)
		words, _ := consumerWords(s)
		got := auxConsumerRow{Input: in, Fuel: s.Fuel, Words: words, Flags: consumerFlags(s)}
		expected := auxConsumerRow{Input: in, Fuel: native.Fuel, Words: native.Words, PriceWords: native.PriceWords, Flags: native.Flags}
		for n, h := range s.Holds {
			got.PriceWords[n] = uint16(h.Price)
			got.LabelEmpty[n] = h.ItemKey == ""
			expected.LabelEmpty[n] = native.Labels[n][0] == 0
		}
		difference("aux", i, expected, got, &report.AuxDifferences)
		report.Aux = append(report.Aux, got)
	}
	for i, expected := range levels.CoreNative {
		in := expected.Input
		s := consumerState(false, [2]int{})
		s.Fuel, s.ShipPhase, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = in.Fuel, in.Phase, in.Weapon, in.Shield, in.Setting
		s.ShipControls.Guns, s.ShieldsActive, s.ShipControls.Secondary[4] = in.GunsActive, in.ShieldsActive, false
		s.CurrentSpeed, s.TargetSpeed = 0, 0
		for n := range s.ShipSystems {
			s.ShipSystems[n].Status = 0
			switch s.ShipSystems[n].ID {
			case ShipSystemShipGuns:
				s.ShipSystems[n].Status = in.GunStatus
			case ShipSystemForceShields:
				s.ShipSystems[n].Status = in.ShieldStatus
			}
		}
		updateShipEnergy(&s, func(n int) int { t.Fatalf("核心出現未納入亂數呼叫 %d", n); return 0 })
		got := levelsConsumerRow{in, s.Fuel, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting, consumerFlags(s)}
		difference("core", i, expected, got, &report.CoreDifferences)
		report.Core = append(report.Core, got)
	}
	report.FuelCases, report.AuxCases, report.CoreCases = len(report.Fuel), len(report.Aux), len(report.Core)
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-fuel-consumers.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.FuelDifferences+report.AuxDifferences+report.CoreDifferences != 0 {
		t.Fatal(fmt.Sprintf("補油／充能差異 %d/%d/%d", report.FuelDifferences, report.AuxDifferences, report.CoreDifferences))
	}
}

type fuelEnergyRow struct {
	Input                                         fuelEnergyInput
	Fuel, Current, Target, Draws, Closing, Course int
	Refresh                                       bool
	Words                                         [2]uint16
	Flags                                         []uint16
}

func TestExportTalosFuelEnergy(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定原版耗能對拍輸入")
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-fuel-energy.json"))
	if err != nil {
		t.Fatal(err)
	}
	var native struct{ Native []fuelEnergyRow }
	if err := json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Native) != 2194 {
		t.Fatal("原版耗能案數不同")
	}
	report := struct {
		Scope              string
		Cases, Differences int
		Groups             map[string]int
		Native             []fuelEnergyRow
	}{Scope: "正式 typed 耗能核心對相同原版輸入；相位已選定、武器／護盾0、原始Draw受控，非混合充能、完整父排程或正常GUI。packed flags僅供測試比較。", Groups: map[string]int{}}
	for index, expected := range native.Native {
		in := expected.Input
		s := NewState()
		s.Fuel, s.ShipPhase = in.Fuel, in.Phase
		s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = 0, 0, 0
		s.CurrentSpeed, s.TargetSpeed = in.Current, in.Target
		s.ShipControls = ShipControls{Warp: true, Subc: true, Guns: true, Tactical: true, Pilotage: true, Secondary: [6]bool{true, true, true, true, in.Cloak, true}}
		s.ShieldsActive = true
		for i := range s.ShipSystems {
			s.ShipSystems[i].Status = 0
			if s.ShipSystems[i].ID == ShipSystemWarpDrive {
				s.ShipSystems[i].Status = in.WarpStatus
			}
		}
		s.TractorActive, s.CombatRange = in.Tractor, in.Range
		s.FuelRescuePending, s.FuelRescueDue = in.Pending, 20*60+in.DueMinute
		s.TimeMinutes = 20*60 + 10
		s.CombatActive, s.FuelJettisonPending, s.CargoAttached = in.Battle, in.Jettison, in.Attached
		s.EnemyCourseMode, s.EnemyClosing = 0, 0
		for i, kind := range in.Kinds {
			s.Holds[i] = Hold{Quality: 1, SourceCode: 0xab}
			switch kind {
			case 0:
				s.Holds[i].Kind = HoldEmpty
			case 1:
				s.Holds[i].Kind = HoldAuxFuel
			default:
				s.Holds[i].Kind, s.Holds[i].Goods = HoldGoods, kind
			}
		}
		draws := 0
		updateShipEnergy(&s, func(n int) int { draws++; return in.Draw % n })
		got := fuelEnergyRow{Input: in, Fuel: s.Fuel, Current: s.CurrentSpeed, Target: s.TargetSpeed, Draws: draws, Closing: s.EnemyClosing, Course: s.EnemyCourseMode, Refresh: s.ShipRefresh, Flags: make([]uint16, 5)}
		flag := func(n int, on bool) {
			if on {
				got.Flags[n/16] |= 1 << uint(n%16)
			}
		}
		for i, id := range []ShipSystemID{ShipSystemWarpDrive, ShipSystemSubcEngine, ShipSystemShipGuns, ShipSystemTacticalDisplay, ShipSystemForceShields, ShipSystemPilotage} {
			flag(i, shipSystemActive(s, id))
			flag(i+8, s.ShipControls.Secondary[i])
		}
		flag(7, s.CargoAttached)
		flag(25, s.FuelRescuePending)
		flag(37, s.TractorActive)
		flag(70, s.FuelJettisonPending)
		for i, h := range s.Holds {
			kind := h.Goods
			if h.Kind == HoldEmpty {
				kind = 0
			} else if h.Kind == HoldAuxFuel {
				kind = 1
			}
			got.Words[i] = uint16(kind) | uint16(h.Quality)<<5 | uint16(h.SourceCode)<<8
		}
		if !reflect.DeepEqual(expected, got) {
			report.Differences++
			if report.Differences <= 6 {
				t.Logf("case %d: original=%+v remake=%+v", index, expected, got)
			}
		}
		report.Cases++
		report.Groups[in.Group]++
		report.Native = append(report.Native, got)
	}
	raw, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "remake-fuel-energy.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 {
		t.Fatalf("正式耗能差異 %d / %d", report.Differences, report.Cases)
	}
}
