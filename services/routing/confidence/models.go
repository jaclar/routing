package confidence

import (
	"time"

	"github.com/jaclar/routing-service/geo"
	"github.com/jaclar/routing-service/isochrone"
)

// RouteConfidence encapsulates the complete confidence analysis across both Strategy A (statistics) and Strategy B (4D members).
type RouteConfidence struct {
	OverallScore       float64             `json:"overall_score"`         // [0..100] Integrated primary confidence score
	Category           string              `json:"category"`              // "Very High" | "High" | "Moderate" | "Low" | "High Uncertainty"
	ScoreStrategyA     float64             `json:"score_strategy_a"`      // [0..100] Precomputed statistical score
	ScoreStrategyB     float64             `json:"score_strategy_b"`      // [0..100] 4D forward ensemble member simulation score
	AgreementScore     float64             `json:"agreement_score"`      // [0..100] Correlation / consistency between Strategy A and Strategy B
	ModelID            string              `json:"model_id"`              // Weather model evaluated (e.g. gefs_0p50, ifs_ens_0p25)
	NumMembers         int                 `json:"num_members"`           // Number of ensemble members evaluated
	Waypoints             []WaypointConfidence   `json:"waypoints"`                      // Per-waypoint confidence breakdown for timeline scrubber
	StatisticalComparison *StatisticalComparison `json:"statistical_comparison,omitempty"` // Theoretical metrics derived from Strategy A
	EnsembleComparison    *EnsembleComparison    `json:"ensemble_comparison,omitempty"`   // Detailed comparison metrics between member outcomes
}

// StatisticalComparison holds theoretical arrival metrics derived from Strategy A statistical error propagation.
type StatisticalComparison struct {
	MeanDurationHours float64 `json:"mean_duration_hours"`
	StdDurationHours  float64 `json:"std_duration_hours"`
	MinDurationHours  float64 `json:"min_duration_hours"` // P10 theoretical duration
	MaxDurationHours  float64 `json:"max_duration_hours"` // P90 theoretical duration
	IQRDurationHours  float64 `json:"iqr_duration_hours"` // Theoretical IQR (P75 - P25)
}

// WaypointConfidence contains point-in-time meteorological uncertainty and boat speed variance at a specific waypoint.
type WaypointConfidence struct {
	Index                  int       `json:"index"`
	Time                   time.Time `json:"time"`
	Score                  float64   `json:"score"`                     // [0..100] Combined score for scrubber
	ScoreStrategyA         float64   `json:"score_strategy_a"`          // [0..100]
	ScoreStrategyB         float64   `json:"score_strategy_b"`          // [0..100]
	WindSpeedMean          float64   `json:"wind_speed_mean_kts"`       // [knots]
	WindSpeedStd           float64   `json:"wind_speed_std_kts"`        // [knots]
	WindSpeedP10           float64   `json:"wind_speed_p10_kts"`        // [knots]
	WindSpeedP90           float64   `json:"wind_speed_p90_kts"`        // [knots]
	WindDirSpreadDeg       float64   `json:"wind_dir_spread_deg"`       // [degrees]
	GaleProbability        float64   `json:"gale_probability"`          // [0..1] P(wind >= 34 kt)
	StrongWindProbability  float64   `json:"strong_wind_probability"`   // [0..1] P(wind >= 25 kt)
	MemberSpeedMean        float64   `json:"member_speed_mean_kts"`     // [knots] SOG across members
	MemberSpeedStd         float64   `json:"member_speed_std_kts"`      // [knots]
	MemberSpeedP10         float64   `json:"member_speed_p10_kts"`      // [knots]
	MemberSpeedP90         float64   `json:"member_speed_p90_kts"`      // [knots]
}

// EnsembleComparison compares the distribution of passage outcomes across individual ensemble members.
type EnsembleComparison struct {
	MeanDurationHours float64         `json:"mean_duration_hours"`
	StdDurationHours  float64         `json:"std_duration_hours"`
	MinDurationHours  float64         `json:"min_duration_hours"`
	MaxDurationHours  float64         `json:"max_duration_hours"`
	IQRDurationHours  float64         `json:"iqr_duration_hours"` // P75 - P25 duration
	P10DurationHours  float64         `json:"p10_duration_hours"`
	P90DurationHours  float64         `json:"p90_duration_hours"`
	FastestMemberID   int             `json:"fastest_member_id"`
	SlowestMemberID   int             `json:"slowest_member_id"`
	MemberCount       int             `json:"member_count"`
	Members           []MemberOutcome `json:"members,omitempty"`
}

// MemberOutcome holds the simulated or solved passage metrics for a single ensemble member.
type MemberOutcome struct {
	MemberID           int                  `json:"member_id"`
	TotalDurationHours float64              `json:"total_duration_hours"`
	TotalDistanceNM    float64              `json:"total_distance_nm"`
	AverageSpeedKts    float64              `json:"average_speed_kts"`
	MaxWindKts         float64              `json:"max_wind_kts"`
	TotalTacks         int                  `json:"total_tacks"`
	DestinationReached bool                 `json:"destination_reached"`
	Waypoints          []isochrone.Waypoint `json:"waypoints,omitempty"`
	Trajectory         []geo.Point          `json:"trajectory,omitempty"` // Spatial coordinates along simulated or solved member route
}

// CategorizeConfidence assigns a human-readable confidence tier given a score from 0 to 100.
func CategorizeConfidence(score float64) string {
	switch {
	case score >= 85.0:
		return "Very High"
	case score >= 70.0:
		return "High"
	case score >= 50.0:
		return "Moderate"
	case score >= 35.0:
		return "Low"
	default:
		return "High Uncertainty"
	}
}
