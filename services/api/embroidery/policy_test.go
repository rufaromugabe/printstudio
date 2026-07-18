package embroidery

import "testing"

func TestProfileForDefaults(t *testing.T) {
	p := ProfileFor(FabricPerformanceKnit)
	if p.DensityMM < 0.45 || !p.PreferEdgeUnderlay {
		t.Fatalf("unexpected performance profile %#v", p)
	}
	w := ProfileFor(FabricWoven)
	if w.DensityMM != 0.40 || w.PullCompensationMM != 0.20 {
		t.Fatalf("unexpected woven profile %#v", w)
	}
}

func TestApplyFabricProfileFillsDensity(t *testing.T) {
	regions, profile := ApplyFabricProfile([]Region{{
		ID: "a", ThreadID: "black", Kind: Tatami,
		Geometry: Polygon{Rings: [][]Point{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}},
	}}, FabricFleece)
	if profile.Class != FabricFleece {
		t.Fatal(profile.Class)
	}
	if regions[0].SpacingMM != profile.DensityMM {
		t.Fatalf("density not applied: %v", regions[0].SpacingMM)
	}
	if !regions[0].EdgeUnderlay {
		t.Fatal("fleece tatami should get edge underlay")
	}
}

func TestPolicyRejectsWideSatin(t *testing.T) {
	// Spine satin 12 mm wide must hard-reject.
	regions := []Region{{
		ID: "wide", ThreadID: "black", Kind: Satin, WidthMM: 12, SpacingMM: 0.4,
		Geometry: Polygon{Rings: [][]Point{{{0, 0}, {20, 0}}}},
	}}
	ds := PolicyValidate(regions, ProfileFor(FabricWoven))
	found := false
	for _, d := range ds {
		if d.Code == "SATIN_TOO_WIDE" && d.Severity == Error {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SATIN_TOO_WIDE, got %#v", ds)
	}
}

func TestPolicyRejectsTinySatinFeature(t *testing.T) {
	regions := []Region{{
		ID: "tiny", ThreadID: "black", Kind: Satin, WidthMM: 2, SpacingMM: 0.4,
		Geometry: Polygon{Rings: [][]Point{{{0, 0}, {8, 0}}}}, // height ~0 from open spine — use filled short glyph
	}}
	// Closed short letter-like column ~4 mm tall, ~2 mm wide.
	regions[0].WidthMM = 0
	regions[0].Geometry = Polygon{Rings: [][]Point{{{0, 0}, {2, 0}, {2, 4}, {0, 4}}}}
	ds := PolicyValidate(regions, ProfileFor(FabricWoven))
	found := false
	for _, d := range ds {
		if d.Code == "TEXT_TOO_SMALL" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TEXT_TOO_SMALL, got %#v", ds)
	}
}

func TestScoreReviewRoutesHuman(t *testing.T) {
	regions := []Region{{
		ID: "logo", ThreadID: "black", Kind: Satin, SpacingMM: 0.36,
		Geometry: Polygon{Rings: [][]Point{{{0, 0}, {3, 0}, {3, 5.5}, {0, 5.5}}}},
	}}
	fabric := ProfileFor(FabricPerformanceKnit)
	review := ScoreReview(regions, fabric, nil)
	if review.Score < 25 {
		t.Fatalf("expected elevated score, got %d %#v", review.Score, review.Factors)
	}
	if review.Decision == ReviewAuto {
		t.Fatalf("expected non-auto decision, got %s", review.Decision)
	}
}

func TestCompileWithFabricAttachesReview(t *testing.T) {
	d, err := CompileWithFabric([]Region{{
		ID: "panel", ThreadID: "black", Kind: Tatami, SpacingMM: 1, StitchLengthMM: 3, EdgeUnderlay: true,
		Geometry: Polygon{Rings: [][]Point{{{-10, -5}, {10, -5}, {10, 5}, {-10, 5}}}},
	}}, DefaultProfile(), FabricTShirt)
	if err != nil {
		t.Fatal(err)
	}
	if d.Fabric.Class != FabricTShirt {
		t.Fatalf("fabric %#v", d.Fabric)
	}
	if d.Review.Decision == "" {
		t.Fatal("missing review")
	}
	if d.CompilerVersion != CompilerVersion {
		t.Fatalf("version %s", d.CompilerVersion)
	}
}
