package game

import (
	"strings"

	"github.com/wicanr2/sundog-remake/internal/source/atarist"
)

// The rules below are read from the original's own p-code and are recorded,
// with a byte signature for each, in docs/atari-st/ship-rules.md and
// docs/atari-st/ship-rules.json. The enemy's own numbers come from the table on
// the disk's copy-protection track, decoded into docs/atari-st/enemy-ships.json.
// What stays remake-owned is marked as such: the substitution of a save-carried
// counter for the original's RNG so a replay reproduces, and the rate at which
// a tactical move closes the range.
const (
	// combatTargets is the roll SHIPLIB:0x21 update_attack makes to pick what a
	// shot hits: six ship systems and, as the seventh outcome, the hull.
	combatTargets = 7
	// overflowPerPart is how much damage past the shields costs one part. The
	// original subtracts it and rolls again while anything is left, so a heavy
	// shot can take several parts at once.
	overflowPerPart = 96
	// minimumShotDamage and maximumShotDamage bound a shot before it is applied.
	minimumShotDamage = 20
	maximumShotDamage = 300
	// hullLossSpan is the range of a hull hit: one to three.
	hullLossSpan = 3
	// weaponRechargePerStatus and shieldRechargePerStatus tie the tactical
	// levels to the systems that produce them. Guns that have been shot up
	// recharge the weapon more slowly; the same holds for the force shields.
	weaponRechargePerStatus = 4
	shieldRechargePerStatus = 8
	// maxWeaponChargeLevel is the ceiling update_levels clamps the weapon to.
	maxWeaponChargeLevel = 255
	// shieldFuelDivisor turns the shield level into the fuel it costs to hold.
	shieldFuelDivisor = 64

	// The numbers below come from the encounter and firing code; each has a
	// byte signature in docs/atari-st/ship-rules.json.

	// initialCombatRange is the distance an encounter opens at, and
	// minimumCombatRange the floor it can close to. The floor is what keeps
	// the accuracy division below from dividing by zero.
	initialCombatRange = 127
	minimumCombatRange = 10
	// accuracyRangeDivisor and accuracyBonus turn the enemy's accuracy byte
	// into a chance out of a hundred: (accuracy + 100) / (range / 8).
	accuracyRangeDivisor = 8
	accuracyBonus        = 100
	// enemyHullBase and enemyHullPerType are the bounds of the hull roll a new
	// enemy gets: rand(120 + type * 20, 255).
	enemyHullBase    = 120
	enemyHullPerType = 20
	enemyHullCeiling = 255
	// The damage a shot carries scales with the enemy's own remaining hull:
	// (hull / 20 + 4) squared, capped at 90.
	damageHullDivisor = 20
	damageHullOffset  = 4
	damageScaleCap    = 90
	// weaponPowerBonus is added to the enemy's weapon byte before it scales the
	// shot; damageRangeDivisor and the roll below form the divisor.
	weaponPowerBonus   = 100
	damageRangeDivisor = 3
	damageRollLow      = 25
	damageRollHigh     = 50
	// 原版 SHIPLIB:0x0b 0x052c 的新遭遇冷卻；來源簽章和值由
	// combat.fire-countdown-initial 釘住，與開火後的重設界限分開。
	initialEnemyFireCountdown = 20
	// fireCountdownLow and fireCountdownHigh bound the reload counter after firing.
	fireCountdownLow  = 5
	fireCountdownHigh = 10
	// The player's shot: a base that depends on whether the guns carry their
	// concentrator, less the range, less a random share of itself, scaled by
	// the weapon charge, divided by the enemy's armour and clamped.
	playerBasePowerGood  = 20
	playerBasePowerPlain = 16
	playerRangeDivisor   = 17
	playerSpreadDivisor  = 3
	playerChargeDivisor  = 10
	playerChargeBonus    = 10
	playerDamageMinimum  = 1
	playerDamageMaximum  = 80
	// The laser, the second of the original's two gun buttons: a higher base,
	// no range falloff, but an aiming window a quarter the cannon's. The
	// window is (15 + DX) / 10, less range/32 for the laser against less
	// range/12 on four times the window for the cannon (fire_it and
	// init_fight, rule attack.player-gun-base).
	playerBaseLaserGood      = 42
	playerBaseLaserPlain     = 30
	aimWindowOffset          = 15
	aimWindowDivisor         = 10
	laserAimRangeDivisor     = 32
	cannonAimRangeDivisor    = 12
	cannonAimToleranceFactor = 4
	// The distance bounds the enemy's own course rules compare against. They
	// are SHIPLIB:0x1e set_course's, one per behaviour mode.
	courseFar       = 90
	courseMid       = 80
	courseNear      = 70
	courseClose     = 60
	courseVeryClose = 50
	// enemyCourseModes is how many behaviour modes set_course dispatches on,
	// and enemySpawnCourseModes how many of them an encounter can start in.
	enemyCourseModes      = 8
	enemySpawnCourseModes = 6
)

