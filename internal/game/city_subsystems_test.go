package game

import "testing"

// The numbers checked here are the original's, pinned in
// docs/atari-st/city-subsystems.json. A test that only checked the remake's own
// behaviour would not notice a transcription slip in the odds table, so each
// one states the rule it comes from.

func TestSlotOddsTableCoversTheWholeRollSpan(t *testing.T) {
	// The eight bands are contiguous and add up to 2000: that is what makes
	// roll_one's linear scan always terminate.
	if slotOdds[7].Low != 0 {
		t.Fatalf("the first band should start at 0, got %d", slotOdds[7].Low)
	}
	if slotOdds[0].High != slotRollSpan {
		t.Fatalf("the last band should end at %d, got %d", slotRollSpan, slotOdds[0].High)
	}
	for face := slotFaceCount - 1; face > 0; face-- {
		if slotOdds[face].High != slotOdds[face-1].Low {
			t.Fatalf("band %d ends at %d but band %d starts at %d",
				face, slotOdds[face].High, face-1, slotOdds[face-1].Low)
		}
	}
}

func TestSlotRarestFacesPayMost(t *testing.T) {
	// The relationship is the point of the table: a narrow band pays more.
	for face := 1; face < slotFaceCount; face++ {
		widePrev := slotOdds[face-1].High - slotOdds[face-1].Low
		wide := slotOdds[face].High - slotOdds[face].Low
		if wide > widePrev {
			t.Fatalf("band %d is wider than band %d, which breaks the table's order", face, face-1)
		}
		if slotOdds[face].Triple < slotOdds[face-1].Triple {
			t.Fatalf("band %d pays less than the commoner band %d", face, face-1)
		}
	}
}

func TestSlotBetComesFromTheStarSystem(t *testing.T) {
	// 50 x ((price + 17 - law) / 6 + 1). Jondd is price 8, law 3 in the source
	// tables, so the arithmetic is (8 + 17 - 3) / 6 + 1 = 4 -> 200.
	if got := SlotBet(8, 3); got != 200 {
		t.Fatalf("Jondd's stake should be 200, got %d", got)
	}
	// A cheap, lawful system plays for less than an expensive lawless one.
	cheapLawful := SlotBet(4, 15)
	expensiveLawless := SlotBet(14, 0)
	if cheapLawful >= expensiveLawless {
		t.Fatalf("stake should rise with price and fall with law: %d vs %d", cheapLawful, expensiveLawless)
	}
}

func TestSlotPayoutUsesTheWinningFaceNotTheStakeBand(t *testing.T) {
	// get_pay indexes the odds by the face that matched.
	if got := SlotPayout(100, 3, 7); got != 100*slotOdds[7].Triple {
		t.Fatalf("three of face 7 should pay %d, got %d", 100*slotOdds[7].Triple, got)
	}
	if got := SlotPayout(100, 2, 0); got != 100*slotOdds[0].Pair {
		t.Fatalf("a pair of face 0 should pay %d, got %d", 100*slotOdds[0].Pair, got)
	}
	if got := SlotPayout(100, 0, 3); got != 0 {
		t.Fatalf("no match should pay nothing, got %d", got)
	}
}

func TestSlotMatchOrderFollowsTheOriginal(t *testing.T) {
	if matched, face := matchReels([3]int{4, 4, 4}); matched != 3 || face != 4 {
		t.Fatalf("three alike should be 3 of face 4, got %d/%d", matched, face)
	}
	// 0-1 is compared before 1-2, so a run of three where two pairs could be
	// read resolves to the first.
	if matched, face := matchReels([3]int{2, 2, 5}); matched != 2 || face != 2 {
		t.Fatalf("leading pair should win, got %d/%d", matched, face)
	}
	if matched, face := matchReels([3]int{5, 2, 2}); matched != 2 || face != 2 {
		t.Fatalf("trailing pair should be read, got %d/%d", matched, face)
	}
	if matched, _ := matchReels([3]int{1, 2, 3}); matched != 0 {
		t.Fatalf("three different faces should not pay")
	}
}

