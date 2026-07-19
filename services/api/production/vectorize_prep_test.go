package production

import (
	"image"
	"image/color"
	"math"
	"slices"
	"testing"
)

func TestAnalyzeVectorArtworkRemovesOpaqueWhiteBackground(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 80, 40))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	// Three letter-like stems on an opaque JPEG-style white background.
	for _, x0 := range []int{12, 33, 54} {
		for y := 8; y < 33; y++ {
			for x := x0; x < x0+8; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 24, B: 28, A: 255})
			}
		}
	}
	analysis, err := analyzeVectorArtwork(img, DefaultAlphaCutoff)
	if err != nil {
		t.Fatal(err)
	}
	if !analysis.backgroundRemoved || analysis.maskSource != "border-color" {
		t.Fatalf("expected border background extraction, got %+v", analysis)
	}
	if analysis.kind != ContentTextLike {
		t.Fatalf("expected text-like detection, got %q", analysis.kind)
	}
	if analysis.foregroundRatio >= 0.5 {
		t.Fatalf("white background leaked into mask: ratio %.3f", analysis.foregroundRatio)
	}
}

func TestPrepareVectorMaskSupersamplesSmallText(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 40))
	for _, rect := range []image.Rectangle{
		image.Rect(8, 8, 22, 34), image.Rect(35, 8, 49, 34), image.Rect(62, 8, 76, 34),
	} {
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				img.SetNRGBA(x, y, color.NRGBA{R: 10, A: 255})
			}
		}
	}
	mask, metadata, profile, err := prepareVectorMask(img, "vinyl", DefaultAlphaCutoff, "")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ContentKind != ContentTextLike || metadata.UpscaleFactor != 4 {
		t.Fatalf("unexpected prep metadata: %+v", metadata)
	}
	if profile.name != "vinyl-cut-crisp-text" {
		t.Fatalf("unexpected profile %q", profile.name)
	}
	if mask.Bounds().Dx() != 400 || mask.Bounds().Dy() != 160 {
		t.Fatalf("unexpected prepared size %v", mask.Bounds())
	}
}

func TestAnalyzeVectorArtworkFlagsContinuousTonePhoto(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 96, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 96; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / 95), G: uint8(y * 255 / 63), B: uint8((x + y) * 255 / 158), A: 255})
		}
	}
	analysis, err := analyzeVectorArtwork(img, DefaultAlphaCutoff)
	if err != nil {
		t.Fatal(err)
	}
	if analysis.kind != ContentPhoto {
		t.Fatalf("continuous-tone artwork must be review-gated as a photo, got %q (%d colours)", analysis.kind, analysis.colorCount)
	}
}

func TestPolishVectorRingsPreservesCorners(t *testing.T) {
	ring := []VectorPoint{{0, 0}, {2, 0.01}, {4, 0}, {4, 2}, {4, 4}, {2, 4.01}, {0, 4}, {0, 2}}
	polished, removed := polishVectorRings([][]VectorPoint{ring}, 0.05)
	if removed == 0 || len(polished) != 1 {
		t.Fatalf("expected redundant points to be polished: removed=%d rings=%v", removed, polished)
	}
	if len(polished[0]) != 4 {
		t.Fatalf("square corners should remain, got %d points: %#v", len(polished[0]), polished[0])
	}
}

func TestPolishVectorRingsPreservesDenseCurves(t *testing.T) {
	// Dense samples along a semicircle used to vanish when every locally flat
	// vertex was dropped in one parallel pass after supersampling.
	const n = 180
	ring := make([]VectorPoint, 0, n+2)
	for i := 0; i <= n; i++ {
		theta := math.Pi * float64(i) / float64(n)
		ring = append(ring, VectorPoint{X: 50 + 40*math.Cos(theta), Y: 50 + 40*math.Sin(theta)})
	}
	ring = append(ring, VectorPoint{X: 10, Y: 50}, VectorPoint{X: 90, Y: 50})
	polished, removed := polishVectorRings([][]VectorPoint{ring}, 0.25)
	if len(polished) != 1 {
		t.Fatalf("expected one polished ring, got %d", len(polished))
	}
	if removed == 0 || len(polished[0]) >= len(ring) {
		t.Fatalf("expected some densifying samples to be removed, removed=%d points=%d", removed, len(polished[0]))
	}
	if len(polished[0]) < 8 {
		t.Fatalf("dense curve collapsed to a chord under polish: points=%d", len(polished[0]))
	}
	// Area should stay within ~5% of the original semicircle bowl.
	orig := math.Abs(signedArea(ring))
	got := math.Abs(signedArea(polished[0]))
	if got < orig*0.95 || got > orig*1.05 {
		t.Fatalf("polished curve area drifted too far: orig=%.1f got=%.1f points=%d", orig, got, len(polished[0]))
	}
}

func TestDrawRotatedUsesContinuousInverseSampling(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 24, 14))
	for y := 0; y < 14; y++ {
		for x := 0; x < 24; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 240, G: 30, B: 20, A: 255})
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, 80, 80))
	drawRotated(dst, src, 40, 40, 48, 28, 27)
	for y := 32; y <= 48; y += 4 {
		first, last := -1, -1
		for x := 0; x < 80; x++ {
			if dst.NRGBAAt(x, y).A > 16 {
				if first < 0 {
					first = x
				}
				last = x
			}
		}
		if first < 0 {
			t.Fatalf("rotated artwork missing on row %d", y)
		}
		for x := first; x <= last; x++ {
			if dst.NRGBAAt(x, y).A == 0 {
				t.Fatalf("forward-map pinhole remained at (%d,%d)", x, y)
			}
		}
	}
}