var (
	enemyShipSource   atarist.EnemyShipData
	enemyShipSourceOK bool
)

func init() {
	data, err := atarist.EmbeddedEnemyShipData()
	if err != nil {
		return
	}
	enemyShipSource = data
	enemyShipSourceOK = true
}

// EnemyShipCount is how many enemy types the source table holds.
func EnemyShipCount() int {
	if !enemyShipSourceOK {
		return 0
	}
	return enemyShipSource.Count()
}

// EnemyShipName is the enemy's own name, as the original's combat screen prints
// it in its ID field. The source name is the fallback, so a missing translation
// shows the ship rather than a key.
func (r Reducer) EnemyShipName(state State) string {
	ship, ok := enemyShip(state.EnemyType)
	if !ok {
		return ""
	}
	return r.localize("ui.enemy."+enemyShipSlug(ship.Name), ship.Name)
}

// enemyShipSlug turns a source name into the catalog key the name builder
// generates for it; the two rules have to stay the same, which
// TestEveryEnemyShipHasATranslatedName checks.
func enemyShipSlug(name string) string {
	var out []rune
	dash := true
	for _, char := range strings.ToLower(name) {
		switch {
		case char < 128 && (char >= 'a' && char <= 'z' || char >= '0' && char <= '9'):
			out = append(out, char)
			dash = false
		case !dash:
			out = append(out, '-')
			dash = true
		}
	}
	return strings.Trim(string(out), "-")
}

// EnemyShipOutline is the enemy's outline as a list of points, in the source's
// own byte coordinates. A screen that draws it decides the scale.
func EnemyShipOutline(state State) [][2]int {
	ship, ok := enemyShip(state.EnemyType)
	if !ok {
		return nil
	}
	return ship.Outline
}

func enemyShip(index int) (atarist.EnemyShip, bool) {
	if !enemyShipSourceOK {
		return atarist.EnemyShip{}, false
	}
	return enemyShipSource.Ship(index)
}

// EnemyHullPercent expresses the enemy's hull on a 0..100 scale for meters. The
// hull's own scale is the source's and runs to 255.
func EnemyHullPercent(state State) int {
	if state.EnemyMaxHull <= 0 {
		return 0
	}
	return clamp(state.EnemyHull, 0, state.EnemyMaxHull) * 100 / state.EnemyMaxHull
}

// PodDrawsEncounter checks update_movement's fourth gate. The original
// SHIPLIB:1f 0afc..0b29 admits codes2..30 and excludes empty0, fuel1 and
// cryogens31. See ship-rules.md and flight-ship-parent-parity-20261004.json.
// Synthetic cargo kinds retain their existing approximation; this gate's
// 1024-case proof covers source-coded holds, not those unmapped kinds.
func PodDrawsEncounter(state State) bool {
	for _, hold := range state.Holds {
		if hold.Kind == HoldGoods {
			// SHIPLIB:1f 0afc..0b29 admits source codes2..30. Code31
			// carries cryogens and does not pass this ordinary cargo gate.
			if hold.Goods >= 2 && hold.Goods < 31 {
				return true
			}
			continue
		}
		if hold.Occupied() && hold.Kind != HoldAuxFuel {
			return true
		}
	}
	return false
}

