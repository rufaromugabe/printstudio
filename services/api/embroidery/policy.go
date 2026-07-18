package embroidery

import (
	"fmt"
	"math"
	"strings"
)

// FabricClass selects manufacturer-backed density / underlay / pull defaults.
type FabricClass string

const (
	FabricWoven           FabricClass = "woven"
	FabricTShirt          FabricClass = "tshirt"
	FabricPique           FabricClass = "pique"
	FabricFleece          FabricClass = "fleece"
	FabricPerformanceKnit FabricClass = "performance-knit"
)

// FabricProfile is the operating baseline applied before stitch generation.
type FabricProfile struct {
	Class              FabricClass `json:"class"`
	Label              string      `json:"label"`
	DensityMM          float64     `json:"densityMm"`
	PullCompensationMM float64     `json:"pullCompensationMm"`
	PreferEdgeUnderlay bool        `json:"preferEdgeUnderlay"`
	PreferZigzag       bool        `json:"preferZigzag"`
	Notes              string      `json:"notes"`
}

// ReviewDecision is the auto / semi / human routing outcome.
type ReviewDecision string

const (
	ReviewAuto     ReviewDecision = "auto"
	ReviewSemiAuto ReviewDecision = "semi-auto"
	ReviewHuman    ReviewDecision = "human"
	ReviewBlocked  ReviewDecision = "blocked"
)

// ReviewScorecard is the human-digitizer rubric attached to a compile.
type ReviewScorecard struct {
	Score     int                `json:"score"`
	Decision  ReviewDecision     `json:"decision"`
	Summary   string             `json:"summary"`
	Factors   []ReviewFactor     `json:"factors"`
	Fabric    FabricProfile      `json:"fabric"`
	HardStops []string           `json:"hardStops,omitempty"`
}

type ReviewFactor struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Points int    `json:"points"`
}

func NormalizeFabric(class string) FabricClass {
	switch FabricClass(strings.ToLower(strings.TrimSpace(class))) {
	case FabricTShirt, "tee", "t-shirt":
		return FabricTShirt
	case FabricPique, "polo":
		return FabricPique
	case FabricFleece, "jumper":
		return FabricFleece
	case FabricPerformanceKnit, "performance", "sport", "knit":
		return FabricPerformanceKnit
	case FabricWoven, "cotton", "drill", "":
		return FabricWoven
	default:
		return FabricWoven
	}
}

func ProfileFor(class FabricClass) FabricProfile {
	switch NormalizeFabric(string(class)) {
	case FabricTShirt:
		return FabricProfile{Class: FabricTShirt, Label: "T-shirt knit", DensityMM: 0.40, PullCompensationMM: 0.35, PreferEdgeUnderlay: true, PreferZigzag: false, Notes: "Wilcom-class tee pull baseline; edge underlay on knits."}
	case FabricPique:
		return FabricProfile{Class: FabricPique, Label: "Pique polo", DensityMM: 0.42, PullCompensationMM: 0.35, PreferEdgeUnderlay: true, PreferZigzag: true, Notes: "Cut-away + topping recommended; more underlay than woven."}
	case FabricFleece:
		return FabricProfile{Class: FabricFleece, Label: "Fleece / jumper", DensityMM: 0.45, PullCompensationMM: 0.40, PreferEdgeUnderlay: true, PreferZigzag: true, Notes: "Highest sink-in risk; topping strongly preferred."}
	case FabricPerformanceKnit:
		return FabricProfile{Class: FabricPerformanceKnit, Label: "Performance knit", DensityMM: 0.48, PullCompensationMM: 0.35, PreferEdgeUnderlay: true, PreferZigzag: true, Notes: "Lighter density; increase underlay before densifying."}
	default:
		return FabricProfile{Class: FabricWoven, Label: "Stable woven", DensityMM: 0.40, PullCompensationMM: 0.20, PreferEdgeUnderlay: false, PreferZigzag: false, Notes: "40 wt baseline density 0.40 mm."}
	}
}

// ApplyFabricProfile fills missing density/underlay from the fabric class.
// Explicit non-zero spacing and already-enabled underlay flags are preserved.
func ApplyFabricProfile(regions []Region, class FabricClass) ([]Region, FabricProfile) {
	profile := ProfileFor(class)
	out := make([]Region, len(regions))
	for i, r := range regions {
		out[i] = r
		if out[i].SpacingMM <= 0 {
			out[i].SpacingMM = profile.DensityMM
		}
		if out[i].StitchLengthMM <= 0 {
			out[i].StitchLengthMM = 3
		}
		underlayUnset := !r.EdgeUnderlay && !r.CenterUnderlay && !r.ZigzagUnderlay
		if underlayUnset {
			switch out[i].Kind {
			case Satin:
				out[i].CenterUnderlay = true
				if profile.PreferZigzag {
					out[i].ZigzagUnderlay = true
				}
			case Tatami, Running:
				if profile.PreferEdgeUnderlay {
					out[i].EdgeUnderlay = true
				}
			}
		}
		// Knit / textured fabrics: strengthen underlay even when auto already set a base.
		if profile.PreferZigzag && out[i].Kind == Satin && out[i].CenterUnderlay {
			out[i].ZigzagUnderlay = true
		}
		if profile.PreferEdgeUnderlay && (out[i].Kind == Tatami || out[i].Kind == Running) {
			out[i].EdgeUnderlay = true
		}
	}
	return out, profile
}

