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
