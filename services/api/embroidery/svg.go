package embroidery

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// DiagnosticSVG renders stitch and jump paths for review; it is not a product mockup.
// The viewBox is fitted to stitch content (plus margin), not the full hoop outline.
func DiagnosticSVG(d Document) string {
	var s strings.Builder
	bounds, ok := stitchContentBounds(d)
	if !ok {
		w, h := d.Machine.HoopWidthMM, d.Machine.HoopHeightMM
		bounds = Bounds{MinX: -w / 2, MinY: -h / 2, MaxX: w / 2, MaxY: h / 2}
	}
	pad := 4.0
	minX, minY := bounds.MinX-pad, bounds.MinY-pad
	width := math.Max(1, bounds.MaxX-bounds.MinX+pad*2)
	height := math.Max(1, bounds.MaxY-bounds.MinY+pad*2)
	fmt.Fprintf(&s, "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"%g %g %g %g\" preserveAspectRatio=\"xMidYMid meet\">", minX, minY, width, height)
	fmt.Fprintf(&s, "<rect x=\"%g\" y=\"%g\" width=\"%g\" height=\"%g\" fill=\"white\" stroke=\"#bbb\" stroke-width=\"0.4\"/>", minX, minY, width, height)

	for _, b := range d.Plan {
		thread := stitchColor(b.ThreadID)
		all := append(append([]Stitch{}, b.Underlay...), b.Stitches...)
		for i := 1; i < len(all); i++ {
			dash := ""
			color := thread
			if all[i].Command == CommandJump {
				dash = " stroke-dasharray=\"1 1\""
				color = "#d14343"
			}
			fmt.Fprintf(&s, "<line data-region=\"%s\" data-thread=\"%s\" x1=\"%.3f\" y1=\"%.3f\" x2=\"%.3f\" y2=\"%.3f\" stroke=\"%s\" stroke-width=\"0.18\"%s/>", html.EscapeString(b.RegionID), html.EscapeString(b.ThreadID), all[i-1].Position.X, all[i-1].Position.Y, all[i].Position.X, all[i].Position.Y, color, dash)
		}
	}
	s.WriteString("</svg>")
	return s.String()
}

func stitchContentBounds(d Document) (Bounds, bool) {
	b := Bounds{MinX: math.Inf(1), MinY: math.Inf(1), MaxX: math.Inf(-1), MaxY: math.Inf(-1)}
	found := false
	for _, block := range d.Plan {
		for _, stitch := range append(append([]Stitch{}, block.Underlay...), block.Stitches...) {
			found = true
			if stitch.Position.X < b.MinX {
				b.MinX = stitch.Position.X
			}
			if stitch.Position.Y < b.MinY {
				b.MinY = stitch.Position.Y
			}
			if stitch.Position.X > b.MaxX {
				b.MaxX = stitch.Position.X
			}
			if stitch.Position.Y > b.MaxY {
				b.MaxY = stitch.Position.Y
			}
		}
	}
	return b, found
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
