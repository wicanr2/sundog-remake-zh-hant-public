package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// TestNewGameStartsWithoutFuel pins worklist D6. The original's set_new_game
// writes six fields and none of them is fuel, so LIFTOFF answers "Out of fuel"
// until the player buys some (docs/atari-st/ship-rules.md). The remake shipped
// Fuel: 100 from its baseline commit, which was never checked against the
// original.
func TestNewGameStartsWithoutFuel(t *testing.T) {
	if got := NewState().Fuel; got != 0 {
		t.Fatalf("new game fuel = %d, want 0", got)
	}
}

// TestNewGameCannotLiftOffUntilRefuelled is the player-visible half: an empty
// tank has to stop the trip, not merely read as zero.
func TestNewGameCannotLiftOffUntilRefuelled(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	// 此案例測航程／任務，先滿足原版非首城降落所需的掃描器。
	fitGroundScannerForTest(t, &state)
	for _, action := range []Action{ActionPlay, ActionOpenNavigation} {
		if state, err = reducer.Step(state, action); err != nil {
			t.Fatalf("step %s: %v", action, err)
		}
	}
	grounded, err := reducer.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if grounded.Screen == ScreenFlight {
		t.Fatalf("an empty tank still started a flight: %+v", grounded)
	}
	if grounded.MessageKey != "ui.message.out_of_fuel" {
		t.Fatalf("message = %q, want ui.message.out_of_fuel", grounded.MessageKey)
	}

	// With fuel the same action flies, so the block is the tank and nothing else.
	state.Fuel = maxFuel
	flying, err := reducer.Step(state, ActionNavigate)
	if err != nil {
		t.Fatal(err)
	}
	if flying.Screen != ScreenFlight && flying.Screen != ScreenShipboard {
		t.Fatalf("a full tank did not fly: %+v", flying)
	}
}

// newGameWithFuel is NewState with a full tank. A new game has none — the
// original writes no fuel in set_new_game (worklist D6) — and tests about
// flying, tractoring or banking are not about the starting tank.
func newGameWithFuel() State {
	state := NewState()
	state.Fuel = maxFuel
	// A new game's ship also carries random damage (XSTARTUP:0x31). Tests about
	// flying, trading or repairing want a known ship, so this hands back the
	// intact one; the damage itself is pinned by ship_damage_test.go.
	systems := make([]ShipSystem, 0, len(shipSystemOrder))
	for _, id := range shipSystemOrder {
		system := ShipSystem{ID: id, Rows: intactRows(id)}
		system.Status = shipSystemStatus(system)
		systems = append(systems, system)
	}
	state.ShipSystems = systems
	return state
}

// XSTARTUP 的世界迴圈（段內 0x1532）給十二個星系各一筆開局存款：
// 千位是 `3 + random() mod 3`，起始星系另加 32，個位是 `random() mod 1000`；
// 迴圈之後 Enlie（第三筆）的兩個字被填成 0。
func TestNewGameBankBalancesFollowTheWorldLoop(t *testing.T) {
	systems := world.FactorySystems()
	if len(systems) != 12 {
		t.Fatalf("星系表有 %d 筆，原版是 12 筆", len(systems))
	}
	for seed := int64(1); seed <= 500; seed++ {
		balances := newGameBankBalances(seed, systems)
		if len(balances) != len(systems) {
			t.Fatalf("seed %d 只給了 %d 筆餘額", seed, len(balances))
		}
		for index, system := range systems {
			got := balances[system.ID]
			switch index {
			case 0:
				if got < 35000 || got > 37999 {
					t.Fatalf("seed %d 的起始星系存款 %d 不在 35,000–37,999", seed, got)
				}
			case tellerPickNoBank:
				if got != 0 {
					t.Fatalf("seed %d 的 Enlie 存款是 %d，那個星系沒有銀行", seed, got)
				}
			default:
				if got < 3000 || got > 5999 {
					t.Fatalf("seed %d 的 %s 存款 %d 不在 3,000–5,999", seed, system.ID, got)
				}
			}
		}
	}
}

// 實測的兩組值都要落在規則裡：Uniteller 擷取的一筆，與記憶體 dump 的十二筆。
func TestTheMeasuredBalancesFitTheRule(t *testing.T) {
	systems := world.FactorySystems()
	inRange := func(index, value int) bool {
		switch index {
		case 0:
			return value >= 35000 && value <= 37999
		case tellerPickNoBank:
			return value == 0
		default:
			return value >= 3000 && value <= 5999
		}
	}
	// docs/atari-st/ux-scene-ledger.md 第八輪的 `In bank: 36,306`。
	if !inRange(0, 36306) {
		t.Fatal("Uniteller 擷取的 36,306 不在起始星系的範圍裡")
	}
	// 走到城市地面那一局的記憶體 dump，十二筆逐一。
	dumped := []int{37623, 5142, 0, 3379, 4191, 4506, 4430, 3549, 3261, 5955, 3822, 3828}
	if len(dumped) != len(systems) {
		t.Fatalf("dump 有 %d 筆，星系表有 %d 筆", len(dumped), len(systems))
	}
	for index, value := range dumped {
		if !inRange(index, value) {
			t.Fatalf("dump 第 %d 筆 %d 不在規則的範圍裡", index+1, value)
		}
	}
}

// 新遊戲買得起貨，靠的就是起始星系那筆存款。
func TestANewGameCanStillAffordALotThroughTheBank(t *testing.T) {
	state := NewState()
	if state.Bank != state.BankBalances[world.StartSystemID] {
		t.Fatalf("Bank 是 %d，帳面是 %d", state.Bank, state.BankBalances[world.StartSystemID])
	}
	if state.Bank < 35000 {
		t.Fatalf("開局存款只有 %d，買不起一批貨", state.Bank)
	}
}

// armForEncounter puts one lot in the pod. update_movement's fourth gate only
// lets an encounter start when a hold carries goods (PodDrawsEncounter), so a
// combat test has to load the ship before it opens the tactical display —
// an empty ship draws no pirate, on the original or here.
func armForEncounter(state State) State {
	state.Holds[0] = Hold{Kind: HoldGoods, Goods: 2}
	return state
}
