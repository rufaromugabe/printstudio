package embroidery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

const CompilerVersion = "0.3.0"

func Compile(regions []Region, profile MachineProfile) (Document, error) {
	return CompileWithFabric(regions, profile, FabricWoven)
}

func CompileWithFabric(regions []Region, profile MachineProfile, fabricClass FabricClass) (Document, error) {
	if profile.ID == "" {
		profile = DefaultProfile()
	}
	minStitch := profile.MinStitchMM
	if minStitch <= 0 {
		minStitch = .4
	}
	regions, fabric := ApplyFabricProfile(regions, fabricClass)
	source, _ := json.Marshal(struct {
		Regions []Region     `json:"regions"`
		Fabric  FabricClass  `json:"fabric"`
	}{Regions: regions, Fabric: fabric.Class})
	sum := sha256.Sum256(source)
	d := Document{Version: SchemaVersion, Units: "mm", SourceHash: hex.EncodeToString(sum[:]), CompilerVersion: CompilerVersion, Machine: profile, Fabric: fabric, Regions: regions}
	for _, r := range regions {
		if r.ID == "" {
			return d, fmt.Errorf("region ID is required")
		}
		if err := r.ValidateGeometry(); err != nil {
			return d, fmt.Errorf("region %s: %w", r.ID, err)
		}
		block := Block{ID: "block-" + r.ID, RegionID: r.ID, ThreadID: r.ThreadID, Kind: r.Kind, Bounds: polygonBounds(r.Geometry)}
		var err error
		if r.EdgeUnderlay {
			block.Underlay = runningPath(r.Geometry.Rings[0], max(r.StitchLengthMM, 2.5), "edge_underlay")
		}
		switch r.Kind {
		case Running:
			block.Stitches = runningPath(r.Geometry.Rings[0], max(r.StitchLengthMM, 2.5), "running")
		case Tatami:
			block.Stitches, err = tatami(r, minStitch)
			if err != nil && isEmptyFill(err) {
				// Micro-islands from image tracing are often thinner than row spacing.
				// Outline them instead of failing the whole design.
				block.Stitches = runningPath(r.Geometry.Rings[0], max(r.StitchLengthMM, minStitch), "tatami_fallback_running")
				block.Kind = Running
				d.Diagnostics = append(d.Diagnostics, Diagnostic{
					Severity: Warning, Code: "TATAMI_FALLBACK_RUNNING", RegionID: r.ID,
					Message: "fill produced no stitches; used running outline for a sub-density region",
				})
				err = nil
			}
		case Satin:
			block.Underlay, block.Stitches, err = satin(r, profile)
		default:
			err = fmt.Errorf("unsupported stitch kind %q", r.Kind)
		}
		if err != nil {
			return d, fmt.Errorf("region %s: %w", r.ID, err)
		}
		block.Underlay = pruneShortStitches(block.Underlay, minStitch)
		// Satin rungs are width pairs; never drop them for length. Running/tatami are safe to prune.
		if r.Kind != Satin {
			block.Stitches = pruneShortStitches(block.Stitches, minStitch)
		}
		all := append(append([]Stitch{}, block.Underlay...), block.Stitches...)
		if len(all) == 0 {
			d.Diagnostics = append(d.Diagnostics, Diagnostic{
				Severity: Warning, Code: "REGION_SKIPPED_EMPTY", RegionID: r.ID,
				Message: "region produced no sewable stitches and was skipped",
			})
			continue
		}
		block.Entry = all[0].Position
		block.Exit = all[len(all)-1].Position
		d.Plan = append(d.Plan, block)
	}
	d.Plan = routePlan(d.Plan)
	d.Diagnostics = append(d.Diagnostics, Validate(d)...)
	d.Diagnostics = append(d.Diagnostics, PolicyValidate(d.Regions, fabric)...)
	review := ScoreReview(d.Regions, fabric, d.Diagnostics)
	d.Review = review
	if review.Decision == ReviewHuman {
		d.Diagnostics = append(d.Diagnostics, Diagnostic{Severity: Warning, Code: "HUMAN_DIGITIZER_REQUIRED", Message: review.Summary})
	}
	if review.Decision == ReviewSemiAuto {
		d.Diagnostics = append(d.Diagnostics, Diagnostic{Severity: Warning, Code: "SEMI_AUTO_REVIEW", Message: review.Summary})
	}
	if review.Decision == ReviewBlocked && !HasErrors(d.Diagnostics) {
		d.Diagnostics = append(d.Diagnostics, Diagnostic{Severity: Error, Code: "HUMAN_DIGITIZER_REQUIRED", Message: review.Summary})
	}
	// JSON encodes nil slices as null; keep empty arrays for stable clients.
	if d.Diagnostics == nil {
		d.Diagnostics = []Diagnostic{}
	}
	if d.Plan == nil {
		d.Plan = []Block{}
	}
	for i := range d.Plan {
		if d.Plan[i].Underlay == nil {
			d.Plan[i].Underlay = []Stitch{}
		}
		if d.Plan[i].Stitches == nil {
			d.Plan[i].Stitches = []Stitch{}
		}
	}
	return d, nil
}

