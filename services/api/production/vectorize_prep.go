package production

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	ContentTextLike      = "text-like"
	ContentFlatArt       = "flat-art"
	ContentPhoto         = "photo"
	MaxVectorInputPixels = 50_000_000
)

// VectorPrepMetadata makes automatic cleanup visible to the caller. The
// detection is deliberately structural rather than OCR: editable text remains
// glyph-traced, while text embedded in a raster receives a crisp-stroke trace
// profile without guessing or changing the characters.
type VectorPrepMetadata struct {
	ContentKind       string   `json:"contentKind"`
	Confidence        float64  `json:"confidence"`
	MaskSource        string   `json:"maskSource"`
	InputWidthPx      int      `json:"inputWidthPx"`
	InputHeightPx     int      `json:"inputHeightPx"`
	PreparedWidthPx   int      `json:"preparedWidthPx"`
	PreparedHeightPx  int      `json:"preparedHeightPx"`
	UpscaleFactor     int      `json:"upscaleFactor"`
	ForegroundRatio   float64  `json:"foregroundRatio"`
	BackgroundRemoved bool     `json:"backgroundRemoved"`
	Profile           string   `json:"profile"`
	QualityScore      int      `json:"qualityScore"`
	Steps             []string `json:"steps"`
}

type vectorArtworkAnalysis struct {
	kind              string
	confidence        float64
	maskSource        string
	mask              []bool
	width             int
	height            int
	foregroundRatio   float64
	backgroundRemoved bool
	colorCount        int
	edgeDensity       float64
	components        int
}

type vectorQualityProfile struct {
	name              string
	trace             TraceOptions
	maskThreshold     uint8
	minComponentArea  int
	healNeighborCount int
	simplifyTolerance float64
}

func prepareVectorMask(img image.Image, method string, alphaCutoff uint8) (image.Image, VectorPrepMetadata, vectorQualityProfile, error) {
	analysis, err := analyzeVectorArtwork(img, alphaCutoff)
	if err != nil {
		return nil, VectorPrepMetadata{}, vectorQualityProfile{}, err
	}
	profile := vectorProfile(method, analysis.kind)
	steps := []string{"classified raster as " + analysis.kind}
	if analysis.backgroundRemoved {
		steps = append(steps, "isolated foreground from the border background")
	} else {
		steps = append(steps, "preserved the source transparency mask")
	}

	mask := removeSmallMaskComponents(analysis.mask, analysis.width, analysis.height, profile.minComponentArea)
	mask = healMaskDefects(mask, analysis.width, analysis.height, profile.healNeighborCount)
	steps = append(steps, "removed speckles and repaired one-pixel edge defects")

	factor := vectorUpscaleFactor(analysis.width, analysis.height, analysis.kind)
	mask, width, height := upscaleVectorMask(mask, analysis.width, analysis.height, factor, profile.maskThreshold)
	if factor > 1 {
		steps = append(steps, fmt.Sprintf("edge-aware %dx supersampling before trace", factor))
	}
	mask = removeSmallMaskComponents(mask, width, height, profile.minComponentArea*factor*factor)
	if countMask(mask) == 0 {
		return nil, VectorPrepMetadata{}, vectorQualityProfile{}, fmt.Errorf("automatic cleanup found no foreground artwork")
	}

	out := image.NewAlpha(image.Rect(0, 0, width, height))
	for i, on := range mask {
		if on {
			out.Pix[i] = 255
		}
	}
	meta := VectorPrepMetadata{
		ContentKind:       analysis.kind,
		Confidence:        roundTo(analysis.confidence, 3),
		MaskSource:        analysis.maskSource,
		InputWidthPx:      analysis.width,
		InputHeightPx:     analysis.height,
		PreparedWidthPx:   width,
		PreparedHeightPx:  height,
		UpscaleFactor:     factor,
		ForegroundRatio:   roundTo(float64(countMask(mask))/float64(width*height), 4),
		BackgroundRemoved: analysis.backgroundRemoved,
		Profile:           profile.name,
		Steps:             steps,
	}
	return out, meta, profile, nil
}

