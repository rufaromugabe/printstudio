package embroidery

import (
	"strings"
	"testing"
)

func TestBeanTriplesOutline(t *testing.T) {
	d, err := Compile([]Region{{
		ID: "outline", ThreadID: "black", Geometry: rectangle(0, 0, 20, 10), Kind: Bean, StitchLengthMM: 3,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Plan) != 1 || len(d.Plan[0].Stitches) < 8 {
		t.Fatalf("expected heavy bean outline, got %+v", d.Plan)
	}
	for _, s := range d.Plan[0].Stitches {
		if s.Source != "bean" {
			t.Fatalf("source %q", s.Source)
		}
	}
}

func TestAppliqueHasPlacementTackAndCover(t *testing.T) {
	d, err := Compile([]Region{{
		ID: "patch", ThreadID: "navy", Geometry: rectangle(-15, -10, 30, 20), Kind: Applique, WidthMM: 2, SpacingMM: 0.4,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	block := d.Plan[0]
	if len(block.Underlay) < 4 {
		t.Fatalf("expected placement+tack underlay, got %d", len(block.Underlay))
	}
	placement, tack := false, false
	for _, s := range block.Underlay {
		placement = placement || s.Source == "applique_placement"
		tack = tack || s.Source == "applique_tack"
	}
	if !placement || !tack {
		t.Fatalf("missing placement/tack sources: placement=%v tack=%v", placement, tack)
	}
	if len(block.Stitches) < 4 {
		t.Fatalf("expected cover satin, got %d", len(block.Stitches))
	}
	found := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "APPLIQUE_OPERATOR" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing APPLIQUE_OPERATOR: %+v", d.Diagnostics)
	}
}

func TestAppliqueCoverStaysWithinMachineMax(t *testing.T) {
	// Irregular traced-like outline that used to blow up ring-satin pairing.
	poly := Polygon{Rings: [][]Point{{{
		-20, -8}, {-8, -14}, {10, -12}, {22, -4}, {18, 10}, {0, 14}, {-18, 8}, {-22, 0},
	}}}
	d, err := Compile([]Region{{
		ID: "logo", ThreadID: "black", Geometry: poly, Kind: Applique, SpacingMM: 0.4,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	sts := d.Plan[0].Stitches
	for i := 1; i < len(sts); i++ {
		if sts[i].Command != CommandStitch || sts[i-1].Command != CommandStitch {
			continue
		}
		w := distance(sts[i-1].Position, sts[i].Position)
		if w > 4.1 {
			t.Fatalf("cover rung %.2f mm exceeds applique cover limit near stitch %d", w, i)
		}
	}
}

func TestMotifContourCrossCompile(t *testing.T) {
	panel := rectangle(-12, -8, 24, 16)
	for _, kind := range []StitchKind{Motif, Contour, Cross} {
		d, err := Compile([]Region{{
			ID: string(kind), ThreadID: "red", Geometry: panel, Kind: kind, SpacingMM: 2.5, StitchLengthMM: 2.5,
		}}, DefaultProfile())
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if len(d.Plan) != 1 || len(d.Plan[0].Stitches) < 4 {
			t.Fatalf("%s: expected stitches, got plan=%+v", kind, d.Plan)
		}
	}
}

func TestEstitchAndCordOutline(t *testing.T) {
	for _, kind := range []StitchKind{Estitch, Cord} {
		d, err := Compile([]Region{{
			ID: string(kind), ThreadID: "black", Geometry: rectangle(0, 0, 18, 12), Kind: kind, StitchLengthMM: 2.5, WidthMM: 1.5,
		}}, DefaultProfile())
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if len(d.Plan[0].Stitches) < 6 {
			t.Fatalf("%s: sparse stitches %d", kind, len(d.Plan[0].Stitches))
		}
	}
}

func TestSequinAndChenilleSpecialty(t *testing.T) {
	panel := rectangle(-10, -10, 20, 20)
	seq, err := Compile([]Region{{
		ID: "seq", ThreadID: "gold", Geometry: panel, Kind: Sequin, SpacingMM: 5,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if len(seq.Plan[0].Stitches) < 3 {
		t.Fatalf("sequin stitches %d", len(seq.Plan[0].Stitches))
	}
	chen, err := Compile([]Region{{
		ID: "chen", ThreadID: "cream", Geometry: panel, Kind: Chenille, SpacingMM: 1, StitchLengthMM: 2,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if len(chen.Plan[0].Stitches) < 8 {
		t.Fatalf("chenille stitches %d", len(chen.Plan[0].Stitches))
	}
	for _, code := range []string{"SEQUIN_OPERATOR", "CHENILLE_OPERATOR"} {
		found := false
		for _, d := range append(seq.Diagnostics, chen.Diagnostics...) {
			if d.Code == code {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s", code)
		}
	}
}

func TestSpecialtyReviewFactors(t *testing.T) {
	d, err := Compile([]Region{{
		ID: "patch", ThreadID: "navy", Geometry: rectangle(-10, -8, 20, 16), Kind: Applique,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range d.Review.Factors {
		if f.Code == "APPLIQUE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected APPLIQUE review factor: %+v", d.Review.Factors)
	}
	if !strings.Contains(d.CompilerVersion, "0.5") {
		t.Fatalf("compiler version %s", d.CompilerVersion)
	}
}
