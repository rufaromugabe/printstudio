// Package production implements deterministic print-production algorithms.
package production

import (
	"image"
	"math"
)

type UnderbaseConfig struct {
	Threshold    uint8
	SpreadPixels int
}

// Underbase uses an exact squared Euclidean distance transform. Positive spread
// dilates the alpha mask; negative spread chokes it. Runtime is O(width*height),
// independent of radius.
func Underbase(src image.Image, c UnderbaseConfig) *image.Alpha {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if c.Threshold == 0 {
		c.Threshold = 1
	}
	mask := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			mask[y*w+x] = uint8(a>>8) >= c.Threshold
		}
	}
	out := image.NewAlpha(image.Rect(0, 0, w, h))
	if c.SpreadPixels == 0 {
		for i, on := range mask {
			if on {
				out.Pix[i] = 255
			}
		}
		return out
	}
	radius := c.SpreadPixels
	if radius > 0 {
		dist := distanceTransform(mask, w, h, true)
		r2 := float64(radius * radius)
		for i, d := range dist {
			if d <= r2 {
				out.Pix[i] = 255
			}
		}
	} else {
		radius = -radius
		dist := distanceTransform(mask, w, h, false)
		r2 := float64(radius * radius)
		for i, on := range mask {
			if on && dist[i] > r2 {
				out.Pix[i] = 255
			}
		}
	}
	return out
}

func distanceTransform(mask []bool, w, h int, target bool) []float64 {
	const inf = 1e20
	found := false
	for _, v := range mask {
		found = found || v == target
	}
	if !found {
		out := make([]float64, w*h)
		for i := range out {
			out[i] = inf
		}
		return out
	}
	tmp := make([]float64, w*h)
	f := make([]float64, maxInt(w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] == target {
				f[x] = 0
			} else {
				f[x] = inf
			}
		}
		d := edt1d(f[:w])
		copy(tmp[y*w:(y+1)*w], d)
	}
	out := make([]float64, w*h)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			f[y] = tmp[y*w+x]
		}
		d := edt1d(f[:h])
		for y := 0; y < h; y++ {
			out[y*w+x] = d[y]
		}
	}
	return out
}
func edt1d(f []float64) []float64 {
	n := len(f)
	d := make([]float64, n)
	first := -1
	for i, value := range f {
		if value < 1e19 {
			first = i
			break
		}
	}
	if first < 0 {
		for i := range d {
			d[i] = 1e20
		}
		return d
	}
	v := make([]int, n)
	z := make([]float64, n+1)
	k := 0
	v[0] = first
	z[0] = math.Inf(-1)
	z[1] = math.Inf(1)
	for q := first + 1; q < n; q++ {
		s := intersection(f, q, v[k])
		for s <= z[k] {
			k--
			s = intersection(f, q, v[k])
		}
		k++
		v[k] = q
		z[k] = s
		z[k+1] = math.Inf(1)
	}
	k = 0
	for q := 0; q < n; q++ {
		for z[k+1] < float64(q) {
			k++
		}
		dx := q - v[k]
		d[q] = float64(dx*dx) + f[v[k]]
	}
	return d
}
func intersection(f []float64, q, v int) float64 {
	return ((f[q] + float64(q*q)) - (f[v] + float64(v*v))) / float64(2*q-2*v)
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