func estimateSatinWidthMM(r Region) float64 {
	if r.Kind != Satin {
		return 0
	}
	if r.WidthMM > 0 {
		return r.WidthMM
	}
	if len(r.Geometry.Rings) == 0 {
		return 0
	}
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 0.4
	}
	angle := r.AngleDegrees * math.Pi / 180
	rotated := Polygon{Rings: make([][]Point, len(r.Geometry.Rings))}
	for i, ring := range r.Geometry.Rings {
		for _, p := range ring {
			rotated.Rings[i] = append(rotated.Rings[i], rotate(p, -angle))
		}
	}
	bounds := polygonBounds(rotated)
	// Prefer the scan axis that yields narrower columns (letter stems).
	maxA := maxScanWidth(rotated, bounds, spacing, true)
	maxB := maxScanWidth(rotated, bounds, spacing, false)
	if maxA <= 0 {
		return maxB
	}
	if maxB <= 0 {
		return maxA
	}
	return math.Min(maxA, maxB)
}

func maxScanWidth(p Polygon, b Bounds, spacing float64, horizontal bool) float64 {
	maxW := 0.0
	if horizontal {
		for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
			for _, seg := range scanlineSegments(p, y) {
				w := math.Abs(seg[1].X - seg[0].X)
				if w > maxW {
					maxW = w
				}
			}
		}
		return maxW
	}
	// Vertical scan: rotate 90° into x-scan space.
	turned := Polygon{Rings: make([][]Point, len(p.Rings))}
	for i, ring := range p.Rings {
		for _, q := range ring {
			turned.Rings[i] = append(turned.Rings[i], Point{X: q.Y, Y: -q.X})
		}
	}
	tb := polygonBounds(turned)
	for y := tb.MinY + spacing/2; y < tb.MaxY; y += spacing {
		for _, seg := range scanlineSegments(turned, y) {
			w := math.Abs(seg[1].X - seg[0].X)
			if w > maxW {
				maxW = w
			}
		}
	}
	return maxW
}

func estimateLetterHeightMM(r Region) float64 {
	b := polygonBounds(r.Geometry)
	h := b.MaxY - b.MinY
	if h <= 0 {
		return 0
	}
	return h
}

func densestSpacing(regions []Region) float64 {
	best := 0.0
	for _, r := range regions {
		if r.SpacingMM <= 0 {
			continue
		}
		if best == 0 || r.SpacingMM < best {
			best = r.SpacingMM
		}
	}
	return best
}

func dualUnderlay(r Region) bool {
	n := 0
	if r.EdgeUnderlay {
		n++
	}
	if r.CenterUnderlay {
		n++
	}
	if r.ZigzagUnderlay {
		n++
	}
	return n >= 2
}

// PolicyValidate adds fabric-aware hard rejects and density warnings.
func PolicyValidate(regions []Region, fabric FabricProfile) []Diagnostic {
	var out []Diagnostic
	add := func(s Severity, code, msg, region string) {
		out = append(out, Diagnostic{Severity: s, Code: code, Message: msg, RegionID: region})
	}
	for _, r := range regions {
		if r.Kind == Satin {
			width := estimateSatinWidthMM(r)
			if width > 10 {
				add(Error, "SATIN_TOO_WIDE", fmt.Sprintf("satin span %.1f mm exceeds 10 mm flat-goods auto limit; split columns or use tatami", width), r.ID)
			} else if width > 7 {
				add(Error, "SATIN_TOO_WIDE", fmt.Sprintf("satin span %.1f mm exceeds 7 mm long-stitch warning; split or declare a special process", width), r.ID)
			} else if width > 0 && width < 1.0 {
				add(Error, "SATIN_TOO_NARROW", fmt.Sprintf("satin span %.1f mm is too thin for automatic 40 wt satin", width), r.ID)
			}
			height := estimateLetterHeightMM(r)
			if height > 0 && height < 5 {
				add(Error, "TEXT_TOO_SMALL", fmt.Sprintf("feature height %.1f mm is below 5 mm; use run-stitch or a validated fine-thread path", height), r.ID)
			} else if height >= 5 && height < 6 {
				add(Warning, "TEXT_BORDERLINE", fmt.Sprintf("feature height %.1f mm is borderline for auto satin lettering", height), r.ID)
			}
		}
		if r.SpacingMM > 0 && (r.SpacingMM < 0.35 || r.SpacingMM > 0.55) {
			add(Warning, "DENSITY_OUT_OF_RANGE", fmt.Sprintf("density %.2f mm is outside the 0.35–0.55 mm production band for 40 wt", r.SpacingMM), r.ID)
		}
	}
	if fabric.Class == FabricPerformanceKnit {
		for _, r := range regions {
			if r.SpacingMM > 0 && r.SpacingMM < 0.40 {
				add(Error, "PERFORMANCE_TOO_DENSE", fmt.Sprintf("performance knit with density %.2f mm is too heavy; lighten fill and increase underlay", r.SpacingMM), r.ID)
			}
		}
	}
	return out
}

