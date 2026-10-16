package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// #16 原版基線匯出。一般測試不讀玩家磁碟；原版收據由 verify_talos_food.py 提供。
// 差異留在收據，不把未修正的 prototype 測成 CONFORMED。
func TestExportTalosAttributeClock(t *testing.T) {
	path := os.Getenv("SUNDOG_ATTRIBUTE_NATIVE")
	if path == "" {
		t.Skip("原版屬性時鐘輸入未設定")
	}
	type row struct {
		Current, Target, After               [5]int
		MinuteBefore, MinuteAfter, HourAfter int
	}
	var native struct{ Rows []row }
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &native); err != nil {
		t.Fatal(err)
	}
	report := struct{ Cases, Differences int }{}
	for _, r := range native.Rows {
		s := NewState()
		s.Attributes, s.AttributeMax = r.Current, r.Target
		s.TimeMinutes = r.MinuteBefore
		advanceTime(&s, 1)
		report.Cases++
		if s.Attributes != r.After || s.TimeMinutes != r.HourAfter*60+r.MinuteAfter {
			report.Differences++
		}
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(os.Getenv("SUNDOG_PARITY_OUTPUT"), "remake-food-attribute-clock.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式屬性時鐘比較 %d案／%d差異", report.Cases, report.Differences)
}