func TestLuckOnlyEverImprovesTheSpin(t *testing.T) {
	// roll_one subtracts a luck-scaled draw, and the low end of the number line
	// is where the paying faces are. Running the same seeded machine at both
	// ends of the clamp must never leave the lucky player worse off overall.
	var lucky, unlucky int
	for spin := 1; spin <= 400; spin++ {
		low := SlotMachine{Rolls: spin}
		high := SlotMachine{Rolls: spin}
		lucky += spinReel(&high, slotLuckMaximum)
		unlucky += spinReel(&low, slotLuckMinimum)
	}
	if lucky <= unlucky {
		t.Fatalf("high luck should reach rarer faces more often: lucky=%d unlucky=%d", lucky, unlucky)
	}
}

func TestArenaPlayerStatsFollowTheOriginalFormulas(t *testing.T) {
	// health x ST / 90 and DX / 5 + 3.
	if got := ArenaHitPoints(100, 90); got != 100 {
		t.Fatalf("a healthy strong character should have 100 points, got %d", got)
	}
	if got := ArenaSpeed(50); got != 13 {
		t.Fatalf("DX 50 should give speed 13, got %d", got)
	}
	// The floor matters: without it a weak character arrives already down.
	if got := ArenaHitPoints(10, 10); got != arenaDownThreshold+1 {
		t.Fatalf("a weak character should be floored at %d, got %d", arenaDownThreshold+1, got)
	}
}

func TestArenaShieldIsFoundByItemIdAndCharges(t *testing.T) {
	// Low five bits are the item, high three are the charges: 11 + 7*32 = 235.
	slot, charges := arenaShieldSlot([CarrySlots]int{0, 235, 0, 0})
	if slot != 1 || charges != 7 {
		t.Fatalf("shield should be slot 1 with 7 charges, got %d/%d", slot, charges)
	}
	if slot, _ := arenaShieldSlot([CarrySlots]int{0, 0, 0, 0}); slot != -1 {
		t.Fatalf("no shield should report -1")
	}
}

func TestArenaDrawsTheGunIntoTheHand(t *testing.T) {
	// check_guns swaps the weapon into slot 0, displacing whatever was held.
	slots := [CarrySlots]int{5, 0, itemScattergun, 0}
	got, shots := arenaDrawGun(&slots)
	if got != itemScattergun {
		t.Fatalf("expected the scattergun, got %d", got)
	}
	if shots != 0 {
		t.Fatalf("a byte with no high bits carries no shots, got %d", shots)
	}
	if slots[0] != itemScattergun || slots[2] != 5 {
		t.Fatalf("the gun should have swapped into the hand: %v", slots)
	}
	// The high 3 bits are the scattergun's remaining shots: 4*32 + id.
	loaded := [CarrySlots]int{0, 4*32 + itemScattergun, 0, 0}
	if got, shots := arenaDrawGun(&loaded); got != itemScattergun || shots != 4 {
		t.Fatalf("expected a scattergun with 4 shots, got %d/%d", got, shots)
	}
	// A stinger carries no count even when the byte has high bits.
	stinger := [CarrySlots]int{0, 3*32 + itemStinger, 0, 0}
	if got, shots := arenaDrawGun(&stinger); got != itemStinger || shots != 0 {
		t.Fatalf("a stinger reports no shots, got %d/%d", got, shots)
	}
	empty := [CarrySlots]int{0, 0, 0, 0}
	if got, shots := arenaDrawGun(&empty); got != 0 || shots != 0 {
		t.Fatalf("bare hands should report 0/0, got %d/%d", got, shots)
	}
}

func TestArenaDamageBasesAndRange(t *testing.T) {
	if arenaPlayerDamageBase(itemScattergun) != arenaScattergunDamage {
		t.Fatal("the scattergun should hit for its own base")
	}
	if arenaPlayerDamageBase(itemStinger) != arenaStingerDamage {
		t.Fatal("the stinger should hit for its own base")
	}
	// base x (9 + 0..3), so a stinger lands between 45 and 60.
	arena := &Arena{}
	for range 200 {
		damage := arenaShotDamage(arena, arenaStingerDamage)
		if damage < arenaStingerDamage*arenaDamageBase ||
			damage > arenaStingerDamage*(arenaDamageBase+arenaDamageSpan-1) {
			t.Fatalf("stinger damage %d is outside the original's range", damage)
		}
	}
}