// ScoreReview implements the human-digitizer rubric from the embroidery research pack.
func ScoreReview(regions []Region, fabric FabricProfile, diagnostics []Diagnostic) ReviewScorecard {
	score := 0
	var factors []ReviewFactor
	add := func(code, label string, points int) {
		if points == 0 {
			return
		}
		score += points
		factors = append(factors, ReviewFactor{Code: code, Label: label, Points: points})
	}

	switch fabric.Class {
	case FabricPique:
		add("FABRIC_PIQUE", "Pique fabric", 10)
	case FabricFleece:
		add("FABRIC_FLEECE", "Fleece / jumper", 15)
	case FabricPerformanceKnit:
		add("FABRIC_PERFORMANCE", "Performance knit", 20)
	case FabricTShirt:
		add("FABRIC_TSHIRT", "T-shirt knit", 10)
	}

	if fabric.PreferZigzag || fabric.Class == FabricFleece || fabric.Class == FabricPique {
		add("TEXTURE_TOPPING", "Textured surface / topping risk", 10)
	}

	minText := math.Inf(1)
	maxSatin := 0.0
	needsDual := false
	for _, r := range regions {
		if h := estimateLetterHeightMM(r); h > 0 && h < minText {
			minText = h
		}
		if r.Kind == Satin {
			if w := estimateSatinWidthMM(r); w > maxSatin {
				maxSatin = w
			}
		}
		if dualUnderlay(r) {
			needsDual = true
		}
	}
	if !math.IsInf(minText, 1) {
		switch {
		case minText < 5:
			add("TEXT_LT_5", "Smallest text under 5 mm", 25)
		case minText < 6:
			add("TEXT_5_6", "Smallest text 5–5.9 mm", 15)
		case minText < 8:
			add("TEXT_6_8", "Smallest text 6–7.9 mm", 5)
		}
	}
	switch {
	case maxSatin > 10:
		add("SATIN_GT_10", "Longest satin over 10 mm", 30)
	case maxSatin > 7:
		add("SATIN_7_10", "Longest satin 7.1–10 mm", 20)
	case maxSatin > 5:
		add("SATIN_5_7", "Longest satin 5.1–7 mm", 5)
	}

	density := densestSpacing(regions)
	if density <= 0 {
		density = fabric.DensityMM
	}
	switch {
	case density < 0.35 || density > 0.55:
		add("DENSITY_EXTREME", "Density outside 0.35–0.55 mm", 10)
	case density < 0.40 || density > 0.50:
		add("DENSITY_EDGE", "Density near band edge", 5)
	}
	if needsDual {
		add("DUAL_UNDERLAY", "Dual underlay required", 5)
	}
	if fabric.Class == FabricFleece || fabric.Class == FabricPerformanceKnit {
		add("SUPPORT_STACK", "Topper + dual support stack", 10)
	}

	var hardStops []string
	blocked := false
	for _, d := range diagnostics {
		if d.Severity == Error {
			blocked = true
			hardStops = append(hardStops, d.Code)
		}
	}

	decision := ReviewAuto
	summary := "Auto-digitize allowed; sew-out still required before production."
	switch {
	case blocked:
		decision = ReviewBlocked
		summary = "Hard reject triggered — stop automatic route."
	case score >= 70:
		decision = ReviewBlocked
		summary = "Full human digitizing required before production release."
	case score >= 50:
		decision = ReviewHuman
		summary = "Human digitizer review required before production release."
	case score >= 25:
		decision = ReviewSemiAuto
		summary = "Semi-automated only — mandatory stitch-out review."
	}

	return ReviewScorecard{
		Score:     score,
		Decision:  decision,
		Summary:   summary,
		Factors:   factors,
		Fabric:    fabric,
		HardStops: hardStops,
	}
}
