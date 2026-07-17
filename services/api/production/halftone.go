package production

import (
	"image"
	"math"
)

type HalftoneConfig struct{ DPI, LPI, AngleDegrees, Gamma float64 }
type ScreeningMode string

const (
	ScreeningAM ScreeningMode = "am"
	ScreeningFM ScreeningMode = "fm"
)

// AMHalftone generates a deterministic amplitude-modulated round-dot screen.
func AMHalftone(src image.Image, c HalftoneConfig) *image.Gray {
	if c.DPI <= 0 {
		c.DPI = 300
	}
	if c.LPI <= 0 {
		c.LPI = 45
	}
	if c.Gamma <= 0 {
		c.Gamma = 1
	}
	cell := c.DPI / c.LPI
	if cell < 2 {
		cell = 2
	}
	angle := c.AngleDegrees * math.Pi / 180
	co, si := math.Cos(angle), math.Sin(angle)
	b := src.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			luma := (.2126*float64(r) + .7152*float64(g) + .0722*float64(bl)) / 65535
			coverage := math.Pow((1-luma)*float64(a)/65535, c.Gamma)
			u := float64(x)*co + float64(y)*si
			v := -float64(x)*si + float64(y)*co
			du := math.Mod(u, cell)
			if du < 0 {
				du += cell
			}
			dv := math.Mod(v, cell)
			if dv < 0 {
				dv += cell
			}
			du -= cell / 2
			dv -= cell / 2
			rank := math.Min(1, math.Pi*(du*du+dv*dv)/(cell*cell))
			if coverage >= rank {
				out.Pix[y*out.Stride+x] = 255
			}
		}
	}
	return out
}

// FMHalftone generates a deterministic frequency-modulated (stochastic) screen
// using a fixed blue-noise-like 64×64 threshold tile. Prefer this when AM
// screen-angle conflicts cannot be resolved.
func FMHalftone(src image.Image, gamma float64) *image.Gray {
	if gamma <= 0 {
		gamma = 1
	}
	b := src.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	tile := fmThresholdTile()
	const n = 64
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			luma := (.2126*float64(r) + .7152*float64(g) + .0722*float64(bl)) / 65535
			coverage := math.Pow((1-luma)*float64(a)/65535, gamma)
			threshold := float64(tile[(y%n)*n+(x%n)]) / 255
			if coverage > threshold {
				out.Pix[y*out.Stride+x] = 255
			}
		}
	}
	return out
}

func FMHalftoneCoverage(mask *image.Gray, gamma float64) *image.Gray {
	inverted := image.NewGray(mask.Bounds())
	for i, value := range mask.Pix {
		inverted.Pix[i] = 255 - value
	}
	return FMHalftone(inverted, gamma)
}

// fmThresholdTile returns a deterministic 64×64 dispersed-dot threshold matrix.
func fmThresholdTile() [64 * 64]uint8 {
	var tile [64 * 64]uint8
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			// Bit-reversal / Bayer-style recursive index, remapped to 0..255.
			v := uint8(((x ^ y) * 37) + (x * 17) + (y * 29))
			tile[y*64+x] = v
		}
	}
	return tile
}

type CMYKSeparations struct{ Cyan, Magenta, Yellow, Black *image.Gray }

func SeparateCMYK(src image.Image) CMYKSeparations {
	b := src.Bounds()
	newBand := func() *image.Gray { return image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy())) }
	s := CMYKSeparations{newBand(), newBand(), newBand(), newBand()}
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			rf, gf, bf := float64(r)/65535, float64(g)/65535, float64(bl)/65535
			k := 1 - math.Max(rf, math.Max(gf, bf))
			c, m, yy := 0.0, 0.0, 0.0
			if k < 1 {
				c = (1 - rf - k) / (1 - k)
				m = (1 - gf - k) / (1 - k)
				yy = (1 - bf - k) / (1 - k)
			}
			alpha := float64(a) / 65535
			i := y*s.Cyan.Stride + x
			s.Cyan.Pix[i] = uint8(math.Round(c * alpha * 255))
			s.Magenta.Pix[i] = uint8(math.Round(m * alpha * 255))
			s.Yellow.Pix[i] = uint8(math.Round(yy * alpha * 255))
			s.Black.Pix[i] = uint8(math.Round(k * alpha * 255))
		}
	}
	return s
}
