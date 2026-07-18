package production

import (
	"fmt"
	"image"
	"image/color"
	"os"
)

// WriteAlphaPBM writes a binary PBM (P4) mask from opaque pixels.
// Potrace treats black (1) as the traced foreground.
func WriteAlphaPBM(img image.Image, path string, alphaCutoff uint8) error {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("empty image")
	}
	rowBytes := (w + 7) / 8
	buf := make([]byte, rowBytes*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			nrgba := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if nrgba.A >= alphaCutoff {
				byteIndex := y*rowBytes + x/8
				buf[byteIndex] |= 0x80 >> uint(x%8)
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "P4\n%d %d\n", w, h); err != nil {
		return err
	}
	_, err = f.Write(buf)
	return err
}
