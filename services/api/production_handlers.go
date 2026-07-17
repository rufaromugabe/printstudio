package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "image/jpeg"

	prod "printstudio/api/production"
)

func productionCapabilities(w http.ResponseWriter, _ *http.Request) {
	tools := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN")}
	capabilities := tools.Probe()
	capabilities.MaxRenderPixels = productionMaxPixels()
	write(w, http.StatusOK, capabilities)
}
func productionUnderbase(w http.ResponseWriter, r *http.Request) {
	src, ok := decodeProductionImage(w, r)
	if !ok {
		return
	}
	spread, _ := strconv.Atoi(r.URL.Query().Get("spread"))
	threshold64, _ := strconv.ParseUint(r.URL.Query().Get("threshold"), 10, 8)
	mask := prod.Underbase(src, prod.UnderbaseConfig{Threshold: uint8(threshold64), SpreadPixels: spread})
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-PrintStudio-Operation", "euclidean-underbase")
	_ = png.Encode(w, mask)
}

func productionDTFPack(w http.ResponseWriter, r *http.Request) {
	data, src, ok := decodeProductionPNG(w, r)
	if !ok {
		return
	}
	spread := integerQuery(r, "spread", 2)
	threshold := integerQuery(r, "threshold", 1)
	if spread < -100 || spread > 100 || threshold < 1 || threshold > 255 {
		problem(w, http.StatusUnprocessableEntity, "DTF spread must be between -100 and 100 pixels and threshold between 1 and 255")
		return
	}
	if err := validateRasterMetadata(r, src.Bounds()); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	files, err := prod.BuildDTFFiles(src, data, prod.DTFPackConfig{SpreadPixels: spread, Threshold: uint8(threshold)})
	if err != nil {
		problem(w, http.StatusInternalServerError, "DTF package generation failed")
		return
	}
	metadata := productionMetadata(r, src.Bounds())
	metadata["underbase"] = map[string]any{"algorithm": "exact-euclidean-distance-transform", "spreadPixels": spread, "threshold": threshold}
	metadata["warning"] = "Final ink limits, white density and printer-specific RIP settings must be verified by the operator."
	writeProductionArchive(w, safeProductionName(r.URL.Query().Get("name"))+"-dtf-package.zip", "DTF", files, metadata)
}

func productionScreenPack(w http.ResponseWriter, r *http.Request) {
	_, src, ok := decodeProductionPNG(w, r)
	if !ok {
		return
	}
	dpi := numberQuery(r, "dpi", 300)
	lpi := numberQuery(r, "lpi", 45)
	gamma := numberQuery(r, "gamma", 1)
	choke := integerQuery(r, "underbaseChoke", -2)
	if dpi < 72 || dpi > 1200 || lpi < 10 || lpi > 200 || lpi > dpi/2 || gamma < 0.1 || gamma > 5 || choke < -100 || choke > 100 {
		problem(w, http.StatusUnprocessableEntity, "invalid screen-pack DPI, LPI, gamma or underbase choke")
		return
	}
	if err := validateRasterMetadata(r, src.Bounds()); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	files, err := prod.BuildScreenFiles(src, prod.ScreenPackConfig{DPI: dpi, LPI: lpi, Gamma: gamma, UnderbaseChokePX: choke})
	if err != nil {
		problem(w, http.StatusInternalServerError, "screen package generation failed")
		return
	}
	metadata := productionMetadata(r, src.Bounds())
	metadata["screen"] = map[string]any{"model": "uncalibrated-cmyk-gcr", "dpi": dpi, "lpi": lpi, "gamma": gamma, "anglesDegrees": map[string]float64{"cyan": 15, "magenta": 75, "yellow": 0, "black": 45}, "underbaseChokePixels": choke}
	metadata["warning"] = "These process separations are not ICC calibrated. Confirm mesh, dot gain, ink set, substrate and screen-angle conflicts before exposing screens."
	writeProductionArchive(w, safeProductionName(r.URL.Query().Get("name"))+"-screen-package.zip", "Screen print", files, metadata)
}

