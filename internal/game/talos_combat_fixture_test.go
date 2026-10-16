package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 原版直譯器獨立驗證這份正式規則輸出；此測試本身不宣稱原版一致。
func TestExportTalosCombatAim(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定本機原版對拍輸出")
	}
	type row struct {
		DX, Range, Bearing, Pitch int
		Laser                     bool
		Window, Error             int
		Hit                       bool
	}
	rows := []row{}
	for _, dx := range []int{1, 5, 10, 15, 20, 30, 50, 80} {
		for _, distance := range []int{10, 31, 32, 59, 60, 63, 64, 95, 96, 127} {
			for _, laser := range []bool{false, true} {
				for _, delta := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, -1}, {-3, 2}, {10, -10}} {
					state := State{EnemyType: 1, EnemyHull: 200, EnemyMaxHull: 200, WeaponCharge: 100,
						CombatRange: distance, EnemyBearing: aimCentre + delta[0], EnemyPitch: aimCentre + delta[1], CombatRolls: 17}
					state.Attributes[AttrDexterity] = dx
					l, c := aimTolerances(state)
					window, hit := c, aimError(state) <= c
					if laser {
						window = l
						_, hit = playerLaserShot(&state)
					}
					rows = append(rows, row{dx, distance, state.EnemyBearing, state.EnemyPitch, laser, window, aimError(state), hit})
				}
			}
		}
	}
	report := struct {
		Schema            int
		Scope, Randomness string
		Rows              []row
	}{
		1, "正式容差與命中入口；傷害與全戰鬥動態不在比較範圍",
		"瞄準分支無亂數；每案正式雷射傷害前固定 CombatRolls=17，傷害值不比較", rows}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-aim.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式瞄準規則輸出 %d 案，等待原版獨立比較", len(rows))
}

func TestExportTalosCombatSlew(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定本機原版對拍輸出")
	}
	type row struct{ X, Y, Bearing, Pitch int }
	rows := []row{}
	for x := 0; x < 320; x++ {
		b, p := SlewRates(x, 90)
		rows = append(rows, row{x, 90, b, p})
	}
	for y := 0; y < 200; y++ {
		b, p := SlewRates(160, y)
		rows = append(rows, row{160, y, b, p})
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-slew.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("正式完整 X／Y 座標軸 520 案，等待原版獨立比較")
}

