package game

import (
	"testing"

	"github.com/wicanr2/sundog-remake/internal/world"
)

// encounter.which: r = random() mod 100; a += G33 div 2; b += a; c += b;
// r < a -> 16, r < b -> 18, r < c -> 22, otherwise 0.
func TestWhichStreetKindWalksTheCumulativeWeights(t *testing.T) {
	weights := [3]int{5, 5, 15} // ground code 2
	cases := []struct {
		roll int
		want int
	}{
		{0, StreetKindMugger},
		{4, StreetKindMugger},
		{5, StreetKindBeggar},
		{9, StreetKindBeggar},
		{10, StreetKindDealer},
		{24, StreetKindDealer},
		{25, 0},
		{99, 0},
	}
	for _, tc := range cases {
		if got := whichStreetKind(tc.roll, weights, 0); got != tc.want {
			t.Fatalf("roll %d gave kind %d, want %d", tc.roll, got, tc.want)
		}
	}
}

// The cash bias only touches the first weight, so money buys a player more
// muggers and fewer of everything else. G33 is the thousands.
func TestCashWidensTheMuggerBandAndNarrowsTheRest(t *testing.T) {
	weights := [3]int{5, 5, 15}
	if got := whichStreetKind(9, weights, 10); got != StreetKindMugger {
		t.Fatalf("a walker carrying 10000 Cr got kind %d at roll 9, want a mugger", got)
	}
	if got := whichStreetKind(9, weights, 0); got != StreetKindBeggar {
		t.Fatalf("an empty purse got kind %d at roll 9, want a beggar", got)
	}
	// The band the dealer occupies is the same width either way, just moved.
	if got := whichStreetKind(29, weights, 10); got != StreetKindDealer {
		t.Fatalf("roll 29 with 10000 Cr gave kind %d, want a dealer", got)
	}
	if got := whichStreetKind(30, weights, 10); got != 0 {
		t.Fatalf("roll 30 with 10000 Cr gave kind %d, want nothing", got)
	}
}

// encounter.type-confirm: a matching kind is cancelled when random() mod 20
// falls under the system's threshold, so a higher threshold means fewer.
func TestConfirmCancelsOnlyTheKindItNames(t *testing.T) {
	if got := confirmStreetKind(StreetKindMugger, StreetKindMugger, 5, 12); got != 0 {
		t.Fatalf("roll 5 under threshold 12 left kind %d, want it cancelled", got)
	}
	if got := confirmStreetKind(StreetKindMugger, StreetKindMugger, 12, 12); got != StreetKindMugger {
		t.Fatalf("roll 12 at threshold 12 cancelled the mugger, want him kept")
	}
	if got := confirmStreetKind(StreetKindBeggar, StreetKindMugger, 0, 20); got != StreetKindBeggar {
		t.Fatalf("the mugger's threshold cancelled a beggar")
	}
}

// The three thresholds are nibbles 3, 4 and 5 of the star system record, which
// the remake already carries as three named fields.
func TestStreetThresholdsAreTheSystemsOwnNibbles(t *testing.T) {
	system := world.System{EncounterRating: 12, PriceLevel: 8, StreetDealRating: 14}
	want := [3]int{12, 8, 14}
	if got := streetThresholds(system); got != want {
		t.Fatalf("thresholds %v, want %v", got, want)
	}
}

// Jondd's record is the one worked through in ground-rules.md: n3 12, n4 8,
// n5 14. The world the remake ships has to agree, or every threshold above is
// reading the wrong field.
func TestJonddCarriesTheDecodedThresholds(t *testing.T) {
	registry, err := world.NewSourceRegistry()
	if err != nil {
		t.Fatalf("load world: %v", err)
	}
	system, ok := registry.System("jondd")
	if !ok {
		t.Fatal("no system jondd")
	}
	if got := streetThresholds(system); got != [3]int{12, 8, 14} {
		t.Fatalf("jondd thresholds %v, want [12 8 14]", got)
	}
}