// beginEncounter uses the source initialization order and persistent course
// state. Scope and the mode0 legacy boundary are in ship-rules.md.
func beginEncounter(state *State, encounterRating int) {
	base := 0
	if encounterRating > 0 {
		base = 1 - (encounterRating-1)/4
	}
	spread := combatRoll(state, 4) + combatRoll(state, 3) + combatRoll(state, 2)
	highest := EnemyShipCount() - 2 // the last type is not a normal encounter
	if highest < 0 {
		highest = 0
	}
	state.EnemyType = clamp(base+spread, 0, highest)
	// An encounter starts in one of the first six behaviour modes; the last two
	// are only entered later.
	state.EnemyCourseMode = combatRoll(state, enemySpawnCourseModes)

	state.CombatRange = initialCombatRange
	state.EnemyClosing = 0
	state.EnemyCourse.FireReady = false
	state.EnemyCourse.CanPrepareFire = true
	initializeEnemyManeuver(state, &state.EnemyCourse)
	initializeEnemyPositionAndHull(state)
	if !selectEnemyCourse(state, &state.EnemyCourse) {
		// The unproven mode0/cloak boundary keeps its legacy projection.
		setEnemyDrift(state)
	}
	state.CombatActive = true
	state.CombatVictory = false
}

// Historical isolated mode4/5 projection. Production initializes the complete
// persistent maneuver through initializeEnemyManeuver.
func initializeEnemyCourseDrift(state *State) {
	switch state.EnemyCourseMode {
	case 4:
		state.EnemyDriftBearing, state.EnemyDriftPitch = 3, -3
	case 5:
		state.EnemyDriftBearing = 2
		if combatRoll(state, 2) != 0 {
			state.EnemyDriftBearing = -3
		}
		state.EnemyDriftPitch = 3
		if combatRoll(state, 2) != 0 {
			state.EnemyDriftPitch = -2
		}
	}
}

// 原版 SHIPLIB 0x0c0f..0x0c37：先方位、再俯仰，最後抽船體。
// beginEncounter calls it after the source mode-specific maneuver initializer.
func initializeEnemyPositionAndHull(state *State) {
	state.EnemyBearing = combatRoll(state, enemyAngleTurn)
	state.EnemyPitch = combatRoll(state, enemyAngleTurn)
	floor := enemyHullBase + state.EnemyType*enemyHullPerType
	state.EnemyMaxHull = floor + combatRoll(state, enemyHullCeiling-floor+1)
	state.EnemyHull = state.EnemyMaxHull
}

// setEnemyCourse 是原版 set_course 接近率的部分投影，完整跨次狀態／退避仍DRAFT。
// 漂移另由setEnemyDrift更新，會影響瞄準與船位；兩helper不能當成完整原版次序。
func setEnemyCourse(state *State) int {
	distance := combatRange(*state)
	switch state.EnemyCourseMode {
	case 0:
		return -2
	case 1:
		if distance >= courseNear {
			return -1
		}
		return -2 + combatRoll(state, 5)
	case 2:
		if distance > courseFar {
			return -2
		}
		if distance >= courseClose {
			return -1
		}
		return 1
	case 3:
		if distance > courseFar {
			return -3
		}
		return 0
	case 4:
		if distance <= courseMid {
			// 原版 SHIPLIB 0x07d4..0x07de：rand(-2,+1)，兩端都含。
			return -2 + combatRoll(state, 4)
		}
		return -1
	case 5:
		if distance <= courseNear {
			// 原版 SHIPLIB 0x07f2..0x07fc：rand(-2,+1)，兩端都含。
			return -2 + combatRoll(state, 4)
		}
		return -2
	case 6:
		if distance <= courseClose {
			return 0
		}
		return -2
	}
	// 模式7原版會讀取跨次接近率及Aux值；現行投影尚未整合，不能宣稱原版不改距離。
	return 0
}

const (
	enemyAngleTurn  = 0x400 // both axes run 0–0x3ff
	enemyDriftLimit = 4     // set_course clamps both drifts to ±4 at the end
	enemyDriftStep  = 4     // each tick moves an axis by drift × 4 (SHIPLIB 0x0d42)
)

// rollBetween is MAINLIB:0x0c rand(low, high) on the combat die.
func rollBetween(state *State, low, high int) int {
	return low + combatRoll(state, high-low+1)
}

