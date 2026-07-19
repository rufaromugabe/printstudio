package embroidery

import "fmt"

// Optimizer thresholds mirror the PolicyValidate hard stops so that optimized
// regions always pass policy instead of hard-rejecting the whole design.
const (
	maxAutoSatinWidthMM = 7.0
	minColumnWidthMM    = 1.0
	minAutoTextMM       = 5.0
)

// OptimizeRegions rewrites regions that would hard-reject into the nearest
// sewable alternative instead of failing the compile: oversize satin becomes
// tatami fill, sub-5 mm lettering becomes a run-stitch outline, puff panels
// fall back to flat fill, and out-of-band widths/densities are clamped.
// Every rewrite is reported as a warning diagnostic so the operator can see
// exactly what the compiler changed.
func OptimizeRegions(regions []Region, profile MachineProfile, fabric FabricProfile) ([]Region, []Diagnostic) {
	out := make([]Region, len(regions))
	var diags []Diagnostic
	warn := func(code, msg, region string) {
		diags = append(diags, Diagnostic{Severity: Warning, Code: code, Message: msg, RegionID: region})
	}
	for i, r := range regions {
		out[i] = optimizeRegion(r, profile, fabric, warn)
	}
	return out, diags
}

func optimizeRegion(r Region, profile MachineProfile, fabric FabricProfile, warn func(code, msg, region string)) Region {
	if !knownKind(r.Kind) {
		warn("AUTO_KIND_TATAMI", fmt.Sprintf("unknown stitch kind %q; compiled as tatami fill", r.Kind), r.ID)
		r = toTatami(r, fabric)
	}
	if fabric.Class == FabricPerformanceKnit && r.SpacingMM > 0 && r.SpacingMM < 0.40 {
		warn("AUTO_DENSITY_LIGHTENED", fmt.Sprintf("density %.2f mm is too heavy for performance knit; lightened to 0.40 mm", r.SpacingMM), r.ID)
		r.SpacingMM = 0.40
	}
	switch r.Kind {
	case Puff:
		return optimizePuff(r, fabric, warn)
	case Satin:
		return optimizeSatin(r, profile, fabric, warn)
	}
	return r
}

func optimizePuff(r Region, fabric FabricProfile, warn func(code, msg, region string)) Region {
	if r.FoamHeightMM > 0 && r.FoamHeightMM != FoamHeight2MM && r.FoamHeightMM != FoamHeight3MM {
		normalized := NormalizeFoamHeight(r.FoamHeightMM)
		warn("AUTO_PUFF_FOAM_NORMALIZED", fmt.Sprintf("puff foam height %.1f mm is not stocked; using %.0f mm foam", r.FoamHeightMM, normalized), r.ID)
		r.FoamHeightMM = normalized
	}
	if r.WidthMM > 0 {
		if r.WidthMM > MaxPuffColumnMM {
			warn("AUTO_PUFF_WIDTH_CLAMPED", fmt.Sprintf("puff column %.1f mm exceeds the %.0f mm badge limit; clamped", r.WidthMM, MaxPuffColumnMM), r.ID)
			r.WidthMM = MaxPuffColumnMM
		}
		if r.WidthMM < minColumnWidthMM {
			warn("AUTO_PUFF_WIDTH_RAISED", fmt.Sprintf("puff column %.1f mm is too thin for foam cover; widened to %.0f mm", r.WidthMM, minColumnWidthMM), r.ID)
			r.WidthMM = minColumnWidthMM
		}
		if h := estimateLetterHeightMM(r); h > 0 && h < minAutoTextMM {
			warn("AUTO_SMALL_TEXT_RUNNING", fmt.Sprintf("puff feature height %.1f mm is below %.0f mm; compiled as a run-stitch outline", h, minAutoTextMM), r.ID)
			return toRunning(r)
		}
		return r
	}
	if h := estimateLetterHeightMM(r); h > 0 && h < minAutoTextMM {
		warn("AUTO_SMALL_TEXT_RUNNING", fmt.Sprintf("puff feature height %.1f mm is below %.0f mm; compiled as a run-stitch outline", h, minAutoTextMM), r.ID)
		return toRunning(r)
	}
	if len(r.Geometry.Rings) > 2 {
		warn("AUTO_PUFF_TO_TATAMI", "puff cannot cover multi-hole panels; compiled as flat tatami fill", r.ID)
		return toTatami(r, fabric)
	}
	b := polygonBounds(r.Geometry)
	if b.MaxX-b.MinX > MaxPuffPanelMM && b.MaxY-b.MinY > MaxPuffPanelMM {
		warn("AUTO_PUFF_TO_TATAMI", fmt.Sprintf("puff is for raised columns/lettering, not %.0f×%.0f mm panels; compiled as flat tatami fill", b.MaxX-b.MinX, b.MaxY-b.MinY), r.ID)
		return toTatami(r, fabric)
	}
	width := estimateSatinWidthMM(r)
	if width > MaxPuffColumnMM {
		warn("AUTO_PUFF_TO_TATAMI", fmt.Sprintf("puff column span %.1f mm exceeds %.0f mm; compiled as flat tatami fill", width, MaxPuffColumnMM), r.ID)
		return toTatami(r, fabric)
	}
	if width > 0 && width < minColumnWidthMM {
		warn("AUTO_SMALL_TEXT_RUNNING", fmt.Sprintf("puff column span %.1f mm is too thin for foam cover; compiled as a run-stitch outline", width), r.ID)
		return toRunning(r)
	}
	return r
}

