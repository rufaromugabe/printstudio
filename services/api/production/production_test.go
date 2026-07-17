package production

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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

func TestDTFPackContainsColourAndEuclideanUnderbase(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 7, 7))
	src.SetNRGBA(3, 3, color.NRGBA{R: 240, G: 50, B: 20, A: 255})
	colour, err := EncodePNG(src)
	if err != nil {
		t.Fatal(err)
	}
	files, err := BuildDTFFiles(src, colour, DTFPackConfig{SpreadPixels: 1, Threshold: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Name != "color.png" || files[1].Name != "white-underbase.png" || len(files[0].SHA256) != 64 {
		t.Fatalf("unexpected DTF files: %#v", files)
	}
	mask, err := png.Decode(bytes.NewReader(files[1].Data))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, adjacent := mask.At(2, 3).RGBA()
	_, _, _, diagonal := mask.At(2, 2).RGBA()
	if adjacent == 0 || diagonal != 0 {
		t.Fatal("pack underbase did not preserve Euclidean spread geometry")
	}
}

func TestScreenPackHalftoneUsesCoveragePolarity(t *testing.T) {
	mask := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range mask.Pix {
		mask.Pix[i] = 255
	}
	screen := AMHalftoneCoverage(mask, HalftoneConfig{DPI: 300, LPI: 45, AngleDegrees: 15})
	on := 0
	for _, value := range screen.Pix {
		if value > 0 {
			on++
		}
	}
	if on < len(screen.Pix)*9/10 {
		t.Fatalf("full ink coverage produced only %d/%d screened pixels", on, len(screen.Pix))
	}
}

func TestServerGangRenderPlacementAndTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 10, 20))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	sheet, placements, err := RenderGangSheet(src, GangConfig{Sheet: Sheet{WidthMM: 40, HeightMM: 30, GapMM: 2}, SourceWMM: 10, SourceHMM: 20, Copies: 3, DPI: 25.4, AllowRotate: true, MaxPixels: 10_000})
	if err != nil {
		t.Fatal(err)
	}
	if sheet.Bounds().Dx() != 40 || sheet.Bounds().Dy() != 30 || len(placements) != 3 {
		t.Fatalf("unexpected gang output %v, %d placements", sheet.Bounds(), len(placements))
	}
	if sheet.NRGBAAt(39, 29).A != 0 {
		t.Fatal("unused gang-sheet area must remain transparent")
	}
}

func TestBilinearScalePreservesColourAtTransparentEdges(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	scaled := scaleBilinear(src, 4, 1)
	edge := scaled.NRGBAAt(2, 0)
	if edge.A == 0 || edge.R < 250 {
		t.Fatalf("transparent-edge interpolation introduced a dark fringe: %#v", edge)
	}
}

func TestFMHalftoneIsDeterministicAndMonotonic(t *testing.T) {
	coverage := func(gray uint8) int {
		img := image.NewGray(image.Rect(0, 0, 96, 96))
		for i := range img.Pix {
			img.Pix[i] = gray
		}
		out := FMHalftone(img, 1)
		n := 0
		for _, v := range out.Pix {
			if v > 0 {
				n++
			}
		}
		again := FMHalftone(img, 1)
		for i := range out.Pix {
			if out.Pix[i] != again.Pix[i] {
				t.Fatal("FM halftone is nondeterministic")
			}
		}
		return n
	}
	dark, mid, light := coverage(0), coverage(128), coverage(240)
	if !(dark > mid && mid > light) {
		t.Fatalf("FM coverage not monotonic: %d %d %d", dark, mid, light)
	}
}

func TestSpotMatchDeltaE(t *testing.T) {
	match, err := MatchSpot("#C8102E", DefaultNamedInks(), 2)
	if err != nil || match.Ink.ID != "spot-red" {
		t.Fatalf("expected spot-red, got %#v err=%v", match, err)
	}
	if _, err := MatchSpot("#00FF00", DefaultNamedInks(), 1); err == nil {
		t.Fatal("expected unmatched colour to fail closed")
	}
}

func TestScreenAngleConflictDetection(t *testing.T) {
	ok := DefaultScreenAngles()
	if len(DetectScreenAngleConflicts(ok)) != 0 {
		t.Fatal("default angles should not conflict")
	}
	bad := ScreenAngleSet{Cyan: 45, Magenta: 45, Yellow: 0, Black: 45}
	if err := RejectScreenAngleConflicts(bad); err == nil {
		t.Fatal("identical C/M/K angles must be rejected")
	}
}

func TestTrapPresetLookup(t *testing.T) {
	preset, err := LookupTrapPreset("dtf-pet-film-standard")
	if err != nil || preset.SpreadPixels != 2 {
		t.Fatalf("unexpected preset %#v err=%v", preset, err)
	}
}

func TestICCProfileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewICCProfileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Minimal acsp signature at offset 36.
	data := make([]byte, 128)
	copy(data[36:40], []byte("acsp"))
	meta, err := store.Put("srgb-test", "sRGB test", "unit", data)
	if err != nil || meta.Version != 1 {
		t.Fatalf("put failed: %#v %v", meta, err)
	}
	got, path, err := store.Get("srgb-test")
	if err != nil || got.SHA256 != meta.SHA256 || path == "" {
		t.Fatalf("get failed: %#v %v", got, err)
	}
	list, err := store.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list failed: %v %#v", err, list)
	}
}