func productionGangRender(w http.ResponseWriter, r *http.Request) {
	_, src, ok := decodeProductionPNG(w, r)
	if !ok {
		return
	}
	dpi := numberQuery(r, "dpi", 300)
	config := prod.GangConfig{
		Sheet: prod.Sheet{
			WidthMM:  numberQuery(r, "sheetWidthMm", 300),
			HeightMM: numberQuery(r, "sheetHeightMm", 400),
			MarginMM: numberQuery(r, "marginMm", 0),
			GapMM:    numberQuery(r, "gapMm", 5),
		},
		SourceWMM:   numberQuery(r, "sourceWidthMm", 0),
		SourceHMM:   numberQuery(r, "sourceHeightMm", 0),
		Copies:      integerQuery(r, "copies", 1),
		DPI:         dpi,
		AllowRotate: r.URL.Query().Get("allowRotate") == "true",
		MaxPixels:   productionMaxPixels(),
	}
	if dpi < 72 || dpi > 600 || config.Sheet.WidthMM > 2000 || config.Sheet.HeightMM > 2000 || config.Sheet.MarginMM < 0 || config.Sheet.GapMM < 0 {
		problem(w, http.StatusUnprocessableEntity, "invalid gang-sheet dimensions, resolution, margin or gap")
		return
	}
	expectedW := int(math.Ceil(config.SourceWMM / 25.4 * dpi))
	expectedH := int(math.Ceil(config.SourceHMM / 25.4 * dpi))
	if absInt(src.Bounds().Dx()-expectedW) > 2 || absInt(src.Bounds().Dy()-expectedH) > 2 {
		problem(w, http.StatusUnprocessableEntity, "source PNG pixels do not match its physical dimensions and requested DPI; server upscaling is not permitted")
		return
	}
	sheet, placements, err := prod.RenderGangSheet(src, config)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeProductionName(r.URL.Query().Get("name"))+`-gang-sheet.png"`)
	w.Header().Set("X-PrintStudio-Renderer", "server-maxrects")
	w.Header().Set("X-PrintStudio-Placement-Count", strconv.Itoa(len(placements)))
	_ = png.Encode(w, sheet)
}
func productionHalftone(w http.ResponseWriter, r *http.Request) {
	src, ok := decodeProductionImage(w, r)
	if !ok {
		return
	}
	number := func(name string, fallback float64) float64 {
		v, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
		if err != nil {
			return fallback
		}
		return v
	}
	screen := prod.AMHalftone(src, prod.HalftoneConfig{DPI: number("dpi", 300), LPI: number("lpi", 45), AngleDegrees: number("angle", 22.5), Gamma: number("gamma", 1)})
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-PrintStudio-Operation", "am-halftone")
	_ = png.Encode(w, screen)
}
func productionCMYK(w http.ResponseWriter, r *http.Request) {
	src, ok := decodeProductionImage(w, r)
	if !ok {
		return
	}
	bands := prod.SeparateCMYK(src)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="cmyk-separations.zip"`)
	archive := zip.NewWriter(w)
	for _, band := range []struct {
		name  string
		image image.Image
	}{{"cyan.png", bands.Cyan}, {"magenta.png", bands.Magenta}, {"yellow.png", bands.Yellow}, {"black.png", bands.Black}} {
		file, _ := archive.Create(band.name)
		_ = png.Encode(file, band.image)
	}
	manifest, _ := archive.Create("manifest.json")
	_ = json.NewEncoder(manifest).Encode(map[string]any{"schemaVersion": 1, "model": "device-independent-naive-cmyk", "warning": "Use ICC conversion for calibrated production colour."})
	_ = archive.Close()
}
func productionNest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Sheet prod.Sheet  `json:"sheet"`
		Items []prod.Item `json:"items"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	placements, err := prod.Nest(in.Sheet, in.Items)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"placements": placements, "count": len(placements)})
}
func productionBoolean(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Subject   prod.PolygonPaths     `json:"subject"`
		Clip      prod.PolygonPaths     `json:"clip"`
		Operation prod.BooleanOperation `json:"operation"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	paths, err := prod.BooleanPolygons(in.Subject, in.Clip, in.Operation)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if !prod.Clipper2Available() {
			status = http.StatusNotImplemented
		}
		problem(w, status, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"paths": paths})
}
func productionOffset(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths      prod.PolygonPaths `json:"paths"`
		DeltaMM    float64           `json:"deltaMm"`
		Join       prod.OffsetJoin   `json:"join"`
		MiterLimit float64           `json:"miterLimit"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	paths, err := prod.OffsetPolygons(in.Paths, in.DeltaMM, in.Join, in.MiterLimit)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if !prod.Clipper2Available() {
			status = http.StatusNotImplemented
		}
		problem(w, status, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"paths": paths})
}
func decodeProductionImage(w http.ResponseWriter, r *http.Request) (image.Image, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	src, _, err := image.Decode(r.Body)
	if err != nil {
		problem(w, http.StatusBadRequest, "body must be a decodable PNG or JPEG")
		return nil, false
	}
	bounds := src.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || int64(bounds.Dx())*int64(bounds.Dy()) > 100_000_000 {
		problem(w, http.StatusUnprocessableEntity, "image dimensions exceed the 100-megapixel production limit")
		return nil, false
	}
	return src, true
}

