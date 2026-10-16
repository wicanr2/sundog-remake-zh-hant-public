package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Research exporter; typed input only, no original interpreter memory.
func TestExportTalosCombatLethalParent(t *testing.T) {
	output := os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("lethal-shot research not enabled")
	}
	type input struct{ Enemy, Mode, Phase, Hull, Seed, Layout int }
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
		Step           snapshot
		Screen         Screen
		Outcome        string
		SleepActive    bool
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
	for layout := 0; layout < 2; layout++ {
		for enemy := 0; enemy < 7; enemy++ {
			for mode := 0; mode < 8; mode++ {
				for phase := 0; phase < 3; phase++ {
					for _, hull := range []int{1, 2, 3, 4, 16} {
						for _, seed := range []int{0, 1, 17, 41} {
							fuel, ready, initialShield := 0, true, 0
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
								if layout == 1 && n != 1 {
									for row := range s.ShipSystems[n].Rows {
										s.ShipSystems[n].Rows[row] = make([]int, 4)
									}
								}
								s.ShipSystems[n].LastHitSlot = 0
							}
							normalizeShipSystems(&s)
							if shipSystemStatusByID(s, ShipSystemSubcEngine) != 16 || layout == 0 && (shipSystemStatusByID(s, ShipSystemShipGuns) != 8 || shipSystemStatusByID(s, ShipSystemForceShields) != 8) {
								t.Fatal("initial part rows/status mismatch")
							}
							s.Fuel, s.ShipPhase, s.WeaponCharge, s.ShieldLevel, s.ShieldSetting = fuel, phase, 0, initialShield, 255
							s.HullCondition = hull
							s.FlightTicks = 1000
							s.FlightFromAngle, s.FlightFromRadius, s.FlightAngle, s.FlightRadius = 250, 20, 250, 20
							s.FlightDeltaAngle, s.FlightDeltaRadius = 0, 0
							s.EnemyType, s.EnemyCourseMode, s.EnemyFireCountdown = enemy, mode, 1
							s.CombatRange, s.EnemyHull, s.EnemyMaxHull = 70, 200, 200
							s.EnemyBearing, s.EnemyPitch, s.EnemyDriftBearing, s.EnemyDriftPitch = 512, 512, 3, -3
							s.EnemyClosing = -1
							s.CombatActive = true
							s.CombatRolls = seed
							s.EnemyCourse = EnemyCourseState{Ticks: 1, FireReady: ready, CanPrepareFire: true, Maneuver: [2]int{2, 1}}

							r := row{Input: input{enemy, mode, phase, hull, seed, layout}, InitialSystems: captureSystems(s)}
							advanceShipClock(&s)
							words, _ := consumerWords(s)
							x := snapshot{Phase: s.ShipPhase, Fuel: s.Fuel, Weapon: s.WeaponCharge, Shield: s.ShieldLevel, Setting: s.ShieldSetting,
								Hull: s.HullCondition, Cooldown: s.EnemyFireCountdown, Range: s.CombatRange, Bearing: s.EnemyBearing, Pitch: s.EnemyPitch,
								BearingDrift: s.EnemyDriftBearing, PitchDrift: s.EnemyDriftPitch, Counter: s.CombatRolls, Active: s.CombatActive, Words: words}
							x.Timer, x.Ready, x.Closing, x.Aux8, x.Aux9 = s.EnemyCourse.Ticks, s.EnemyCourse.FireReady, s.EnemyClosing, s.EnemyCourse.Maneuver[0], s.EnemyCourse.Maneuver[1]
							x.CanPrepareFire = s.EnemyCourse.CanPrepareFire
							x.Systems = captureSystems(s)

							r.Step = x
							r.Screen = s.Screen
							r.Outcome = s.OutcomeKey
							r.SleepActive = s.Sleep.Active
							rows = append(rows, r)
						}
					}
				}
			}
		}
	}
	if len(rows) != 6720 {
		t.Fatal("missing lethal boundary inputs", len(rows))
	}
	raw, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-combat-lethal.json"), append(raw, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("formal prepared lethal-shot parent: %d cases", len(rows))
}
