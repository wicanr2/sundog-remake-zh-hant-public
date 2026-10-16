package game

// XLANDFX is the fourteen procedures that put a ship on the ground and take it
// off again. It is presentation, but it is presentation with a decoded shape,
// so the state machine below follows the original rather than inventing one.
//
// do_landing runs 51 frames of nothing but starfield, then walks a horizon line
// from y=117 up to y=6, drawing the starfield and the horizon on every step.
// do_take_off is the same walk in reverse with no opening starfield. do_stars
// only draws while the horizon is between 6 and 117, and the band it fills is
// horizon - 6, so the sky thickens as the ground comes up. The city silhouette
// comes from the city's own map (init_city on the way down, city_up on the way
// up). Rules landing.descent, landing.ascent and landing.starfield in
// docs/atari-st/city-subsystems.json.

const (
	// landingHorizonBottom and landingHorizonTop are the two ends of the sweep.
	landingHorizonBottom = 117
	landingHorizonTop    = 6
	// landingStarFrames is the opening starfield, descent only.
	landingStarFrames = 51
)

// LandingPhase names where a landing or take-off has got to.
type LandingPhase string

const (
	LandingIdle     LandingPhase = ""
	LandingStars    LandingPhase = "stars"
	LandingHorizon  LandingPhase = "horizon"
	LandingFinished LandingPhase = "finished"
)

// Landing is a landing or take-off in progress.
type Landing struct {
	Phase LandingPhase `json:"phase"`
	// Descending is true for a landing, false for a take-off.
	Descending bool `json:"descending"`
	// Frame counts the opening starfield frames.
	Frame int `json:"frame"`
	// Horizon is the scan line the ground is currently at.
	Horizon int `json:"horizon"`
	// CityID is whose silhouette is on the horizon.
	CityID string `json:"cityId"`
	// Skyline is the six buildings XLANDFX:0x02 init_city generates for this
	// arrival or departure. See SkylineBuilding.
	Skyline []SkylineBuilding `json:"skyline"`
}

// The city on the horizon is generated, not drawn from a picture.
// XLANDFX:0x02 init_city lays out six buildings (rule landing.skyline):
//
//	x     = index × 26 + (random() and 15)
//	style = random() and 7
//	size  = (random() and 7) + 2
//
// The spacing is therefore fixed and the placement different every time. The
// style indexes the original's own height table below; the size roll is kept
// because the original also burns rolls after the shape (its colour loop), and
// dropping it would shift every later roll.
const (
	skylineBuildings = 6
	skylineSpacing   = 26
	skylineJitter    = 16
	skylineStyles    = 8
	skylineSizeBase  = 2
	// SkylineColumns is how many bars dbldngs draws per building; the table
	// rows carry an eighth byte that is always zero and never read.
	SkylineColumns = 7
)

// SkylineHeights is the original's 8-shape column-height table, read from the
// disk at raw 0x401c — inside the copy-protection track's data, loaded by
// SHIPLIB's opening moveblock (rule landing.skyline-column-heights;
// docs/atari-st/city-subsystems.json, skylineHeightTable). dbldngs draws each
// building as seven vertical bars whose heights these rows give; how many
// pixels one height unit is on screen is the remake's scaling, because the
// original paints a far-away strip inside its 165×116 viewport while the
// remake's landing overlay fills the frame.
var SkylineHeights = [skylineStyles][SkylineColumns]int{
	{1, 1, 3, 3, 5, 5, 5},
	{6, 6, 4, 2, 2, 4, 4},
	{1, 1, 3, 3, 3, 1, 1},
	{5, 5, 5, 5, 5, 5, 5},
	{4, 4, 4, 4, 3, 2, 2},
	{6, 6, 1, 1, 1, 5, 5},
	{2, 3, 4, 5, 6, 5, 4},
	{3, 5, 5, 5, 2, 2, 2},
}

// SkylineBuilding is one of the six.
type SkylineBuilding struct {
	X     int `json:"x"`
	Style int `json:"style"`
	Size  int `json:"size"`
}

// rollSkyline is init_city.
func rollSkyline(state *State) []SkylineBuilding {
	buildings := make([]SkylineBuilding, skylineBuildings)
	for index := range buildings {
		buildings[index] = SkylineBuilding{
			X:     index*skylineSpacing + combatRoll(state, skylineJitter),
			Style: combatRoll(state, skylineStyles),
			Size:  combatRoll(state, skylineStyles) + skylineSizeBase,
		}
	}
	return buildings
}

// beginLanding starts the descent: starfield first, then the horizon sweep.
func beginLanding(state *State, cityID string) Landing {
	return Landing{
		Phase:      LandingStars,
		Descending: true,
		Horizon:    landingHorizonBottom,
		CityID:     cityID,
		Skyline:    rollSkyline(state),
	}
}

// beginTakeOff starts the ascent. The original has no opening starfield here.
func beginTakeOff(state *State, cityID string) Landing {
	return Landing{
		Phase:      LandingHorizon,
		Descending: false,
		Horizon:    landingHorizonTop,
		CityID:     cityID,
		Skyline:    rollSkyline(state),
	}
}

// StarBand is how tall the starfield is at the current horizon, and whether it
// is drawn at all. do_stars fills horizon - 6 and skips the draw entirely when
// the horizon is outside 6..117.
func (l Landing) StarBand() (int, bool) {
	if l.Horizon <= landingHorizonTop || l.Horizon > landingHorizonBottom {
		return 0, false
	}
	return l.Horizon - landingHorizonTop, true
}

// advanceLanding steps the animation one frame and reports whether it is still
// running.
func advanceLanding(landing *Landing) bool {
	switch landing.Phase {
	case LandingStars:
		landing.Frame++
		if landing.Frame > landingStarFrames {
			landing.Phase = LandingHorizon
		}
		return true
	case LandingHorizon:
		if landing.Descending {
			landing.Horizon--
			if landing.Horizon <= landingHorizonTop {
				landing.Horizon = landingHorizonTop
				landing.Phase = LandingFinished
				return false
			}
			return true
		}
		landing.Horizon++
		if landing.Horizon >= landingHorizonBottom {
			landing.Horizon = landingHorizonBottom
			landing.Phase = LandingFinished
			return false
		}
		return true
	default:
		return false
	}
}
