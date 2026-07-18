package vinyl

import (
	"fmt"
	"math"
	"strings"
)

// MaterialClass selects HTV vs adhesive baselines and feature thresholds.
type MaterialClass string

const (
	MaterialHTVSmooth         MaterialClass = "htv-smooth"
	MaterialHTVFlock          MaterialClass = "htv-flock"
	MaterialHTVGlitter        MaterialClass = "htv-glitter"
	MaterialAdhesivePermanent MaterialClass = "adhesive-permanent"
	MaterialAdhesiveRemovable MaterialClass = "adhesive-removable"
	MaterialAdhesiveGlitter   MaterialClass = "adhesive-glitter"
)

// MaterialProfile is the operating baseline applied before cut export.
type MaterialProfile struct {
	Class           MaterialClass `json:"class"`
	Label           string        `json:"label"`
	Family          string        `json:"family"` // "htv" or "adhesive"
	MirrorDefault   bool          `json:"mirrorDefault"`
	WarnFeatureMM   float64       `json:"warnFeatureMm"`
	RejectFeatureMM float64       `json:"rejectFeatureMm"`
	Notes           string        `json:"notes"`
}

func NormalizeMaterial(class string) MaterialClass {
	switch MaterialClass(strings.ToLower(strings.TrimSpace(class))) {
	case MaterialHTVFlock, "flock", "stripflock":
		return MaterialHTVFlock
	case MaterialHTVGlitter, "glitter-htv", "htv glitter":
		return MaterialHTVGlitter
	case MaterialAdhesivePermanent, "651", "oracal-651", "permanent":
		return MaterialAdhesivePermanent
	case MaterialAdhesiveRemovable, "631", "oracal-631", "removable":
		return MaterialAdhesiveRemovable
	case MaterialAdhesiveGlitter, "851", "oracal-851", "glitter-adhesive":
		return MaterialAdhesiveGlitter
	case MaterialHTVSmooth, "easyweed", "smooth", "htv", "":
		return MaterialHTVSmooth
	default:
		return MaterialHTVSmooth
	}
}

func ProfileFor(class MaterialClass) MaterialProfile {
	switch NormalizeMaterial(string(class)) {
	case MaterialHTVFlock:
		return MaterialProfile{
			Class: MaterialHTVFlock, Label: "HTV flock", Family: "htv",
			MirrorDefault: true, WarnFeatureMM: 1.2, RejectFeatureMM: 0.8,
			Notes: "StripFlock Pro-class flock HTV. Higher force and slower send than smooth HTV. Mirror on. Reject micro-detail unless your threshold file proves it.",
		}
	case MaterialHTVGlitter:
		return MaterialProfile{
			Class: MaterialHTVGlitter, Label: "HTV glitter", Family: "htv",
			MirrorDefault: true, WarnFeatureMM: 1.5, RejectFeatureMM: 1.0,
			Notes: "Glitter HTV specialty lane. Mirror on. Use conservative minimum features; do not assume smooth-HTV-safe script sizes are safe.",
		}
	case MaterialAdhesivePermanent:
		return MaterialProfile{
			Class: MaterialAdhesivePermanent, Label: "Adhesive permanent (651-class)", Family: "adhesive",
			MirrorDefault: false, WarnFeatureMM: 1.0, RejectFeatureMM: 0.6,
			Notes: "ORACAL 651-class permanent calendared vinyl. Mirror off. Test-cut so face film and adhesive cut cleanly while barely scoring the liner. Weed soon; use transfer tape after weeding.",
		}
	case MaterialAdhesiveRemovable:
		return MaterialProfile{
			Class: MaterialAdhesiveRemovable, Label: "Adhesive removable (631-class)", Family: "adhesive",
			MirrorDefault: false, WarnFeatureMM: 1.0, RejectFeatureMM: 0.6,
			Notes: "ORACAL 631-class removable matte vinyl. Mirror off. Avoid deep liner cuts; transfer-tape tack and reverse burnishing matter more than for permanent film.",
		}
	case MaterialAdhesiveGlitter:
		return MaterialProfile{
			Class: MaterialAdhesiveGlitter, Label: "Adhesive glitter (851-class)", Family: "adhesive",
			MirrorDefault: false, WarnFeatureMM: 1.5, RejectFeatureMM: 1.0,
			Notes: "ORACAL 851-class glitter adhesive specialty lane. Mirror off. Treat like glitter HTV for feature severity until shop tests override thresholds.",
		}
	default:
		return MaterialProfile{
			Class: MaterialHTVSmooth, Label: "HTV smooth (EasyWeed-class)", Family: "htv",
			MirrorDefault: true, WarnFeatureMM: 1.0, RejectFeatureMM: 0.6,
			Notes: "Siser EasyWeed-class smooth PU HTV. Mirror on. Carrier-led weeding; start from the maker's smooth HTV preset and test-cut a small nested square first.",
		}
	}
}

type pathStats struct {
	ID      string
	MinAxis float64
	IsHole  bool
	HasArea bool
}

func ringArea(ring []Point) float64 {
	if len(ring) < 3 {
		return 0
	}
	sum := 0.0
	for i := 0; i < len(ring); i++ {
		p, q := ring[i], ring[(i+1)%len(ring)]
		sum += p.X*q.Y - q.X*p.Y
	}
	return sum / 2
}

