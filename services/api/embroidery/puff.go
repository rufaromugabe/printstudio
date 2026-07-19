package embroidery

import "fmt"

const (
	FoamHeight2MM = 2.0
	FoamHeight3MM = 3.0
	// Max puff column width — badge lettering, not panel fills.
	MaxPuffColumnMM = 7.0
	// Reject panel-like exteriors that should stay tatami/flat satin.
	MaxPuffPanelMM = 28.0
)

// NormalizeFoamHeight returns 2 or 3 mm (default 3).
func NormalizeFoamHeight(mm float64) float64 {
	if mm <= 2.5 && mm > 0 {
		return FoamHeight2MM
	}
	return FoamHeight3MM
}

// FoamCoverSpacingMM is the default cover density for a foam height.
func FoamCoverSpacingMM(foamHeightMM float64) float64 {
	switch NormalizeFoamHeight(foamHeightMM) {
	case FoamHeight2MM:
		return 0.35
	default:
		return 0.30
	}
}

// puff builds dense cover satin for operator-placed foam. Soft fabric underlay
// is omitted so foam is not crushed; foam height is metadata for sew-out.
func puff(r Region, profile MachineProfile) ([]Stitch, []Stitch, float64, error) {
	foam := NormalizeFoamHeight(r.FoamHeightMM)
	if err := validatePuffGeometry(r); err != nil {
		return nil, nil, foam, err
	}

	cover := r
	cover.Kind = Satin
	cover.EdgeUnderlay = false
	cover.CenterUnderlay = false
	cover.ZigzagUnderlay = false
	cover.FoamHeightMM = foam
	if cover.SpacingMM <= 0 {
		cover.SpacingMM = FoamCoverSpacingMM(foam)
	}
	// Row spacing is independent of MinStitchMM (needle travel length).

	underlay, stitches, err := satin(cover, profile)
	if err != nil {
		return nil, nil, foam, fmt.Errorf("puff cover: %w", err)
	}
	// Soft underlay must stay empty for foam; discard anything satin auto-added.
	underlay = nil
	for i := range stitches {
		stitches[i].Source = "puff_satin"
	}
	return underlay, stitches, foam, nil
}

func validatePuffGeometry(r Region) error {
	if r.WidthMM > 0 {
		if r.WidthMM > MaxPuffColumnMM {
			return fmt.Errorf("puff spine width %.1f mm exceeds %.0f mm badge-column limit", r.WidthMM, MaxPuffColumnMM)
		}
		return nil
	}
	if len(r.Geometry.Rings) > 2 {
		return fmt.Errorf("puff does not support multi-hole panels; split into satin columns or use flat tatami")
	}
	b := polygonBounds(r.Geometry)
	w, h := b.MaxX-b.MinX, b.MaxY-b.MinY
	if w > MaxPuffPanelMM && h > MaxPuffPanelMM {
		return fmt.Errorf("puff is for raised columns/lettering, not panel fills (%.0f×%.0f mm)", w, h)
	}
	// Probe column width using satin estimator with temporary Kind.
	probe := r
	probe.Kind = Satin
	width := estimateSatinWidthMM(probe)
	if width > MaxPuffColumnMM {
		return fmt.Errorf("puff column span %.1f mm exceeds %.0f mm; split lettering or use flat satin/tatami", width, MaxPuffColumnMM)
	}
	if width > 0 && width < 1.0 {
		return fmt.Errorf("puff column span %.1f mm is too thin for foam cover", width)
	}
	return nil
}

func puffOperatorMessage(foam float64) string {
	return fmt.Sprintf("Puff sew-out: place %.0f mm embroidery foam over the hoop, sew the cover satin, then tear away excess foam", foam)
}