// setEnemyDrift 部分投影原版set_course的方位Gd3／俯仰Gd2，夾限±4。
// 它們經steerEnemy與瞄準計算影響正式玩法；模式7持久Aux與完整抽值次序仍待整合。
func setEnemyDrift(state *State) {
	b, p := state.EnemyDriftBearing, state.EnemyDriftPitch
	switch state.EnemyCourseMode {
	case 0:
		b, p = rollBetween(state, -2, 2), rollBetween(state, -2, 2)
	case 1:
		p, b = rollBetween(state, -1, 1), rollBetween(state, -1, 1)
	case 2:
		b++
		if b > enemyDriftLimit {
			b = -enemyDriftLimit
		}
		p--
		if p < -enemyDriftLimit {
			p = enemyDriftLimit
		}
	case 3:
		b, p = rollBetween(state, -4, 4), rollBetween(state, -4, 4)
	case 4:
		b = -b
		if combatRoll(state, 5) == 0 {
			b++
			p = -p + 1
		}
	case 5:
		// 原版SHIPLIB 0x07fc..0x0815：布林真翻方位，假翻俯仰。
		if combatRoll(state, 2) != 0 {
			b = -b
		} else {
			p = -p
		}
	case 6:
		b, p = rollBetween(state, -4, 0), rollBetween(state, -4, 4)
	default:
		if b >= enemyDriftLimit {
			b--
		} else {
			b++
		}
		if p >= enemyDriftLimit {
			p--
		} else {
			p++
		}
	}
	state.EnemyDriftBearing = clamp(b, -enemyDriftLimit, enemyDriftLimit)
	state.EnemyDriftPitch = clamp(p, -enemyDriftLimit, enemyDriftLimit)
}

// steerEnemy applies one tick of the enemy's course. The floor is the source's,
// and it is what keeps the accuracy division from dividing by zero. The two
// axes move by their drifts first (SHIPLIB 0x0d42: bearing − Gd3×4, pitch +
// Gd2×4, both mod 0x400), then the distance closes.
func steerEnemy(state *State) {
	state.EnemyBearing = wrapAngle(state.EnemyBearing - state.EnemyDriftBearing*enemyDriftStep)
	state.EnemyPitch = wrapAngle(state.EnemyPitch + state.EnemyDriftPitch*enemyDriftStep)
	state.CombatRange = combatRange(*state) + setEnemyCourse(state)
	if state.CombatRange < minimumCombatRange {
		state.CombatRange = minimumCombatRange
	}
}

func wrapAngle(v int) int {
	v %= enemyAngleTurn
	if v < 0 {
		v += enemyAngleTurn
	}
	return v
}

// playerShotDamage is what one of the player's cannon shots takes off the
// enemy. Every term is the source's, the base included: XDOFIGHT:0x06 fire_it
// reads MAINLIB_G53 bit 18, which SHIPLIB:0x0e update_system sets when the
// ship's guns are carrying their alternate part -- the concentrator. Fitting
// one takes the base from 16 to 20 (rule attack.player-gun-base).
func playerShotDamage(state *State) int {
	ship, ok := enemyShip(state.EnemyType)
	if !ok || ship.Armour <= 0 {
		return 0
	}
	power := playerBasePowerPlain
	if alternatePartFitted(*state, ShipSystemShipGuns) {
		power = playerBasePowerGood
	}
	power -= combatRange(*state) / playerRangeDivisor
	if power > 0 {
		power -= combatRoll(state, power/playerSpreadDivisor+1)
	}
	power *= state.WeaponCharge/playerChargeDivisor + playerChargeBonus
	power /= ship.Armour
	return clamp(power, playerDamageMinimum, playerDamageMaximum)
}

// aimTolerances is the pair of windows fire_it compares the aiming error
// against: (15 + DX) / 10 less range/32 for the laser, and four times that
// window less range/12 for the cannon. Both formulas are the source's; DX is
// MAINLIB_G35[2], read the same way the arena reads it.
func aimTolerances(state State) (laser, cannon int) {
	window := (aimWindowOffset + state.Attribute(AttrDexterity)) / aimWindowDivisor
	distance := combatRange(state)
	// XDOFIGHT 0x021b..0x0222 clamps negative windows to zero; an enemy
	// exactly on the cross-hair still lands inside that zero-width window.
	laser = max(0, window-distance/laserAimRangeDivisor)
	cannon = max(0, window*cannonAimToleranceFactor-distance/cannonAimRangeDivisor)
	return laser, cannon
}

