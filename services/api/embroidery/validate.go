package embroidery

import "fmt"

func Validate(d Document) []Diagnostic {
	var out []Diagnostic
	count := 0
	colors := map[string]bool{}
	shortByRegion := map[string]int{}
	add := func(s Severity, code, msg, region string) {
		out = append(out, Diagnostic{Severity: s, Code: code, Message: msg, RegionID: region})
	}
	for _, b := range d.Plan {
		colors[b.ThreadID] = true
		stitches := append(append([]Stitch{}, b.Underlay...), b.Stitches...)
		count += len(stitches)
		for i := 1; i < len(stitches); i++ {
			n := distance(stitches[i-1].Position, stitches[i].Position)
			if stitches[i].Command == CommandStitch && n > d.Machine.MaxStitchMM {
				add(Error, "STITCH_TOO_LONG", fmt.Sprintf("stitch is %.2f mm; maximum is %.2f mm", n, d.Machine.MaxStitchMM), b.RegionID)
			}
			if stitches[i].Command == CommandStitch && n > 0 && n < d.Machine.MinStitchMM {
				shortByRegion[b.RegionID]++
			}
		}
		if b.Bounds.MaxX-b.Bounds.MinX > d.Machine.HoopWidthMM || b.Bounds.MaxY-b.Bounds.MinY > d.Machine.HoopHeightMM {
			add(Error, "OUTSIDE_HOOP", "region bounds exceed the selected hoop", b.RegionID)
		}
	}
	for region, n := range shortByRegion {
		add(Warning, "STITCH_TOO_SHORT", fmt.Sprintf("%d stitches are shorter than %.2f mm", n, d.Machine.MinStitchMM), region)
	}
	if count > d.Machine.MaxStitches {
		add(Error, "STITCH_LIMIT", fmt.Sprintf("%d stitches exceed machine limit %d", count, d.Machine.MaxStitches), "")
	}
	if len(colors) > d.Machine.MaxColors {
		add(Error, "COLOR_LIMIT", fmt.Sprintf("%d colors exceed machine limit %d", len(colors), d.Machine.MaxColors), "")
	}
	return out
}

func HasErrors(ds []Diagnostic) bool {
	for _, d := range ds {
		if d.Severity == Error {
			return true
		}
	}
	return false
}
