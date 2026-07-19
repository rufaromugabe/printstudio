package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
)

type ColorVectorLayer struct {
	Color      string           `json:"color"`
	PixelCount int              `json:"pixelCount"`
	Coverage   float64          `json:"coverage"`
	Contours   VectorContourSet `json:"contours"`
}

type ColorSimilarityReport struct {
	Score        int     `json:"score"`
	Status       string  `json:"status"` // pass|review|fail
	MeanDeltaE   float64 `json:"meanDeltaE"`
	P95DeltaE    float64 `json:"p95DeltaE"`
	MaxColors    int     `json:"maxColors"`
	ActualColors int     `json:"actualColors"`
}

type ColorVectorResult struct {
	Layers        []ColorVectorLayer    `json:"layers"`
	SourceHash    string                `json:"sourceHash"`
	PaletteMethod string                `json:"paletteMethod"`
	ContentKind   string                `json:"contentKind"`
	Similarity    ColorSimilarityReport `json:"similarity"`
	OCR           OCRReport             `json:"ocr"`
	Diagnostics   []VectorDiagnostic    `json:"diagnostics,omitempty"`
}

type labBucket struct {
	r, g, b float64
	count   int
	lab     Lab
}

type labCluster struct {
	lab   Lab
	count int
}

// VectorizeColor performs deterministic, server-owned Lab clustering and
// traces every resulting separation through the same production QA pipeline.
func VectorizeColor(ctx context.Context, img image.Image, opt VectorizeOptions, maxColors int) (*ColorVectorResult, error) {
	if img == nil {
		return nil, fmt.Errorf("image is required")
	}
	if maxColors <= 0 {
		maxColors = 8
	}
	if maxColors < 2 || maxColors > 16 {
		return nil, fmt.Errorf("maxColors must be between 2 and 16")
	}
	prepared := img
	if opt.Prep != nil {
		var err error
		prepared, _, err = opt.Prep.PrepareForVectorize(img)
		if err != nil {
			return nil, fmt.Errorf("colour prep: %w", err)
		}
	}
	analysis, err := analyzeVectorArtwork(prepared, opt.AlphaCutoff)
	if err != nil {
		return nil, err
	}
	buckets := buildLabBuckets(prepared, analysis.mask, analysis.width, analysis.height)
	if len(buckets) == 0 {
		return nil, fmt.Errorf("no opaque colours remained after foreground isolation")
	}
	clusters := clusterLabBuckets(buckets, maxColors)
	clusters = mergeLabClusters(clusters, 3.0)
	if len(clusters) == 0 {
		return nil, fmt.Errorf("colour clustering produced no separations")
	}

	bounds := prepared.Bounds()
	masks := make([]*image.Alpha, len(clusters))
	counts := make([]int, len(clusters))
	sumR, sumG, sumB := make([]float64, len(clusters)), make([]float64, len(clusters)), make([]float64, len(clusters))
	for i := range masks {
		masks[i] = image.NewAlpha(image.Rect(0, 0, analysis.width, analysis.height))
	}
	for y := 0; y < analysis.height; y++ {
		for x := 0; x < analysis.width; x++ {
			i := y*analysis.width + x
			if !analysis.mask[i] {
				continue
			}
			pixel := color.NRGBAModel.Convert(prepared.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			lab := RGBToLab(float64(pixel.R)/255, float64(pixel.G)/255, float64(pixel.B)/255)
			cluster := nearestLabCluster(lab, clusters)
			masks[cluster].Pix[i] = 255
			counts[cluster]++
			sumR[cluster] += float64(pixel.R)
			sumG[cluster] += float64(pixel.G)
			sumB[cluster] += float64(pixel.B)
		}
	}

	result := &ColorVectorResult{PaletteMethod: "cielab-weighted-kmeans", ContentKind: analysis.kind}
	result.SourceHash = colorVectorHash(prepared, opt.Method, maxColors)
	result.Similarity = evaluateColorSimilarity(buckets, clusters, maxColors)
	if opt.EnableOCR && analysis.kind == ContentTextLike {
		result.OCR = opt.Tools.RecognizeRasterText(ctx, binaryOCRImage(analysis.mask, analysis.width, analysis.height))
	}
	if analysis.kind == ContentPhoto {
		result.Diagnostics = append(result.Diagnostics, VectorDiagnostic{Severity: "warning", Code: "PHOTO_COLOR_VECTOR_REVIEW", Message: "continuous-tone artwork was reduced to spot-colour vectors; DTF/sublimation raster or screen halftones will retain more detail"})
	}
	switch result.Similarity.Status {
	case "fail":
		result.Diagnostics = append(result.Diagnostics, VectorDiagnostic{Severity: "error", Code: "COLOR_SIMILARITY_FAILED", Message: fmt.Sprintf("palette reconstruction loses too much colour detail (mean ΔE00 %.2f, 95th percentile %.2f)", result.Similarity.MeanDeltaE, result.Similarity.P95DeltaE)})
	case "review":
		result.Diagnostics = append(result.Diagnostics, VectorDiagnostic{Severity: "warning", Code: "COLOR_SIMILARITY_REVIEW", Message: fmt.Sprintf("review palette reduction (mean ΔE00 %.2f, 95th percentile %.2f)", result.Similarity.MeanDeltaE, result.Similarity.P95DeltaE)})
	}

	foregroundPixels := countMask(analysis.mask)
	minimumPixels := maxInt(6, foregroundPixels/20_000)
	for cluster, mask := range masks {
		if counts[cluster] < minimumPixels {
			continue
		}
		layerOpt := opt
		layerOpt.Prep = nil
		layerOpt.EnableOCR = false
		layerOpt.IncludeProof = opt.IncludeProof && cluster == 0
		contours, traceErr := Vectorize(ctx, mask, layerOpt)
		if traceErr != nil {
			return result, fmt.Errorf("colour layer %d failed: %w", cluster+1, traceErr)
		}
		layerColor := averageHex(sumR[cluster], sumG[cluster], sumB[cluster], counts[cluster])
		result.Layers = append(result.Layers, ColorVectorLayer{
			Color: layerColor, PixelCount: counts[cluster], Coverage: roundTo(float64(counts[cluster])/float64(foregroundPixels), 4), Contours: *contours,
		})
	}
	if len(result.Layers) == 0 {
		return result, fmt.Errorf("no colour separation survived production tracing")
	}
	if HasVectorErrors(result.Diagnostics) {
		return result, fmt.Errorf("server colour vectorization failed quality gates")
	}
	return result, nil
}

func buildLabBuckets(img image.Image, mask []bool, width, height int) []labBucket {
	type accumulator struct {
		r, g, b float64
		count   int
	}
	buckets := map[uint16]accumulator{}
	bounds := img.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			if !mask[i] {
				continue
			}
			pixel := color.NRGBAModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			key := uint16(pixel.R>>3)<<10 | uint16(pixel.G>>3)<<5 | uint16(pixel.B>>3)
			bucket := buckets[key]
			bucket.r += float64(pixel.R)
			bucket.g += float64(pixel.G)
			bucket.b += float64(pixel.B)
			bucket.count++
			buckets[key] = bucket
		}
	}
	out := make([]labBucket, 0, len(buckets))
	for _, bucket := range buckets {
		r, g, b := bucket.r/float64(bucket.count), bucket.g/float64(bucket.count), bucket.b/float64(bucket.count)
		out = append(out, labBucket{r: r, g: g, b: b, count: bucket.count, lab: RGBToLab(r/255, g/255, b/255)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].count > out[j].count })
	return out
}

