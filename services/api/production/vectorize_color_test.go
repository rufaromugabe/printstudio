package production

import (
	"context"
	"image"
	"image/color"
	"os"
	"os/exec"
	"testing"
)

func TestMergeLabClustersAbsorbsAntiAliasFringe(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 80))
	mask := make([]bool, 120*80)
	for y := 10; y < 70; y++ {
		for x := 10; x < 55; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 51, G: 68, B: 190, A: 255})
			mask[y*120+x] = true
		}
		for x := 65; x < 110; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 198, G: 29, B: 61, A: 255})
			mask[y*120+x] = true
		}
	}
	// Mid-blue AA fringe between the spots — within ΔE00≈5 of the blue fill.
	for y := 20; y < 60; y++ {
		for x := 55; x < 58; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 58, G: 74, B: 182, A: 255})
			mask[y*120+x] = true
		}
	}
	buckets := buildLabBuckets(img, mask, 120, 80)
	clusters := mergeLabClusters(clusterLabBuckets(buckets, 8), 5.0)
	if len(clusters) != 2 {
		t.Fatalf("expected AA fringe to merge into two spot colours, got %d: %#v", len(clusters), clusters)
	}
}

func TestVectorizeColorDropsSpecklyFringeLayer(t *testing.T) {
	potrace := os.Getenv("POTRACE_BIN")
	if potrace == "" {
		var err error
		potrace, err = exec.LookPath("potrace")
		if err != nil {
			t.Skip("potrace not installed")
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, 160, 120))
	for y := 15; y < 105; y++ {
		for x := 15; x < 95; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 51, G: 68, B: 190, A: 255})
		}
		for x := 105; x < 145; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 198, G: 29, B: 61, A: 255})
		}
	}
	// Scatter a third near-blue fringe that previously survived ΔE00=3 merges.
	for i, p := range []image.Point{{96, 20}, {97, 40}, {98, 60}, {96, 80}, {99, 90}} {
		c := color.NRGBA{R: uint8(65 + i*2), G: uint8(80 + i), B: 170, A: 255}
		img.SetNRGBA(p.X, p.Y, c)
		img.SetNRGBA(p.X+1, p.Y, c)
	}
	result, err := VectorizeColor(context.Background(), img, VectorizeOptions{
		Method: "embroidery", Tools: NativeTools{Potrace: potrace},
	}, 8)
	if err != nil {
		t.Fatalf("colour vectorize: %v result=%+v", err, result)
	}
	if len(result.Layers) != 2 {
		t.Fatalf("expected two production colour layers, got %d: %+v", len(result.Layers), result.Layers)
	}
	for _, layer := range result.Layers {
		if layer.Contours.Similarity.Status == "fail" {
			t.Fatalf("layer %s failed similarity: %+v", layer.Color, layer.Contours.Similarity)
		}
	}
}

func TestClassifyKeepsChunkyLogoAsFlatArt(t *testing.T) {
	kind, _ := classifyVectorContent(true, 0.39, 0.007, 3, 6)
	if kind != ContentFlatArt {
		t.Fatalf("chunky two-tone logo should be flat-art, got %q", kind)
	}
	kind, _ = classifyVectorContent(false, 0.18, 0.05, 3, 2)
	if kind != ContentTextLike {
		t.Fatalf("sparse letter stems should stay text-like, got %q", kind)
	}
	kind, _ = classifyVectorContent(true, 0.273, 0.04, 3, 1)
	if kind != ContentTextLike {
		t.Fatalf("three opaque letter stems should stay text-like, got %q", kind)
	}
}
