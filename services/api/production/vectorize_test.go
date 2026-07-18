package production

import (
	"context"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseSVGPathRingsPotraceTransform(t *testing.T) {
	svg := `<?xml version="1.0"?>
<svg width="40pt" height="40pt" viewBox="0 0 40 40">
<g transform="translate(0.000000,40.000000) scale(0.100000,-0.100000)" fill="#000000" stroke="none">
<path d="M50 50 L350 50 L350 350 L50 350 Z"/>
</g>
</svg>`
	rings, err := ParseSVGPathRings(svg, 40, 40)
	if err != nil {
		t.Fatal(err)
	}
	if len(rings) != 1 {
		t.Fatalf("want 1 ring, got %d", len(rings))
	}
	// 50*0.1=5, 40-50*0.1=35 → corners near (5,35)-(35,35)-(35,5)-(5,5)
	minX, maxX, minY, maxY := bbox(rings[0])
	if minX < 4 || minX > 6 || maxX < 34 || maxX > 36 || minY < 4 || minY > 6 || maxY < 34 || maxY > 36 {
		t.Fatalf("unexpected bbox (%.1f,%.1f)-(%.1f,%.1f)", minX, minY, maxX, maxY)
	}
}

func TestParseSVGPathRingsDonut(t *testing.T) {
	svg := `<svg viewBox="0 0 100 100"><g transform="translate(0,100) scale(1,-1)">
<path d="M10 10 L90 10 L90 90 L10 90 Z"/>
<path d="M30 30 L70 30 L70 70 L30 70 Z"/>
</g></svg>`
	rings, err := ParseSVGPathRings(svg, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rings) != 2 {
		t.Fatalf("want outer+hole, got %d rings", len(rings))
	}
}

func TestWriteAlphaPBM(t *testing.T) {
	img := solidSquare(32, 8, 8, 16)
	dir := t.TempDir()
	path := filepath.Join(dir, "mask.pbm")
	if err := WriteAlphaPBM(img, path, DefaultAlphaCutoff); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 10 || string(data[:2]) != "P4" {
		t.Fatalf("expected P4 header, got %q", data[:min(20, len(data))])
	}
}

func TestVectorizeAlphaSquare(t *testing.T) {
	potrace, err := exec.LookPath("potrace")
	if err != nil {
		t.Skip("potrace not installed")
	}
	img := solidSquare(48, 8, 8, 32)
	set, err := Vectorize(context.Background(), img, VectorizeOptions{Tools: NativeTools{Potrace: potrace}})
	if err != nil {
		t.Fatal(err)
	}
	if set.PathCount < 1 {
		t.Fatal("expected at least one contour")
	}
	if set.SourceHash == "" {
		t.Fatal("missing source hash")
	}
	if set.Tracer != TracerPotrace {
		t.Fatalf("tracer=%s", set.Tracer)
	}
	// Determinism: same image → same hash
	set2, err := Vectorize(context.Background(), img, VectorizeOptions{Tools: NativeTools{Potrace: potrace}})
	if err != nil {
		t.Fatal(err)
	}
	if set.SourceHash != set2.SourceHash {
		t.Fatal("source hash not stable")
	}
}

func TestVectorizeDonut(t *testing.T) {
	potrace, err := exec.LookPath("potrace")
	if err != nil {
		t.Skip("potrace not installed")
	}
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
	set, err := Vectorize(context.Background(), img, VectorizeOptions{Tools: NativeTools{Potrace: potrace}})
	if err != nil {
		t.Fatal(err)
	}
	if set.PathCount < 2 {
		t.Fatalf("expected outer+hole, got %d paths", set.PathCount)
	}
}

func TestVectorizeMissingPotrace(t *testing.T) {
	img := solidSquare(24, 4, 4, 16)
	_, err := Vectorize(context.Background(), img, VectorizeOptions{Tools: NativeTools{Potrace: "definitely-missing-potrace-bin"}})
	if err == nil {
		t.Fatal("expected potrace unavailable error")
	}
}

func TestQualityGatePathExplosion(t *testing.T) {
	rings := make([][]VectorPoint, MaxVectorPaths+1)
	for i := range rings {
		rings[i] = []VectorPoint{{0, 0}, {10, 0}, {10, 10}}
	}
	ds := QualityGate(rings, "px")
	if !HasVectorErrors(ds) {
		t.Fatal("expected path explosion error")
	}
}

func solidSquare(size, x0, y0, side int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := y0; y < y0+side; y++ {
		for x := x0; x < x0+side; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	return img
}

func bbox(ring []VectorPoint) (minX, maxX, minY, maxY float64) {
	minX, minY = ring[0].X, ring[0].Y
	maxX, maxY = minX, minY
	for _, p := range ring[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	return
}
