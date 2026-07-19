package production

import (
	"image"
	"image/color"
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
	mask, metadata, profile, err := prepareVectorMask(img, "vinyl", DefaultAlphaCutoff)
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