// playerLaserShot is the second gun: base 30, or 42 with the concentrator, no
// range falloff, spread and charge scaling as the cannon's, then the enemy's
// armour divisor (fire_it, rule attack.player-gun-base).
//
// What the original prices the laser at is aim: the shot lands only when the
// player holds the enemy inside a window a quarter the cannon's, measured by
// aimError on the two angles the mouse steers (slew.go).
func playerLaserShot(state *State) (int, bool) {
	ship, ok := enemyShip(state.EnemyType)
	if !ok || ship.Armour <= 0 {
		return 0, false
	}
	laserWindow, _ := aimTolerances(*state)
	if aimError(*state) > laserWindow {
		return 0, false
	}
	power := playerBaseLaserPlain
	if alternatePartFitted(*state, ShipSystemShipGuns) {
		power = playerBaseLaserGood
	}
	power -= combatRoll(state, power/playerSpreadDivisor+1)
	power *= state.WeaponCharge/playerChargeDivisor + playerChargeBonus
	power /= ship.Armour
	return clamp(power, playerDamageMinimum, playerDamageMaximum), true
}

// enemyShotDamage is the damage of the enemy's shot, or zero when it misses.
// Accuracy and damage both read the same distance, so closing in cuts both ways.
func enemyShotDamage(state *State) (int, bool) {
	ship, ok := enemyShip(state.EnemyType)
	if !ok {
		return 0, false
	}
	distance := combatRange(*state)
	chance := (ship.Accuracy + accuracyBonus) / (distance / accuracyRangeDivisor)
	if chance <= combatRoll(state, 100)+1 {
		return 0, false
	}

	scale := state.EnemyHull/damageHullDivisor + damageHullOffset
	scale *= scale
	if scale > damageScaleCap {
		scale = damageScaleCap
	}
	spread := damageRollLow + combatRoll(state, damageRollHigh-damageRollLow+1)
	damage := (ship.WeaponPower + weaponPowerBonus) * scale / (distance/damageRangeDivisor + spread)
	return clamp(damage, minimumShotDamage, maximumShotDamage), true
}

// combatRange never returns less than the floor, so the two divisions that use
// it cannot divide by zero even on a save written before the field existed.
func combatRange(state State) int {
	if state.CombatRange < minimumCombatRange {
		return minimumCombatRange
	}
	return state.CombatRange
}

// MaxHullCondition is the hull a new game starts with. The original rolls it
// between two bounds; the remake takes the top of that range so a new game is
// reproducible, which is the same choice it makes about the original's random
// starting damage.
func MaxHullCondition() int {
	if !loadoutSourceOK {
		return 0
	}
	_, maximum, ok := loadoutSource.HullRange()
	if !ok {
		return 0
	}
	return maximum
}

// HullPercent expresses the hull as a proportion of a whole ship, for meters
// and bars that are drawn on a 0..100 scale. The hull's own scale is the
// source's and is much smaller.
func HullPercent(state State) int {
	maximum := MaxHullCondition()
	if maximum <= 0 {
		return 0
	}
	return clamp(state.HullCondition, 0, maximum) * 100 / maximum
}

// blastedPartMarker is what a destroyed part becomes in place: its own code
// with bit 7 set. The repair screen turns anything above 0x90 into a dead part.
func blastedPartMarker(item int) int { return item | 0x80 }

// isBlastedPart reports whether a slot holds a part the enemy has destroyed.
func isBlastedPart(item int) bool { return item > 0x90 }

// IsBlastedPart is isBlastedPart for the shells.
func IsBlastedPart(item int) bool { return isBlastedPart(item) }