func ringMinAxis(ring []Point) float64 {
	if len(ring) == 0 {
		return 0
	}
	minX, maxX := ring[0].X, ring[0].X
	minY, maxY := ring[0].Y, ring[0].Y
	for _, p := range ring[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	w, h := maxX-minX, maxY-minY
	if w < h {
		return w
	}
	return h
}

func analyzePaths(paths [][]Point) []pathStats {
	out := make([]pathStats, 0, len(paths))
	for i, ring := range paths {
		if len(ring) < 3 {
			continue
		}
		area := ringArea(ring)
		minAxis := ringMinAxis(ring)
		out = append(out, pathStats{
			ID:      fmt.Sprintf("path-%d", i+1),
			MinAxis: minAxis,
			IsHole:  area < 0, // Clipper2 / even-odd convention: holes opposite exterior winding
			HasArea: math.Abs(area) > 1e-9,
		})
	}
	return out
}

// PolicyValidate adds material-aware feature warnings and hard rejects.
func PolicyValidate(paths [][]Point, profile MaterialProfile) []Diagnostic {
	var out []Diagnostic
	add := func(s Severity, code, msg, pathID string) {
		out = append(out, Diagnostic{Severity: s, Code: code, Message: msg, PathID: pathID})
	}
	stats := analyzePaths(paths)
	for _, st := range stats {
		if !st.HasArea || st.MinAxis <= 0 {
			continue
		}
		if st.MinAxis < profile.RejectFeatureMM {
			add(Error, "FEATURE_TOO_SMALL",
				fmt.Sprintf("detail %.2f mm is below the %.1f mm hard-stop for %s", st.MinAxis, profile.RejectFeatureMM, profile.Label),
				st.ID)
		} else if st.MinAxis < profile.WarnFeatureMM {
			add(Warning, "FEATURE_BORDERLINE",
				fmt.Sprintf("detail %.2f mm is below the %.1f mm warn threshold for %s", st.MinAxis, profile.WarnFeatureMM, profile.Label),
				st.ID)
		}
		if st.IsHole && st.MinAxis < profile.WarnFeatureMM {
			add(Warning, "COUNTER_RISK",
				fmt.Sprintf("interior cutout %.2f mm may lift or tear during weeding on %s", st.MinAxis, profile.Label),
				st.ID)
		}
	}
	return out
}

// ScoreReview implements the vinyl research-pack rubric.
func ScoreReview(paths [][]Point, profile MaterialProfile, diagnostics []Diagnostic, mirrored bool) ReviewScorecard {
	score := 0
	var factors []ReviewFactor
	add := func(code, label string, points int) {
		if points == 0 {
			return
		}
		score += points
		factors = append(factors, ReviewFactor{Code: code, Label: label, Points: points})
	}

	switch profile.Class {
	case MaterialHTVFlock:
		add("MATERIAL_FLOCK", "Flock HTV specialty", 15)
	case MaterialHTVGlitter, MaterialAdhesiveGlitter:
		add("MATERIAL_GLITTER", "Glitter specialty", 20)
	case MaterialAdhesiveRemovable:
		add("MATERIAL_REMOVABLE", "Removable adhesive transfer sensitivity", 5)
	}

	if profile.Family == "htv" && !mirrored {
		add("MIRROR_OFF_HTV", "HTV sent without mirror", 15)
	}
	if profile.Family == "adhesive" && mirrored {
		add("MIRROR_ON_ADHESIVE", "Adhesive vinyl mirrored (unusual for front application)", 10)
	}

	stats := analyzePaths(paths)
	minFeature := math.Inf(1)
	holeCount := 0
	tinyHoles := 0
	for _, st := range stats {
		if st.MinAxis > 0 && st.MinAxis < minFeature {
			minFeature = st.MinAxis
		}
		if st.IsHole {
			holeCount++
			if st.MinAxis < profile.WarnFeatureMM {
				tinyHoles++
			}
		}
	}
	if !math.IsInf(minFeature, 1) {
		switch {
		case minFeature < profile.RejectFeatureMM:
			add("FEATURE_HARD_STOP", "Feature below hard-stop threshold", 30)
		case minFeature < profile.WarnFeatureMM:
			add("FEATURE_WARN", "Feature below warn threshold", 15)
		case minFeature < profile.WarnFeatureMM*1.25:
			add("FEATURE_NEAR", "Feature near warn threshold", 5)
		}
	}
	if holeCount > 0 {
		add("HAS_COUNTERS", "Interior counters present", 5)
	}
	if tinyHoles > 0 {
		add("TINY_COUNTERS", "Small counters under warn threshold", 15)
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
	summary := "Auto cut export allowed; test-cut still required before production."
	switch {
	case blocked:
		decision = ReviewBlocked
		summary = "Hard reject triggered — enlarge detail or change material class before cut export."
	case score >= 70:
		decision = ReviewBlocked
		summary = "Full human weed/cut review required before production release."
	case score >= 50:
		decision = ReviewHuman
		summary = "Operator review required before production release."
	case score >= 25:
		decision = ReviewSemiAuto
		summary = "Semi-automated only — mandatory test-cut and weed check."
	}

	return ReviewScorecard{
		Score:     score,
		Decision:  decision,
		Summary:   summary,
		Factors:   factors,
		Material:  profile,
		HardStops: hardStops,
	}
}

// Review runs material policy validation and scoring for cleaned cut paths.
func Review(materialClass string, paths [][]Point, mirrored bool) (MaterialProfile, []Diagnostic, ReviewScorecard) {
	profile := ProfileFor(NormalizeMaterial(materialClass))
	diagnostics := PolicyValidate(paths, profile)
	review := ScoreReview(paths, profile, diagnostics, mirrored)
	return profile, diagnostics, review
}
