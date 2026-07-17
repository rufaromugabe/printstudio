package embroidery

import (
	"fmt"
	"math"
)

// satin constructs corresponding left/right rails from even-odd scanline
// intersections. Branching columns are rejected because they require explicit
// rail editing; the compiler never guesses across a fork.
func satin(r Region, profile MachineProfile) ([]Stitch, []Stitch, error) {
	if len(r.Geometry.Rings) == 2 {
		return ringSatin(r, profile)
	}
	if len(r.Geometry.Rings) > 2 {
		return nil, nil, fmt.Errorf("satin region has %d holes; split it into editable columns or use tatami", len(r.Geometry.Rings)-1)
	}
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = .4
	}
	angle := r.AngleDegrees * math.Pi / 180
	rotated := Polygon{Rings: make([][]Point, len(r.Geometry.Rings))}
	for i, ring := range r.Geometry.Rings {
		for _, p := range ring {
			rotated.Rings[i] = append(rotated.Rings[i], rotate(p, -angle))
		}
	}
	bounds := polygonBounds(rotated)
	if bounds.MaxX-bounds.MinX > bounds.MaxY-bounds.MinY {
		angle += math.Pi / 2
		rotated = Polygon{Rings: make([][]Point, len(r.Geometry.Rings))}
		for i, ring := range r.Geometry.Rings {
			for _, p := range ring {
				rotated.Rings[i] = append(rotated.Rings[i], rotate(p, -angle))
			}
		}
		bounds = polygonBounds(rotated)
	}
	type rung struct{ left, right, center Point }
	var rungs []rung
	for y := bounds.MinY + spacing/2; y < bounds.MaxY; y += spacing {
		segments := scanlineSegments(rotated, y)
		if len(segments) == 0 {
			continue
		}
		if len(segments) != 1 {
			return nil, nil, fmt.Errorf("satin column branches or crosses a hole at %.2f mm; split or edit its rails", y)
		}
		left, right := segments[0][0], segments[0][1]
		width := distance(left, right)
		if width > profile.MaxStitchMM {
			return nil, nil, fmt.Errorf("satin width %.2f mm exceeds machine maximum %.2f mm; use split satin or tatami", width, profile.MaxStitchMM)
		}
		rungs = append(rungs, rung{left: left, right: right, center: Point{X: (left.X + right.X) / 2, Y: y}})
	}
	if len(rungs) < 2 {
		return nil, nil, fmt.Errorf("satin column is too short for the selected density")
	}
	var underlay []Stitch
	if r.CenterUnderlay || (!r.CenterUnderlay && !r.ZigzagUnderlay) {
		for _, q := range rungs {
			underlay = append(underlay, Stitch{Position: rotate(q.center, angle), Command: CommandStitch, Source: "satin_center_underlay"})
		}
	}
	if r.ZigzagUnderlay {
		for i := 0; i < len(rungs); i += maxInt(1, int(math.Round(1.5/spacing))) {
			q := rungs[i]
			p := q.left
			if (i/maxInt(1, int(math.Round(1.5/spacing))))%2 == 1 {
				p = q.right
			}
			underlay = append(underlay, Stitch{Position: rotate(p, angle), Command: CommandStitch, Source: "satin_zigzag_underlay"})
		}
	}
	stitches := make([]Stitch, 0, len(rungs)*2)
	for i, q := range rungs {
		a, b := q.left, q.right
		if i%2 == 1 {
			a, b = b, a
		}
		stitches = append(stitches, Stitch{Position: rotate(a, angle), Command: CommandStitch, Source: "satin"}, Stitch{Position: rotate(b, angle), Command: CommandStitch, Source: "satin"})
	}
	return underlay, stitches, nil
}

// ringSatin pairs an exterior rail with one enclosed rail. This covers closed
// columns such as O, P and many badge outlines without inventing a skeleton.
func ringSatin(r Region, profile MachineProfile) ([]Stitch, []Stitch, error) {
	outer, inner := ringClosed(r.Geometry.Rings[0]), ringClosed(r.Geometry.Rings[1])
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = .4
	}
	count := int(math.Ceil(math.Max(ringLength(outer), ringLength(inner)) / spacing))
	if count < 8 {
		count = 8
	}
	outerRail, innerRail := resampleRing(outer, count), resampleRing(inner, count)
	innerRail = alignRail(outerRail, innerRail)
	var underlay, stitches []Stitch
	for i := 0; i < count; i++ {
		width := distance(outerRail[i], innerRail[i])
		if width > profile.MaxStitchMM {
			return nil, nil, fmt.Errorf("ring satin width %.2f mm exceeds machine maximum %.2f mm near rung %d", width, profile.MaxStitchMM, i)
		}
	}
	for i := 0; i < count; i++ {
		center := Point{X: (outerRail[i].X + innerRail[i].X) / 2, Y: (outerRail[i].Y + innerRail[i].Y) / 2}
		underlay = append(underlay, Stitch{Position: center, Command: CommandStitch, Source: "ring_satin_center_underlay"})
	}
	if r.ZigzagUnderlay {
		step := maxInt(1, int(math.Round(1.5/spacing)))
		for i := 0; i < count; i += step {
			p := outerRail[i]
			if (i/step)%2 == 1 {
				p = innerRail[i]
			}
			underlay = append(underlay, Stitch{Position: p, Command: CommandStitch, Source: "ring_satin_zigzag_underlay"})
		}
	}
	for i := 0; i <= count; i++ {
		j := i % count
		a, b := outerRail[j], innerRail[j]
		if i%2 == 1 {
			a, b = b, a
		}
		stitches = append(stitches, Stitch{Position: a, Command: CommandStitch, Source: "ring_satin"}, Stitch{Position: b, Command: CommandStitch, Source: "ring_satin"})
	}
	return underlay, stitches, nil
}

func ringLength(r []Point) float64 {
	n := 0.0
	for i := 1; i < len(r); i++ {
		n += distance(r[i-1], r[i])
	}
	return n
}
func resampleRing(r []Point, count int) []Point {
	total := ringLength(r)
	out := make([]Point, 0, count)
	segment, cumulative := 1, 0.0
	for i := 0; i < count; i++ {
		target := float64(i) * total / float64(count)
		for segment < len(r) && cumulative+distance(r[segment-1], r[segment]) < target {
			cumulative += distance(r[segment-1], r[segment])
			segment++
		}
		if segment >= len(r) {
			out = append(out, r[0])
			continue
		}
		a, b := r[segment-1], r[segment]
		length := distance(a, b)
		t := 0.0
		if length > 0 {
			t = (target - cumulative) / length
		}
		out = append(out, Point{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t})
	}
	return out
}
func alignRail(outer, inner []Point) []Point {
	best := make([]Point, len(inner))
	bestScore := math.Inf(1)
	for _, reverse := range []bool{false, true} {
		for offset := 0; offset < len(inner); offset++ {
			score := 0.0
			for i := 0; i < len(outer); i++ {
				j := (i + offset) % len(inner)
				if reverse {
					j = (offset - i + len(inner)*2) % len(inner)
				}
				score += distance(outer[i], inner[j])
			}
			if score < bestScore {
				bestScore = score
				for i := range outer {
					j := (i + offset) % len(inner)
					if reverse {
						j = (offset - i + len(inner)*2) % len(inner)
					}
					best[i] = inner[j]
				}
			}
		}
	}
	return best
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
