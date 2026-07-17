package embroidery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

const CompilerVersion = "0.1.0"

func Compile(regions []Region, profile MachineProfile) (Document, error) {
	if profile.ID == "" {
		profile = DefaultProfile()
	}
	source, _ := json.Marshal(regions)
	sum := sha256.Sum256(source)
	d := Document{Version: SchemaVersion, Units: "mm", SourceHash: hex.EncodeToString(sum[:]), CompilerVersion: CompilerVersion, Machine: profile, Regions: regions}
	for _, r := range regions {
		if r.ID == "" {
			return d, fmt.Errorf("region ID is required")
		}
		if err := r.Geometry.Validate(); err != nil {
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
			block.Stitches, err = tatami(r)
		case Satin:
			block.Underlay, block.Stitches, err = satin(r, profile)
		default:
			err = fmt.Errorf("unsupported stitch kind %q", r.Kind)
		}
		if err != nil {
			return d, fmt.Errorf("region %s: %w", r.ID, err)
		}
		all := append(append([]Stitch{}, block.Underlay...), block.Stitches...)
		if len(all) > 0 {
			block.Entry = all[0].Position
			block.Exit = all[len(all)-1].Position
		}
		d.Plan = append(d.Plan, block)
	}
	d.Plan = routePlan(d.Plan)
	d.Diagnostics = Validate(d)
	return d, nil
}

func runningPath(ring []Point, length float64, source string) []Stitch {
	r := ringClosed(ring)
	if len(r) < 2 {
		return nil
	}
	out := []Stitch{{Position: r[0], Command: CommandStitch, Source: source}}
	for i := 1; i < len(r); i++ {
		for _, p := range interpolate(r[i-1], r[i], length) {
			out = append(out, Stitch{Position: p, Command: CommandStitch, Source: source})
		}
	}
	return out
}

func tatami(r Region) ([]Stitch, error) {
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = .4
	}
	length := r.StitchLengthMM
	if length <= 0 {
		length = 3
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
	for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
		segments := scanlineSegments(rotated, y)
		if row%2 == 1 {
			for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
				segments[i], segments[j] = segments[j], segments[i]
			}
		}
		for _, s := range segments {
			a, z := s[0], s[1]
			if row%2 == 1 {
				a, z = z, a
			}
			a, z = rotate(a, angle), rotate(z, angle)
			if len(out) > 0 {
				out = append(out, Stitch{Position: a, Command: CommandJump, Source: "tatami_travel"})
			} else {
				out = append(out, Stitch{Position: a, Command: CommandStitch, Source: "tatami"})
			}
			for _, p := range interpolate(a, z, length) {
				out = append(out, Stitch{Position: p, Command: CommandStitch, Source: "tatami"})
			}
		}
		row++
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	return out, nil
}

func max(v, fallback float64) float64 {
	if v > 0 {
		return v
	}
	return fallback
}
