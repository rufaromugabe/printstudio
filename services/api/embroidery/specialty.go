package embroidery

import (
	"fmt"
	"math"
)

const (
	DefaultAppliqueCoverMM = 2.0
	DefaultEstitchLegMM    = 1.8
	DefaultCordAmplitudeMM = 1.2
)

// bean triples each running segment (forward–back–forward) for a heavier outline.
func bean(r Region) []Stitch {
	length := max(r.StitchLengthMM, 2.5)
	samples := resampleClosed(r.Geometry.Rings[0], length)
	if len(samples) < 2 {
		return nil
	}
	out := make([]Stitch, 0, len(samples)*3)
	appendBean := func(a, b Point) {
		out = append(out,
			Stitch{Position: a, Command: CommandStitch, Source: "bean"},
			Stitch{Position: b, Command: CommandStitch, Source: "bean"},
			Stitch{Position: a, Command: CommandStitch, Source: "bean"},
			Stitch{Position: b, Command: CommandStitch, Source: "bean"},
		)
	}
	for i := 0; i < len(samples); i++ {
		a := samples[i]
		b := samples[(i+1)%len(samples)]
		if distance(a, b) < 1e-6 {
			continue
		}
		appendBean(a, b)
	}
	return out
}

// applique builds placement run → tack-down → narrow cover satin, with an
// operator pause note for fabric placement / trim.
//
// Cover uses a fixed-width perimeter column (not ring satin). Ring pairing on
// traced artwork often invents long rungs that exceed MaxStitchMM.
func applique(r Region, profile MachineProfile) ([]Stitch, []Stitch, error) {
	if err := r.Geometry.Validate(); err != nil {
		return nil, nil, err
	}
	coverW := r.WidthMM
	if coverW <= 0 {
		coverW = DefaultAppliqueCoverMM
	}
	maxCover := profile.MaxStitchMM
	if maxCover <= 0 {
		maxCover = 12.1
	}
	// Keep cover bite practical for appliqué edges.
	if maxCover > 4 {
		maxCover = 4
	}
	if coverW > maxCover {
		coverW = maxCover
	}
	if coverW < 1 {
		coverW = 1
	}
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 0.4
	}
	exterior := r.Geometry.Rings[0]
	poly := r.Geometry
	placement := runningPath(exterior, max(r.StitchLengthMM, 2.5), "applique_placement")
	tackRing := offsetRingInward(exterior, poly, coverW*0.55)
	if len(tackRing) < 3 {
		return nil, nil, fmt.Errorf("applique shape is too small for a %.1f mm cover satin", coverW)
	}
	tack := runningPath(tackRing, max(r.StitchLengthMM, 2.0), "applique_tack")
	underlay := append(append([]Stitch{}, placement...), tack...)
	stitches, err := appliqueCover(exterior, poly, coverW, spacing)
	if err != nil {
		return nil, nil, fmt.Errorf("applique cover: %w", err)
	}
	return underlay, stitches, nil
}

// appliqueCover zigzags a constant-width satin along the exterior, biting inward.
func appliqueCover(exterior []Point, poly Polygon, coverW, spacing float64) ([]Stitch, error) {
	samples := resampleClosed(exterior, spacing)
	if len(samples) < 3 {
		return nil, fmt.Errorf("outline is too short for cover satin")
	}
	out := make([]Stitch, 0, len(samples)*2+2)
	for i := 0; i < len(samples); i++ {
		a := samples[i]
		n := closedRingNormal(samples, i)
		probe := Point{X: a.X + n.X*coverW*0.5, Y: a.Y + n.Y*coverW*0.5}
		if !pointInPolygon(probe, poly) {
			n = Point{X: -n.X, Y: -n.Y}
		}
		inner := Point{X: a.X + n.X*coverW, Y: a.Y + n.Y*coverW}
		left, right := a, inner
		if i%2 == 1 {
			left, right = right, left
		}
		out = append(out,
			Stitch{Position: left, Command: CommandStitch, Source: "applique_cover"},
			Stitch{Position: right, Command: CommandStitch, Source: "applique_cover"},
		)
	}
	// Close the column back to the start rail.
	if len(out) > 0 {
		out = append(out, Stitch{Position: samples[0], Command: CommandStitch, Source: "applique_cover"})
	}
	return out, nil
}