// encounter.type-table: one row of weights per ground code, and index 0x0f
// makes no call at all. Building bytes are biased down by 16 first.
func TestStreetWeightsFollowTheJumpTable(t *testing.T) {
	if got, ok := streetWeightsFor(0x03); !ok || got != [3]int{40, 40, 20} {
		t.Fatalf("code 3 gave %v ok=%v, want [40 40 20]", got, ok)
	}
	if got, ok := streetWeightsFor(0x09); !ok || got != [3]int{8, 10, 15} {
		t.Fatalf("code 9 gave %v ok=%v, want the shared 6-9 row", got, ok)
	}
	if _, ok := streetWeightsFor(0x0f); ok {
		t.Fatal("code 15 produced weights, but the original makes no call there")
	}
	// A building byte of 34 biases down to 18, which is the row [10 40 40].
	if got, ok := streetWeightsFor(34); !ok || got != [3]int{10, 40, 40} {
		t.Fatalf("byte 34 gave %v ok=%v, want row 18", got, ok)
	}
	if _, ok := streetWeightsFor(0x40); ok {
		t.Fatal("a byte past the table produced weights")
	}
}

// XDOINTER:0x26 start_interaction: each kind has one opening frame.
func TestStreetFramesAreTheOriginalsDispatch(t *testing.T) {
	cases := map[int]int{
		StreetKindMugger: 0x41,
		StreetKindBeggar: 0x31,
		StreetKindDealer: 0x21,
	}
	for kind, want := range cases {
		got, ok := StreetFrame(kind)
		if !ok || got != want {
			t.Fatalf("kind %d gave frame 0x%02x ok=%v, want 0x%02x", kind, got, ok, want)
		}
	}
	if _, ok := StreetFrame(17); ok {
		t.Fatal("kind 17 is a shop counter, not a street encounter")
	}
}

// check_building claims the tile before anything is rolled, so a player
// standing on a building's door is never accosted.
func TestABuildingCancelsTheEncounter(t *testing.T) {
	system := world.System{ID: "jondd", EncounterRating: 0, PriceLevel: 0, StreetDealRating: 0}
	found := false
	for step := 0; step < 400; step++ {
		state := State{}
		state.StreetRolls = step
		if resolveStreetEncounter(&state, system, 0x03, false) != 0 {
			found = true
		}
		state.StreetRolls = step
		if kind := resolveStreetEncounter(&state, system, 0x03, true); kind != 0 {
			t.Fatalf("a building tile still produced kind %d", kind)
		}
	}
	if !found {
		t.Fatal("no encounter happened in 400 steps, so the building check proves nothing")
	}
}

// The jammed system is the original's own doubling: it adds a second trigger
// roll, so encounters there are twice as common as anywhere else.
func TestTheJammedSystemDoublesTheTriggerAndForcesMuggers(t *testing.T) {
	const steps = 4000
	count := func(id string) (encounters, muggers int) {
		state := State{SystemID: id}
		system := world.System{ID: id, EncounterRating: 0, PriceLevel: 0, StreetDealRating: 0}
		for step := 0; step < steps; step++ {
			if kind := resolveStreetEncounter(&state, system, 0x0a, false); kind != 0 {
				encounters++
				if kind == StreetKindMugger {
					muggers++
				}
			}
		}
		return
	}
	quiet, _ := count("jondd")
	jammed, jammedMuggers := count(commJammedSystemID)
	if jammed <= quiet {
		t.Fatalf("the jammed system produced %d encounters against %d elsewhere, want more", jammed, quiet)
	}
	// Ground code 0x0a weights the three kinds equally, so without the rewrite
	// a third of them would be muggers.
	if jammedMuggers*3 <= jammed*2 {
		t.Fatalf("only %d of %d encounters in the jammed system were muggers, want nearly all", jammedMuggers, jammed)
	}
}

// The draw sequence is the remake's own substitution for the original's RNG,
// and it has to replay: the same save walking the same way sees the same
// street.
func TestTheStreetSequenceReplays(t *testing.T) {
	system := world.System{ID: "jondd", EncounterRating: 4, PriceLevel: 4, StreetDealRating: 4}
	run := func() []int {
		state := State{}
		kinds := make([]int, 0, 200)
		for step := 0; step < 200; step++ {
			kinds = append(kinds, resolveStreetEncounter(&state, system, 0x05, false))
		}
		return kinds
	}
	first, second := run(), run()
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("step %d gave %d then %d", index, first[index], second[index])
		}
	}
}

