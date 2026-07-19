package production

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"sort"
)

const maxSimilarityDimension = 768

// VectorSimilarityReport closes the QA loop by comparing a rasterized copy of
// the final contours with the exact binary mask sent to Potrace. Metrics are
// deterministic and operate at a bounded proof resolution.
type VectorSimilarityReport struct {
	Score                 int     `json:"score"`
	Status                string  `json:"status"` // pass|review|fail
	IntersectionOverUnion float64 `json:"intersectionOverUnion"`
	Precision             float64 `json:"precision"`
	Recall                float64 `json:"recall"`
	EdgeF1                float64 `json:"edgeF1"`
	SourceComponents      int     `json:"sourceComponents"`
	TracedComponents      int     `json:"tracedComponents"`
	SourceCounters        int     `json:"sourceCounters"`
	TracedCounters        int     `json:"tracedCounters"`
	MissingComponents     int     `json:"missingComponents"`
	MissingCounters       int     `json:"missingCounters"`
	ProofWidthPx          int     `json:"proofWidthPx"`
	ProofHeightPx         int     `json:"proofHeightPx"`
	ProofPNGBase64        string  `json:"proofPngBase64,omitempty"`
}

func EvaluateVectorSimilarity(source image.Image, rings [][]VectorPoint, contentKind string, includeProof bool) (VectorSimilarityReport, []VectorDiagnostic) {
	if source == nil || len(rings) == 0 {
		return VectorSimilarityReport{Status: "fail"}, []VectorDiagnostic{{Severity: "error", Code: "SIMILARITY_UNAVAILABLE", Message: "vector similarity proof could not be generated"}}
	}
	bounds := source.Bounds()
	proofWidth, proofHeight := similarityProofSize(bounds.Dx(), bounds.Dy())
	sourceMask := sampleAlphaMask(source, proofWidth, proofHeight)
	tracedMask := rasterizeVectorRings(rings, bounds.Dx(), bounds.Dy(), proofWidth, proofHeight)

	intersection, union, sourceCount, tracedCount := 0, 0, 0, 0
	for i, sourceOn := range sourceMask {
		traceOn := tracedMask[i]
		if sourceOn {
			sourceCount++
		}
		if traceOn {
			tracedCount++
		}
		if sourceOn && traceOn {
			intersection++
		}
		if sourceOn || traceOn {
			union++
		}
	}
	precision := safeRatio(intersection, tracedCount)
	recall := safeRatio(intersection, sourceCount)
	iou := safeRatio(intersection, union)
	edgeF1 := tolerantEdgeF1(sourceMask, tracedMask, proofWidth, proofHeight)
	sourceComponents, _ := maskComponentStats(sourceMask, proofWidth, proofHeight)
	tracedComponents, _ := maskComponentStats(tracedMask, proofWidth, proofHeight)
	sourceCounters := enclosedBackgroundComponents(sourceMask, proofWidth, proofHeight)
	tracedCounters := enclosedBackgroundComponents(tracedMask, proofWidth, proofHeight)
	missingComponents := maxInt(0, sourceComponents-tracedComponents)
	missingCounters := maxInt(0, sourceCounters-tracedCounters)

	score := int(math.Round(100 * (0.45*iou + 0.2*precision + 0.2*recall + 0.15*edgeF1)))
	score -= minInt(20, missingComponents*4)
	score -= minInt(24, missingCounters*8)
	score = maxInt(0, minInt(100, score))
	failAt, reviewAt := 76, 90
	if contentKind == ContentTextLike {
		failAt, reviewAt = 82, 94
	}
	status := "pass"
	if score < failAt || recall < 0.78 || (contentKind == ContentTextLike && missingCounters > 0) {
		status = "fail"
	} else if score < reviewAt || recall < 0.9 || missingComponents > 0 {
		status = "review"
	}
	report := VectorSimilarityReport{
		Score: score, Status: status,
		IntersectionOverUnion: roundTo(iou, 4), Precision: roundTo(precision, 4), Recall: roundTo(recall, 4), EdgeF1: roundTo(edgeF1, 4),
		SourceComponents: sourceComponents, TracedComponents: tracedComponents,
		SourceCounters: sourceCounters, TracedCounters: tracedCounters,
		MissingComponents: missingComponents, MissingCounters: missingCounters,
		ProofWidthPx: proofWidth, ProofHeightPx: proofHeight,
	}
	if includeProof {
		report.ProofPNGBase64 = encodeSimilarityProof(sourceMask, tracedMask, proofWidth, proofHeight)
	}
	var diagnostics []VectorDiagnostic
	switch status {
	case "fail":
		diagnostics = append(diagnostics, VectorDiagnostic{Severity: "error", Code: "SIMILARITY_FAILED", Message: similarityMessage(report)})
	case "review":
		diagnostics = append(diagnostics, VectorDiagnostic{Severity: "warning", Code: "SIMILARITY_REVIEW", Message: similarityMessage(report)})
	}
	if missingCounters > 0 {
		diagnostics = append(diagnostics, VectorDiagnostic{Severity: "error", Code: "COUNTERS_LOST", Message: "vector proof lost one or more enclosed counters; raster lettering must be repaired or rebuilt as editable text"})
	}
	return report, diagnostics
}

