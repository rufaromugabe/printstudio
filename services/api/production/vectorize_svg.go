package production

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	pathDAttr     = regexp.MustCompile(`(?is)<path[^>]*\sd=["']([^"']+)["']`)
	gTransformAttr = regexp.MustCompile(`(?is)<g[^>]*\stransform=["']([^"']+)["']`)
	translateRE   = regexp.MustCompile(`(?i)translate\(\s*([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?)\s*[, ]\s*([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?)\s*\)`)
	scaleRE       = regexp.MustCompile(`(?i)scale\(\s*([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?)\s*(?:[, ]\s*([+-]?\d*\.?\d+(?:[eE][+-]?\d+)?))?\s*\)`)
)

// ParseSVGPathRings extracts closed polygon rings from Potrace (or similar) SVG path data.
// Potrace wraps paths in translate/scale transforms; those are applied so rings land in image pixel space (Y down).
func ParseSVGPathRings(svg string, width, height float64) ([][]VectorPoint, error) {
	matches := pathDAttr.FindAllStringSubmatch(svg, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("svg contains no path data")
	}
	tx, ty, sx, sy := 0.0, 0.0, 1.0, 1.0
	if tm := gTransformAttr.FindStringSubmatch(svg); len(tm) == 2 {
		tx, ty, sx, sy = parseSVGTransform(tm[1])
	} else if height > 0 {
		// Fallback: many tracers use bottom-left origin without an explicit group transform.
		ty, sy = height, -1
	}
	var rings [][]VectorPoint
	for _, m := range matches {
		parsed, err := parsePathData(m[1])
		if err != nil {
			return nil, err
		}
		for _, ring := range parsed {
			if len(ring) < 3 {
				continue
			}
			mapped := make([]VectorPoint, len(ring))
			for i, p := range ring {
				mapped[i] = VectorPoint{X: tx + p.X*sx, Y: ty + p.Y*sy}
			}
			rings = append(rings, closeRing(mapped))
		}
	}
	if width > 0 {
		_ = width
	}
	return rings, nil
}

func parseSVGTransform(transform string) (tx, ty, sx, sy float64) {
	sx, sy = 1, 1
	if m := translateRE.FindStringSubmatch(transform); len(m) == 3 {
		tx, _ = strconv.ParseFloat(m[1], 64)
		ty, _ = strconv.ParseFloat(m[2], 64)
	}
	if m := scaleRE.FindStringSubmatch(transform); len(m) >= 2 {
		sx, _ = strconv.ParseFloat(m[1], 64)
		if m[2] != "" {
			sy, _ = strconv.ParseFloat(m[2], 64)
		} else {
			sy = sx
		}
	}
	return tx, ty, sx, sy
}

func closeRing(ring []VectorPoint) []VectorPoint {
	if len(ring) < 3 {
		return ring
	}
	first, last := ring[0], ring[len(ring)-1]
	if math.Hypot(first.X-last.X, first.Y-last.Y) < 1e-6 {
		return ring[:len(ring)-1]
	}
	return ring
}

