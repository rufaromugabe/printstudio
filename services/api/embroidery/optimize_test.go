package embroidery

import "testing"

func TestSmallTextSatinOptimizesToRunning(t *testing.T) {
	// 4 mm tall letterform — below the 5 mm auto-satin floor.
	d, err := Compile([]Region{{ID: "tiny", ThreadID: "black", Geometry: rectangle(0, 0, 2, 4), Kind: Satin, SpacingMM: .4}}, DefaultProfile())
	if err != nil {
		t.Fatalf("small text should optimize, not reject: %v", err)
	}
	if len(d.Plan) != 1 || d.Plan[0].Kind != Running {
		t.Fatalf("expected running outline, got %+v", d.Plan)
	}
	if !hasDiagnostic(d.Diagnostics, "AUTO_SMALL_TEXT_RUNNING") {
		t.Fatalf("expected AUTO_SMALL_TEXT_RUNNING, got %+v", d.Diagnostics)
	}
	if HasErrors(d.Diagnostics) {
		t.Fatalf("optimized document must not carry errors: %+v", d.Diagnostics)
	}
}

func TestPerformanceKnitDensityLightened(t *testing.T) {
	d, err := CompileWithFabric([]Region{{
		ID: "dense", ThreadID: "black", Kind: Tatami, SpacingMM: 0.3, StitchLengthMM: 3,
		Geometry: rectangle(-10, -5, 20, 10),
	}}, DefaultProfile(), FabricPerformanceKnit)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDiagnostic(d.Diagnostics, "AUTO_DENSITY_LIGHTENED") {
		t.Fatalf("expected AUTO_DENSITY_LIGHTENED, got %+v", d.Diagnostics)
	}
	if d.Regions[0].SpacingMM != 0.40 {
		t.Fatalf("density not lightened: %v", d.Regions[0].SpacingMM)
	}
	for _, diag := range d.Diagnostics {
		if diag.Code == "PERFORMANCE_TOO_DENSE" {
			t.Fatalf("density should be optimized, not rejected: %+v", diag)
		}
	}
}

func TestRingSatinKeepsColumnWidthEstimate(t *testing.T) {
	// An "O" letterform: bounding span is 12 mm but the sewn rung is ~3 mm,
	// so it must stay satin instead of being demoted to tatami.
	p := Polygon{Rings: [][]Point{{{-6, -8}, {6, -8}, {6, 8}, {-6, 8}}, {{-3, -5}, {-3, 5}, {3, 5}, {3, -5}}}}
	regions, diags := OptimizeRegions([]Region{{ID: "o", ThreadID: "black", Geometry: p, Kind: Satin, SpacingMM: .5}}, DefaultProfile(), ProfileFor(FabricWoven))
	if regions[0].Kind != Satin {
		t.Fatalf("ring letterform demoted to %s: %+v", regions[0].Kind, diags)
	}
}

func TestSplitLongStitches(t *testing.T) {
	in := []Stitch{
		{Position: Point{0, 0}, Command: CommandStitch, Source: "test"},
		{Position: Point{30, 0}, Command: CommandStitch, Source: "test"},
	}
	out := splitLongStitches(in, 12.1)
	if len(out) < 4 {
		t.Fatalf("expected long stitch split, got %d stitches", len(out))
	}
	for i := 1; i < len(out); i++ {
		if d := distance(out[i-1].Position, out[i].Position); d > 12.1 {
			t.Fatalf("stitch %d still %.2f mm", i, d)
		}
	}
	last := out[len(out)-1].Position
	if last.X != 30 || last.Y != 0 {
		t.Fatalf("split changed the endpoint: %+v", last)
	}
}

func TestGeneratorFailureFallsBackInsteadOfRejecting(t *testing.T) {
	// Applique too small for its cover satin used to reject the compile.
	d, err := Compile([]Region{{ID: "chip", ThreadID: "red", Geometry: rectangle(0, 0, 0.5, 0.6), Kind: Applique, WidthMM: 2}}, DefaultProfile())
	if err != nil {
		t.Fatalf("expected fallback rescue, got %v", err)
	}
	if len(d.Plan) != 1 {
		t.Fatalf("expected rescued plan block, got %d", len(d.Plan))
	}
	if d.Plan[0].Kind != Tatami && d.Plan[0].Kind != Running {
		t.Fatalf("unexpected fallback kind %s", d.Plan[0].Kind)
	}
}