// offsetRingInward samples the ring and steps each point along the inward normal.
func offsetRingInward(ring []Point, poly Polygon, dist float64) []Point {
	if dist <= 0 {
		return openPath(ring)
	}
	samples := resampleClosed(ring, math.Max(1.5, dist))
	if len(samples) < 3 {
		return nil
	}
	out := make([]Point, 0, len(samples))
	for i, p := range samples {
		n := closedRingNormal(samples, i)
		probe := Point{X: p.X + n.X*dist*0.5, Y: p.Y + n.Y*dist*0.5}
		if !pointInPolygon(probe, poly) {
			n = Point{X: -n.X, Y: -n.Y}
		}
		out = append(out, Point{X: p.X + n.X*dist, Y: p.Y + n.Y*dist})
	}
	return out
}

func closedRingNormal(samples []Point, i int) Point {
	n := len(samples)
	if n < 2 {
		return Point{X: 0, Y: 1}
	}
	prev := samples[(i+n-1)%n]
	next := samples[(i+1)%n]
	dir := Point{X: next.X - prev.X, Y: next.Y - prev.Y}
	length := math.Hypot(dir.X, dir.Y)
	if length < 1e-9 {
		return Point{X: 0, Y: 1}
	}
	return Point{X: -dir.Y / length, Y: dir.X / length}
}

func appliqueOperatorMessage() string {
	return "Appliqué sew-out: sew placement, lay fabric, sew tack-down, trim excess, then sew cover satin"
}

// motif places a small diamond motif on a fill grid inside the polygon.
func motif(r Region, minStitch float64) ([]Stitch, error) {
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 2.5
	}
	size := max(r.StitchLengthMM, minStitch*2)
	if size > spacing*0.9 {
		size = spacing * 0.9
	}
	angle := r.AngleDegrees * math.Pi / 180
	inv := -angle
	rotated := rotatePolygon(r.Geometry, inv)
	b := polygonBounds(rotated)
	var out []Stitch
	have := false
	current := Point{}
	row := 0
	for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
		xs := gridCentersX(rotated, y, spacing)
		if row%2 == 1 {
			reversePoints(xs)
		}
		for _, x := range xs {
			c := rotate(Point{X: x, Y: y}, angle)
			motifPts := []Point{
				rotate(Point{X: x, Y: y - size/2}, angle),
				rotate(Point{X: x + size/2, Y: y}, angle),
				rotate(Point{X: x, Y: y + size/2}, angle),
				rotate(Point{X: x - size/2, Y: y}, angle),
				rotate(Point{X: x, Y: y - size/2}, angle),
			}
			if have {
				out = append(out, Stitch{Position: motifPts[0], Command: CommandJump, Source: "motif_travel"})
			}
			for _, p := range motifPts {
				out = append(out, Stitch{Position: p, Command: CommandStitch, Source: "motif"})
			}
			current, have = c, true
		}
		row++
	}
	if !have {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	_ = current
	return out, nil
}

// contour walks concentric inset outlines (spiral-style contour fill).
func contour(r Region) ([]Stitch, error) {
	if err := r.Geometry.Validate(); err != nil {
		return nil, err
	}
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 1.2
	}
	length := max(r.StitchLengthMM, 2.5)
	ring := openPath(r.Geometry.Rings[0])
	var out []Stitch
	for level := 0; level < 64; level++ {
		if len(ring) < 3 {
			break
		}
		b := boundsOf(ring)
		if b.MaxX-b.MinX < spacing*2 || b.MaxY-b.MinY < spacing*2 {
			break
		}
		path := runningPath(ring, length, "contour")
		if len(path) == 0 {
			break
		}
		if len(out) > 0 {
			out = append(out, Stitch{Position: path[0].Position, Command: CommandJump, Source: "contour_travel"})
		}
		out = append(out, path...)
		next := insetRing(ring, spacing)
		if len(next) < 3 || ringLength(ringClosed(next)) < spacing*4 {
			break
		}
		// Stop if inset barely moved (collapse / self-intersection).
		if similarRing(ring, next, spacing*0.15) {
			break
		}
		ring = next
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	return out, nil
}