func analyzeVectorArtwork(img image.Image, alphaCutoff uint8) (vectorArtworkAnalysis, error) {
	if img == nil {
		return vectorArtworkAnalysis{}, fmt.Errorf("image is required")
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 2 || h < 2 {
		return vectorArtworkAnalysis{}, fmt.Errorf("image is too small to vectorize")
	}
	if int64(w)*int64(h) > MaxVectorInputPixels {
		return vectorArtworkAnalysis{}, fmt.Errorf("image exceeds the %d-megapixel vector preflight limit", MaxVectorInputPixels/1_000_000)
	}
	if alphaCutoff == 0 {
		alphaCutoff = DefaultAlphaCutoff
	}
	total := w * h
	transparent := 0
	opaqueForeground := 0
	colorBuckets := map[uint16]int{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if c.A < 250 {
				transparent++
			}
			if c.A >= alphaCutoff {
				opaqueForeground++
				key := uint16(c.R>>4)<<8 | uint16(c.G>>4)<<4 | uint16(c.B>>4)
				colorBuckets[key]++
			}
		}
	}
	meaningfulAlpha := transparent > maxInt(2, total/1000) && opaqueForeground > 0
	mask := make([]bool, total)
	maskSource := "alpha"
	backgroundRemoved := false
	if meaningfulAlpha {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				mask[y*w+x] = c.A >= alphaCutoff
			}
		}
	} else {
		background := borderMedianColor(img)
		distances := make([]uint8, total)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				dr := float64(int(c.R) - int(background.R))
				dg := float64(int(c.G) - int(background.G))
				db := float64(int(c.B) - int(background.B))
				// Normalize RGB Euclidean distance into one histogram byte.
				distances[y*w+x] = uint8(math.Min(255, math.Sqrt(dr*dr+dg*dg+db*db)/math.Sqrt(3)))
			}
		}
		threshold := maxInt(10, otsuThreshold(distances))
		for i, d := range distances {
			mask[i] = int(d) >= threshold
		}
		maskSource = "border-color"
		backgroundRemoved = true
		ratio := float64(countMask(mask)) / float64(total)
		if ratio < 0.001 || ratio > 0.88 {
			luminance := make([]uint8, total)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
					luminance[y*w+x] = pixelLuminance(c)
				}
			}
			lumThreshold := otsuThreshold(luminance)
			bgLum := int(pixelLuminance(background))
			candidate := make([]bool, total)
			for i, lum := range luminance {
				if bgLum >= lumThreshold {
					candidate[i] = int(lum) < lumThreshold
				} else {
					candidate[i] = int(lum) > lumThreshold
				}
			}
			candidateRatio := float64(countMask(candidate)) / float64(total)
			if candidateRatio >= 0.001 && candidateRatio < 0.88 {
				mask = candidate
				maskSource = "luminance"
			}
		}
	}

	foreground := countMask(mask)
	if foreground == 0 {
		return vectorArtworkAnalysis{}, fmt.Errorf("no foreground could be separated from the image")
	}
	components, _ := maskComponentStats(mask, w, h)
	edges := maskEdgeDensity(mask, w, h)
	colorCount := significantColorCount(colorBuckets, total)
	ratio := float64(foreground) / float64(total)
	kind, confidence := classifyVectorContent(meaningfulAlpha, ratio, edges, components, colorCount)
	return vectorArtworkAnalysis{
		kind: kind, confidence: confidence, maskSource: maskSource, mask: mask,
		width: w, height: h, foregroundRatio: ratio, backgroundRemoved: backgroundRemoved,
		colorCount: colorCount, edgeDensity: edges, components: components,
	}, nil
}

func classifyVectorContent(hasAlpha bool, foregroundRatio, edgeDensity float64, components, colors int) (string, float64) {
	// Several discrete, relatively sparse components are characteristic of
	// rasterized words. This changes cleanup, never the actual character data.
	// Chunky multi-island logos (large average component area) stay on the
	// flat-art path so counter/similarity gates are not over-tightened.
	if components >= 2 && components <= 300 && foregroundRatio >= 0.003 && foregroundRatio < 0.62 && (colors <= 24 || hasAlpha) {
		avgComponent := foregroundRatio / float64(components)
		if components >= 4 || avgComponent < 0.1 {
			return ContentTextLike, 0.86
		}
	}
	if colors <= 32 && (hasAlpha || edgeDensity < 0.18) {
		return ContentFlatArt, 0.9
	}
	if colors > 64 || edgeDensity > 0.2 {
		return ContentPhoto, 0.78
	}
	return ContentFlatArt, 0.72
}