func TestVectorPlacementRejectsInvalidPhysicalMapping(t *testing.T) {
	err := validateVectorPlacement(VectorizePlacement{CanvasWidth: 0, CanvasHeight: 100, PhysicalWidthMm: 50, PhysicalHeightMm: 50, W: 20, H: 20})
	if err == nil {
		t.Fatal("zero canvas width must fail before producing non-finite cut coordinates")
	}
}

func TestMethodFeatureThresholdsProtectProcessDetail(t *testing.T) {
	vinylWarn, vinylReject := methodFeatureThresholds("vinyl", "mm")
	screenWarn, screenReject := methodFeatureThresholds("screen", "mm")
	if vinylWarn <= screenWarn || vinylReject <= screenReject {
		t.Fatalf("vinyl must be more conservative than screen: vinyl %.2f/%.2f screen %.2f/%.2f", vinylWarn, vinylReject, screenWarn, screenReject)
	}
}

func TestTraceProfileProducesExplicitPotraceArguments(t *testing.T) {
	profile := vectorProfile("vinyl", ContentTextLike)
	args := tracePBMArgs("mask.pbm", "mask.svg", profile.trace)
	for _, expected := range []string{"--turdsize", "--alphamax", "--opttolerance", "--turnpolicy", "minority", "--output", "mask.svg", "mask.pbm"} {
		if !slices.Contains(args, expected) {
			t.Fatalf("missing %q in Potrace args %#v", expected, args)
		}
	}
}

func TestVectorSimilarityPassesFaithfulContour(t *testing.T) {
	img := solidSquare(64, 16, 16, 32)
	rings := [][]VectorPoint{{{16, 16}, {48, 16}, {48, 48}, {16, 48}}}
	report, diagnostics := EvaluateVectorSimilarity(img, rings, ContentFlatArt, true)
	if report.Status != "pass" || report.Score < 95 {
		t.Fatalf("faithful contour should pass: %+v diagnostics=%+v", report, diagnostics)
	}
	if report.ProofPNGBase64 == "" {
		t.Fatal("requested similarity overlay was omitted")
	}
}

func TestVectorSimilarityRejectsLostTextCounter(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 8; y < 56; y++ {
		for x := 8; x < 56; x++ {
			img.SetNRGBA(x, y, color.NRGBA{A: 255})
		}
	}
	for y := 24; y < 40; y++ {
		for x := 24; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{})
		}
	}
	// Deliberately omit the inner counter from the reconstructed rings.
	rings := [][]VectorPoint{{{8, 8}, {56, 8}, {56, 56}, {8, 56}}}
	report, diagnostics := EvaluateVectorSimilarity(img, rings, ContentTextLike, false)
	if report.Status != "fail" || report.MissingCounters != 1 || !HasVectorErrors(diagnostics) {
		t.Fatalf("lost counter must hard fail: %+v diagnostics=%+v", report, diagnostics)
	}
}

func TestParseTesseractTSVBuildsLinesAndConfidence(t *testing.T) {
	tsv := []byte("level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t10\t10\t40\t20\t95.0\tHELLO\n" +
		"5\t1\t1\t1\t1\t2\t55\t10\t40\t20\t85.0\tSHOP\n" +
		"5\t1\t1\t1\t2\t1\t10\t35\t30\t20\t90.0\t2026\n")
	words, text, confidence := parseTesseractTSV(tsv)
	if len(words) != 3 || text != "HELLO SHOP\n2026" {
		t.Fatalf("unexpected OCR parse: %q %#v", text, words)
	}
	if confidence < 89 || confidence > 93 {
		t.Fatalf("unexpected weighted confidence %.2f", confidence)
	}
	candidates := inferFontCandidates(text, words)
	if len(candidates) == 0 || candidates[0].Family != "Impact" {
		t.Fatalf("uppercase display text should surface Impact as a candidate: %#v", candidates)
	}
}

func TestServerLabPalettePreservesFlatSpotColors(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 80, 40))
	mask := make([]bool, 80*40)
	for y := 5; y < 35; y++ {
		for x := 5; x < 75; x++ {
			pixel := color.NRGBA{R: 210, G: 25, B: 45, A: 255}
			if x >= 40 {
				pixel = color.NRGBA{R: 20, G: 65, B: 190, A: 255}
			}
			img.SetNRGBA(x, y, pixel)
			mask[y*80+x] = true
		}
	}
	buckets := buildLabBuckets(img, mask, 80, 40)
	clusters := mergeLabClusters(clusterLabBuckets(buckets, 8), 5)
	report := evaluateColorSimilarity(buckets, clusters, 8)
	if len(clusters) != 2 || report.Status != "pass" || report.MeanDeltaE > 0.5 {
		t.Fatalf("flat two-colour artwork should survive server clustering: clusters=%#v report=%+v", clusters, report)
	}
}
