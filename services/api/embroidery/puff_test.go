package embroidery

import (
	"strings"
	"testing"
)

func TestPuffCompilesDenseCoverWithoutUnderlay(t *testing.T) {
	// Letter-stem column suitable for badge puff.
	col := rectangle(-1.5, -10, 3, 20)
	d, err := Compile([]Region{{
		ID: "badge-stem", ThreadID: "gold", Geometry: col, Kind: Puff, FoamHeightMM: 3,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if d.CompilerVersion != CompilerVersion {
		t.Fatalf("version %s", d.CompilerVersion)
	}
	if len(d.Plan) != 1 {
		t.Fatalf("plan len %d", len(d.Plan))
	}
	block := d.Plan[0]
	if block.Kind != Puff {
		t.Fatalf("kind %s", block.Kind)
	}
	if len(block.Underlay) != 0 {
		t.Fatalf("puff must not crush foam with soft underlay, got %d underlay stitches", len(block.Underlay))
	}
	if len(block.Stitches) < 4 {
		t.Fatalf("expected dense cover stitches, got %d", len(block.Stitches))
	}
	for _, s := range block.Stitches {
		if s.Source != "puff_satin" {
			t.Fatalf("source %q", s.Source)
		}
	}
	found := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "PUFF_OPERATOR" && strings.Contains(diag.Message, "3 mm") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing PUFF_OPERATOR diagnostic: %+v", d.Diagnostics)
	}
	reviewPuff := false
	for _, f := range d.Review.Factors {
		if f.Code == "PUFF_FOAM" {
			reviewPuff = true
		}
	}
	if !reviewPuff {
		t.Fatalf("expected PUFF_FOAM review factor: %+v", d.Review.Factors)
	}
}

func TestPuffFoamHeightChangesDensity(t *testing.T) {
	col := rectangle(-1.5, -10, 3, 20)
	a, err := Compile([]Region{{ID: "a", ThreadID: "gold", Geometry: col, Kind: Puff, FoamHeightMM: 2}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	b, err := Compile([]Region{{ID: "b", ThreadID: "gold", Geometry: col, Kind: Puff, FoamHeightMM: 3}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	// Denser (3 mm foam → 0.30 spacing) should produce more rungs than 2 mm (0.35).
	if len(b.Plan[0].Stitches) <= len(a.Plan[0].Stitches) {
		t.Fatalf("expected 3 mm foam denser than 2 mm: 2mm=%d 3mm=%d", len(a.Plan[0].Stitches), len(b.Plan[0].Stitches))
	}
}

func TestPuffWidePanelOptimizesToTatami(t *testing.T) {
	panel := rectangle(-20, -15, 40, 30)
	d, err := Compile([]Region{{ID: "panel", ThreadID: "navy", Geometry: panel, Kind: Puff, FoamHeightMM: 3}}, DefaultProfile())
	if err != nil {
		t.Fatalf("wide puff panel should optimize, not reject: %v", err)
	}
	if len(d.Plan) != 1 || d.Plan[0].Kind != Tatami {
		t.Fatalf("expected flat tatami conversion, got %+v", d.Plan)
	}
	found := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "AUTO_PUFF_TO_TATAMI" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected AUTO_PUFF_TO_TATAMI diagnostic, got %+v", d.Diagnostics)
	}
	if HasErrors(d.Diagnostics) {
		t.Fatalf("optimized document must not carry errors: %+v", d.Diagnostics)
	}
}

func TestPuffNormalizesInvalidFoamHeight(t *testing.T) {
	col := rectangle(-1.5, -10, 3, 20)
	d, err := Compile([]Region{{ID: "bad", ThreadID: "gold", Geometry: col, Kind: Puff, FoamHeightMM: 5}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	normalized := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "AUTO_PUFF_FOAM_NORMALIZED" && strings.Contains(diag.Message, "3 mm") {
			normalized = true
		}
		if diag.Code == "PUFF_FOAM_INVALID" {
			t.Fatalf("foam height should be normalized, not rejected: %+v", diag)
		}
	}
	if !normalized {
		t.Fatalf("expected AUTO_PUFF_FOAM_NORMALIZED, got %+v", d.Diagnostics)
	}
	if d.Review.Decision == ReviewBlocked {
		t.Fatalf("normalized foam must not block the design, got %s", d.Review.Decision)
	}
	if d.Regions[0].FoamHeightMM != FoamHeight3MM {
		t.Fatalf("foam height not normalized: %v", d.Regions[0].FoamHeightMM)
	}
}

func TestPuffDSTExportsCover(t *testing.T) {
	col := rectangle(-1.5, -10, 3, 20)
	d, err := Compile([]Region{{ID: "badge", ThreadID: "gold", Geometry: col, Kind: Puff, FoamHeightMM: 2}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeDST(d, "puff")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 512 {
		t.Fatalf("DST too small: %d", len(raw))
	}
}
