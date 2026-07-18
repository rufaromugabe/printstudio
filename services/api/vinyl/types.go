// Package vinyl contains PrintStudio's HTV / adhesive cut-policy core.
package vinyl

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	PathID   string   `json:"pathId,omitempty"`
}

type ReviewDecision string

const (
	ReviewAuto     ReviewDecision = "auto"
	ReviewSemiAuto ReviewDecision = "semi-auto"
	ReviewHuman    ReviewDecision = "human"
	ReviewBlocked  ReviewDecision = "blocked"
)

type ReviewFactor struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Points int    `json:"points"`
}

// ReviewScorecard is the operator rubric attached to a vinyl review.
type ReviewScorecard struct {
	Score     int             `json:"score"`
	Decision  ReviewDecision  `json:"decision"`
	Summary   string          `json:"summary"`
	Factors   []ReviewFactor  `json:"factors"`
	Material  MaterialProfile `json:"material"`
	HardStops []string        `json:"hardStops,omitempty"`
}

func HasErrors(diagnostics []Diagnostic) bool {
	for _, d := range diagnostics {
		if d.Severity == Error {
			return true
		}
	}
	return false
}