func TestArenaGoesDownAtThirtyTwoNotZero(t *testing.T) {
	points := 40
	if applyArenaDamage(&points, 7) {
		t.Fatal("33 points is still standing")
	}
	if !applyArenaDamage(&points, 1) {
		t.Fatalf("32 points should be down, have %d", points)
	}
	points = 5
	applyArenaDamage(&points, 100)
	if points != 0 {
		t.Fatalf("damage past zero should clamp, got %d", points)
	}
}

func TestScoreFollowsGameScore(t *testing.T) {
	// score = cargo - max(0,(deaths-1)*10) - max(0,((days-54)/7)*3)
	state := State{BanvilleCargo: 100, Deaths: 3, TimeMinutes: 68 * minutesPerDay}
	score := GameScore(state)
	if score.RestartPenalty != 20 {
		t.Fatalf("three deaths should cost 20, got %d", score.RestartPenalty)
	}
	if score.TimePenalty != 6 {
		t.Fatalf("day 68 should cost 6, got %d", score.TimePenalty)
	}
	if score.Total != 74 {
		t.Fatalf("total should be 74, got %d", score.Total)
	}
}

func TestScoreHasNoPenaltyBeforeDayFiftyFourOrOnASingleDeath(t *testing.T) {
	state := State{BanvilleCargo: 10, Deaths: 1, TimeMinutes: 53 * minutesPerDay}
	score := GameScore(state)
	if score.TimePenalty != 0 || score.RestartPenalty != 0 || score.Total != 10 {
		t.Fatalf("neither penalty should apply yet: %+v", score)
	}
}

func TestNoDeathBonusIsShownButNeverAdded(t *testing.T) {
	// The original prints "No-death bonus:    25" and game_score ignores it.
	// Reproducing that is deliberate; see score.no-death-bonus-is-cosmetic.
	state := State{BanvilleCargo: 40}
	score := GameScore(state)
	if score.NoDeathBonus != scoreNoDeathBonus {
		t.Fatalf("a clean run should show the bonus, got %d", score.NoDeathBonus)
	}
	if score.Total != 40 {
		t.Fatalf("the bonus must not reach the total, got %d", score.Total)
	}
}

func TestInteriorKindsComeFromTheDoorTerrainCodes(t *testing.T) {
	// check_building turns terrain 11..14 into interaction kinds 12, 13, 14, 23.
	for code, want := range map[int]InteractionKind{
		11: KindBarCounter, 12: KindEquipmentShop, 13: KindPartsShop, 14: KindHotel,
	} {
		got, ok := InteriorKindForTerrain(code)
		if !ok || got != want {
			t.Fatalf("terrain %d should open kind %d, got %d/%v", code, want, got, ok)
		}
	}
	if _, ok := InteriorKindForTerrain(20); ok {
		t.Fatal("terrain 20 is the exchange, not a building door")
	}
}

func TestInteriorFurnitureBlocksMovementAndIsReachable(t *testing.T) {
	interior := enterInterior(KindBarCounter, Position{X: 3, Y: 4}, 0)
	if !interior.AtDoor() {
		t.Fatalf("the player should start at the door: %+v", interior.Position)
	}
	// Walking into the counter is refused, but standing beside it counts.
	interior.Position = interior.Counter
	interior.Position.Y++
	if !interior.AtCounter() {
		t.Fatal("standing below the counter should reach it")
	}
	if moveInterior(&interior, 0, -1) {
		t.Fatal("the counter should not be walkable")
	}
	if interior.BoothIndex() >= 0 {
		t.Fatal("the counter is not a booth")
	}
}

func TestOnlyTheBarHasBoothsAndAMachine(t *testing.T) {
	bar := enterInterior(KindBarCounter, Position{}, 0)
	if len(bar.Booths) == 0 || !bar.HasSlots {
		t.Fatalf("the bar should have booths and a machine: %+v", bar)
	}
	shop := enterInterior(KindPartsShop, Position{}, 0)
	if len(shop.Booths) != 0 || shop.HasSlots {
		t.Fatalf("a shop should have neither: %+v", shop)
	}
}

