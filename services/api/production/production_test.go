package production

import (
	"image"
	"image/color"
	"testing"
)

func TestUnderbaseSpreadAndChoke(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 9, 9))
	for y := 3; y <= 5; y++ {
		for x := 3; x <= 5; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	spread := Underbase(src, UnderbaseConfig{SpreadPixels: 1})
	if spread.AlphaAt(2, 4).A != 255 || spread.AlphaAt(2, 2).A != 0 {
		t.Fatal("spread is not Euclidean")
	}
	choke := Underbase(src, UnderbaseConfig{SpreadPixels: -1})
	if choke.AlphaAt(4, 4).A != 255 || choke.AlphaAt(3, 4).A != 0 {
		t.Fatal("choke did not erode edge")
	}
}

func TestUnderbaseEmptyAndFull(t *testing.T) {
	empty := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	if Underbase(empty, UnderbaseConfig{SpreadPixels: 5}).AlphaAt(0, 0).A != 0 {
		t.Fatal("empty mask spread became nonempty")
	}
	full := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range full.Pix {
		full.Pix[i] = 255
	}
	if Underbase(full, UnderbaseConfig{SpreadPixels: -5}).AlphaAt(2, 2).A != 255 {
		t.Fatal("full mask choke should remain full without background samples")
	}
}

func TestHalftoneIsDeterministicAndMonotonic(t *testing.T) {
	coverage := func(gray uint8) int {
		img := image.NewGray(image.Rect(0, 0, 128, 128))
		for i := range img.Pix {
			img.Pix[i] = gray
		}
		out := AMHalftone(img, HalftoneConfig{DPI: 300, LPI: 45, AngleDegrees: 22.5})
		n := 0
		for _, v := range out.Pix {
			if v > 0 {
				n++
			}
		}
		again := AMHalftone(img, HalftoneConfig{DPI: 300, LPI: 45, AngleDegrees: 22.5})
		for i := range out.Pix {
			if out.Pix[i] != again.Pix[i] {
				t.Fatal("halftone is nondeterministic")
			}
		}
		return n
	}
	dark, mid, light := coverage(0), coverage(128), coverage(240)
	if !(dark > mid && mid > light) {
		t.Fatalf("coverage not monotonic: %d %d %d", dark, mid, light)
	}
}

func TestCMYKSeparationPrimaries(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 0, G: 255, B: 255, A: 255})
	img.Set(1, 0, color.Black)
	s := SeparateCMYK(img)
	if s.Cyan.GrayAt(0, 0).Y < 250 || s.Magenta.GrayAt(0, 0).Y != 0 || s.Black.GrayAt(1, 0).Y < 250 {
		t.Fatal("unexpected CMYK separation")
	}
}

func TestMaxRectsNesting(t *testing.T) {
	p, err := Nest(Sheet{WidthMM: 100, HeightMM: 70, MarginMM: 2, GapMM: 2}, []Item{{ID: "logo", WidthMM: 30, HeightMM: 20, Quantity: 4, AllowRotate: true}, {ID: "tag", WidthMM: 12, HeightMM: 40, Quantity: 2, AllowRotate: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 6 {
		t.Fatalf("got %d placements", len(p))
	}
	for i, a := range p {
		if a.XMM < 2 || a.YMM < 2 || a.XMM+a.WidthMM > 98.001 || a.YMM+a.HeightMM > 68.001 {
			t.Fatalf("out of bounds: %#v", a)
		}
		for j, b := range p {
			if i < j && a.XMM < b.XMM+b.WidthMM && a.XMM+a.WidthMM > b.XMM && a.YMM < b.YMM+b.HeightMM && a.YMM+a.HeightMM > b.YMM {
				t.Fatalf("placements overlap: %#v %#v", a, b)
			}
		}
	}
}

func TestNestingDeterministic(t *testing.T) {
	s := Sheet{WidthMM: 100, HeightMM: 100, GapMM: 1}
	items := []Item{{ID: "b", WidthMM: 20, HeightMM: 10, Quantity: 3, AllowRotate: true}, {ID: "a", WidthMM: 20, HeightMM: 10, Quantity: 3, AllowRotate: true}}
	a, _ := Nest(s, items)
	b, _ := Nest(s, items)
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("nesting is nondeterministic")
		}
	}
}

func TestNativeCapabilitiesAreExplicit(t *testing.T) {
	c := NativeTools{Vips: "definitely-missing-vips", Potrace: "definitely-missing-potrace"}.Probe()
	if c.ICC || c.VectorTrace {
		t.Fatal("missing native tools reported available")
	}
}
