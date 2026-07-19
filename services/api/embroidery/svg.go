package embroidery

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// DiagnosticSVG renders stitch paths in the same centered-mm frame as the
// shirt print area (origin at print-area center, Y down). printWidthMM /
// printHeightMM should match the product view physical size so placement
// matches the mockup. Falls back to the machine hoop when unset.
func DiagnosticSVG(d Document, printWidthMM, printHeightMM float64) string {
	w, h := printWidthMM, printHeightMM
	if w <= 0 {
		w = d.Machine.HoopWidthMM
	}
	if h <= 0 {
		h = d.Machine.HoopHeightMM
	}
	if w <= 0 {
		w = 130
	}
	if h <= 0 {
		h = 180
	}
	minX, minY := -w/2, -h/2

	var s strings.Builder
	fmt.Fprintf(&s, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%g %g %g %g" preserveAspectRatio="xMidYMid meet" data-print-width-mm="%g" data-print-height-mm="%g">`, minX, minY, w, h, w, h)
	// Print-area frame (matches shirt print box).
	fmt.Fprintf(&s, `<rect class="print-area" x="%g" y="%g" width="%g" height="%g" fill="#f7f8f5" stroke="#8a938d" stroke-width="0.6"/>`, minX, minY, w, h)
	// Safe margin hint (~same proportion as product safeMargin when unknown: 8 mm on 300 mm ≈ 2.7%).
	safe := math.Min(8, math.Min(w, h)*0.03)
	if safe > 0.5 {
		fmt.Fprintf(&s, `<rect class="safe-area" x="%g" y="%g" width="%g" height="%g" fill="none" stroke="#b8c4bb" stroke-width="0.35" stroke-dasharray="2 1.5"/>`, minX+safe, minY+safe, w-safe*2, h-safe*2)
	}
	// Hoop outline centered in the print area (machine limit, not the crop frame).
	hw, hh := d.Machine.HoopWidthMM, d.Machine.HoopHeightMM
	if hw > 0 && hh > 0 {
		fmt.Fprintf(&s, `<rect class="hoop" x="%g" y="%g" width="%g" height="%g" fill="none" stroke="#5b8f7a" stroke-width="0.45" stroke-dasharray="3 2"/>`, -hw/2, -hh/2, hw, hh)
	}

	for _, b := range d.Plan {
		thread := stitchColor(b.ThreadID)
		all := append(append([]Stitch{}, b.Underlay...), b.Stitches...)
		for i := 1; i < len(all); i++ {
			dash := ""
			color := thread
			if all[i].Command == CommandJump {
				dash = ` stroke-dasharray="1 1"`
				color = "#d14343"
			}
			fmt.Fprintf(&s, `<line data-region="%s" data-thread="%s" x1="%.3f" y1="%.3f" x2="%.3f" y2="%.3f" stroke="%s" stroke-width="0.18"%s/>`, html.EscapeString(b.RegionID), html.EscapeString(b.ThreadID), all[i-1].Position.X, all[i-1].Position.Y, all[i].Position.X, all[i].Position.Y, color, dash)
		}
	}
	s.WriteString("</svg>")
	return s.String()
}

func stitchColor(threadID string) string {
	id := strings.TrimSpace(threadID)
	if strings.HasPrefix(id, "#") {
		switch len(id) {
		case 4, 7:
			return id
		}
	}
	return "#152238"
}