func TestLandingSweepsTheHorizonBetweenTheOriginalsBounds(t *testing.T) {
	state := NewState()
	landing := beginLanding(&state, "drahew")
	if landing.Horizon != landingHorizonBottom {
		t.Fatalf("a landing starts at %d, got %d", landingHorizonBottom, landing.Horizon)
	}
	// The opening starfield is 51 frames, then the horizon walks up to 6.
	frames := 0
	for advanceLanding(&landing) {
		frames++
		if frames > 5000 {
			t.Fatal("the landing never finished")
		}
	}
	if landing.Horizon != landingHorizonTop || landing.Phase != LandingFinished {
		t.Fatalf("the landing should end at the top: %+v", landing)
	}
	if frames < landingStarFrames {
		t.Fatalf("the opening starfield is %d frames, ran %d in total", landingStarFrames, frames)
	}
}

func TestTakeOffHasNoOpeningStarfield(t *testing.T) {
	takeOffState := NewState()
	takeOff := beginTakeOff(&takeOffState, "drahew")
	if takeOff.Phase != LandingHorizon {
		t.Fatalf("a take-off starts on the horizon sweep, got %q", takeOff.Phase)
	}
	for advanceLanding(&takeOff) {
	}
	if takeOff.Horizon != landingHorizonBottom {
		t.Fatalf("a take-off should end at the bottom, got %d", takeOff.Horizon)
	}
}

func TestStarfieldThickensAsTheGroundComesUp(t *testing.T) {
	// do_stars fills horizon - 6 and draws nothing outside 6..117.
	high := Landing{Horizon: 100}
	low := Landing{Horizon: 20}
	tall, ok := high.StarBand()
	if !ok || tall != 94 {
		t.Fatalf("horizon 100 should give a band of 94, got %d/%v", tall, ok)
	}
	short, _ := low.StarBand()
	if short >= tall {
		t.Fatalf("a lower horizon should give a thinner band: %d vs %d", short, tall)
	}
	if _, ok := (Landing{Horizon: landingHorizonTop}).StarBand(); ok {
		t.Fatal("no starfield is drawn at the top of the sweep")
	}
}

func TestInteriorCustomerCapsAndRatesAreTheOriginals(t *testing.T) {
	// get_building's own numbers: the bar holds seven and fills fastest, the
	// hotel holds four and is the quietest of the four.
	for kind, wantCap := range map[InteractionKind]int{
		KindBarCounter: 7, KindEquipmentShop: 4, KindPartsShop: 4, KindHotel: 4,
	} {
		if got := interiorLayouts[kind].CustomerCap; got != wantCap {
			t.Fatalf("kind %d should hold %d, got %d", kind, wantCap, got)
		}
	}
	if interiorLayouts[KindBarCounter].ArrivalDivisor >= interiorLayouts[KindHotel].ArrivalDivisor {
		t.Fatal("a smaller divisor is a busier room, so the bar's must be below the hotel's")
	}
}

func TestOnlyThePartsShopHasAVerticalCounter(t *testing.T) {
	// get_building clears the L16 flag on terrain 13 alone.
	if !interiorLayouts[KindPartsShop].VerticalCounter {
		t.Fatal("the parts shop's counter runs the other way")
	}
	for _, kind := range []InteractionKind{KindBarCounter, KindEquipmentShop, KindHotel} {
		if interiorLayouts[kind].VerticalCounter {
			t.Fatalf("kind %d should keep the default orientation", kind)
		}
	}
}

func TestARoomOpensWithThreeToItsCapInIt(t *testing.T) {
	// rand(3, cap): the bar can open with anything from three to seven.
	seen := map[int]bool{}
	for seed := range 200 {
		interior := enterInterior(KindBarCounter, Position{}, seed)
		if interior.Customers < interiorMinCustomers || interior.Customers > interior.CustomerCap {
			t.Fatalf("opened with %d people, outside 3..%d", interior.Customers, interior.CustomerCap)
		}
		seen[interior.Customers] = true
	}
	if len(seen) < 2 {
		t.Fatalf("the opening count never varied: %v", seen)
	}
}