func clusterLabBuckets(buckets []labBucket, maxColors int) []labCluster {
	minimumBucket := maxInt(1, totalBucketCount(buckets)/5000)
	significant := 0
	for _, bucket := range buckets {
		if bucket.count >= minimumBucket {
			significant++
		}
	}
	k := minInt(maxColors, maxInt(1, significant))
	centers := []labCluster{{lab: buckets[0].lab}}
	for len(centers) < k {
		bestIndex, bestDistance := 0, -1.0
		for i, bucket := range buckets {
			distance := math.Inf(1)
			for _, center := range centers {
				distance = math.Min(distance, labDistanceSquared(bucket.lab, center.lab))
			}
			distance *= float64(bucket.count)
			if distance > bestDistance {
				bestIndex, bestDistance = i, distance
			}
		}
		centers = append(centers, labCluster{lab: buckets[bestIndex].lab})
	}
	for iteration := 0; iteration < 15; iteration++ {
		sumL, sumA, sumB, weights := make([]float64, k), make([]float64, k), make([]float64, k), make([]int, k)
		for _, bucket := range buckets {
			cluster := nearestLabCluster(bucket.lab, centers)
			weight := bucket.count
			sumL[cluster] += bucket.lab.L * float64(weight)
			sumA[cluster] += bucket.lab.A * float64(weight)
			sumB[cluster] += bucket.lab.B * float64(weight)
			weights[cluster] += weight
		}
		for i := range centers {
			if weights[i] > 0 {
				centers[i] = labCluster{lab: Lab{L: sumL[i] / float64(weights[i]), A: sumA[i] / float64(weights[i]), B: sumB[i] / float64(weights[i])}, count: weights[i]}
			}
		}
	}
	sort.Slice(centers, func(i, j int) bool { return centers[i].count > centers[j].count })
	return centers
}

