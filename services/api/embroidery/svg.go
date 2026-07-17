package embroidery

import (
	"fmt"
	"html"
	"strings"
)

// DiagnosticSVG renders stitch and jump paths for review; it is not a product mockup.
func DiagnosticSVG(d Document) string {
	var s strings.Builder
	w, h := d.Machine.HoopWidthMM, d.Machine.HoopHeightMM
	fmt.Fprintf(&s, "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"%g %g %g %g\" preserveAspectRatio=\"xMidYMid meet\">", -w/2, -h/2, w, h)
	fmt.Fprintf(&s, "<rect x=\"%g\" y=\"%g\" width=\"%g\" height=\"%g\" fill=\"white\" stroke=\"#bbb\" stroke-width=\"0.4\"/>", -w/2, -h/2, w, h)

	for _, b := range d.Plan {
		all := append(append([]Stitch{}, b.Underlay...), b.Stitches...)
		for i := 1; i < len(all); i++ {
			dash := ""
			color := "#152238"
			if all[i].Command == CommandJump {
				dash = " stroke-dasharray=\"1 1\""
				color = "#d14343"
			}
			fmt.Fprintf(&s, "<line data-region=\"%s\" x1=\"%.3f\" y1=\"%.3f\" x2=\"%.3f\" y2=\"%.3f\" stroke=\"%s\" stroke-width=\"0.18\"%s/>", html.EscapeString(b.RegionID), all[i-1].Position.X, all[i-1].Position.Y, all[i].Position.X, all[i].Position.Y, color, dash)
		}
	}
	s.WriteString("</svg>")
	return s.String()
}
