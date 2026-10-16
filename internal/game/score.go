package game

// The original has no "you won" flag. It has a score, and MAINLIB:0x11
// game_score is twenty instructions long:
//
//	score = G41 - max(0, (G42-1) * 10) - max(0, ((G13-54)/7) * 3)
//
// G41 is cargo delivered to Banville, G42 is how many times the character has
// died, and G13 is elapsed time. XSTARTUP:0x0d show_score prints the three
// parts under the headings "Banville cargo:", "Restart penalty:" and "Time
// penalty:". All three rules carry byte signatures: score.formula,
// score.banville-gate and score.cargo-accumulation in
// docs/atari-st/ground-rules.json.
//
// One quirk is reproduced deliberately. When the character has never died the
// original prints "No-death bonus:    25" — but nothing ever adds those 25
// points. G41 has exactly two writers, both in XDOTRADI:0x1b do_selling, and
// neither adds a bonus; game_score does not add one either. The line is
// displayed and the total ignores it (rule score.no-death-bonus-is-cosmetic).

const (
	// scoreFreeDays is how long the player has before the clock starts costing
	// points: the formula subtracts 54 before dividing.
	scoreFreeDays = 54
	// scoreDaysPerPenalty and scorePenaltyPerWeek: three points per week late.
	scoreDaysPerPenalty = 7
	scorePenaltyPerWeek = 3
	// scorePenaltyPerDeath is what each death past the first costs.
	scorePenaltyPerDeath = 10
	// scoreNoDeathBonus is the number in the original's label. It is displayed,
	// never added — see the note above.
	scoreNoDeathBonus = 25
	// minutesPerDay converts the remake's clock to the original's day count.
	minutesPerDay = 24 * 60
)

// ScoreBreakdown is the final score with the parts the original shows.
type ScoreBreakdown struct {
	// BanvilleCargo is how many lots of cargo reached the colony.
	BanvilleCargo int
	// TimePenalty and RestartPenalty are positive numbers to subtract.
	TimePenalty    int
	RestartPenalty int
	// NoDeathBonus is the 25 the original displays when Deaths is zero. It is
	// not part of Total; see the note above.
	NoDeathBonus int
	// Total is what game_score returns.
	Total int
}

// ElapsedDays is the original's G13: how many days the character has been out.
func ElapsedDays(state State) int {
	if state.TimeMinutes <= 0 {
		return 0
	}
	return state.TimeMinutes / minutesPerDay
}

// GameScore applies MAINLIB:0x11 game_score to the remake's state.
func GameScore(state State) ScoreBreakdown {
	breakdown := ScoreBreakdown{BanvilleCargo: state.BanvilleCargo}

	late := ElapsedDays(state) - scoreFreeDays
	if late > 0 {
		breakdown.TimePenalty = late / scoreDaysPerPenalty * scorePenaltyPerWeek
	}
	if state.Deaths > 1 {
		breakdown.RestartPenalty = (state.Deaths - 1) * scorePenaltyPerDeath
	}
	if state.Deaths == 0 {
		breakdown.NoDeathBonus = scoreNoDeathBonus
	}
	breakdown.Total = breakdown.BanvilleCargo - breakdown.RestartPenalty - breakdown.TimePenalty
	return breakdown
}
