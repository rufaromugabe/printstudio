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

func TestPuffRejectsWidePanel(t *testing.T) {
	panel := rectangle(-20, -15, 40, 30)
	_, err := Compile([]Region{{ID: "panel", ThreadID: "navy", Geometry: panel, Kind: Puff, FoamHeightMM: 3}}, DefaultProfile())
	if err == nil || (!strings.Contains(err.Error(), "panel") && !strings.Contains(err.Error(), "exceeds")) {
		t.Fatalf("expected panel/width reject, got %v", err)
	}
}

func TestPuffRejectsInvalidFoamHeight(t *testing.T) {
	col := rectangle(-1.5, -10, 3, 20)
	d, err := Compile([]Region{{ID: "bad", ThreadID: "gold", Geometry: col, Kind: Puff, FoamHeightMM: 5}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	blocked := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "PUFF_FOAM_INVALID" {
			blocked = true
		}
	}
	if !blocked {
		t.Fatalf("expected PUFF_FOAM_INVALID, got %+v", d.Diagnostics)
	}
	if d.Review.Decision != ReviewBlocked {
		t.Fatalf("decision %s", d.Review.Decision)
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
