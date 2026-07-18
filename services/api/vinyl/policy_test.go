package vinyl

import "testing"

func square(minX, minY, size float64, ccw bool) []Point {
	if ccw {
		return []Point{
			{minX, minY}, {minX + size, minY}, {minX + size, minY + size}, {minX, minY + size},
		}
	}
	return []Point{
		{minX, minY}, {minX, minY + size}, {minX + size, minY + size}, {minX + size, minY},
	}
}

func TestProfileForMirrorAndThresholds(t *testing.T) {
	smooth := ProfileFor(MaterialHTVSmooth)
	if !smooth.MirrorDefault || smooth.WarnFeatureMM != 1.0 || smooth.RejectFeatureMM != 0.6 {
		t.Fatalf("smooth %#v", smooth)
	}
	flock := ProfileFor(MaterialHTVFlock)
	if flock.WarnFeatureMM != 1.2 || flock.RejectFeatureMM != 0.8 {
		t.Fatalf("flock %#v", flock)
	}
	glitter := ProfileFor(MaterialHTVGlitter)
	if glitter.WarnFeatureMM != 1.5 || glitter.RejectFeatureMM != 1.0 {
		t.Fatalf("glitter %#v", glitter)
	}
	perm := ProfileFor(MaterialAdhesivePermanent)
	if perm.MirrorDefault || perm.WarnFeatureMM != 1.0 {
		t.Fatalf("permanent %#v", perm)
	}
	adhGlitter := ProfileFor(MaterialAdhesiveGlitter)
	if adhGlitter.MirrorDefault || adhGlitter.RejectFeatureMM != 1.0 {
		t.Fatalf("adhesive glitter %#v", adhGlitter)
	}
}

func TestNormalizeMaterialAliases(t *testing.T) {
	if NormalizeMaterial("651") != MaterialAdhesivePermanent {
		t.Fatal(NormalizeMaterial("651"))
	}
	if NormalizeMaterial("flock") != MaterialHTVFlock {
		t.Fatal(NormalizeMaterial("flock"))
	}
	if NormalizeMaterial("") != MaterialHTVSmooth {
		t.Fatal(NormalizeMaterial(""))
	}
}

func TestPolicyRejectsTinyFeature(t *testing.T) {
	paths := [][]Point{square(0, 0, 0.5, true)}
	ds := PolicyValidate(paths, ProfileFor(MaterialHTVSmooth))
	found := false
	for _, d := range ds {
		if d.Code == "FEATURE_TOO_SMALL" && d.Severity == Error {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected FEATURE_TOO_SMALL, got %#v", ds)
	}
}

func TestPolicyWarnsBorderlineFeature(t *testing.T) {
	paths := [][]Point{square(0, 0, 0.8, true)}
	ds := PolicyValidate(paths, ProfileFor(MaterialHTVSmooth))
	found := false
	for _, d := range ds {
		if d.Code == "FEATURE_BORDERLINE" && d.Severity == Warning {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected FEATURE_BORDERLINE, got %#v", ds)
	}
}

func TestPolicyCounterRiskOnTinyHole(t *testing.T) {
	// Exterior 10 mm CCW, hole 0.7 mm CW (negative area).
	paths := [][]Point{
		square(0, 0, 10, true),
		square(4.65, 4.65, 0.7, false),
	}
	ds := PolicyValidate(paths, ProfileFor(MaterialHTVSmooth))
	found := false
	for _, d := range ds {
		if d.Code == "COUNTER_RISK" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected COUNTER_RISK, got %#v", ds)
	}
}

func TestScoreReviewBlocksOnHardStop(t *testing.T) {
	paths := [][]Point{square(0, 0, 0.4, true)}
	profile := ProfileFor(MaterialHTVGlitter)
	ds := PolicyValidate(paths, profile)
	review := ScoreReview(paths, profile, ds, true)
	if review.Decision != ReviewBlocked {
		t.Fatalf("expected blocked, got %s score=%d %#v", review.Decision, review.Score, review)
	}
	if len(review.HardStops) == 0 {
		t.Fatal("expected hard stops")
	}
}

func TestScoreReviewFlagsHTVWithoutMirror(t *testing.T) {
	paths := [][]Point{square(0, 0, 20, true)}
	profile := ProfileFor(MaterialHTVSmooth)
	review := ScoreReview(paths, profile, nil, false)
	found := false
	for _, f := range review.Factors {
		if f.Code == "MIRROR_OFF_HTV" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected MIRROR_OFF_HTV, got %#v", review.Factors)
	}
}

func TestReviewConvenience(t *testing.T) {
	paths := [][]Point{square(0, 0, 15, true)}
	profile, ds, review := Review("htv-smooth", paths, true)
	if profile.Class != MaterialHTVSmooth {
		t.Fatal(profile.Class)
	}
	if HasErrors(ds) {
		t.Fatalf("unexpected errors %#v", ds)
	}
	if review.Decision == "" {
		t.Fatal("missing decision")
	}
}
