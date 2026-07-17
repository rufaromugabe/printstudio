package embroidery

import "math"

func distance(a, b Point) float64 { return math.Hypot(b.X-a.X, b.Y-a.Y) }
func rotate(p Point, radians float64) Point {
	c, s := math.Cos(radians), math.Sin(radians)
	return Point{p.X*c - p.Y*s, p.X*s + p.Y*c}
}

func ringClosed(r []Point) []Point {
	if len(r) == 0 || r[0] == r[len(r)-1] {
		return r
	}
	out := append([]Point(nil), r...)
	return append(out, r[0])
}

func interpolate(a, b Point, maxLength float64) []Point {
	n := int(math.Ceil(distance(a, b) / maxLength))
	if n < 1 {
		n = 1
	}
	out := make([]Point, n)
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		out[i-1] = Point{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t}
	}
	return out
}

// resampleClosed walks a closed ring by arc length so tiny contour edges from
// traced artwork do not become individual sub-minimum stitches.
func resampleClosed(ring []Point, spacing float64) []Point {
	if spacing <= 0 {
		spacing = 2.5
	}
	r := ringClosed(ring)
	if len(r) < 2 {
		return nil
	}
	path := append([]Point(nil), r...)
	if path[0] != path[len(path)-1] {
		path = append(path, path[0])
	}
	samples := resampleOpen(path, spacing)
	if len(samples) > 1 && distance(samples[0], samples[len(samples)-1]) < 1e-6 {
		samples = samples[:len(samples)-1]
	}
	return samples
}

// pruneShortStitches drops needle moves shorter than minLen (machine minimum).
// Jumps are kept so travel still reaches the next run; zero-length duplicates go away.
func pruneShortStitches(stitches []Stitch, minLen float64) []Stitch {
	if len(stitches) < 2 || minLen <= 0 {
		return stitches
	}
	out := make([]Stitch, 0, len(stitches))
	out = append(out, stitches[0])
	for i := 1; i < len(stitches); i++ {
		s := stitches[i]
		prev := out[len(out)-1]
		d := distance(prev.Position, s.Position)
		if d < 1e-9 {
			continue
		}
		if s.Command == CommandStitch && d < minLen {
			continue
		}
		out = append(out, s)
	}
	return out
}

func polygonBounds(p Polygon) Bounds {
	b := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	for _, r := range p.Rings {
		for _, q := range r {
			b.MinX = math.Min(b.MinX, q.X)
			b.MinY = math.Min(b.MinY, q.Y)
			b.MaxX = math.Max(b.MaxX, q.X)
			b.MaxY = math.Max(b.MaxY, q.Y)
		}
	}
	return b
}

// scanlineSegments applies the even-odd rule across every ring, so holes are excluded.
func scanlineSegments(p Polygon, y float64) [][2]Point {
	var xs []float64
	for _, raw := range p.Rings {
		r := ringClosed(raw)
		for i := 0; i < len(r)-1; i++ {
			a, b := r[i], r[i+1]
			if (a.Y <= y && b.Y > y) || (b.Y <= y && a.Y > y) {
				xs = append(xs, a.X+(y-a.Y)*(b.X-a.X)/(b.Y-a.Y))
			}
		}
	}
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
	segments := make([][2]Point, 0, len(xs)/2)
	for i := 0; i+1 < len(xs); i += 2 {
		if xs[i+1] > xs[i] {
			segments = append(segments, [2]Point{{xs[i], y}, {xs[i+1], y}})
		}
	}
	return segments
}