func vectorProfile(method, contentKind string) vectorQualityProfile {
	method = normalizeVectorMethod(method)
	profile := vectorQualityProfile{
		name:          method + "-clean",
		trace:         TraceOptions{TurdSize: 2, AlphaMax: 0.9, OptTolerance: 0.16, TurnPolicy: "minority"},
		maskThreshold: 128, minComponentArea: 2, healNeighborCount: 7, simplifyTolerance: 0.11,
	}
	switch method {
	case "vinyl":
		profile.name = "vinyl-cut-crisp"
		profile.trace = TraceOptions{TurdSize: 2, AlphaMax: 0.75, OptTolerance: 0.12, TurnPolicy: "minority"}
		profile.simplifyTolerance = 0.08
	case "embroidery":
		profile.name = "embroidery-stitch-stable"
		profile.trace = TraceOptions{TurdSize: 5, AlphaMax: 1.0, OptTolerance: 0.24, TurnPolicy: "majority"}
		profile.maskThreshold = 116
		profile.minComponentArea = 5
		profile.healNeighborCount = 6
		profile.simplifyTolerance = 0.18
	case "screen":
		profile.name = "screen-separation-clean"
		profile.trace = TraceOptions{TurdSize: 2, AlphaMax: 0.9, OptTolerance: 0.14, TurnPolicy: "minority"}
	case "dtf":
		profile.name = "dtf-detail-preserving"
		profile.trace = TraceOptions{TurdSize: 1, AlphaMax: 1.0, OptTolerance: 0.1, TurnPolicy: "minority"}
		profile.minComponentArea = 1
		profile.simplifyTolerance = 0.07
	}
	if contentKind == ContentTextLike {
		profile.name += "-text"
		profile.trace.AlphaMax = math.Min(profile.trace.AlphaMax, 0.72)
		profile.trace.OptTolerance = math.Min(profile.trace.OptTolerance, 0.1)
		profile.minComponentArea = 1 // Preserve punctuation and small counters.
		profile.simplifyTolerance = math.Min(profile.simplifyTolerance, 0.07)
	} else if contentKind == ContentPhoto {
		profile.name += "-detail"
		profile.trace.TurdSize = maxInt(1, profile.trace.TurdSize/2)
		profile.trace.OptTolerance = math.Min(profile.trace.OptTolerance, 0.1)
	}
	return profile
}

func normalizeVectorMethod(method string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	switch method {
	case "vinyl", "embroidery", "screen", "dtf":
		return method
	case "screen print", "screen-print":
		return "screen"
	default:
		return "vinyl"
	}
}

func vectorUpscaleFactor(width, height int, kind string) int {
	maxDim := maxInt(width, height)
	factor := 1
	switch {
	case maxDim < 320:
		factor = 4
	case maxDim < 720:
		factor = 3
	case maxDim < 1400:
		factor = 2
	}
	if kind == ContentTextLike && maxDim < 1600 && factor < 2 {
		factor = 2
	}
	for factor > 1 && (width*factor > 6000 || height*factor > 6000 || int64(width*factor)*int64(height*factor) > 24_000_000) {
		factor--
	}
	return factor
}

func upscaleVectorMask(mask []bool, width, height, factor int, threshold uint8) ([]bool, int, int) {
	if factor <= 1 {
		return append([]bool(nil), mask...), width, height
	}
	src := image.NewGray(image.Rect(0, 0, width, height))
	for i, on := range mask {
		if on {
			src.Pix[i] = 255
		}
	}
	targetWidth, targetHeight := width*factor, height*factor
	dst := image.NewGray(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	out := make([]bool, targetWidth*targetHeight)
	for i, value := range dst.Pix {
		out[i] = value >= threshold
	}
	return out, targetWidth, targetHeight
}

func healMaskDefects(mask []bool, width, height, fillNeighbors int) []bool {
	if width < 3 || height < 3 {
		return append([]bool(nil), mask...)
	}
	out := append([]bool(nil), mask...)
	for y := 1; y < height-1; y++ {
		for x := 1; x < width-1; x++ {
			neighbors := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if (dx != 0 || dy != 0) && mask[(y+dy)*width+x+dx] {
						neighbors++
					}
				}
			}
			i := y*width + x
			if !mask[i] && neighbors >= fillNeighbors {
				out[i] = true
			} else if mask[i] && neighbors == 0 {
				out[i] = false
			}
		}
	}
	return out
}