// 固定測試種子直接寫入 CombatRolls；原版側以同一測試序列供應外部亂數呼叫。
// 不把這個替代亂數來源當成原版 random() 實作或完整戰鬥對拍。
func TestExportTalosCombatDamage(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定本機原版對拍輸出")
	}
	type row struct {
		Enemy, Range, Charge, Seed, Damage, RollsAfter   int
		InitialHull, HullAfter, ChargeAfter, ActionRolls int
		Victory                                          bool
		Laser, Concentrator                              bool
	}
	rows := []row{}
	for enemy := 0; enemy < EnemyShipCount(); enemy++ {
		for _, distance := range []int{10, 16, 17, 34, 59, 60, 96, 127} {
			for _, charge := range []int{1, 9, 10, 100, 254, 255} {
				for _, laser := range []bool{false, true} {
					for _, concentrator := range []bool{false, true} {
						for _, seed := range []int{0, 1, 17, 41} {
							s := NewState()
							s.EnemyType, s.EnemyHull, s.EnemyMaxHull = enemy, 255, 255
							s.EnemyBearing, s.EnemyPitch = 512, 512
							s.CombatRange, s.WeaponCharge, s.CombatRolls = distance, charge, seed
							s.Screen, s.CombatActive, s.EnemyFireCountdown = ScreenTactical, true, 100
							initialHull := 255
							if charge < 10 {
								initialHull = 1
							} else if charge <= 100 {
								initialHull = 16
							}
							s.EnemyHull = initialHull
							if concentrator {
								part, ok := sourceShipSystem(ShipSystemShipGuns)
								if !ok || part.Alternate == nil {
									t.Fatal("缺少正式 concentrator 定義")
								}
								for i := range s.ShipSystems {
									if s.ShipSystems[i].ID == ShipSystemShipGuns {
										s.ShipSystems[i].Rows[0][part.Alternate.Column] = part.Alternate.ItemIndex
									}
								}
							}
							if alternatePartFitted(s, ShipSystemShipGuns) != concentrator {
								t.Fatal("正式裝配旗標與測試輸入不符")
							}
							damage := 0
							if laser {
								var hit bool
								damage, hit = playerLaserShot(&s)
								if !hit {
									t.Fatal("完全對準的雷射未命中")
								}
							} else {
								damage = playerShotDamage(&s)
							}
							rollsAfter := s.CombatRolls
							s.CombatRolls = seed
							action := ActionFire
							if laser {
								action = ActionFireLaser
							}
							shot, err := reducer.Step(s, action)
							if err != nil {
								t.Fatal(err)
							}
							rows = append(rows, row{enemy, distance, charge, seed, damage, rollsAfter,
								initialHull, shot.EnemyHull, shot.WeaponCharge, shot.CombatRolls, shot.CombatVictory, laser, concentrator})
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
	if err = os.WriteFile(filepath.Join(output, "remake-damage.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式傷害輸出 %d 案，兩側種子與受控亂數方式見收據", len(rows))
}

func TestExportTalosCombatEnemyDamage(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定本機原版對拍輸出")
	}
	type row struct {
		Enemy, Range, Hull, Seed, Damage, RollsAfter int
		Hit                                          bool
	}
	rows := []row{}
	for enemy := 0; enemy < EnemyShipCount(); enemy++ {
		for _, distance := range []int{10, 16, 17, 34, 59, 60, 96, 127} {
			for _, hull := range []int{1, 19, 20, 99, 119, 120, 189, 255} {
				for _, seed := range []int{0, 1, 17, 41} {
					s := State{EnemyType: enemy, CombatRange: distance, EnemyHull: hull, CombatRolls: seed}
					damage, hit := enemyShotDamage(&s)
					rows = append(rows, row{enemy, distance, hull, seed, damage, s.CombatRolls, hit})
				}
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-enemy-damage.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式敵艦命中／傷害輸出 %d 案，種子在執行前固定", len(rows))
}

func TestExportTalosCombatHullDamage(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定本機原版對拍輸出")
	}
	type row struct{ Hull, Seed, HullAfter, RollsAfter int }
	rows := []row{}
	for _, hull := range []int{0, 1, 2, 3, 4, 13, 16} {
		for _, seed := range []int{0, 1, 17, 41} {
			s := State{HullCondition: hull, CombatRolls: seed}
			damageHull(&s)
			rows = append(rows, row{hull, seed, s.HullCondition, s.CombatRolls})
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-hull-damage.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式船體扣損 %d 案，測試 seed 在執行前固定", len(rows))
}

// 正式敵艦射擊的狀態投影；原版另執行命中、傷害、護盾與穿透分支。
func TestExportTalosCombatOverflow(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("未指定本機原版對拍輸出")
	}
	type row struct {
		Enemy, Range, Hull, Shield, Seed, ShotSeed, Layout int
		Before, After                                      []ShipSystem
		HullAfter, ShieldAfter, RollsAfter                 int
		ChargeAfter, SettingAfter                          int
	}
	rows := []row{}
	for enemy := 0; enemy < EnemyShipCount(); enemy++ {
		for seed := 0; seed < 32; seed++ {
			for layout := 0; layout < 6; layout++ {
				for _, shield := range []int{0, 19, 20, 95, 96, 300} {
					s := State{EnemyType: enemy, EnemyHull: 189, CombatRange: 60,
						HullCondition: 16, ShieldLevel: shield, CombatRolls: seed,
						EnemyCourseMode: 2, EnemyFireCountdown: 0, WeaponCharge: 100, ShieldSetting: 96}
					for _, id := range ShipSystemIDs() {
						system := ShipSystem{ID: id, Rows: intactRows(id), LastHitSlot: 13}
						source, _ := sourceShipSystem(id)
						for r := range system.Rows {
							switch layout {
							case 1: // 尚有正常列，亦有空槽；測試抽中無效槽後的分支。
								if r < 2 {
									system.Rows[r][1] = 0
								}
							case 2:
								for c := 1; c < len(system.Rows[r]); c++ {
									system.Rows[r][c] = 23
								}
							case 3:
								if source.Alternate != nil {
									system.Rows[r][source.Alternate.Column] = source.Alternate.ItemIndex
								}
							case 4:
								if r < 2 {
									system.Rows[r][2] = blastedPartMarker(system.Rows[r][2])
								}
							case 5:
								system.Rows[r][0] = 7
							}
						}
						system.Status = shipSystemStatus(system)
						s.ShipSystems = append(s.ShipSystems, system)
					}
					before := make([]ShipSystem, len(s.ShipSystems))
					for i, system := range s.ShipSystems {
						before[i] = system
						before[i].Rows = copyShipSystemRows(system.Rows)
					}
					// 模式 2 的漂移不抽亂數；倒數重設先抽一次，原版比較從命中入口開始。
					shotSeed := seed + 1
					resolveEnemyFire(&s)
					rows = append(rows, row{enemy, 60, 16, shield, seed, shotSeed, layout,
						before, s.ShipSystems, s.HullCondition, s.ShieldLevel, s.CombatRolls, s.WeaponCharge, s.ShieldSetting})
				}
			}
		}
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-overflow.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("正式敵艦護盾／穿透狀態 %d 案，seed 在執行前固定", len(rows))
}

// 固定原版寫回向量，來源見戰鬥證據入口及 combat-overflow-before／zero-before 收據。
func TestTalosOverflowReferenceVectors(t *testing.T) {
	for _, ref := range []struct{ seed, layout, hull, rolls, charge, setting int }{
		{4, 1, 16, 13, 100, 96}, // 抽中空槽不追加船體扣損。
		{0, 2, 15, 28, 100, 0},  // 分流器掃描後仍炸毀原選槽。
		{10, 2, 16, 59, 0, 0},   // 砲與護盾被打到歸零，對應等級亦歸零。
	} {
		s := NewState()
		s.EnemyType, s.EnemyHull, s.CombatRange = 0, 189, 60
		s.HullCondition, s.ShieldLevel, s.CombatRolls = 16, 0, ref.seed
		s.EnemyCourseMode, s.EnemyFireCountdown = 2, 0
		s.WeaponCharge, s.ShieldSetting = 100, 96
		for i := range s.ShipSystems {
			system := &s.ShipSystems[i]
			system.Rows, system.LastHitSlot = intactRows(system.ID), 13
			for row := range system.Rows {
				if ref.layout == 1 && row < 2 {
					system.Rows[row][1] = 0
				}
				if ref.layout == 2 {
					for col := 1; col < 4; col++ {
						system.Rows[row][col] = 23
					}
				}
			}
			system.Status = shipSystemStatus(*system)
		}
		resolveEnemyFire(&s)
		if s.HullCondition != ref.hull || s.CombatRolls != ref.rolls || s.WeaponCharge != ref.charge || s.ShieldSetting != ref.setting {
			t.Fatalf("原版向量 seed=%d layout=%d：hull=%d rolls=%d charge=%d setting=%d", ref.seed, ref.layout, s.HullCondition, s.CombatRolls, s.WeaponCharge, s.ShieldSetting)
		}
		if ref.seed == 0 {
			shield := s.ShipSystems[4]
			if shield.LastHitSlot != 9 || shield.Status != 0 {
				t.Fatalf("原選分流器定位不同：%+v", shield)
			}
			for _, row := range shield.Rows {
				if row[1] != 151 || row[2] != 23 || row[3] != 23 {
					t.Fatalf("分流器原版向量不同：%v", shield.Rows)
				}
			}
		}
	}
}
