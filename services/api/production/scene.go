package production

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type SceneView struct {
	CanvasWidth      float64 `json:"canvasWidth"`
	CanvasHeight     float64 `json:"canvasHeight"`
	PhysicalWidthMm  float64 `json:"physicalWidthMm"`
	PhysicalHeightMm float64 `json:"physicalHeightMm"`
	BleedMm          float64 `json:"bleedMm"`
}

type SceneElement struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"`
	Value         string  `json:"value"`
	AssetID       string  `json:"assetId"`
	X             float64 `json:"x"`
	Y             float64 `json:"y"`
	W             float64 `json:"w"`
	H             float64 `json:"h"`
	Rotation      float64 `json:"rotation"`
	Color         string  `json:"color"`
	FontSize      float64 `json:"fontSize"`
	FontWeight    int     `json:"fontWeight"`
	LetterSpacing float64 `json:"letterSpacing"`
	LineHeight    float64 `json:"lineHeight"`
	SourceWidth   int     `json:"sourceWidth"`
	SourceHeight  int     `json:"sourceHeight"`
}

type SceneRenderRequest struct {
	Name          string         `json:"name"`
	Method        string         `json:"method"`
	DPI           float64        `json:"dpi"`
	View          SceneView      `json:"view"`
	Elements      []SceneElement `json:"elements"`
	CropToContent bool           `json:"cropToContent"`
}

type AssetFetcher func(assetID string) (io.ReadCloser, error)

// RenderScene composites design elements at physical size on the server.
func RenderScene(req SceneRenderRequest, fetch AssetFetcher) (*image.NRGBA, error) {
	if len(req.Elements) == 0 {
		return nil, fmt.Errorf("add at least one design element before rendering")
	}
	if req.DPI <= 0 {
		req.DPI = 300
	}
	if req.DPI < 72 || req.DPI > 600 {
		return nil, fmt.Errorf("DPI must be between 72 and 600")
	}
	view := req.View
	if view.CanvasWidth <= 0 || view.CanvasHeight <= 0 || view.PhysicalWidthMm <= 0 || view.PhysicalHeightMm <= 0 {
		return nil, fmt.Errorf("view physical and canvas dimensions are required")
	}
	bleed := view.BleedMm
	if bleed < 0 || bleed > 50 {
		return nil, fmt.Errorf("bleed must be between 0 and 50 mm")
	}
	widthMM := view.PhysicalWidthMm + bleed*2
	heightMM := view.PhysicalHeightMm + bleed*2
	width := int(math.Ceil(widthMM / 25.4 * req.DPI))
	height := int(math.Ceil(heightMM / 25.4 * req.DPI))
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 100_000_000 {
		return nil, fmt.Errorf("scene exceeds the 100-megapixel server render limit")
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	bleedPx := bleed / 25.4 * req.DPI
	sx := float64(width-int(math.Ceil(2*bleedPx))) / view.CanvasWidth
	sy := float64(height-int(math.Ceil(2*bleedPx))) / view.CanvasHeight
	offset := bleedPx
	for _, element := range req.Elements {
		switch strings.ToLower(element.Type) {
		case "image":
			img, err := loadSceneImage(element, fetch)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", element.ID, err)
			}
			drawRotated(out, img, offset+(element.X+element.W/2)*sx, offset+(element.Y+element.H/2)*sy, element.W*sx, element.H*sy, element.Rotation)
		case "text":
			tile, err := renderTextTile(element, sx, sy)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", element.ID, err)
			}
			drawRotated(out, tile, offset+(element.X+element.W/2)*sx, offset+(element.Y+element.H/2)*sy, float64(tile.Bounds().Dx()), float64(tile.Bounds().Dy()), element.Rotation)
		default:
			return nil, fmt.Errorf("unsupported element type %q", element.Type)
		}
	}
	if req.CropToContent {
		// Keep a small pad so DTF underbase/choke and cutters have room around ink.
		marginPx := int(math.Ceil(req.DPI / 25.4 * 1.5))
		cropped, err := CropOpaqueContent(out, marginPx, 8)
		if err != nil {
			return nil, err
		}
		return cropped, nil
	}
	return out, nil
}

