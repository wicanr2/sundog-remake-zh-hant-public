package storage

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
)

func TestActiveCombatSurvivesDisplayReentryAndLibraryReload(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, hold := range []game.Hold{{Kind: game.HoldGoods, Goods: 2}, {}, {Kind: game.HoldAuxFuel}} {
		for _, reload := range []bool{false, true} {
			s := game.NewState()
			s.Screen = game.ScreenShipboard
			s.CombatActive = true
			s.EnemyType = 3
			s.EnemyHull = 166
			s.EnemyMaxHull = 245
			s.CombatRange = 60
			s.EnemyBearing = 515
			s.EnemyPitch = 515
			s.CombatRolls = 139
			s.Holds[0] = hold
			if reload {
				path := filepath.Join(t.TempDir(), "library.json")
				if err := MakeLibrary(path); err != nil {
					t.Fatal(err)
				}
				s.Screen = game.ScreenCharacterLibrary
				if err := StoreGame(path, "1", s); err != nil {
					t.Fatal(err)
				}
				s, err = LoadGame(path, "1")
				if err != nil {
					t.Fatal(err)
				}
			}
			want := s
			want.Screen = game.ScreenTactical
			want.MessageKey = "ui.message.tactical"
			got, err := r.Step(s, game.ActionOpenTactical)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("重入改變遊戲狀態：hold=%+v reload=%v\ngot=%+v\nwant=%+v", hold, reload, got, want)
			}
		}
	}
}

// 明示受控狀態的正式戰術→受損零件→保存→重入整合；不是正常 GUI 或原版全機收據。
func TestOverflowDamageSurvivesSaveAndTacticalReentry(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := game.NewState()
	s.Screen, s.CombatActive = game.ScreenTactical, true
	s.EnemyType, s.EnemyHull, s.EnemyMaxHull, s.CombatRange = 0, 189, 189, 60
	s.CombatRolls, s.EnemyCourseMode, s.EnemyFireCountdown = 10, 2, 0
	s.HullCondition, s.ShieldLevel, s.ShieldSetting, s.ShieldsActive = 16, 0, 0, false
	for i := range s.ShipSystems {
		for row := range s.ShipSystems[i].Rows {
			s.ShipSystems[i].Rows[row] = []int{31, 23, 23, 23}
		}
		s.ShipSystems[i].Status, s.ShipSystems[i].LastHitSlot = 4, 13
	}
	initial := s.ShipSystems
	for turn := 0; turn < 8; turn++ {
		s, err = r.Step(s, game.ActionRechargeWeapon)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s.ShipSystems, initial) {
			break
		}
	}
	if reflect.DeepEqual(s.ShipSystems, initial) || !s.CombatActive {
		t.Fatal("固定正式戰術序列未取得可保存的受損零件狀態")
	}
	for _, system := range s.ShipSystems {
		if system.ID == game.ShipSystemShipGuns && system.Status == 0 && s.WeaponCharge != 0 {
			t.Fatal("砲狀態歸零仍保留充能")
		}
	}
	want := s
	s, err = r.Step(s, game.ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "damaged-combat.json")
	if err = Save(path, s); err != nil {
		t.Fatal(err)
	}
	s, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err = r.Step(s, game.ActionOpenTactical)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.ShipSystems, want.ShipSystems) || s.HullCondition != want.HullCondition ||
		s.WeaponCharge != want.WeaponCharge || s.ShieldLevel != want.ShieldLevel ||
		s.ShieldSetting != want.ShieldSetting || s.CombatRolls != want.CombatRolls {
		t.Fatalf("受損零件或戰鬥狀態未保存：got=%+v want=%+v", s, want)
	}
}

// Talos 執行 XDOFIGHT 的零容差分支；此測試補正式射擊與保存垂直鏈，非正常 GUI 證據。
func TestZeroAimWindowFiringSurvivesSaveReload(t *testing.T) {
	previousSeed := game.NewGameSeed
	game.NewGameSeed = func() int64 { return 1 }
	t.Cleanup(func() { game.NewGameSeed = previousSeed })
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	for _, weapon := range []game.Action{game.ActionFire, game.ActionFireLaser} {
		for _, errorOffset := range []int{0, 1} {
			s := game.NewState()
			// This fixture isolates aim and persistence. A clock-seeded new
			// game can have broken guns, which refuse the shot before aiming.
			s.ShipSystems = game.IntactShipSystems()
			s.Screen = game.ScreenTactical
			s.CombatActive = true
			s.Attributes[game.AttrDexterity] = 1
			s.EnemyType, s.EnemyHull, s.EnemyMaxHull = 1, 200, 200
			s.CombatRange = 127
			s.EnemyBearing, s.EnemyPitch = 512+errorOffset, 512
			s.CombatRolls, s.WeaponCharge = 17, 100
			s.EnemyFireCountdown = 100 // 隔離敵艦當拍命中與傷害，仍執行正式減倒數。
			shot, err := r.Step(s, weapon)
			if err != nil {
				t.Fatal(err)
			}
			if shot.WeaponCharge != 0 {
				t.Fatalf("射擊未耗盡充能：%d", shot.WeaponCharge)
			}
			if hit := shot.EnemyHull < s.EnemyHull; hit != (errorOffset == 0) {
				t.Fatalf("weapon=%v offset=%d hull=%d→%d", weapon, errorOffset, s.EnemyHull, shot.EnemyHull)
			}
			path := filepath.Join(t.TempDir(), "library.json")
			shot.Screen = game.ScreenCharacterLibrary // 正式 F5 保存庫入口會先切到此畫面。
			if err := MakeLibrary(path); err != nil {
				t.Fatal(err)
			}
			if err := StoreGame(path, "1", shot); err != nil {
				t.Fatal(err)
			}
			reloaded, err := LoadGame(path, "1")
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.EnemyHull != shot.EnemyHull || reloaded.WeaponCharge != 0 || reloaded.CombatRolls != shot.CombatRolls {
				t.Fatal("射擊結果或固定亂數計數未保存")
			}
			reentered, err := r.Step(reloaded, game.ActionOpenTactical)
			if err != nil {
				t.Fatal(err)
			}
			if reentered.EnemyHull != shot.EnemyHull || reentered.CombatRolls != shot.CombatRolls {
				t.Fatal("保存重入更換了射擊結果")
			}
		}
	}
}