func runningPath(ring []Point, length float64, source string) []Stitch {
	if length <= 0 {
		length = 2.5
	}
	samples := resampleClosed(ring, length)
	if len(samples) < 2 {
		return nil
	}
	out := make([]Stitch, 0, len(samples)+1)
	for _, p := range samples {
		out = append(out, Stitch{Position: p, Command: CommandStitch, Source: source})
	}
	// Close the contour so underlay/running returns to the start.
	if distance(samples[0], samples[len(samples)-1]) >= 1e-6 {
		out = append(out, Stitch{Position: samples[0], Command: CommandStitch, Source: source})
	}
	return out
}

func tatami(r Region, minStitch float64) ([]Stitch, error) {
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = .4
	}
	length := r.StitchLengthMM
	if length <= 0 {
		length = 3
	}
	if minStitch <= 0 {
		minStitch = .4
	}
	// Never place fill stitches shorter than the machine minimum.
	if length < minStitch {
		length = minStitch
	}
	angle := r.AngleDegrees * math.Pi / 180
	inv := -angle
	rotated := Polygon{Rings: make([][]Point, len(r.Geometry.Rings))}
	for i, ring := range r.Geometry.Rings {
		for _, p := range ring {
			rotated.Rings[i] = append(rotated.Rings[i], rotate(p, inv))
		}
	}
	b := polygonBounds(rotated)
	var out []Stitch
	row := 0
	current := Point{}
	haveCurrent := false
	for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
		segments := scanlineSegments(rotated, y)
		// Prefer nearest remaining segment within the row (biased travel),
		// then reverse stitch direction so exits stay near the next entry.
		remaining := append([][2]Point(nil), segments...)
		for len(remaining) > 0 {
			best := 0
			bestScore := math.Inf(1)
			for i, s := range remaining {
				a, z := s[0], s[1]
				score := 0.0
				if haveCurrent {
					da, dz := distance(current, a), distance(current, z)
					if dz < da {
						score = dz
					} else {
						score = da
					}
				} else {
					score = a.X
					if row%2 == 1 {
						score = -a.X
					}
				}
				if score < bestScore {
					bestScore, best = score, i
				}
			}
			s := remaining[best]
			remaining = append(remaining[:best], remaining[best+1:]...)
			a, z := s[0], s[1]
			if haveCurrent && distance(current, z) < distance(current, a) {
				a, z = z, a
			} else if !haveCurrent && row%2 == 1 {
				a, z = z, a
			}
			a, z = rotate(a, angle), rotate(z, angle)
			if distance(a, z) < minStitch {
				continue
			}
			if haveCurrent {
				out = append(out, Stitch{Position: a, Command: CommandJump, Source: "tatami_travel"})
			} else {
				out = append(out, Stitch{Position: a, Command: CommandStitch, Source: "tatami"})
			}
			for _, p := range interpolate(a, z, length) {
				out = append(out, Stitch{Position: p, Command: CommandStitch, Source: "tatami"})
			}
			current, haveCurrent = z, true
		}
		row++
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	return out, nil
}

func isEmptyFill(err error) bool {
	return err != nil && err.Error() == "fill produced no stitches"
}

func max(v, fallback float64) float64 {
	if v > 0 {
		return v
	}
	return fallback
}