// CropOpaqueContent trims empty transparent padding so export size matches inked artwork.
func CropOpaqueContent(src *image.NRGBA, marginPx int, alphaCutoff uint8) (*image.NRGBA, error) {
	if src == nil {
		return nil, fmt.Errorf("image is required")
	}
	bounds := src.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("image is empty")
	}
	if marginPx < 0 {
		marginPx = 0
	}
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X-1, bounds.Min.Y-1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if src.NRGBAAt(x, y).A >= alphaCutoff {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x > maxX {
					maxX = x
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return nil, fmt.Errorf("no opaque artwork found to size the export")
	}
	minX = maxInt(bounds.Min.X, minX-marginPx)
	minY = maxInt(bounds.Min.Y, minY-marginPx)
	maxX = minInt(bounds.Max.X-1, maxX+marginPx)
	maxY = minInt(bounds.Max.Y-1, maxY+marginPx)
	rect := image.Rect(0, 0, maxX-minX+1, maxY-minY+1)
	out := image.NewNRGBA(rect)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			out.SetNRGBA(x-minX, y-minY, src.NRGBAAt(x, y))
		}
	}
	return out, nil
}

func EncodeScenePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func loadSceneImage(element SceneElement, fetch AssetFetcher) (image.Image, error) {
	if element.AssetID != "" {
		if fetch == nil {
			return nil, fmt.Errorf("asset fetcher unavailable")
		}
		body, err := fetch(element.AssetID)
		if err != nil {
			return nil, err
		}
		defer body.Close()
		img, _, err := image.Decode(body)
		return img, err
	}
	value := strings.TrimSpace(element.Value)
	if strings.HasPrefix(value, "data:image/") {
		return decodeDataURL(value)
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Get(value)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("image URL returned status %d", resp.StatusCode)
		}
		img, _, err := image.Decode(resp.Body)
		return img, err
	}
	return nil, fmt.Errorf("image layer requires assetId or readable image URL")
}

func decodeDataURL(value string) (image.Image, error) {
	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid data URL")
	}
	raw, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid data URL payload")
		}
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

func renderTextTile(element SceneElement, sx, sy float64) (*image.NRGBA, error) {
	fontSize := element.FontSize * sy
	if fontSize < 8 {
		fontSize = 8
	}
	face, err := sceneFontFace(fontSize)
	if err != nil {
		return nil, err
	}
	w := int(math.Ceil(math.Max(1, element.W*sx)))
	h := int(math.Ceil(math.Max(1, element.H*sy)))
	tile := image.NewNRGBA(image.Rect(0, 0, w, h))
	col := parseColor(element.Color)
	lines := strings.Split(element.Value, "\n")
	lineHeight := fontSize * 1.1
	if element.LineHeight > 0 {
		lineHeight = fontSize * element.LineHeight
	}
	d := &font.Drawer{Dst: tile, Src: image.NewUniform(col), Face: face}
	for i, line := range lines {
		width := font.MeasureString(face, line).Round()
		x := (w - width) / 2
		y := int(float64(h)/2 + (float64(i)-float64(len(lines)-1)/2)*lineHeight + fontSize*0.35)
		d.Dot = fixed.P(x, y)
		d.DrawString(line)
	}
	return tile, nil
}

func drawRotated(dst *image.NRGBA, src image.Image, cx, cy, dw, dh, degrees float64) {
	srcBounds := src.Bounds()
	if srcBounds.Dx() == 0 || srcBounds.Dy() == 0 || dw <= 0 || dh <= 0 {
		return
	}
	scaled := scaleBilinear(src, int(math.Max(1, math.Round(dw))), int(math.Max(1, math.Round(dh))))
	rad := degrees * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)
	bw, bh := scaled.Bounds().Dx(), scaled.Bounds().Dy()
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			px := float64(x) - float64(bw)/2
			py := float64(y) - float64(bh)/2
			dx := int(math.Round(cx + px*cos - py*sin))
			dy := int(math.Round(cy + px*sin + py*cos))
			if dx < 0 || dy < 0 || dx >= dst.Bounds().Dx() || dy >= dst.Bounds().Dy() {
				continue
			}
			sc := scaled.NRGBAAt(x, y)
			if sc.A == 0 {
				continue
			}
			dst.SetNRGBA(dx, dy, over(dst.NRGBAAt(dx, dy), sc))
		}
	}
}

func over(dst, src color.NRGBA) color.NRGBA {
	as := float64(src.A) / 255
	ad := float64(dst.A) / 255
	outA := as + ad*(1-as)
	if outA == 0 {
		return color.NRGBA{}
	}
	blend := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*as + float64(d)*ad*(1-as)) / outA))
	}
	return color.NRGBA{R: blend(src.R, dst.R), G: blend(src.G, dst.G), B: blend(src.B, dst.B), A: uint8(math.Round(outA * 255))}
}

func sceneFontFace(size float64) (font.Face, error) {
	parsed, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(parsed, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func parseColor(value string) color.NRGBA {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) != 6 {
		return color.NRGBA{A: 255}
	}
	var n uint32
	_, _ = fmt.Sscanf(value, "%06x", &n)
	return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}
}
