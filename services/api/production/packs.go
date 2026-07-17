package production

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

type ArtifactFile struct {
	Name   string
	Role   string
	MIME   string
	Data   []byte
	SHA256 string
}

type DTFPackConfig struct {
	SpreadPixels int
	Threshold    uint8
}

type ScreenPackConfig struct {
	DPI              float64
	LPI              float64
	Gamma            float64
	UnderbaseChokePX int
	Screening        ScreeningMode
	Angles           ScreenAngleSet
	TrapPresetID     string
}

type GangConfig struct {
	Sheet       Sheet
	SourceWMM   float64
	SourceHMM   float64
	Copies      int
	FillSheet   bool
	DPI         float64
	AllowRotate bool
	MaxPixels   int64
}

func NewArtifact(name, role, mime string, data []byte) ArtifactFile {
	digest := sha256.Sum256(data)
	return ArtifactFile{Name: name, Role: role, MIME: mime, Data: data, SHA256: hex.EncodeToString(digest[:])}
}

func EncodePNG(src image.Image) ([]byte, error) {
	var out bytes.Buffer
	if err := png.Encode(&out, src); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func BuildDTFFiles(src image.Image, colorPNG []byte, config DTFPackConfig) ([]ArtifactFile, error) {
	underbase := Underbase(src, UnderbaseConfig{Threshold: config.Threshold, SpreadPixels: config.SpreadPixels})
	underbasePNG, err := EncodePNG(underbase)
	if err != nil {
		return nil, fmt.Errorf("encode DTF underbase: %w", err)
	}
	return []ArtifactFile{
		NewArtifact("color.png", "colour-artwork", "image/png", colorPNG),
		NewArtifact("white-underbase.png", "white-underbase", "image/png", underbasePNG),
	}, nil
}

func BuildSublimationFiles(colorPNG []byte) ([]ArtifactFile, error) {
	if len(colorPNG) == 0 {
		return nil, fmt.Errorf("sublimation package requires colour artwork")
	}
	return []ArtifactFile{
		NewArtifact("print-ready.png", "sublimation-print", "image/png", colorPNG),
		NewArtifact("README-sublimation.txt", "instructions", "text/plain", []byte("Sublimation package\r\n- print-ready.png includes configured bleed\r\n- Mirror on the RIP/printer if your press workflow requires it\r\n- No white underbase is included for dye-sub\r\n- Verify paper ICC / media profile on press\r\n")),
	}, nil
}

func BuildScreenFiles(src image.Image, config ScreenPackConfig) ([]ArtifactFile, error) {
	if config.DPI <= 0 {
		config.DPI = 300
	}
	if config.LPI <= 0 {
		config.LPI = 45
	}
	if config.Gamma <= 0 {
		config.Gamma = 1
	}
	if config.Screening == "" {
		config.Screening = ScreeningAM
	}
	if config.Angles == (ScreenAngleSet{}) {
		config.Angles = DefaultScreenAngles()
	}
	if config.Screening == ScreeningAM {
		if err := RejectScreenAngleConflicts(config.Angles); err != nil {
			return nil, err
		}
	}
	if config.Screening != ScreeningAM && config.Screening != ScreeningFM {
		return nil, fmt.Errorf("unsupported screening mode %q", config.Screening)
	}
	separations := SeparateCMYK(src)
	bands := []struct {
		name  string
		angle float64
		mask  *image.Gray
	}{
		{"cyan", config.Angles.Cyan, separations.Cyan},
		{"magenta", config.Angles.Magenta, separations.Magenta},
		{"yellow", config.Angles.Yellow, separations.Yellow},
		{"black", config.Angles.Black, separations.Black},
	}
	files := make([]ArtifactFile, 0, 10)
	for _, band := range bands {
		continuous, err := EncodePNG(band.mask)
		if err != nil {
			return nil, fmt.Errorf("encode %s separation: %w", band.name, err)
		}
		var screen *image.Gray
		if config.Screening == ScreeningFM {
			screen = FMHalftoneCoverage(band.mask, config.Gamma)
		} else {
			screen = AMHalftoneCoverage(band.mask, HalftoneConfig{DPI: config.DPI, LPI: config.LPI, AngleDegrees: band.angle, Gamma: config.Gamma})
		}
		halftone, err := EncodePNG(screen)
		if err != nil {
			return nil, fmt.Errorf("encode %s halftone: %w", band.name, err)
		}
		name := fmt.Sprintf("separations/%s-fm.png", band.name)
		if config.Screening == ScreeningAM {
			name = fmt.Sprintf("separations/%s-%.1fdeg-%.0flpi.png", band.name, band.angle, config.LPI)
		}
		files = append(files,
			NewArtifact("separations/"+band.name+"-continuous.png", band.name+"-continuous", "image/png", continuous),
			NewArtifact(name, band.name+"-halftone", "image/png", halftone),
		)
	}
	underbase := Underbase(src, UnderbaseConfig{SpreadPixels: config.UnderbaseChokePX})
	underbasePNG, err := EncodePNG(underbase)
	if err != nil {
		return nil, fmt.Errorf("encode screen underbase: %w", err)
	}
	files = append(files, NewArtifact("separations/white-underbase.png", "white-underbase", "image/png", underbasePNG))
	return files, nil
}

// AMHalftoneCoverage screens an 8-bit ink-coverage mask where 0 means no ink
// and 255 means full ink. AMHalftone accepts image luminance, whose polarity is
// the opposite, so this conversion is deliberately explicit.
func AMHalftoneCoverage(mask *image.Gray, config HalftoneConfig) *image.Gray {
	inverted := image.NewGray(mask.Bounds())
	for i, value := range mask.Pix {
		inverted.Pix[i] = 255 - value
	}
	return AMHalftone(inverted, config)
}

func RenderGangSheet(src image.Image, config GangConfig) (*image.NRGBA, []Placement, error) {
	if config.DPI <= 0 {
		config.DPI = 300
	}
	if config.FillSheet {
		copies, err := MaxCopiesForSheet(config.Sheet, config.SourceWMM, config.SourceHMM, config.AllowRotate, 500)
		if err != nil {
			return nil, nil, err
		}
		config.Copies = copies
	}
	if config.Copies < 1 || config.Copies > 500 {
		return nil, nil, fmt.Errorf("gang copy count must be between 1 and 500")
	}
	if config.SourceWMM <= 0 || config.SourceHMM <= 0 {
		return nil, nil, fmt.Errorf("source physical dimensions must be positive")
	}
	width := int(math.Ceil(config.Sheet.WidthMM / 25.4 * config.DPI))
	height := int(math.Ceil(config.Sheet.HeightMM / 25.4 * config.DPI))
	if width <= 0 || height <= 0 {
		return nil, nil, fmt.Errorf("invalid gang-sheet pixel dimensions")
	}
	if config.MaxPixels <= 0 {
		config.MaxPixels = 100_000_000
	}
	if int64(width)*int64(height) > config.MaxPixels {
		return nil, nil, fmt.Errorf("gang sheet exceeds the %d-megapixel server limit", config.MaxPixels/1_000_000)
	}
	placements, err := Nest(config.Sheet, []Item{{ID: "artwork", WidthMM: config.SourceWMM, HeightMM: config.SourceHMM, Quantity: config.Copies, AllowRotate: config.AllowRotate}})
	if err != nil {
		return nil, nil, err
	}
	targetW := int(math.Round(config.SourceWMM / 25.4 * config.DPI))
	targetH := int(math.Round(config.SourceHMM / 25.4 * config.DPI))
	var normal image.Image = src
	if src.Bounds().Dx() != targetW || src.Bounds().Dy() != targetH {
		normal = scaleBilinear(src, targetW, targetH)
	}
	rotated := rotate90(normal)
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for _, placement := range placements {
		tile := normal
		if placement.Rotated {
			tile = rotated
		}
		x := int(math.Round(placement.XMM / 25.4 * config.DPI))
		y := int(math.Round(placement.YMM / 25.4 * config.DPI))
		placementW := tile.Bounds().Dx()
		placementH := tile.Bounds().Dy()
		draw.Draw(out, image.Rect(x, y, x+placementW, y+placementH), tile, tile.Bounds().Min, draw.Over)
	}
	return out, placements, nil
}

func rotate90(src image.Image) *image.NRGBA {
	bounds := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, bounds.Dy(), bounds.Dx()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			out.Set(bounds.Dy()-1-y, x, src.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return out
}

func scaleBilinear(src image.Image, width, height int) *image.NRGBA {
	if width <= 0 || height <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, 0, 0))
	}
	bounds := src.Bounds()
	sourceW, sourceH := bounds.Dx(), bounds.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sy := (float64(y)+0.5)*float64(sourceH)/float64(height) - 0.5
		y0 := clampInt(int(math.Floor(sy)), 0, sourceH-1)
		y1 := clampInt(y0+1, 0, sourceH-1)
		fy := sy - math.Floor(sy)
		for x := 0; x < width; x++ {
			sx := (float64(x)+0.5)*float64(sourceW)/float64(width) - 0.5
			x0 := clampInt(int(math.Floor(sx)), 0, sourceW-1)
			x1 := clampInt(x0+1, 0, sourceW-1)
			fx := sx - math.Floor(sx)
			c00 := rgba64(src.At(bounds.Min.X+x0, bounds.Min.Y+y0))
			c10 := rgba64(src.At(bounds.Min.X+x1, bounds.Min.Y+y0))
			c01 := rgba64(src.At(bounds.Min.X+x0, bounds.Min.Y+y1))
			c11 := rgba64(src.At(bounds.Min.X+x1, bounds.Min.Y+y1))
			blend := func(a, b, c, d uint32) uint32 {
				top := float64(a)*(1-fx) + float64(b)*fx
				bottom := float64(c)*(1-fx) + float64(d)*fx
				return uint32(math.Round(top*(1-fy) + bottom*fy))
			}
			r := blend(c00[0], c10[0], c01[0], c11[0])
			g := blend(c00[1], c10[1], c01[1], c11[1])
			b := blend(c00[2], c10[2], c01[2], c11[2])
			a := blend(c00[3], c10[3], c01[3], c11[3])
			if a == 0 {
				out.SetNRGBA(x, y, color.NRGBA{})
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{R: uint8(minUint32(65535, r*65535/a) >> 8), G: uint8(minUint32(65535, g*65535/a) >> 8), B: uint8(minUint32(65535, b*65535/a) >> 8), A: uint8(a >> 8)})
		}
	}
	return out
}

func rgba64(value color.Color) [4]uint32 {
	r, g, b, a := value.RGBA()
	return [4]uint32{r, g, b, a}
}

func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