// A mugger says his line before he draws: xdo_combat calls bring_up_window
// before it sets the arena up. Walking away from that conversation is where the
// fight starts, and the crowd is the rand(2,6) the original picks for kind 16.
func TestLeavingAMuggersConversationOpensTheArena(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenDialog
	state.GroundReturn = ScreenGround
	state.StreetPending = StreetKindMugger
	next, err := reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if next.Screen != ScreenArena || !next.Arena.Active {
		t.Fatalf("walking away from a mugger left screen %q, arena active=%v", next.Screen, next.Arena.Active)
	}
	// A mugging draws rand(2,6) *records*, one of which is the player.
	if count := len(next.Arena.Thugs); count < arenaMinRecords-1 || count > arenaMaxRecords-1 {
		t.Fatalf("the mugging drew %d opponents, want %d..%d", count, arenaMinRecords-1, arenaMaxRecords-1)
	}
	if next.StreetPending != 0 {
		t.Fatal("the pending encounter survived the fight starting")
	}
}

// Walking away from a beggar or a dealer is just walking away.
func TestLeavingAPeacefulConversationDoesNotStartAFight(t *testing.T) {
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	state := NewState()
	state.Screen = ScreenDialog
	state.GroundReturn = ScreenGround
	state.StreetAsk = 40
	next, err := reducer.Step(state, ActionBack)
	if err != nil {
		t.Fatal(err)
	}
	if next.Screen == ScreenArena || next.Arena.Active {
		t.Fatal("leaving a beggar started a fight")
	}
	if next.StreetAsk != 0 {
		t.Fatal("the beggar's figure outlived his conversation")
	}
}

// TestBanvilleTurnsAwayMuggersAndBeggars pins the city gate the original
// applies right after the three confirms: MAINLIB_G2a field (6,9) against
// 0x3c, city 60, the colony. Dealers are not on that list.
func TestBanvilleTurnsAwayMuggersAndBeggars(t *testing.T) {
	// Thresholds at 0 so the three confirms never cancel anything: this test is
	// about the city gate that comes after them.
	system := world.System{ID: world.StartSystemID, EncounterRating: 0, PriceLevel: 0, StreetDealRating: 0}
	// Walk enough steps in each city to see what the street produces. Banville
	// must never turn up a mugger or a beggar; the start city, with the same
	// walk, must turn up something — otherwise this test would pass on a
	// street that simply never fires.
	sample := func(cityID string) (muggers, beggars, any int) {
		state := NewState()
		state.CityID = cityID
		for step := 0; step < 4000; step++ {
			switch resolveStreetEncounter(&state, system, 2, false) {
			case StreetKindMugger:
				muggers++
				any++
			case StreetKindBeggar:
				beggars++
				any++
			case StreetKindDealer:
				any++
			}
		}
		return muggers, beggars, any
	}
	if m, b, _ := sample(world.MissionDestinationID); m != 0 || b != 0 {
		t.Fatalf("Banville produced %d muggers and %d beggars", m, b)
	}
	m, b, _ := sample(world.StartDestinationID)
	if m == 0 && b == 0 {
		t.Fatal("the start city produced neither, so the Banville result proves nothing")
	}
}

// The pixel walk runs the original's own loop, so the trigger is the
// original's: random() mod 1000 on every pass. The grid walk has no such loop
// and keeps its own per-step rate, which is why the two are separate numbers.
func TestTheRealTimeTriggerUsesTheOriginalsOdds(t *testing.T) {
	if streetTriggerRealTime != 1000 {
		t.Fatalf("即時觸發是 1/%d，原版是 1/1000", streetTriggerRealTime)
	}
	reducer, err := NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	system, ok := reducer.World.System(world.StartSystemID)
	if !ok {
		t.Fatal("找不到起始星系")
	}
	const ticks = 40000
	state := NewState()
	hits := 0
	for i := 0; i < ticks; i++ {
		state.StreetPending = 0
		if resolveStreetEncounterAt(&state, system, 0x05, false, streetTriggerRealTime) != 0 {
			hits++
		}
	}
	// A tenth of a percent, give or take: what the test holds is the order of
	// magnitude, not the sequence — the roller is the remake's.
	if hits < ticks/2000 || hits > ticks/300 {
		t.Fatalf("%d 次判定觸發 %d 次，偏離 1/1000 太遠", ticks, hits)
	}
	// The per-step rate the grid walk uses has to stay far higher, or a walk
	// there would never meet anybody.
	stepHits := 0
	state = NewState()
	for i := 0; i < 4000; i++ {
		if resolveStreetEncounterAt(&state, system, 0x05, false, streetTriggerIn) != 0 {
			stepHits++
		}
	}
	if stepHits <= hits {
		t.Fatalf("逐格的觸發 %d 次不比逐幀的 %d 次多", stepHits, hits)
	}
}