func similarityMessage(report VectorSimilarityReport) string {
	return fmt.Sprintf("vector similarity %d/100 (IoU %.1f%%, edge fidelity %.1f%%, recall %.1f%%)", report.Score, report.IntersectionOverUnion*100, report.EdgeF1*100, report.Recall*100)
}

func similarityProofSize(width, height int) (int, int) {
	if width <= maxSimilarityDimension && height <= maxSimilarityDimension {
		return width, height
	}
	scale := math.Min(float64(maxSimilarityDimension)/float64(width), float64(maxSimilarityDimension)/float64(height))
	return maxInt(2, int(math.Round(float64(width)*scale))), maxInt(2, int(math.Round(float64(height)*scale)))
}

func sampleAlphaMask(source image.Image, width, height int) []bool {
	b := source.Bounds()
	out := make([]bool, width*height)
	for y := 0; y < height; y++ {
		sy := b.Min.Y + minInt(b.Dy()-1, int((float64(y)+0.5)*float64(b.Dy())/float64(height)))
		for x := 0; x < width; x++ {
			sx := b.Min.X + minInt(b.Dx()-1, int((float64(x)+0.5)*float64(b.Dx())/float64(width)))
			_, _, _, alpha := source.At(sx, sy).RGBA()
			out[y*width+x] = alpha>>8 >= uint32(DefaultAlphaCutoff)
		}
	}
	return out
}

func rasterizeVectorRings(rings [][]VectorPoint, sourceWidth, sourceHeight, width, height int) []bool {
	out := make([]bool, width*height)
	intersections := make([]float64, 0, len(rings)*4)
	for y := 0; y < height; y++ {
		sourceY := (float64(y) + 0.5) * float64(sourceHeight) / float64(height)
		intersections = intersections[:0]
		for _, ring := range rings {
			for i, point := range ring {
				next := ring[(i+1)%len(ring)]
				if (point.Y > sourceY) == (next.Y > sourceY) {
					continue
				}
				x := point.X + (sourceY-point.Y)*(next.X-point.X)/(next.Y-point.Y)
				intersections = append(intersections, x*float64(width)/float64(sourceWidth))
			}
		}
		sort.Float64s(intersections)
		for i := 0; i+1 < len(intersections); i += 2 {
			start := maxInt(0, int(math.Ceil(intersections[i]-0.5)))
			end := minInt(width-1, int(math.Floor(intersections[i+1]-0.5)))
			for x := start; x <= end; x++ {
				out[y*width+x] = true
			}
		}
	}
	return out
}

func tolerantEdgeF1(source, traced []bool, width, height int) float64 {
	sourceEdges, tracedEdges := maskEdges(source, width, height), maskEdges(traced, width, height)
	match := func(edge, other []bool) (matched, total int) {
		for i, on := range edge {
			if !on {
				continue
			}
			total++
			x, y := i%width, i/width
			found := false
			for dy := -1; dy <= 1 && !found; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx >= 0 && ny >= 0 && nx < width && ny < height && other[ny*width+nx] {
						found = true
						break
					}
				}
			}
			if found {
				matched++
			}
		}
		return
	}
	matchedSource, sourceTotal := match(sourceEdges, tracedEdges)
	matchedTrace, traceTotal := match(tracedEdges, sourceEdges)
	recall, precision := safeRatio(matchedSource, sourceTotal), safeRatio(matchedTrace, traceTotal)
	if recall+precision == 0 {
		return 0
	}
	return 2 * recall * precision / (recall + precision)
}

func maskEdges(mask []bool, width, height int) []bool {
	out := make([]bool, len(mask))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			if !mask[i] {
				continue
			}
			if x == 0 || y == 0 || x+1 == width || y+1 == height || !mask[i-1] || !mask[i+1] || !mask[i-width] || !mask[i+width] {
				out[i] = true
			}
		}
	}
	return out
}

func enclosedBackgroundComponents(mask []bool, width, height int) int {
	seen := make([]bool, len(mask))
	queue := make([]int, 0, 64)
	counters := 0
	for start, foreground := range mask {
		if foreground || seen[start] {
			continue
		}
		queue = append(queue[:0], start)
		seen[start] = true
		touchesBorder := false
		for len(queue) > 0 {
			i := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			x, y := i%width, i/width
			if x == 0 || y == 0 || x+1 == width || y+1 == height {
				touchesBorder = true
			}
			for _, neighbor := range maskNeighbors(x, y, width, height) {
				if !mask[neighbor] && !seen[neighbor] {
					seen[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
		if !touchesBorder {
			counters++
		}
	}
	return counters
}

func encodeSimilarityProof(source, traced []bool, width, height int) string {
	proof := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i, sourceOn := range source {
		traceOn := traced[i]
		switch {
		case sourceOn && traceOn:
			proof.Pix[i*4], proof.Pix[i*4+1], proof.Pix[i*4+2], proof.Pix[i*4+3] = 36, 190, 110, 220
		case sourceOn:
			proof.Pix[i*4], proof.Pix[i*4+1], proof.Pix[i*4+2], proof.Pix[i*4+3] = 240, 56, 64, 255
		case traceOn:
			proof.Pix[i*4], proof.Pix[i*4+1], proof.Pix[i*4+2], proof.Pix[i*4+3] = 45, 120, 240, 255
		}
	}
	var buffer bytes.Buffer
	if png.Encode(&buffer, proof) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func safeRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		if numerator == 0 {
			return 1
		}
		return 0
	}
	return float64(numerator) / float64(denominator)
}