func removeSmallMaskComponents(mask []bool, width, height, minArea int) []bool {
	if minArea <= 1 || len(mask) == 0 {
		return append([]bool(nil), mask...)
	}
	out := append([]bool(nil), mask...)
	seen := make([]bool, len(mask))
	queue := make([]int, 0, 64)
	component := make([]int, 0, 64)
	for start, on := range mask {
		if !on || seen[start] {
			continue
		}
		queue = append(queue[:0], start)
		component = component[:0]
		seen[start] = true
		for len(queue) > 0 {
			i := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			component = append(component, i)
			x, y := i%width, i/width
			for _, n := range maskNeighbors(x, y, width, height) {
				if mask[n] && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		if len(component) < minArea {
			for _, i := range component {
				out[i] = false
			}
		}
	}
	return out
}

func maskComponentStats(mask []bool, width, height int) (int, int) {
	seen := make([]bool, len(mask))
	components, largest := 0, 0
	queue := make([]int, 0, 64)
	for start, on := range mask {
		if !on || seen[start] {
			continue
		}
		components++
		size := 0
		queue = append(queue[:0], start)
		seen[start] = true
		for len(queue) > 0 {
			i := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			size++
			x, y := i%width, i/width
			for _, n := range maskNeighbors(x, y, width, height) {
				if mask[n] && !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		largest = maxInt(largest, size)
	}
	return components, largest
}

func maskNeighbors(x, y, width, height int) []int {
	out := make([]int, 0, 4)
	if x > 0 {
		out = append(out, y*width+x-1)
	}
	if x+1 < width {
		out = append(out, y*width+x+1)
	}
	if y > 0 {
		out = append(out, (y-1)*width+x)
	}
	if y+1 < height {
		out = append(out, (y+1)*width+x)
	}
	return out
}

func polishVectorRings(rings [][]VectorPoint, tolerance float64) ([][]VectorPoint, int) {
	if tolerance <= 0 {
		return rings, 0
	}
	out := make([][]VectorPoint, 0, len(rings))
	removed := 0
	for _, ring := range rings {
		clean := removeNearDuplicatePoints(ring, math.Max(1e-6, tolerance/4))
		if len(clean) < 3 {
			continue
		}
		// Ramer–Douglas–Peucker on the closed ring. The previous parallel
		// "drop every locally flat vertex" pass collapsed dense curves after
		// supersampling because every sample looked colinear with its
		// immediate neighbours, nuking whole arcs in a single sweep.
		simplified := simplifyClosedRingRDP(clean, tolerance)
		if len(simplified) < 3 {
			simplified = clean
		}
		removed += len(clean) - len(simplified)
		out = append(out, simplified)
	}
	return out, removed
}

func simplifyClosedRingRDP(ring []VectorPoint, tolerance float64) []VectorPoint {
	n := len(ring)
	if n < 4 || tolerance <= 0 {
		return ring
	}
	keep := make([]bool, n)
	// Anchor opposite extremes of the ring so the closed loop has a stable
	// chord before recursive RDP. Bounding-box extrema are O(n) and enough
	// to keep both halves of logos/letterforms from collapsing.
	a, b := 0, 0
	for i := 1; i < n; i++ {
		if ring[i].X < ring[a].X || (ring[i].X == ring[a].X && ring[i].Y < ring[a].Y) {
			a = i
		}
		if ring[i].X > ring[b].X || (ring[i].X == ring[b].X && ring[i].Y > ring[b].Y) {
			b = i
		}
	}
	if a == b {
		b = (a + n/2) % n
	}
	keep[a], keep[b] = true, true
	rdpMark(ring, a, b, tolerance, keep)
	rdpMark(ring, b, a, tolerance, keep)
	out := make([]VectorPoint, 0, n)
	for i, on := range keep {
		if on {
			out = append(out, ring[i])
		}
	}
	if len(out) < 3 {
		return ring
	}
	return out
}

func rdpMark(ring []VectorPoint, start, end int, tolerance float64, keep []bool) {
	n := len(ring)
	if n == 0 || start == end {
		return
	}
	maxDist := -1.0
	farthest := -1
	for i := (start + 1) % n; i != end; i = (i + 1) % n {
		dist := pointSegmentDistance(ring[i], ring[start], ring[end])
		if dist > maxDist {
			maxDist = dist
			farthest = i
		}
	}
	if farthest < 0 || maxDist <= tolerance {
		return
	}
	keep[farthest] = true
	rdpMark(ring, start, farthest, tolerance, keep)
	rdpMark(ring, farthest, end, tolerance, keep)
}

func removeNearDuplicatePoints(ring []VectorPoint, tolerance float64) []VectorPoint {
	if len(ring) < 2 {
		return ring
	}
	out := make([]VectorPoint, 0, len(ring))
	for _, point := range ring {
		if len(out) == 0 || math.Hypot(point.X-out[len(out)-1].X, point.Y-out[len(out)-1].Y) > tolerance {
			out = append(out, point)
		}
	}
	if len(out) > 1 && math.Hypot(out[0].X-out[len(out)-1].X, out[0].Y-out[len(out)-1].Y) <= tolerance {
		out = out[:len(out)-1]
	}
	return out
}

func pointSegmentDistance(point, start, end VectorPoint) float64 {
	dx, dy := end.X-start.X, end.Y-start.Y
	if dx == 0 && dy == 0 {
		return math.Hypot(point.X-start.X, point.Y-start.Y)
	}
	t := ((point.X-start.X)*dx + (point.Y-start.Y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(point.X-(start.X+t*dx), point.Y-(start.Y+t*dy))
}

func vectorQualityScore(kind string, diagnostics []VectorDiagnostic) int {
	score := 100
	if kind == ContentPhoto {
		score -= 18
	}
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case "error":
			score -= 30
		case "warning":
			if diagnostic.Code != "HOLES_PRESENT" {
				score -= 4
			}
		}
	}
	return maxInt(0, minInt(100, score))
}

func borderMedianColor(img image.Image) color.NRGBA {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	rs, gs, bs := make([]int, 0, 2*(width+height)), make([]int, 0, 2*(width+height)), make([]int, 0, 2*(width+height))
	add := func(c color.NRGBA) {
		rs = append(rs, int(c.R))
		gs = append(gs, int(c.G))
		bs = append(bs, int(c.B))
	}
	for x := 0; x < width; x++ {
		add(color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y)).(color.NRGBA))
		add(color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Max.Y-1)).(color.NRGBA))
	}
	for y := 1; y < height-1; y++ {
		add(color.NRGBAModel.Convert(img.At(bounds.Min.X, bounds.Min.Y+y)).(color.NRGBA))
		add(color.NRGBAModel.Convert(img.At(bounds.Max.X-1, bounds.Min.Y+y)).(color.NRGBA))
	}
	sort.Ints(rs)
	sort.Ints(gs)
	sort.Ints(bs)
	mid := len(rs) / 2
	return color.NRGBA{R: uint8(rs[mid]), G: uint8(gs[mid]), B: uint8(bs[mid]), A: 255}
}