func optimizeSatin(r Region, profile MachineProfile, fabric FabricProfile, warn func(code, msg, region string)) Region {
	maxSatin := maxAutoSatinWidthMM
	if profile.MaxStitchMM > 0 && profile.MaxStitchMM < maxSatin {
		maxSatin = profile.MaxStitchMM
	}
	if r.WidthMM > 0 {
		if r.WidthMM > maxSatin {
			warn("AUTO_SATIN_WIDTH_CLAMPED", fmt.Sprintf("satin column %.1f mm exceeds the %.1f mm auto limit; clamped — use tatami for wide areas", r.WidthMM, maxSatin), r.ID)
			r.WidthMM = maxSatin
		}
		if r.WidthMM < minColumnWidthMM {
			warn("AUTO_SATIN_WIDTH_RAISED", fmt.Sprintf("satin column %.1f mm is too thin for 40 wt thread; widened to %.0f mm", r.WidthMM, minColumnWidthMM), r.ID)
			r.WidthMM = minColumnWidthMM
		}
		if h := estimateLetterHeightMM(r); h > 0 && h < minAutoTextMM {
			warn("AUTO_SMALL_TEXT_RUNNING", fmt.Sprintf("feature height %.1f mm is below %.0f mm; compiled as a run-stitch outline", h, minAutoTextMM), r.ID)
			return toRunning(r)
		}
		return r
	}
	if h := estimateLetterHeightMM(r); h > 0 && h < minAutoTextMM {
		warn("AUTO_SMALL_TEXT_RUNNING", fmt.Sprintf("feature height %.1f mm is below %.0f mm; compiled as a run-stitch outline", h, minAutoTextMM), r.ID)
		return toRunning(r)
	}
	if len(r.Geometry.Rings) > 2 {
		warn("AUTO_SATIN_TO_TATAMI", fmt.Sprintf("satin region has %d holes; compiled as tatami fill", len(r.Geometry.Rings)-1), r.ID)
		return toTatami(r, fabric)
	}
	width := estimateSatinWidthMM(r)
	if width > maxSatin {
		warn("AUTO_SATIN_TO_TATAMI", fmt.Sprintf("satin span %.1f mm exceeds the %.1f mm auto limit; compiled as tatami fill", width, maxSatin), r.ID)
		return toTatami(r, fabric)
	}
	if width > 0 && width < minColumnWidthMM {
		warn("AUTO_SATIN_TO_RUNNING", fmt.Sprintf("satin span %.1f mm is too thin for 40 wt satin; compiled as a run-stitch outline", width), r.ID)
		return toRunning(r)
	}
	return r
}

func toTatami(r Region, fabric FabricProfile) Region {
	r.Kind = Tatami
	r.WidthMM = 0
	r.FoamHeightMM = 0
	if r.SpacingMM <= 0 {
		r.SpacingMM = fabric.DensityMM
	}
	if r.SpacingMM <= 0 {
		r.SpacingMM = 0.4
	}
	if r.StitchLengthMM <= 0 {
		r.StitchLengthMM = 3
	}
	r.CenterUnderlay = false
	r.ZigzagUnderlay = false
	r.EdgeUnderlay = r.EdgeUnderlay || fabric.PreferEdgeUnderlay
	return r
}

func toRunning(r Region) Region {
	r.Kind = Running
	r.WidthMM = 0
	r.FoamHeightMM = 0
	if r.StitchLengthMM <= 0 {
		r.StitchLengthMM = 2.5
	}
	r.EdgeUnderlay = false
	r.CenterUnderlay = false
	r.ZigzagUnderlay = false
	return r
}

func knownKind(k StitchKind) bool {
	switch k {
	case Running, Tatami, Satin, Puff, Bean, Applique, Motif, Contour, Estitch, Cross, Sequin, Cord, Chenille:
		return true
	default:
		return false
	}
}

// optimizedFallback rescues a region whose stitch generator failed by trying
// a tatami fill, then a running outline. It returns ok=false only when the
// geometry cannot produce any sewable path at all.
func optimizedFallback(r Region, fabric FabricProfile, minStitch float64) ([]Stitch, StitchKind, bool) {
	if len(r.Geometry.Rings) == 0 || len(r.Geometry.Rings[0]) < 2 {
		return nil, r.Kind, false
	}
	if r.Kind != Tatami && len(r.Geometry.Rings[0]) >= 3 {
		fill := toTatami(r, fabric)
		if stitches, err := tatami(fill, minStitch); err == nil {
			return stitches, Tatami, true
		}
	}
	if stitches := runningPath(r.Geometry.Rings[0], max(r.StitchLengthMM, 2.5), "auto_fallback_running"); len(stitches) >= 2 {
		return stitches, Running, true
	}
	return nil, r.Kind, false
}