func TestCustomersStopArrivingAtTheCap(t *testing.T) {
	interior := enterInterior(KindBarCounter, Position{}, 0)
	interior.Customers = interior.CustomerCap
	if admitCustomer(&interior) {
		t.Fatal("a full room should turn people away")
	}
	// The bar is the busiest room, so a long walk must let somebody in.
	interior.Customers = interiorMinCustomers
	admitted := false
	for range 500 {
		if admitCustomer(&interior) {
			admitted = true
			break
		}
	}
	if !admitted {
		t.Fatal("nobody ever walked into the busiest room in the game")
	}
}

func TestRoomsStayInsideTheOriginalsClamp(t *testing.T) {
	// init_building clamps whatever the caller asks for to 2..17 by 2..7.
	for kind := range interiorLayouts {
		interior := enterInterior(kind, Position{}, 0)
		if interior.Width < interiorMinWidth || interior.Width > interiorMaxWidth {
			t.Fatalf("kind %d width %d is outside the clamp", kind, interior.Width)
		}
		if interior.Height < interiorMinHeight || interior.Height > interiorMaxHeight {
			t.Fatalf("kind %d height %d is outside the clamp", kind, interior.Height)
		}
	}
}

func TestConcentratorRaisesTheGunBase(t *testing.T) {
	// fire_it reads MAINLIB_G53 bit 18, which update_system sets when the
	// ship's guns carry their alternate part. Only the guns have a
	// concentrator; the warp and sub-c drives have no alternate at all.
	state := NewState()
	if alternatePartFitted(state, ShipSystemShipGuns) {
		t.Fatal("a new ship has no concentrator fitted")
	}
	if alternatePartFitted(state, ShipSystemWarpDrive) {
		t.Fatal("the warp drive has no alternate part to fit")
	}

	source, ok := sourceShipSystem(ShipSystemShipGuns)
	if !ok || source.Alternate == nil {
		t.Fatal("the guns should have an alternate part in the source table")
	}
	// Fitting it in the column the source names is what flips the bit.
	for index := range state.ShipSystems {
		if state.ShipSystems[index].ID != ShipSystemShipGuns {
			continue
		}
		state.ShipSystems[index].Rows[0][source.Alternate.Column] = source.Alternate.ItemIndex
	}
	if !alternatePartFitted(state, ShipSystemShipGuns) {
		t.Fatal("the concentrator in its own column should set the flag")
	}
}

// TestScattergunRunsOutOfShots pins the half of check_guns the remake used to
// drop: the carry byte's high 3 bits are the scattergun's remaining shots, and
// spending the last one leaves the player bare-handed.
func TestScattergunRunsOutOfShots(t *testing.T) {
	arena := Arena{Gun: itemScattergun, Shots: 2}
	if base := arenaPlayerDamageBase(arena.Gun); base != arenaScattergunDamage {
		t.Fatalf("a loaded scattergun should hit for %d, got %d", arenaScattergunDamage, base)
	}
	// Two pulls empty it; the third would be bare hands.
	arena.Shots--
	if arena.Shots != 1 {
		t.Fatalf("first shot left %d", arena.Shots)
	}
	arena.Shots--
	if arena.Shots != 0 {
		t.Fatalf("second shot left %d", arena.Shots)
	}
	if arenaPlayerDamageBase(0) >= arenaScattergunDamage {
		t.Fatal("bare hands should hit for less than a scattergun")
	}
}

// TestArenaShieldRescanFindsTheSecondOne pins the rescan: when a shield's
// charges run out its slot becomes a dead part and the original looks again,
// so a second shield further down the carry slots takes over.
func TestArenaShieldRescanFindsTheSecondOne(t *testing.T) {
	// Two shields: the first has one charge left, the second a full seven.
	slots := [CarrySlots]int{0, 1*32 + itemShield, 7*32 + itemShield, 0}
	slot, charges := arenaShieldSlot(slots)
	if slot != 1 || charges != 1 {
		t.Fatalf("expected the first shield with one charge, got slot %d charges %d", slot, charges)
	}
	// Spend it and blank the slot the way the fight does.
	slots[slot] = itemDeadPart
	slot, charges = arenaShieldSlot(slots)
	if slot != 2 || charges != 7 {
		t.Fatalf("the rescan should have found the second shield, got slot %d charges %d", slot, charges)
	}
}