func otsuThreshold(values []uint8) int {
	var histogram [256]int
	for _, value := range values {
		histogram[value]++
	}
	total := len(values)
	if total == 0 {
		return 0
	}
	sum := 0.0
	for value, count := range histogram {
		sum += float64(value * count)
	}
	weightBackground, sumBackground := 0, 0.0
	bestVariance := -1.0
	threshold := 0
	for value, count := range histogram {
		weightBackground += count
		if weightBackground == 0 {
			continue
		}
		weightForeground := total - weightBackground
		if weightForeground == 0 {
			break
		}
		sumBackground += float64(value * count)
		meanBackground := sumBackground / float64(weightBackground)
		meanForeground := (sum - sumBackground) / float64(weightForeground)
		delta := meanBackground - meanForeground
		variance := float64(weightBackground*weightForeground) * delta * delta
		if variance > bestVariance {
			bestVariance = variance
			threshold = value
		}
	}
	return threshold
}

func maskEdgeDensity(mask []bool, width, height int) float64 {
	if width < 2 || height < 2 {
		return 0
	}
	edges := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			if x+1 < width && mask[i] != mask[i+1] {
				edges++
			}
			if y+1 < height && mask[i] != mask[i+width] {
				edges++
			}
		}
	}
	return float64(edges) / float64(2*width*height)
}

func significantColorCount(buckets map[uint16]int, total int) int {
	minimum := maxInt(1, total/2000)
	count := 0
	for _, occurrences := range buckets {
		if occurrences >= minimum {
			count++
		}
	}
	return count
}

func pixelLuminance(c color.NRGBA) uint8 {
	return uint8((299*uint32(c.R) + 587*uint32(c.G) + 114*uint32(c.B)) / 1000)
}

func countMask(mask []bool) int {
	count := 0
	for _, on := range mask {
		if on {
			count++
		}
	}
	return count
}

func roundTo(value float64, places int) float64 {
	power := math.Pow10(places)
	return math.Round(value*power) / power
}
