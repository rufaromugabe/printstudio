package embroidery

import (
	"bytes"
	"fmt"
	"math"
)

// EncodeDST lowers a validated plan to Tajima's 0.1 mm relative-coordinate stream.
func EncodeDST(d Document, label string) ([]byte, error) {
	if HasErrors(d.Diagnostics) {
		return nil, fmt.Errorf("document has validation errors")
	}
	var body bytes.Buffer
	current := Point{}
	stitches, colors := 0, 0
	thread := ""
	minX, minY, maxX, maxY := 0, 0, 0, 0
	writeMove := func(target Point, jump bool) {
		dx := int(math.Round((target.X - current.X) * 10))
		dy := int(math.Round((target.Y - current.Y) * 10))
		for dx != 0 || dy != 0 {
			sx := clamp(dx, -121, 121)
			sy := clamp(dy, -121, 121)
			body.Write(encodeDelta(sx, sy, jump))
			dx -= sx
			dy -= sy
			stitches++
		}
		current = target
		x, y := int(math.Round(current.X*10)), int(math.Round(current.Y*10))
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	for bi, b := range d.Plan {
		if bi == 0 || b.ThreadID != thread {
			if bi > 0 {
				body.Write([]byte{0, 0, 0xC3})
			}
			colors++
			thread = b.ThreadID
		}
		for _, sequence := range [][]Stitch{b.Underlay, b.Stitches} {
			if len(sequence) == 0 {
				continue
			}
			writeMove(sequence[0].Position, true)
			for i := 1; i < len(sequence); i++ {
				writeMove(sequence[i].Position, sequence[i].Command == CommandJump)
			}
		}
	}
	if colors == 0 {
		colors = 1
	}
	body.Write([]byte{0, 0, 0xF3})
	if len(label) > 16 {
		label = label[:16]
	}
	header := fmt.Sprintf("LA:%-16s\rST:%7d\rCO:%3d\r+X:%5d\r-X:%5d\r+Y:%5d\r-Y:%5d\rAX:%+06d\rAY:%+06d\rMX:+00000\rMY:+00000\rPD:******\r", label, stitches, colors, maxX, -minX, maxY, -minY, int(math.Round(current.X*10)), int(math.Round(current.Y*10)))
	h := make([]byte, 512)
	for i := range h {
		h[i] = ' '
	}
	copy(h, header)
	h[511] = 0x1A
	return append(h, body.Bytes()...), nil
}

func encodeDelta(x, y int, jump bool) []byte {
	var b [3]byte
	b[2] = 3
	apply := func(v *int, weight int, index int, positive, negative byte) {
		threshold := weight / 2
		if *v > threshold {
			b[index] |= positive
			*v -= weight
		} else if *v < -threshold {
			b[index] |= negative
			*v += weight
		}
	}
	apply(&x, 81, 2, 0x04, 0x08)
	apply(&x, 27, 1, 0x04, 0x08)
	apply(&x, 9, 0, 0x04, 0x08)
	apply(&x, 3, 1, 0x01, 0x02)
	apply(&x, 1, 0, 0x01, 0x02)
	apply(&y, 81, 2, 0x20, 0x10)
	apply(&y, 27, 1, 0x20, 0x10)
	apply(&y, 9, 0, 0x20, 0x10)
	apply(&y, 3, 1, 0x80, 0x40)
	apply(&y, 1, 0, 0x80, 0x40)
	if jump {
		b[2] |= 0x80
	}
	return b[:]
}
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// DecodeDST reads the stitch command stream emitted by Tajima-compatible DST
// files. Coordinates are returned in millimetres and exclude the terminal END.
func DecodeDST(data []byte) ([]Stitch, error) {
	if len(data) < 515 || data[511] != 0x1a {
		return nil, fmt.Errorf("invalid DST header")
	}
	var out []Stitch
	position := Point{}
	for offset := 512; offset+2 < len(data); offset += 3 {
		b := data[offset : offset+3]
		if b[2] == 0xf3 {
			return out, nil
		}
		if b[2]&0xc3 == 0xc3 {
			out = append(out, Stitch{Position: position, Command: CommandColorChange, Source: "dst_decode"})
			continue
		}
		dx, dy := decodeDelta(b)
		position.X += float64(dx) / 10
		position.Y += float64(dy) / 10
		command := CommandStitch
		if b[2]&0x80 != 0 {
			command = CommandJump
		}
		out = append(out, Stitch{Position: position, Command: command, Source: "dst_decode"})
	}
	return nil, fmt.Errorf("DST stream has no END command")
}

func decodeDelta(b []byte) (int, int) {
	x := bitValue(b[0], 0x01, 0x02, 1) + bitValue(b[1], 0x01, 0x02, 3) + bitValue(b[0], 0x04, 0x08, 9) + bitValue(b[1], 0x04, 0x08, 27) + bitValue(b[2], 0x04, 0x08, 81)
	y := bitValue(b[0], 0x80, 0x40, 1) + bitValue(b[1], 0x80, 0x40, 3) + bitValue(b[0], 0x20, 0x10, 9) + bitValue(b[1], 0x20, 0x10, 27) + bitValue(b[2], 0x20, 0x10, 81)
	return x, y
}

func bitValue(value, positive, negative byte, weight int) int {
	if value&positive != 0 {
		return weight
	}
	if value&negative != 0 {
		return -weight
	}
	return 0
}
