package main

import (
	"archive/zip"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"os"
	"strconv"

	_ "image/jpeg"

	prod "printstudio/api/production"
)

func productionCapabilities(w http.ResponseWriter, _ *http.Request) {
	tools := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN")}
	write(w, http.StatusOK, tools.Probe())
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