// estitch (blanket / E-stitch) runs the outline with perpendicular bite stitches.
func estitch(r Region) []Stitch {
	length := max(r.StitchLengthMM, 2.5)
	leg := r.WidthMM
	if leg <= 0 {
		leg = DefaultEstitchLegMM
	}
	samples := resampleClosed(r.Geometry.Rings[0], length)
	if len(samples) < 2 {
		return nil
	}
	out := make([]Stitch, 0, len(samples)*3)
	for i := 0; i < len(samples); i++ {
		a := samples[i]
		b := samples[(i+1)%len(samples)]
		n := spineNormal([]Point{samples[(i+len(samples)-1)%len(samples)], a, b}, 1)
		// Prefer inward bite for closed shapes.
		if !pointInPolygon(Point{X: a.X + n.X*leg, Y: a.Y + n.Y*leg}, r.Geometry) {
			n = Point{X: -n.X, Y: -n.Y}
		}
		tip := Point{X: a.X + n.X*leg, Y: a.Y + n.Y*leg}
		out = append(out,
			Stitch{Position: a, Command: CommandStitch, Source: "estitch"},
			Stitch{Position: tip, Command: CommandStitch, Source: "estitch"},
			Stitch{Position: a, Command: CommandStitch, Source: "estitch"},
		)
	}
	if len(samples) > 0 {
		out = append(out, Stitch{Position: samples[0], Command: CommandStitch, Source: "estitch"})
	}
	return out
}

// cross fills the region with X (cross-stitch) cells on a grid.
func cross(r Region, minStitch float64) ([]Stitch, error) {
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 2.0
	}
	half := spacing * 0.35
	if half < minStitch {
		half = minStitch
	}
	angle := r.AngleDegrees * math.Pi / 180
	inv := -angle
	rotated := rotatePolygon(r.Geometry, inv)
	b := polygonBounds(rotated)
	var out []Stitch
	have := false
	row := 0
	for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
		xs := gridCentersX(rotated, y, spacing)
		if row%2 == 1 {
			reversePoints(xs)
		}
		for _, x := range xs {
			c := Point{X: x, Y: y}
			a := rotate(Point{X: c.X - half, Y: c.Y - half}, angle)
			b1 := rotate(Point{X: c.X + half, Y: c.Y + half}, angle)
			c2 := rotate(Point{X: c.X + half, Y: c.Y - half}, angle)
			d := rotate(Point{X: c.X - half, Y: c.Y + half}, angle)
			if have {
				out = append(out, Stitch{Position: a, Command: CommandJump, Source: "cross_travel"})
			}
			out = append(out,
				Stitch{Position: a, Command: CommandStitch, Source: "cross"},
				Stitch{Position: b1, Command: CommandStitch, Source: "cross"},
				Stitch{Position: c2, Command: CommandStitch, Source: "cross"},
				Stitch{Position: d, Command: CommandStitch, Source: "cross"},
			)
			have = true
		}
		row++
	}
	if !have {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	return out, nil
}

// sequin places attach stitches on a grid; operator drops sequins at each stop.
func sequin(r Region) ([]Stitch, error) {
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 5.0
	}
	angle := r.AngleDegrees * math.Pi / 180
	inv := -angle
	rotated := rotatePolygon(r.Geometry, inv)
	b := polygonBounds(rotated)
	var out []Stitch
	have := false
	row := 0
	for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
		xs := gridCentersX(rotated, y, spacing)
		if row%2 == 1 {
			reversePoints(xs)
		}
		for _, x := range xs {
			p := rotate(Point{X: x, Y: y}, angle)
			if have {
				out = append(out, Stitch{Position: p, Command: CommandJump, Source: "sequin_travel"})
			}
			// Short lock stitches mark the sequin attach point.
			out = append(out,
				Stitch{Position: p, Command: CommandStitch, Source: "sequin"},
				Stitch{Position: Point{X: p.X + 0.3, Y: p.Y}, Command: CommandStitch, Source: "sequin"},
				Stitch{Position: p, Command: CommandStitch, Source: "sequin"},
			)
			have = true
		}
		row++
	}
	if !have {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	return out, nil
}

