package ai

import (
	"image"
	"image/color"
	"testing"
)

func TestStubPrepHardensAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.SetNRGBA(1, 1, color.NRGBA{R: 10, G: 20, B: 30, A: 120})
	g := &Gateway{Provider: StubProvider{}, Credits: noopCredits{}, Upscale: 1, RemoveBG: false}
	out, tracer, err := g.PrepareForVectorize(img)
	if err != nil {
		t.Fatal(err)
	}
	if tracer != TracerPotrace {
		t.Fatalf("tracer=%s", tracer)
	}
	c := color.NRGBAModel.Convert(out.At(1, 1)).(color.NRGBA)
	if c.A != 255 {
		t.Fatalf("expected hardened alpha, got %d", c.A)
	}
}

func TestStubPrepRemoveBGMarksML(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{A: 200})
	g := &Gateway{Provider: StubProvider{}, Credits: noopCredits{}, Upscale: 1, RemoveBG: true}
	_, tracer, err := g.PrepareForVectorize(img)
	if err != nil {
		t.Fatal(err)
	}
	if tracer != TracerMLAssisted {
		t.Fatalf("tracer=%s", tracer)
	}
}