func TestExportTalosFoodBaseline(t *testing.T) {
	root := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if root == "" {
		t.Skip("需先以Talos重生原版食物／條件收據")
	}
	var native struct {
		Rows []struct {
			Before, After [4]int
			Asleep        bool
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "native-food-condition.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &native); err != nil || len(native.Rows) == 0 {
		t.Fatalf("原版初值矩陣無效：%v", err)
	}
	type conditionRow struct {
		Before, Native, Remake [4]int
		Asleep                 bool
	}
	report := struct {
		Scope                            string
		ConditionCases, ConditionDiffs   int
		EatCases, EatDiffs               int
		BurgerHeadCases, BurgerHeadDiffs int
		NoHourCases, NoHourDiffs         int
		Examples                         []conditionRow
	}{Scope: "正式advanceConditions同初值／sleepflag；正式ActionEat以拖動中的漢堡byte8；advanceTime只比尚未跨整點的四項讀數。不是正常GUI或睡眠父流程驗收。"}
	for _, row := range native.Rows {
		state := NewState()
		state.Vigor, state.Rest, state.Health, state.Nourishment = row.Before[0], row.Before[1], row.Before[2], row.Before[3]
		advanceConditions(&state, row.Asleep)
		actual := [4]int{state.Vigor, state.Rest, state.Health, state.Nourishment}
		report.ConditionCases++
		if actual != row.After {
			report.ConditionDiffs++
			if len(report.Examples) < 12 {
				report.Examples = append(report.Examples, conditionRow{row.Before, row.After, actual, row.Asleep})
			}
		}
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for nourishment := 0; nourishment < 256; nourishment++ {
		state := NewState()
		state.Screen, state.Nourishment, state.HandItem = ScreenCharacterStatus, nourishment, 8
		after, err := reducer.Step(state, ActionEat)
		if err != nil {
			t.Fatal(err)
		}
		report.EatCases++
		if after.Nourishment != min(255, nourishment+70) {
			report.EatDiffs++
		}
	}
	var burgers struct {
		Rows []struct {
			PackedItem, Target, ItemAfter     int
			ConditionsBefore, ConditionsAfter [4]int
		}
	}
	data, err = os.ReadFile(filepath.Join(root, "native-food-burger.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &burgers); err != nil || len(burgers.Rows) != 4096 {
		t.Fatalf("invalid burger matrix %v", err)
	}
	for _, row := range burgers.Rows {
		if row.Target != 1 {
			continue
		} // Target2 general item-use is not ActionEat.
		s := NewState()
		s.Screen, s.HandItem = ScreenCharacterStatus, row.PackedItem
		s.Vigor, s.Rest, s.Health, s.Nourishment = row.ConditionsBefore[0], row.ConditionsBefore[1], row.ConditionsBefore[2], row.ConditionsBefore[3]
		after, err := reducer.Step(s, ActionEat)
		if err != nil {
			t.Fatal(err)
		}
		report.BurgerHeadCases++
		if ([4]int{after.Vigor, after.Rest, after.Health, after.Nourishment}) != row.ConditionsAfter || after.HandItem != row.ItemAfter || after.TimeMinutes != s.TimeMinutes {
			report.BurgerHeadDiffs++
		}
	}
	if report.BurgerHeadCases != 2048 || report.BurgerHeadDiffs != 0 || report.EatDiffs != 0 {
		t.Fatal("formal burger differs from original")
	}
	for minute := 0; minute < 60; minute++ {
		for _, delta := range []int{0, 1, 5, 10, 59} {
			if minute+delta >= 60 {
				continue
			}
			state := NewState()
			state.TimeMinutes = minute
			before := [4]int{state.Vigor, state.Rest, state.Health, state.Nourishment}
			advanceTime(&state, delta)
			actual := [4]int{state.Vigor, state.Rest, state.Health, state.Nourishment}
			report.NoHourCases++
			if before != actual {
				report.NoHourDiffs++
			}
		}
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "remake-food-baseline.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("未修正式基線：條件%d/%d、進食%d/%d、未跨整點%d/%d差異", report.ConditionDiffs, report.ConditionCases, report.EatDiffs, report.EatCases, report.NoHourDiffs, report.NoHourCases)
}

// Each original row supplies its own initial slots and outputs; no remake
// formula generates the expected result. The native harness controls devices,
// so this is a rule-chain comparison, separately from normal GUI verification.
func TestExportTalosStoresComparison(t *testing.T) {
	root := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if root == "" {
		t.Skip("需Talos Stores原生收據")
	}
	type slotsRow struct {
		Slot, NourishmentBefore, NourishmentAfter, HeldAfter int
		Flow                                                 string
		MiddleStores, StoresBefore, StoresAfter              [20]int
		MiddleCarry, CarryBefore, CarryAfter                 [4]int
	}
	var native struct{ Rows []slotsRow }
	data, err := os.ReadFile(filepath.Join(root, "native-food-stores-chain.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &native); err != nil || len(native.Rows) != 700 {
		t.Fatalf("invalid native matrix: %v", err)
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	report := struct {
		Scope                                 string
		Cases, Differences, MiddleComparisons int
		Rows                                  []slotsRow
	}{Scope: "正式20 Stores／四個body／獨立拖動來源，以相同初態及等價輸入比較原生700鏈；其他三項讀數與分鐘不變。不是原版GUI／正常玩家或睡眠驗收。"}
	for _, row := range native.Rows {
		s := NewState()
		s.Screen, s.SelectedPartIndex = ScreenStores, row.Slot
		s.Stores = append([]int(nil), row.StoresBefore[:]...)
		s.Parts = append([]int(nil), row.CarryBefore[:]...)
		s.Vigor, s.Rest, s.Health, s.Nourishment = 113, 70, 169, row.NourishmentBefore
		s.TimeMinutes = 100
		s, err = r.Step(s, ActionTakeStore)
		if err != nil {
			t.Fatal(err)
		}
		if s.HandItem != 8 || s.Stores[row.Slot] != 0 {
			t.Fatal("source not cleared during drag")
		}
		var middleStores [20]int
		var middleCarry [4]int
		switch row.Flow {
		case "eat":
			s, err = r.Step(s, ActionEat)
		case "cancel":
			s, err = r.Step(s, ActionCancelItem)
		case "occupied1":
			s = r.PickPocketPart(s, 1)
		case "carry0", "carry3":
			pocket := 0
			if row.Flow == "carry3" {
				pocket = 3
			}
			s = r.PickPocketPart(s, pocket)
			copy(middleStores[:], s.Stores)
			copy(middleCarry[:], s.Parts)
			report.MiddleComparisons++
			if middleStores != row.MiddleStores || middleCarry != row.MiddleCarry || s.Nourishment != row.NourishmentBefore || s.HandItem != 0 {
				t.Fatalf("middle transfer differs: %+v", row)
			}
			s = r.PickPocketPart(s, pocket)
			s, err = r.Step(s, ActionEat)
		default:
			t.Fatalf("unknown original flow %q", row.Flow)
		}
		if err != nil {
			t.Fatal(err)
		}
		if row.Flow != "carry0" && row.Flow != "carry3" {
			copy(middleStores[:], s.Stores)
			copy(middleCarry[:], s.Parts)
		}
		actual := slotsRow{Slot: row.Slot, Flow: row.Flow, NourishmentBefore: row.NourishmentBefore, NourishmentAfter: s.Nourishment, HeldAfter: s.HandItem,
			StoresBefore: row.StoresBefore, CarryBefore: row.CarryBefore, MiddleStores: middleStores, MiddleCarry: middleCarry}
		copy(actual.StoresAfter[:], s.Stores)
		copy(actual.CarryAfter[:], s.Parts)
		report.Cases++
		if actual.StoresAfter != row.StoresAfter || actual.CarryAfter != row.CarryAfter || actual.NourishmentAfter != row.NourishmentAfter || actual.HeldAfter != row.HeldAfter || s.HandOrigin != (ItemOrigin{}) || s.Vigor != 113 || s.Rest != 70 || s.Health != 169 || s.TimeMinutes != 100 {
			report.Differences++
		}
		if err := ValidateItemState(s); err != nil {
			t.Fatal(err)
		}
		report.Rows = append(report.Rows, actual)
	}
	data, err = json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "remake-food-stores-chain.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 {
		t.Fatalf("Stores chain differences %d/%d", report.Differences, report.Cases)
	}
	t.Logf("Stores %d cases, %d middle transfers, zero differences", report.Cases, report.MiddleComparisons)
}