func sequinOperatorMessage() string {
	return "Sequin sew-out: use a sequin-capable head or hand-place sequins at each attach stop before continuing"
}

// cord couches a zigzag (cording) along the region outline.
func cord(r Region) []Stitch {
	step := max(r.StitchLengthMM, 1.5)
	amp := r.WidthMM
	if amp <= 0 {
		amp = DefaultCordAmplitudeMM
	}
	samples := resampleClosed(r.Geometry.Rings[0], step)
	if len(samples) < 2 {
		return nil
	}
	out := make([]Stitch, 0, len(samples)*2)
	for i := 0; i < len(samples); i++ {
		a := samples[i]
		n := spineNormal(samples, i)
		side := 1.0
		if i%2 == 1 {
			side = -1
		}
		p := Point{X: a.X + n.X*amp*side, Y: a.Y + n.Y*amp*side}
		out = append(out, Stitch{Position: p, Command: CommandStitch, Source: "cord"})
	}
	if len(out) > 0 {
		out = append(out, Stitch{Position: out[0].Position, Command: CommandStitch, Source: "cord"})
	}
	return out
}

func cordOperatorMessage() string {
	return "Cord sew-out: feed filler cord under the zigzag foot / cording attachment while the outline sews"
}

// chenille builds a dense looping zigzag fill (loop-pile style column rows).
func chenille(r Region, minStitch float64) ([]Stitch, error) {
	spacing := r.SpacingMM
	if spacing <= 0 {
		spacing = 0.8
	}
	amp := r.WidthMM
	if amp <= 0 {
		amp = spacing * 0.9
	}
	length := max(r.StitchLengthMM, minStitch)
	angle := r.AngleDegrees * math.Pi / 180
	inv := -angle
	rotated := rotatePolygon(r.Geometry, inv)
	b := polygonBounds(rotated)
	var out []Stitch
	have := false
	current := Point{}
	row := 0
	for y := b.MinY + spacing/2; y < b.MaxY; y += spacing {
		segments := scanlineSegments(rotated, y)
		if row%2 == 1 {
			for i, j := 0, len(segments)-1; i < j; i, j = i+1, j-1 {
				segments[i], segments[j] = segments[j], segments[i]
			}
		}
		for _, seg := range segments {
			a, z := rotate(seg[0], angle), rotate(seg[1], angle)
			if row%2 == 1 {
				a, z = z, a
			}
			if distance(a, z) < minStitch {
				continue
			}
			if have {
				out = append(out, Stitch{Position: a, Command: CommandJump, Source: "chenille_travel"})
			} else {
				out = append(out, Stitch{Position: a, Command: CommandStitch, Source: "chenille"})
			}
			dir := Point{X: z.X - a.X, Y: z.Y - a.Y}
			nlen := math.Hypot(dir.X, dir.Y)
			normal := Point{X: -dir.Y / nlen, Y: dir.X / nlen}
			samples := interpolate(a, z, length)
			for i, p := range samples {
				side := 1.0
				if i%2 == 1 {
					side = -1
				}
				q := Point{X: p.X + normal.X*amp*side, Y: p.Y + normal.Y*amp*side}
				out = append(out, Stitch{Position: q, Command: CommandStitch, Source: "chenille"})
				current = q
			}
			have = true
		}
		row++
	}
	if !have {
		return nil, fmt.Errorf("fill produced no stitches")
	}
	_ = current
	return out, nil
}

func chenilleOperatorMessage() string {
	return "Chenille sew-out: use a chenille/looping head or heavy zigzag on terry; trim loops only if the design requires cut-pile"
}

func rotatePolygon(p Polygon, radians float64) Polygon {
	out := Polygon{Rings: make([][]Point, len(p.Rings))}
	for i, ring := range p.Rings {
		for _, q := range ring {
			out.Rings[i] = append(out.Rings[i], rotate(q, radians))
		}
	}
	return out
}