func parsePathData(d string) ([][]VectorPoint, error) {
	tokens := tokenizePath(d)
	var rings [][]VectorPoint
	var ring []VectorPoint
	var cx, cy, startX, startY float64
	var lastCmd byte
	i := 0
	flush := func() {
		if len(ring) >= 3 {
			rings = append(rings, ring)
		}
		ring = nil
	}
	for i < len(tokens) {
		tok := tokens[i]
		cmd := byte(0)
		if len(tok) == 1 && unicode.IsLetter(rune(tok[0])) {
			cmd = tok[0]
			i++
		} else if lastCmd != 0 {
			cmd = lastCmd
			if lastCmd == 'M' {
				cmd = 'L'
			} else if lastCmd == 'm' {
				cmd = 'l'
			}
		} else {
			return nil, fmt.Errorf("path data missing command near %q", tok)
		}
		rel := cmd >= 'a' && cmd <= 'z'
		upper := cmd
		if rel {
			upper = cmd - ('a' - 'A')
		}
		read := func(n int) ([]float64, error) {
			vals := make([]float64, 0, n)
			for len(vals) < n {
				if i >= len(tokens) {
					return nil, fmt.Errorf("path data truncated for %c", cmd)
				}
				v, err := strconv.ParseFloat(tokens[i], 64)
				if err != nil {
					return nil, fmt.Errorf("invalid path number %q", tokens[i])
				}
				vals = append(vals, v)
				i++
			}
			return vals, nil
		}
		switch upper {
		case 'M':
			vals, err := read(2)
			if err != nil {
				return nil, err
			}
			flush()
			x, y := vals[0], vals[1]
			if rel {
				x += cx
				y += cy
			}
			cx, cy = x, y
			startX, startY = x, y
			ring = []VectorPoint{{X: cx, Y: cy}}
			lastCmd = cmd
			for i < len(tokens) && !isCommand(tokens[i]) {
				vals, err = read(2)
				if err != nil {
					return nil, err
				}
				x, y = vals[0], vals[1]
				if rel {
					x += cx
					y += cy
				}
				cx, cy = x, y
				ring = append(ring, VectorPoint{X: cx, Y: cy})
				if rel {
					lastCmd = 'l'
				} else {
					lastCmd = 'L'
				}
			}
		case 'L':
			vals, err := read(2)
			if err != nil {
				return nil, err
			}
			x, y := vals[0], vals[1]
			if rel {
				x += cx
				y += cy
			}
			cx, cy = x, y
			ring = append(ring, VectorPoint{X: cx, Y: cy})
			lastCmd = cmd
		case 'H':
			vals, err := read(1)
			if err != nil {
				return nil, err
			}
			x := vals[0]
			if rel {
				x += cx
			}
			cx = x
			ring = append(ring, VectorPoint{X: cx, Y: cy})
			lastCmd = cmd
		case 'V':
			vals, err := read(1)
			if err != nil {
				return nil, err
			}
			y := vals[0]
			if rel {
				y += cy
			}
			cy = y
			ring = append(ring, VectorPoint{X: cx, Y: cy})
			lastCmd = cmd
		case 'C':
			vals, err := read(6)
			if err != nil {
				return nil, err
			}
			x1, y1, x2, y2, x, y := vals[0], vals[1], vals[2], vals[3], vals[4], vals[5]
			if rel {
				x1 += cx
				y1 += cy
				x2 += cx
				y2 += cy
				x += cx
				y += cy
			}
			ring = append(ring, sampleCubic(cx, cy, x1, y1, x2, y2, x, y)...)
			cx, cy = x, y
			lastCmd = cmd
		case 'Q':
			vals, err := read(4)
			if err != nil {
				return nil, err
			}
			x1, y1, x, y := vals[0], vals[1], vals[2], vals[3]
			if rel {
				x1 += cx
				y1 += cy
				x += cx
				y += cy
			}
			ring = append(ring, sampleQuad(cx, cy, x1, y1, x, y)...)
			cx, cy = x, y
			lastCmd = cmd
		case 'Z':
			if math.Hypot(cx-startX, cy-startY) > 1e-6 {
				ring = append(ring, VectorPoint{X: startX, Y: startY})
			}
			cx, cy = startX, startY
			flush()
			lastCmd = cmd
		default:
			return nil, fmt.Errorf("unsupported path command %c", cmd)
		}
	}
	flush()
	return rings, nil
}

func isCommand(tok string) bool {
	return len(tok) == 1 && unicode.IsLetter(rune(tok[0]))
}

func tokenizePath(d string) []string {
	d = strings.ReplaceAll(d, ",", " ")
	var tokens []string
	var cur strings.Builder
	flushNum := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(d); i++ {
		ch := d[i]
		switch {
		case unicode.IsSpace(rune(ch)):
			flushNum()
		case unicode.IsLetter(rune(ch)):
			flushNum()
			tokens = append(tokens, string(ch))
		case ch == '-' || ch == '+':
			if cur.Len() > 0 {
				flushNum()
			}
			cur.WriteByte(ch)
		case ch == '.' && strings.Contains(cur.String(), "."):
			flushNum()
			cur.WriteByte(ch)
		default:
			cur.WriteByte(ch)
		}
	}
	flushNum()
	return tokens
}

func sampleCubic(x0, y0, x1, y1, x2, y2, x3, y3 float64) []VectorPoint {
	steps := curveSteps(math.Hypot(x1-x0, y1-y0) + math.Hypot(x2-x1, y2-y1) + math.Hypot(x3-x2, y3-y2))
	out := make([]VectorPoint, 0, steps)
	for s := 1; s <= steps; s++ {
		t := float64(s) / float64(steps)
		u := 1 - t
		x := u*u*u*x0 + 3*u*u*t*x1 + 3*u*t*t*x2 + t*t*t*x3
		y := u*u*u*y0 + 3*u*u*t*y1 + 3*u*t*t*y2 + t*t*t*y3
		out = append(out, VectorPoint{X: x, Y: y})
	}
	return out
}

func sampleQuad(x0, y0, x1, y1, x2, y2 float64) []VectorPoint {
	steps := curveSteps(math.Hypot(x1-x0, y1-y0) + math.Hypot(x2-x1, y2-y1))
	out := make([]VectorPoint, 0, steps)
	for s := 1; s <= steps; s++ {
		t := float64(s) / float64(steps)
		u := 1 - t
		x := u*u*x0 + 2*u*t*x1 + t*t*x2
		y := u*u*y0 + 2*u*t*y1 + t*t*y2
		out = append(out, VectorPoint{X: x, Y: y})
	}
	return out
}

func curveSteps(approxLen float64) int {
	steps := int(math.Ceil(approxLen / 1.5))
	if steps < 4 {
		return 4
	}
	if steps > 24 {
		return 24
	}
	return steps
}