// combatRoll is the remake's stand-in for the original's RNG. The original
// rolls; the remake steps a counter that lives in the save, so a battle replays
// to the same state and a screenshot can be checked. Which of the two is in use
// is the only difference from the rules the p-code describes.
//
// The counter is stirred before it is reduced. Taking it modulo the span
// directly would correlate: a roll that always happens at the same point of an
// encounter — the course mode, say, which is the fourth of six — would land on
// the same answer every single time, and every enemy would fly identically.
// Stirring costs nothing and keeps the sequence just as reproducible.
func combatRoll(state *State, span int) int {
	if span <= 0 {
		return 0
	}
	state.CombatRolls++
	mixed := uint32(state.CombatRolls)*1664525 + 1013904223
	mixed ^= mixed >> 16
	return int(mixed % uint32(span))
}

// resolveEnemyFire runs one tick of the enemy's turn: the reload counter steps
// down by the enemy's own fire rate, and only when it runs out does a shot
// leave. The shot may then miss. Everything a shot that lands does follows
// update_attack.
func resolveEnemyFire(state *State) {
	ship, ok := enemyShip(state.EnemyType)
	if !ok {
		return
	}
	state.EnemyFireCountdown -= ship.FireRate
	if state.EnemyFireCountdown > 0 {
		state.MessageKey = "ui.message.enemy_holding"
		return
	}
	setEnemyDrift(state)
	state.EnemyFireCountdown = fireCountdownLow + combatRoll(state, fireCountdownHigh-fireCountdownLow+1)
	resolveEnemyShot(state)
}

// A prepared shot and the retained legacy projection share the same already
// verified accuracy, shield, hull and system-damage consumer.
func resolveEnemyShot(state *State) {
	damage, hit := enemyShotDamage(state)
	if !hit {
		state.MessageKey = "ui.message.enemy_missed"
		return
	}

	// The shields take the shot first. Anything they can absorb stops here.
	if damage <= state.ShieldLevel {
		state.ShieldLevel -= damage
		state.MessageKey = "ui.message.enemy_fired_shields"
		return
	}
	damage -= state.ShieldLevel
	state.ShieldLevel = 0

	hitHull := false
	for damage > 0 {
		if applyOverflowHit(state) {
			hitHull = true
		}
		damage -= overflowPerPart
	}
	if hitHull {
		state.MessageKey = "ui.message.enemy_fired_hull"
		return
	}
	state.MessageKey = "ui.message.enemy_fired_system"
}

// applyOverflowHit spends one unit of overflow damage and reports whether it
// landed on the hull.
func applyOverflowHit(state *State) bool {
	target := combatRoll(state, combatTargets)
	if target >= len(state.ShipSystems) {
		damageHull(state)
		return true
	}
	// A system that is already out does not absorb anything; the shot carries
	// through to the hull.
	if state.ShipSystems[target].Status <= 0 {
		damageHull(state)
		return true
	}
	if !blastShipSystemPart(state, target) {
		// SHIPLIB 0x0ef5 skips an invalid slot; only a dead system falls through
		// to the hull. See the native overflow receipt in the combat audit.
		return false
	}
	state.SelectedSystemIndex = target
	return false
}

func damageHull(state *State) {
	loss := combatRoll(state, hullLossSpan) + 1
	state.HullCondition -= loss
	if state.HullCondition < 0 {
		state.HullCondition = 0
	}
}

// blastShipSystemPart destroys one part of a system, following update_attack:
// a row is picked, then the alternate part's column when that part is fitted,
// otherwise any column. A shunt in the chosen slot is not destroyed on its own
// — the original sweeps the system and may take several shunts — and a slot
// that is already gone consumes this overflow unit without further damage.
func blastShipSystemPart(state *State, index int) bool {
	system := &state.ShipSystems[index]
	source, ok := sourceShipSystem(system.ID)
	if !ok || len(system.Rows) == 0 {
		return false
	}
	row := combatRoll(state, len(system.Rows))
	slots := system.Rows[row]
	column := len(slots)
	if source.Alternate != nil && source.Alternate.Column < len(slots) &&
		slots[source.Alternate.Column] == source.Alternate.ItemIndex {
		column = source.Alternate.Column
	} else {
		column = combatRoll(state, len(slots))
	}

	installed := slots[column]
	if installed == atarist.ShuntItemIndex() {
		sweepShunts(state, index)
	}
	if installed <= atarist.DeadPartItemIndex() || installed >= 0x80 {
		return false
	}
	system.Rows[row][column] = blastedPartMarker(installed)
	system.LastHitSlot = row*len(slots) + column
	system.Status = shipSystemStatus(*system)
	// SHIPLIB update_system 0x0301..0x032c clears a dead system's level.
	if system.Status == 0 {
		switch system.ID {
		case ShipSystemShipGuns:
			state.WeaponCharge = 0
		case ShipSystemForceShields:
			state.ShieldSetting = 0
		}
	}
	return true
}