func decodeProductionPNG(w http.ResponseWriter, r *http.Request) ([]byte, image.Image, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		problem(w, http.StatusRequestEntityTooLarge, "production PNG exceeds the 50 MB upload limit")
		return nil, nil, false
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		problem(w, http.StatusBadRequest, "body must be a valid PNG")
		return nil, nil, false
	}
	bounds := src.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 || int64(bounds.Dx())*int64(bounds.Dy()) > 100_000_000 {
		problem(w, http.StatusUnprocessableEntity, "image dimensions exceed the 100-megapixel production limit")
		return nil, nil, false
	}
	return data, src, true
}

func numberQuery(r *http.Request, name string, fallback float64) float64 {
	value, err := strconv.ParseFloat(r.URL.Query().Get(name), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	return value
}

func integerQuery(r *http.Request, name string, fallback int) int {
	value, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	return value
}

func productionMetadata(r *http.Request, bounds image.Rectangle) map[string]any {
	return map[string]any{
		"physicalDimensionsMm": map[string]float64{"width": numberQuery(r, "widthMm", 0), "height": numberQuery(r, "heightMm", 0)},
		"pixels":               map[string]int{"width": bounds.Dx(), "height": bounds.Dy()},
		"dpi":                  numberQuery(r, "dpi", 300),
	}
}

func validateRasterMetadata(r *http.Request, bounds image.Rectangle) error {
	widthMM := numberQuery(r, "widthMm", 0)
	heightMM := numberQuery(r, "heightMm", 0)
	dpi := numberQuery(r, "dpi", 300)
	if widthMM <= 0 || heightMM <= 0 || dpi < 72 || dpi > 1200 {
		return fmt.Errorf("positive physical dimensions and DPI between 72 and 1200 are required")
	}
	expectedW := int(math.Ceil(widthMM / 25.4 * dpi))
	expectedH := int(math.Ceil(heightMM / 25.4 * dpi))
	if absInt(bounds.Dx()-expectedW) > 2 || absInt(bounds.Dy()-expectedH) > 2 {
		return fmt.Errorf("PNG pixels do not match the declared physical dimensions and DPI")
	}
	return nil
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func productionMaxPixels() int64 {
	value, err := strconv.ParseInt(env("MAX_RENDER_PIXELS", "100000000"), 10, 64)
	if err != nil || value < 1_000_000 || value > 250_000_000 {
		return 100_000_000
	}
	return value
}

func writeProductionArchive(w http.ResponseWriter, fileName, method string, files []prod.ArtifactFile, metadata map[string]any) {
	manifestFiles := make([]map[string]any, 0, len(files))
	for _, file := range files {
		manifestFiles = append(manifestFiles, map[string]any{"name": file.Name, "role": file.Role, "mime": file.MIME, "size": len(file.Data), "sha256": file.SHA256})
	}
	manifest := map[string]any{"schemaVersion": 2, "createdAt": time.Now().UTC().Format(time.RFC3339), "generator": "PrintStudio Go production engine", "method": method, "metadata": metadata, "files": manifestFiles}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	archive := zip.NewWriter(w)
	for _, file := range files {
		header := &zip.FileHeader{Name: file.Name, Method: zip.Deflate}
		header.SetModTime(time.Now().UTC())
		entry, err := archive.CreateHeader(header)
		if err == nil {
			_, _ = entry.Write(file.Data)
		}
	}
	manifestEntry, _ := archive.Create("manifest.json")
	_ = json.NewEncoder(manifestEntry).Encode(manifest)
	instructions, _ := archive.Create("production-instructions.txt")
	_, _ = instructions.Write([]byte("PrintStudio production package\r\nMethod: " + method + "\r\n\r\nVerify physical size, material profile, colour handling, choke/spread and device/RIP settings before production.\r\n"))
	_ = archive.Close()
}

func safeProductionName(value string) string {
	value = strings.TrimSpace(value)
	var clean strings.Builder
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			clean.WriteRune(char)
		} else if clean.Len() > 0 && !strings.HasSuffix(clean.String(), "-") {
			clean.WriteByte('-')
		}
	}
	result := strings.Trim(clean.String(), "-")
	if result == "" {
		return "printstudio-design"
	}
	return result
}