func gridCentersX(p Polygon, y, spacing float64) []float64 {
	var xs []float64
	for _, seg := range scanlineSegments(p, y) {
		for x := seg[0].X + spacing/2; x < seg[1].X; x += spacing {
			pt := Point{X: x, Y: y}
			if pointInPolygon(pt, p) {
				xs = append(xs, x)
			}
		}
	}
	return xs
}

func reversePoints(pts []float64) {
	for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
		pts[i], pts[j] = pts[j], pts[i]
	}
}

func pointInPolygon(pt Point, p Polygon) bool {
	// Even-odd across all rings so holes are excluded.
	inside := false
	for _, raw := range p.Rings {
		r := ringClosed(raw)
		for i := 0; i < len(r)-1; i++ {
			a, b := r[i], r[i+1]
			if (a.Y > pt.Y) != (b.Y > pt.Y) {
				x := a.X + (pt.Y-a.Y)*(b.X-a.X)/(b.Y-a.Y)
				if pt.X < x {
					inside = !inside
				}
			}
		}
	}
	return inside
}

func insetRing(ring []Point, dist float64) []Point {
	path := openPath(ring)
	if len(path) < 3 || dist <= 0 {
		return nil
	}
	// Ensure CCW so left normals point inward for typical digitizer coords (Y up).
	if ringArea(path) < 0 {
		rev := make([]Point, len(path))
		for i := range path {
			rev[i] = path[len(path)-1-i]
		}
		path = rev
	}
	out := make([]Point, 0, len(path))
	n := len(path)
	for i := 0; i < n; i++ {
		prev := path[(i+n-1)%n]
		cur := path[i]
		next := path[(i+1)%n]
		n1 := unitNormal(prev, cur)
		n2 := unitNormal(cur, next)
		bis := Point{X: n1.X + n2.X, Y: n1.Y + n2.Y}
		blen := math.Hypot(bis.X, bis.Y)
		if blen < 1e-9 {
			bis = n1
			blen = 1
		}
		bis.X /= blen
		bis.Y /= blen
		// Scale so the offset distance along the angle bisector approximates dist.
		dot := bis.X*n1.X + bis.Y*n1.Y
		scale := dist
		if math.Abs(dot) > 0.2 {
			scale = dist / math.Abs(dot)
		}
		if scale > dist*3 {
			scale = dist * 3
		}
		out = append(out, Point{X: cur.X + bis.X*scale, Y: cur.Y + bis.Y*scale})
	}
	return out
}

func unitNormal(a, b Point) Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	n := math.Hypot(dx, dy)
	if n < 1e-9 {
		return Point{X: 0, Y: 1}
	}
	// Left normal for CCW ring → inward when Y increases upward.
	return Point{X: -dy / n, Y: dx / n}
}

func ringArea(path []Point) float64 {
	r := ringClosed(path)
	sum := 0.0
	for i := 0; i < len(r)-1; i++ {
		sum += r[i].X*r[i+1].Y - r[i+1].X*r[i].Y
	}
	return sum / 2
}

func boundsOf(ring []Point) Bounds {
	return polygonBounds(Polygon{Rings: [][]Point{ring}})
}

func similarRing(a, b []Point, tol float64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	ba, bb := boundsOf(a), boundsOf(b)
	return math.Abs((ba.MaxX-ba.MinX)-(bb.MaxX-bb.MinX)) < tol &&
		math.Abs((ba.MaxY-ba.MinY)-(bb.MaxY-bb.MinY)) < tol
}

// keepShortStitches is true for kinds whose decorative pairs must not be pruned.
func keepShortStitches(k StitchKind) bool {
	switch k {
	case Satin, Puff, Applique, Estitch, Cross, Motif, Cord, Chenille, Sequin:
		return true
	default:
		return false
	}
}

func managesOwnUnderlay(k StitchKind) bool {
	switch k {
	case Puff, Applique, Sequin:
		return true
	default:
		return false
	}
}
