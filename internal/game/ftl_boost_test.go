package game

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFTLBoostNativeMatrix(t *testing.T) {
	file := os.Getenv("SUNDOG_FTL_NATIVE")
	if file == "" {
		t.Skip("原版矩陣由 tools/verify_talos_ftl_boost.py 重生後提供")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Cases, Differences int
		Native             []struct{ Phase, Range, ShipCity, Bank, Cash, Menu, Price, ResultRange, ResultBank, ResultCash int }
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if native.Cases != 400 || native.Differences != 0 || len(native.Native) != 400 {
		t.Fatal("原版矩陣未通過")
	}
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range native.Native {
		s := NewState()
		s.Screen = ScreenFTLBoost
		s.CityID = "banville"
		s.SystemID = "jondd"
		s.ShipCityID = "other"
		if row.ShipCity == 60 {
			s.ShipCityID = "banville"
		}
		s.Colony.Phase = row.Phase
		s.JumpRange = row.Range
		s.Bank = row.Bank
		s.Cash = row.Cash
		s.BankBalances = map[string]int{"jondd": row.Bank}
		if FTLBoostPrice(s) != row.Price {
			t.Fatalf("case%d price", i)
		}
		action := ActionBack
		if row.Menu == 1 {
			action = ActionInstallFTLBoost
		}
		s, err = r.Step(s, action)
		if err != nil {
			t.Fatal(err)
		}
		if s.JumpRange != row.ResultRange || s.Bank != row.ResultBank || s.Cash != row.ResultCash || s.BankBalances["jondd"] != s.Bank {
			t.Fatalf("case%d remake range%d bank%d cash%d original %+v", i, s.JumpRange, s.Bank, s.Cash, row)
		}
	}
}

func TestFTLBoostNeedsQuoteAndCannotChargeTwice(t *testing.T) {
	r, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := NewState()
	if _, err = r.Step(s, ActionInstallFTLBoost); err == nil {
		t.Fatal("installation without quote")
	}
	if _, err = r.Step(s, ActionOpenFTLBoost); err == nil {
		t.Fatal("engineering away from depot")
	}
	s.Screen = ScreenFTLBoost
	s.CityID = "banville"
	s.ShipCityID = "banville"
	s.Colony.Phase = 8
	s.Bank = 0
	s.Cash = 200000
	s, err = r.Step(s, ActionInstallFTLBoost)
	if err != nil {
		t.Fatal(err)
	}
	if s.JumpRange != 9 || s.Cash != 0 {
		t.Fatal("installation", s.JumpRange, s.Cash)
	}
	s, err = r.Step(s, ActionInstallFTLBoost)
	if err != nil || s.JumpRange != 9 || s.Cash != 0 {
		t.Fatal("charged twice", err)
	}
	if s.MessageKey != "ui.message.ftl_insufficient" {
		t.Fatal("original checks money before the existing boost", s.MessageKey)
	}
	s.ShipCityID = "other"
	s, err = r.Step(s, ActionInstallFTLBoost)
	if err != nil || s.MessageKey != "ui.message.ftl_ship_absent" {
		t.Fatal("original checks ship presence before money", err, s.MessageKey)
	}
}
