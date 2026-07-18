package production

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	TracerPotrace      = "potrace"
	TracerMLAssisted   = "ml-assisted"
	DefaultAlphaCutoff = uint8(32)
	MaxVectorPaths     = 2500
	MaxPathPoints      = 500_000
)

// VectorPoint is a contour vertex.
type VectorPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// VectorContourSet is the canonical IR consumed by vinyl/embroidery/screen.
type VectorContourSet struct {
	Rings       [][]VectorPoint   `json:"rings"`
	SourceHash  string            `json:"sourceHash"`
	Tracer      string            `json:"tracer"`
	WidthPx     int               `json:"widthPx"`
	HeightPx    int               `json:"heightPx"`
	PathCount   int               `json:"pathCount"`
	MinFeature  float64           `json:"minFeatureMm"`
	Units       string            `json:"units"` // "px" or "mm"
	Diagnostics []VectorDiagnostic `json:"diagnostics,omitempty"`
}

type VectorDiagnostic struct {
	Severity string `json:"severity"` // error|warning
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// VectorizePlacement maps image-local pixels into centered print-area millimetres.
type VectorizePlacement struct {
	CanvasWidth      float64 `json:"canvasWidth"`
	CanvasHeight     float64 `json:"canvasHeight"`
	PhysicalWidthMm  float64 `json:"physicalWidthMm"`
	PhysicalHeightMm float64 `json:"physicalHeightMm"`
	X                float64 `json:"x"`
	Y                float64 `json:"y"`
	W                float64 `json:"w"`
	H                float64 `json:"h"`
	Rotation         float64 `json:"rotation"`
}

type VectorizeOptions struct {
	Method      string // vinyl|embroidery|screen
	AlphaCutoff uint8
	MLPrep      bool
	Placement   *VectorizePlacement
	Tools       NativeTools
	Prep        ImagePrep // optional; nil uses passthrough
	MaxPaths    int
}

// ImagePrep is the ML/cleanup stage before Potrace.
type ImagePrep interface {
	PrepareForVectorize(img image.Image) (image.Image, string, error)
}

type passthroughPrep struct{}

func (passthroughPrep) PrepareForVectorize(img image.Image) (image.Image, string, error) {
	return img, TracerPotrace, nil
}

// Vectorize runs alpha→PBM→Potrace→SVG rings, optional Clipper cleanup, optional mm mapping.
func Vectorize(ctx context.Context, img image.Image, opt VectorizeOptions) (*VectorContourSet, error) {
	if img == nil {
		return nil, fmt.Errorf("image is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opt.AlphaCutoff == 0 {
		opt.AlphaCutoff = DefaultAlphaCutoff
	}
	if opt.MaxPaths <= 0 {
		opt.MaxPaths = MaxVectorPaths
	}
	prep := opt.Prep
	if prep == nil {
		prep = passthroughPrep{}
	}
	prepared, tracer, err := prep.PrepareForVectorize(img)
	if err != nil {
		return nil, fmt.Errorf("ml prep: %w", err)
	}
	if tracer == "" {
		tracer = TracerPotrace
	}

	bounds := prepared.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 2 || h < 2 {
		return nil, fmt.Errorf("image is too small to vectorize")
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, prepared); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(pngBuf.Bytes())

	tmp, err := os.MkdirTemp("", "printstudio-vectorize-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	pbmPath := filepath.Join(tmp, "mask.pbm")
	svgPath := filepath.Join(tmp, "mask.svg")
	if err := WriteAlphaPBM(prepared, pbmPath, opt.AlphaCutoff); err != nil {
		return nil, err
	}
	if err := opt.Tools.TracePBM(ctx, pbmPath, svgPath); err != nil {
		return nil, err
	}
	svgBytes, err := os.ReadFile(svgPath)
	if err != nil {
		return nil, err
	}
	rings, err := ParseSVGPathRings(string(svgBytes), float64(w), float64(h))
	if err != nil {
		return nil, err
	}
	if len(rings) == 0 {
		return nil, fmt.Errorf("potrace produced no contours")
	}
	if len(rings) > opt.MaxPaths {
		return nil, fmt.Errorf("path explosion: %d contours exceed cap %d", len(rings), opt.MaxPaths)
	}

	paths := toPolygonPaths(rings)
	if Clipper2Available() {
		if cleaned, err := OffsetPolygons(paths, 0, JoinRound, 2); err == nil && len(cleaned) > 0 {
			paths = cleaned
		}
	}
	rings = fromPolygonPaths(paths)

	out := &VectorContourSet{
		Rings:      rings,
		SourceHash: hex.EncodeToString(sum[:]),
		Tracer:     tracer,
		WidthPx:    w,
		HeightPx:   h,
		PathCount:  len(rings),
		Units:      "px",
	}
	out.Diagnostics = QualityGate(rings, "px")
	if HasVectorErrors(out.Diagnostics) {
		return out, fmt.Errorf("vectorize failed quality gates")
	}

	if opt.Placement != nil {
		mapped := make([][]VectorPoint, len(rings))
		for i, ring := range rings {
			mapped[i] = make([]VectorPoint, len(ring))
			for j, p := range ring {
				lx := p.X / float64(w) * opt.Placement.W
				ly := p.Y / float64(h) * opt.Placement.H
				mapped[i][j] = toPhysicalMM(lx, ly, *opt.Placement)
			}
		}
		out.Rings = mapped
		out.Units = "mm"
		out.MinFeature = minFeatureSize(mapped)
		out.Diagnostics = QualityGate(mapped, "mm")
		if HasVectorErrors(out.Diagnostics) {
			return out, fmt.Errorf("vectorize failed quality gates")
		}
	} else {
		out.MinFeature = minFeatureSize(rings)
	}
	return out, nil
}

func toPolygonPaths(rings [][]VectorPoint) PolygonPaths {
	out := make(PolygonPaths, 0, len(rings))
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		path := make([]PolygonPoint, len(ring))
		for i, p := range ring {
			path[i] = PolygonPoint{X: p.X, Y: p.Y}
		}
		out = append(out, path)
	}
	return out
}

func fromPolygonPaths(paths PolygonPaths) [][]VectorPoint {
	out := make([][]VectorPoint, 0, len(paths))
	for _, path := range paths {
		if len(path) < 3 {
			continue
		}
		ring := make([]VectorPoint, len(path))
		for i, p := range path {
			ring[i] = VectorPoint{X: p.X, Y: p.Y}
		}
		out = append(out, ring)
	}
	return out
}

func toPhysicalMM(localX, localY float64, p VectorizePlacement) VectorPoint {
	x := p.X + localX
	y := p.Y + localY
	cx := p.X + p.W/2
	cy := p.Y + p.H/2
	a := p.Rotation * math.Pi / 180
	c, s := math.Cos(a), math.Sin(a)
	rx := cx + (x-cx)*c - (y-cy)*s
	ry := cy + (x-cx)*s + (y-cy)*c
	return VectorPoint{
		X: rx/p.CanvasWidth*p.PhysicalWidthMm - p.PhysicalWidthMm/2,
		Y: ry/p.CanvasHeight*p.PhysicalHeightMm - p.PhysicalHeightMm/2,
	}
}

func minFeatureSize(rings [][]VectorPoint) float64 {
	min := math.Inf(1)
	for _, ring := range rings {
		if len(ring) < 3 {
			continue
		}
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		for _, p := range ring {
			minX = math.Min(minX, p.X)
			minY = math.Min(minY, p.Y)
			maxX = math.Max(maxX, p.X)
			maxY = math.Max(maxY, p.Y)
		}
		feat := math.Min(maxX-minX, maxY-minY)
		if feat > 0 && feat < min {
			min = feat
		}
	}
	if math.IsInf(min, 1) {
		return 0
	}
	return min
}

// QualityGate applies production limits before further processing.
func QualityGate(rings [][]VectorPoint, units string) []VectorDiagnostic {
	var out []VectorDiagnostic
	if len(rings) == 0 {
		out = append(out, VectorDiagnostic{Severity: "error", Code: "NO_CONTOURS", Message: "no contours produced"})
		return out
	}
	if len(rings) > MaxVectorPaths {
		out = append(out, VectorDiagnostic{Severity: "error", Code: "PATH_EXPLOSION", Message: fmt.Sprintf("%d contours exceed cap %d", len(rings), MaxVectorPaths)})
	}
	points := 0
	holes := 0
	for _, ring := range rings {
		points += len(ring)
		if signedArea(ring) < 0 {
			holes++
		}
	}
	if points > MaxPathPoints {
		out = append(out, VectorDiagnostic{Severity: "error", Code: "POINT_EXPLOSION", Message: fmt.Sprintf("%d vertices exceed cap %d", points, MaxPathPoints)})
	}
	minFeat := minFeatureSize(rings)
	warnMM, rejectMM := 0.8, 0.35
	if units == "px" {
		warnMM, rejectMM = 2, 1
	}
	if minFeat > 0 && minFeat < rejectMM {
		out = append(out, VectorDiagnostic{Severity: "error", Code: "FEATURE_TOO_SMALL", Message: fmt.Sprintf("smallest feature %.2f %s is below reject threshold %.2f", minFeat, units, rejectMM)})
	} else if minFeat > 0 && minFeat < warnMM {
		out = append(out, VectorDiagnostic{Severity: "warning", Code: "FEATURE_SMALL", Message: fmt.Sprintf("smallest feature %.2f %s may weed/sew poorly", minFeat, units)})
	}
	if holes > 0 {
		out = append(out, VectorDiagnostic{Severity: "warning", Code: "HOLES_PRESENT", Message: fmt.Sprintf("kept %d interior cutout(s)", holes)})
	}
	return out
}

func HasVectorErrors(ds []VectorDiagnostic) bool {
	for _, d := range ds {
		if d.Severity == "error" {
			return true
		}
	}
	return false
}

func signedArea(ring []VectorPoint) float64 {
	a := 0.0
	for i := 0; i < len(ring); i++ {
		j := (i + 1) % len(ring)
		a += ring[i].X*ring[j].Y - ring[j].X*ring[i].Y
	}
	return a / 2
}