// sweepShunts is what the original does when the shot lands on a shunt: it
// walks all twenty record positions. Its bitwise condition draws even at a
// non-shunt or a row-status position. The selected shunt is then always blasted
// by blastShipSystemPart, which also records that selected slot as the last hit.
// Native evidence: SHIPLIB 0x0ecc..0x0f0d, combat audit 2026-10-01.
func sweepShunts(state *State, index int) {
	system := &state.ShipSystems[index]
	for row := range system.Rows {
		// The fifth position is derived row status, not a runtime part slot.
		for column := 0; column <= len(system.Rows[row]); column++ {
			odd := combatRoll(state, 2) != 0
			if column == len(system.Rows[row]) || !odd ||
				system.Rows[row][column] != atarist.ShuntItemIndex() {
				continue
			}
			system.Rows[row][column] = blastedPartMarker(system.Rows[row][column])
		}
	}
}

// 已啟用艦砲的一次能量更新；READY範圍見talos-combat-audit-20260907.md。
// 有燃料且艦砲可用才回充，實際增加扣1；否則衰減8。完整相位／開關／關閉流程另待整合。
func rechargeWeapon(state *State) {
	status := shipSystemStatusByID(*state, ShipSystemShipGuns)
	if state.Fuel > 0 && status > 0 {
		before := state.WeaponCharge
		state.WeaponCharge = clamp(before+status*weaponRechargePerStatus, 0, maxWeaponChargeLevel)
		if state.WeaponCharge > before {
			state.Fuel--
		}
		return
	}
	state.WeaponCharge = clamp(state.WeaponCharge-8, 0, maxWeaponChargeLevel)
}

// 無備用油時，原版只在燃料由正值歸零的能量更新末段關閉護盾。
// READY定位見docs/atari-st/talos-combat-audit-20260907.md；保留當拍剩餘護盾。
// 備用油補充、其他系統旗標及完整排程另待整合。
func closeShieldsAfterFuelExhaustion(state *State, oldFuel int) {
	if oldFuel > 0 && state.Fuel == 0 && AuxFuelTanks(*state) == 0 {
		state.ShieldsActive = false
		state.ShieldSetting = 0
	}
}

// 原版一次phase0護盾更新：未達標扣設定值導出的燃料，剛達標不扣，原已達標扣1。
// READY範圍見docs/atari-st/talos-combat-audit-20260907.md；正式三相排程、
// 0–100／原版0–255尺度、其他低燃料系統關閉與備用油箱整合仍待。
func rechargeShields(state *State) {
	if !state.ShieldsActive {
		state.ShieldSetting = 0
	}
	status := shipSystemStatusByID(*state, ShipSystemForceShields)
	wasAtSetting := state.ShieldLevel == state.ShieldSetting
	if state.ShieldLevel < state.ShieldSetting {
		state.ShieldLevel = clamp(state.ShieldLevel+status*shieldRechargePerStatus, 0, state.ShieldSetting)
	}
	if state.ShieldLevel > state.ShieldSetting {
		state.ShieldLevel = state.ShieldSetting
	}
	if wasAtSetting && state.ShieldLevel > 0 {
		state.Fuel = clamp(state.Fuel-1, 0, maxFuel)
	} else if state.ShieldLevel < state.ShieldSetting {
		state.Fuel = clamp(state.Fuel-(state.ShieldSetting+1)/shieldFuelDivisor, 0, maxFuel)
	}
}

func shipSystemStatusByID(state State, id ShipSystemID) int {
	for _, system := range state.ShipSystems {
		if system.ID == id {
			return system.Status
		}
	}
	return 0
}