func mergeLabClusters(clusters []labCluster, threshold float64) []labCluster {
	out := make([]labCluster, 0, len(clusters))
	for _, cluster := range clusters {
		merged := false
		for i := range out {
			if DeltaE2000(cluster.lab, out[i].lab) < threshold {
				total := out[i].count + cluster.count
				if total > 0 {
					out[i].lab = Lab{L: (out[i].lab.L*float64(out[i].count) + cluster.lab.L*float64(cluster.count)) / float64(total), A: (out[i].lab.A*float64(out[i].count) + cluster.lab.A*float64(cluster.count)) / float64(total), B: (out[i].lab.B*float64(out[i].count) + cluster.lab.B*float64(cluster.count)) / float64(total)}
				}
				out[i].count = total
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, cluster)
		}
	}
	return out
}

func nearestLabCluster(lab Lab, clusters []labCluster) int {
	best, distance := 0, math.Inf(1)
	for i, cluster := range clusters {
		candidate := labDistanceSquared(lab, cluster.lab)
		if candidate < distance {
			best, distance = i, candidate
		}
	}
	return best
}

func labDistanceSquared(a, b Lab) float64 {
	dL, dA, dB := a.L-b.L, a.A-b.A, a.B-b.B
	return dL*dL + dA*dA + dB*dB
}

func evaluateColorSimilarity(buckets []labBucket, clusters []labCluster, maxColors int) ColorSimilarityReport {
	type weightedDistance struct {
		value float64
		count int
	}
	distances := make([]weightedDistance, 0, len(buckets))
	weighted, total := 0.0, 0
	for _, bucket := range buckets {
		cluster := clusters[nearestLabCluster(bucket.lab, clusters)]
		distance := DeltaE2000(bucket.lab, cluster.lab)
		distances = append(distances, weightedDistance{value: distance, count: bucket.count})
		weighted += distance * float64(bucket.count)
		total += bucket.count
	}
	sort.Slice(distances, func(i, j int) bool { return distances[i].value < distances[j].value })
	target, cumulative, p95 := int(math.Ceil(float64(total)*0.95)), 0, 0.0
	for _, distance := range distances {
		cumulative += distance.count
		if cumulative >= target {
			p95 = distance.value
			break
		}
	}
	mean := weighted / float64(maxInt(1, total))
	score := int(math.Round(100 - mean*3 - math.Max(0, p95-8)*1.5))
	score = maxInt(0, minInt(100, score))
	status := "pass"
	if mean > 9 || p95 > 22 {
		status = "fail"
	} else if mean > 4.5 || p95 > 12 {
		status = "review"
	}
	return ColorSimilarityReport{Score: score, Status: status, MeanDeltaE: roundTo(mean, 3), P95DeltaE: roundTo(p95, 3), MaxColors: maxColors, ActualColors: len(clusters)}
}

func totalBucketCount(buckets []labBucket) int {
	total := 0
	for _, bucket := range buckets {
		total += bucket.count
	}
	return total
}

func averageHex(r, g, b float64, count int) string {
	if count <= 0 {
		return "#000000"
	}
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(r/float64(count))), int(math.Round(g/float64(count))), int(math.Round(b/float64(count))))
}

func colorVectorHash(img image.Image, method string, maxColors int) string {
	var buffer bytes.Buffer
	_ = png.Encode(&buffer, img)
	hash := sha256.New()
	_, _ = hash.Write(buffer.Bytes())
	_, _ = hash.Write([]byte(fmt.Sprintf("\ncolour:%s:%d", normalizeVectorMethod(method), maxColors)))
	return hex.EncodeToString(hash.Sum(nil))
}
