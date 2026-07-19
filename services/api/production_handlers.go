package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/jpeg"

	"printstudio/api/ai"
	prod "printstudio/api/production"
)

func productionCapabilities(w http.ResponseWriter, _ *http.Request) {
	tools := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN"), Tesseract: os.Getenv("TESSERACT_BIN")}
	capabilities := tools.Probe()
	capabilities.MaxRenderPixels = productionMaxPixels()
	capabilities.ICCProfiles = iccProfiles != nil
	capabilities.RequireICC = requireICCByPolicy()
	capabilities.RequireApproval = strings.EqualFold(env("REQUIRE_PRODUCTION_APPROVAL", "false"), "true")
	capabilities.ProductionReady = capabilities.ProductionReady && capabilities.ICCProfiles
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

func productionDTFPackHandler(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, src, ok := decodeProductionPNG(w, r)
		if !ok {
			return
		}
		digest := sha256Hex(data)
		if err := requireApprovedProof(a, r, r.URL.Query().Get("proofId"), digest); err != nil {
			prod.DefaultMetrics.Failures.Add(1)
			problem(w, http.StatusConflict, err.Error())
			return
		}
		r = r.WithContext(r.Context())
		packDTF(w, r, data, src)
	}
}

func productionScreenPackHandler(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, src, ok := decodeProductionPNG(w, r)
		if !ok {
			return
		}
		digest := sha256Hex(data)
		if err := requireApprovedProof(a, r, r.URL.Query().Get("proofId"), digest); err != nil {
			prod.DefaultMetrics.Failures.Add(1)
			problem(w, http.StatusConflict, err.Error())
			return
		}
		if requireICCByPolicy() && !requireTruthy(r, "allowUncalibrated") {
			if r.URL.Query().Get("sourceProfile") == "" || r.URL.Query().Get("destinationProfile") == "" {
				prod.DefaultMetrics.Failures.Add(1)
				problem(w, http.StatusUnprocessableEntity, "ICC profiles are required for production screen packs (sourceProfile + destinationProfile), or pass allowUncalibrated=true for preview-only")
				return
			}
		}
		packScreen(w, r, data, src)
	}
}

func productionGangRenderHandler(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		productionGangRender(w, r)
		if w.Header().Get("Content-Type") == "image/png" {
			prod.DefaultMetrics.GangRenders.Add(1)
		}
	}
}

func productionBooleanHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		productionBoolean(w, r)
		prod.DefaultMetrics.VectorOps.Add(1)
	}
}

func productionOffsetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		productionOffset(w, r)
		prod.DefaultMetrics.VectorOps.Add(1)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func productionDTFPack(w http.ResponseWriter, r *http.Request) {
	data, src, ok := decodeProductionPNG(w, r)
	if !ok {
		return
	}
	packDTF(w, r, data, src)
}

func productionSublimationPackHandler(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, src, ok := decodeProductionPNG(w, r)
		if !ok {
			return
		}
		digest := sha256Hex(data)
		if err := requireApprovedProof(a, r, r.URL.Query().Get("proofId"), digest); err != nil {
			prod.DefaultMetrics.Failures.Add(1)
			problem(w, http.StatusConflict, err.Error())
			return
		}
		packSublimation(w, r, data, src)
	}
}

func packSublimation(w http.ResponseWriter, r *http.Request, data []byte, src image.Image) {
	if err := validateRasterMetadata(r, src.Bounds()); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	presetID := strings.TrimSpace(r.URL.Query().Get("trapPreset"))
	if presetID == "" {
		presetID = "sublimation-paper-standard"
	}
	if preset, err := prod.LookupTrapPreset(presetID); err != nil || preset.Method != "Sublimation" {
		problem(w, http.StatusUnprocessableEntity, "trap preset must be a sublimation recipe")
		return
	}
	files, err := prod.BuildSublimationFiles(data)
	if err != nil {
		problem(w, http.StatusInternalServerError, "sublimation package generation failed")
		return
	}
	metadata := productionMetadata(r, src.Bounds())
	metadata["sublimation"] = map[string]any{"trapPreset": presetID, "bleedIncluded": true, "underbase": false}
	metadata["warning"] = "Confirm paper ICC, mirror setting and press temperature with your sublimation workflow."
	prod.DefaultMetrics.Packs.Add(1)
	writeProductionArchive(w, safeProductionName(r.URL.Query().Get("name"))+"-sublimation-package.zip", "Sublimation", files, metadata)
}

func packDTF(w http.ResponseWriter, r *http.Request, data []byte, src image.Image) {
	spread := integerQuery(r, "spread", 2)
	threshold := integerQuery(r, "threshold", 1)
	if presetID := strings.TrimSpace(r.URL.Query().Get("trapPreset")); presetID != "" {
		preset, err := prod.LookupTrapPreset(presetID)
		if err != nil {
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if preset.Method != "DTF" {
			problem(w, http.StatusUnprocessableEntity, "trap preset is not a DTF recipe")
			return
		}
		spread = preset.SpreadPixels
		threshold = int(preset.Threshold)
	}
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
	metadata["underbase"] = map[string]any{"algorithm": "exact-euclidean-distance-transform", "spreadPixels": spread, "threshold": threshold, "trapPreset": r.URL.Query().Get("trapPreset")}
	metadata["warning"] = "Final ink limits, white density and printer-specific RIP settings must be verified by the operator."
	prod.DefaultMetrics.Packs.Add(1)
	writeProductionArchive(w, safeProductionName(r.URL.Query().Get("name"))+"-dtf-package.zip", "DTF", files, metadata)
}

func productionScreenPack(w http.ResponseWriter, r *http.Request) {
	data, src, ok := decodeProductionPNG(w, r)
	if !ok {
		return
	}
	packScreen(w, r, data, src)
}

func packScreen(w http.ResponseWriter, r *http.Request, data []byte, src image.Image) {
	dpi := numberQuery(r, "dpi", 300)
	lpi := numberQuery(r, "lpi", 45)
	gamma := numberQuery(r, "gamma", 1)
	choke := integerQuery(r, "underbaseChoke", -2)
	screening := prod.ScreeningMode(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("screening"))))
	if screening == "" {
		screening = prod.ScreeningAM
	}
	angles := prod.DefaultScreenAngles()
	if presetID := strings.TrimSpace(r.URL.Query().Get("trapPreset")); presetID != "" {
		preset, err := prod.LookupTrapPreset(presetID)
		if err != nil {
			problem(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if preset.Method != "Screen print" {
			problem(w, http.StatusUnprocessableEntity, "trap preset is not a screen-print recipe")
			return
		}
		choke = preset.UnderbaseChokePX
	}
	if dpi < 72 || dpi > 1200 || lpi < 10 || lpi > 200 || lpi > dpi/2 || gamma < 0.1 || gamma > 5 || choke < -100 || choke > 100 {
		problem(w, http.StatusUnprocessableEntity, "invalid screen-pack DPI, LPI, gamma or underbase choke")
		return
	}
	if err := validateRasterMetadata(r, src.Bounds()); err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	calibratedPNG := data
	var iccMeta map[string]any
	if r.URL.Query().Get("sourceProfile") != "" || r.URL.Query().Get("destinationProfile") != "" || requireTruthy(r, "requireIcc") {
		transformed, meta, err := applyProductionICC(r.Context(), data, r.URL.Query().Get("sourceProfile"), r.URL.Query().Get("destinationProfile"), r.URL.Query().Get("intent"))
		if err != nil {
			status := http.StatusUnprocessableEntity
			if strings.Contains(err.Error(), "unavailable") {
				status = http.StatusNotImplemented
			}
			problem(w, status, err.Error())
			return
		}
		calibratedPNG = transformed
		decoded, err := png.Decode(bytes.NewReader(calibratedPNG))
		if err != nil {
			problem(w, http.StatusInternalServerError, "ICC-transformed PNG could not be decoded")
			return
		}
		src = decoded
		iccMeta = meta
		prod.DefaultMetrics.ICCTransforms.Add(1)
	}
	files, err := prod.BuildScreenFiles(src, prod.ScreenPackConfig{DPI: dpi, LPI: lpi, Gamma: gamma, UnderbaseChokePX: choke, Screening: screening, Angles: angles, TrapPresetID: r.URL.Query().Get("trapPreset")})
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if iccMeta != nil {
		files = append([]prod.ArtifactFile{prod.NewArtifact("color-icc.png", "icc-calibrated-artwork", "image/png", calibratedPNG)}, files...)
	}
	metadata := productionMetadata(r, src.Bounds())
	model := "uncalibrated-cmyk-gcr"
	warning := "These process separations are not ICC calibrated. Confirm mesh, dot gain, ink set, substrate and screen-angle conflicts before exposing screens."
	if iccMeta != nil {
		model = "icc-calibrated-cmyk-gcr"
		warning = "Artwork was ICC-transformed before separation. Still verify mesh, dot gain and press conditions."
		metadata["icc"] = iccMeta
	}
	metadata["screen"] = map[string]any{"model": model, "dpi": dpi, "lpi": lpi, "gamma": gamma, "screening": screening, "anglesDegrees": angles, "underbaseChokePixels": choke, "trapPreset": r.URL.Query().Get("trapPreset"), "angleConflicts": prod.DetectScreenAngleConflicts(angles)}
	metadata["warning"] = warning
	prod.DefaultMetrics.Packs.Add(1)
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
		FillSheet:   requireTruthy(r, "fillSheet"),
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
	w.Header().Set("X-PrintStudio-Sheet-Width-Mm", strconv.FormatFloat(config.Sheet.WidthMM, 'f', -1, 64))
	w.Header().Set("X-PrintStudio-Sheet-Height-Mm", strconv.FormatFloat(config.Sheet.HeightMM, 'f', -1, 64))
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
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	var screen *image.Gray
	if mode == "fm" {
		screen = prod.FMHalftone(src, number("gamma", 1))
		w.Header().Set("X-PrintStudio-Operation", "fm-halftone")
	} else {
		screen = prod.AMHalftone(src, prod.HalftoneConfig{DPI: number("dpi", 300), LPI: number("lpi", 45), AngleDegrees: number("angle", 22.5), Gamma: number("gamma", 1)})
		w.Header().Set("X-PrintStudio-Operation", "am-halftone")
	}
	w.Header().Set("Content-Type", "image/png")
	_ = png.Encode(w, screen)
}
func productionCMYK(w http.ResponseWriter, r *http.Request) {
	src, ok := decodeProductionImage(w, r)
	if !ok {
		return
	}
	if requireTruthy(r, "requireIcc") {
		problem(w, http.StatusUnprocessableEntity, "raw CMYK endpoint is intentionally uncalibrated; use /v1/production/screen/pack?requireIcc=true with ICC profiles instead")
		return
	}
	bands := prod.SeparateCMYK(src)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="cmyk-separations.zip"`)
	w.Header().Set("X-PrintStudio-Colour-Model", "device-independent-naive-cmyk")
	archive := zip.NewWriter(w)
	for _, band := range []struct {
		name  string
		image image.Image
	}{{"cyan.png", bands.Cyan}, {"magenta.png", bands.Magenta}, {"yellow.png", bands.Yellow}, {"black.png", bands.Black}} {
		file, _ := archive.Create(band.name)
		_ = png.Encode(file, band.image)
	}
	manifest, _ := archive.Create("manifest.json")
	_ = json.NewEncoder(manifest).Encode(map[string]any{"schemaVersion": 1, "model": "device-independent-naive-cmyk", "warning": "Uncalibrated preview only. Use ICC-calibrated screen pack for production colour."})
	_ = archive.Close()
}
func productionSpotMatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Colors    []string `json:"colors"`
		MaxDeltaE float64  `json:"maxDeltaE"`
	}
	if decode(w, r, &in) != nil {
		return
	}
	if len(in.Colors) == 0 {
		problem(w, http.StatusUnprocessableEntity, "colors array is required")
		return
	}
	matches, err := prod.MatchSpots(in.Colors, in.MaxDeltaE)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"matches": matches, "library": "printstudio-default-named-inks"})
}
func productionAngleCheck(w http.ResponseWriter, r *http.Request) {
	var in prod.ScreenAngleSet
	if decode(w, r, &in) != nil {
		return
	}
	conflicts := prod.DetectScreenAngleConflicts(in)
	write(w, http.StatusOK, map[string]any{"angles": in, "conflicts": conflicts, "ok": len(conflicts) == 0 || !hasErrorConflict(conflicts)})
}
func listICCProfiles(w http.ResponseWriter, _ *http.Request) {
	if iccProfiles == nil {
		problem(w, http.StatusNotImplemented, "ICC profile store is not configured; set ICC_PROFILE_DIR")
		return
	}
	items, err := iccProfiles.List()
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not list ICC profiles")
		return
	}
	write(w, http.StatusOK, map[string]any{"profiles": items})
}
func uploadICCProfile(w http.ResponseWriter, r *http.Request) {
	if iccProfiles == nil {
		problem(w, http.StatusNotImplemented, "ICC profile store is not configured; set ICC_PROFILE_DIR")
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	label := strings.TrimSpace(r.URL.Query().Get("label"))
	description := strings.TrimSpace(r.URL.Query().Get("description"))
	if id == "" {
		problem(w, http.StatusUnprocessableEntity, "id query parameter is required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		problem(w, http.StatusRequestEntityTooLarge, "ICC profile exceeds 8 MB")
		return
	}
	meta, err := iccProfiles.Put(id, label, description, data)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "only common bundled") {
			status = http.StatusForbidden
		}
		problem(w, status, err.Error())
		return
	}
	write(w, http.StatusOK, meta)
}
func applyICCTransform(w http.ResponseWriter, r *http.Request) {
	data, _, ok := decodeProductionPNG(w, r)
	if !ok {
		return
	}
	out, meta, err := applyProductionICC(r.Context(), data, r.URL.Query().Get("sourceProfile"), r.URL.Query().Get("destinationProfile"), r.URL.Query().Get("intent"))
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "unavailable") {
			status = http.StatusNotImplemented
		}
		problem(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-PrintStudio-Operation", "icc-transform")
	if raw, err := json.Marshal(meta); err == nil {
		w.Header().Set("X-PrintStudio-ICC", string(raw))
	}
	_, _ = w.Write(out)
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

func productionVectorizeHandler(a *API) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tools := prod.NativeTools{Vips: os.Getenv("VIPS_BIN"), Potrace: os.Getenv("POTRACE_BIN"), Tesseract: os.Getenv("TESSERACT_BIN")}
		if !tools.Probe().VectorTrace {
			problem(w, http.StatusNotImplemented, "potrace is unavailable — advanced vectorize requires POTRACE_BIN or potrace on PATH")
			return
		}
		img, placement, method, mlPrep, alpha, ok := decodeVectorizeRequest(w, r, a)
		if !ok {
			return
		}
		opt := prod.VectorizeOptions{
			Method:       method,
			AlphaCutoff:  alpha,
			MLPrep:       mlPrep,
			Placement:    placement,
			Tools:        tools,
			IncludeProof: strings.EqualFold(r.URL.Query().Get("proof"), "true") || r.URL.Query().Get("proof") == "1",
			EnableOCR:    !strings.EqualFold(r.URL.Query().Get("ocr"), "false") && r.URL.Query().Get("ocr") != "0",
		}
		if mlPrep {
			opt.Prep = ai.NewGatewayFromEnv()
		}
		if strings.EqualFold(r.URL.Query().Get("mode"), "color") || strings.EqualFold(r.URL.Query().Get("mode"), "colour") {
			maxColors := integerQuery(r, "maxColors", 8)
			result, err := prod.VectorizeColor(r.Context(), img, opt, maxColors)
			if err != nil {
				if result != nil {
					write(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error(), "separations": result})
					return
				}
				problem(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			write(w, http.StatusOK, result)
			return
		}
		set, err := prod.Vectorize(r.Context(), img, opt)
		if err != nil {
			status := http.StatusUnprocessableEntity
			if strings.Contains(err.Error(), "unavailable") {
				status = http.StatusNotImplemented
			}
			if set != nil {
				write(w, status, map[string]any{"error": err.Error(), "contours": set})
				return
			}
			problem(w, status, err.Error())
			return
		}
		write(w, http.StatusOK, set)
	}
}

func decodeVectorizeRequest(w http.ResponseWriter, r *http.Request, a *API) (image.Image, *prod.VectorizePlacement, string, bool, uint8, bool) {
	method := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("method")))
	if method == "" {
		method = "vinyl"
	}
	mlPrep := strings.EqualFold(r.URL.Query().Get("mlPrep"), "true") || r.URL.Query().Get("mlPrep") == "1"
	alpha := prod.DefaultAlphaCutoff
	if raw := r.URL.Query().Get("alphaCutoff"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n < 256 {
			alpha = uint8(n)
		}
	}
	var placement *prod.VectorizePlacement
	if raw := r.Header.Get("X-PrintStudio-Placement"); raw != "" {
		var p prod.VectorizePlacement
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			problem(w, http.StatusBadRequest, "invalid X-PrintStudio-Placement JSON")
			return nil, nil, "", false, 0, false
		}
		placement = &p
	}

	ct := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.Contains(ct, "application/json") {
		var in struct {
			AssetID     string                   `json:"assetId"`
			ImageBase64 string                   `json:"imageBase64"`
			Method      string                   `json:"method"`
			MLPrep      bool                     `json:"mlPrep"`
			AlphaCutoff uint8                    `json:"alphaCutoff"`
			Placement   *prod.VectorizePlacement `json:"placement"`
		}
		if decode(w, r, &in) != nil {
			return nil, nil, "", false, 0, false
		}
		if in.Method != "" {
			method = strings.ToLower(strings.TrimSpace(in.Method))
		}
		mlPrep = mlPrep || in.MLPrep
		if in.AlphaCutoff > 0 {
			alpha = in.AlphaCutoff
		}
		if in.Placement != nil {
			placement = in.Placement
		}
		if in.AssetID != "" {
			id := identity(r)
			fetch := a.assetFetcher(r, id.WorkspaceID)
			body, err := fetch(in.AssetID)
			if err != nil {
				problem(w, http.StatusUnprocessableEntity, err.Error())
				return nil, nil, "", false, 0, false
			}
			defer body.Close()
			img, _, err := image.Decode(body)
			if err != nil {
				problem(w, http.StatusBadRequest, "asset is not a decodable image")
				return nil, nil, "", false, 0, false
			}
			return img, placement, method, mlPrep, alpha, true
		}
		if in.ImageBase64 == "" {
			problem(w, http.StatusBadRequest, "imageBase64 or assetId is required")
			return nil, nil, "", false, 0, false
		}
		raw := in.ImageBase64
		if i := strings.Index(raw, ","); i >= 0 && strings.Contains(raw[:i], "base64") {
			raw = raw[i+1:]
		}
		data, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			problem(w, http.StatusBadRequest, "imageBase64 is invalid")
			return nil, nil, "", false, 0, false
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			problem(w, http.StatusBadRequest, "imageBase64 must decode to PNG or JPEG")
			return nil, nil, "", false, 0, false
		}
		return img, placement, method, mlPrep, alpha, true
	}

	img, ok := decodeProductionImage(w, r)
	if !ok {
		return nil, nil, "", false, 0, false
	}
	return img, placement, method, mlPrep, alpha, true
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

var iccProfiles *prod.ICCProfileStore

func requireTruthy(r *http.Request, name string) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(name)))
	return value == "1" || value == "true" || value == "yes"
}

func hasErrorConflict(conflicts []prod.AngleConflict) bool {
	for _, conflict := range conflicts {
		if conflict.Severity == "error" {
			return true
		}
	}
	return false
}

func applyProductionICC(ctx context.Context, pngData []byte, sourceID, destinationID, intent string) ([]byte, map[string]any, error) {
	if iccProfiles == nil {
		return nil, nil, fmt.Errorf("ICC profile store is unavailable; set ICC_PROFILE_DIR")
	}
	tools := prod.NativeTools{Vips: os.Getenv("VIPS_BIN")}
	if !tools.Probe().ICC {
		return nil, nil, fmt.Errorf("libvips ICC transform is unavailable")
	}
	sourceID = strings.TrimSpace(sourceID)
	destinationID = strings.TrimSpace(destinationID)
	if sourceID == "" || destinationID == "" {
		return nil, nil, fmt.Errorf("sourceProfile and destinationProfile query parameters are required for ICC conversion")
	}
	if _, err := prod.LookupCommonICC(sourceID); err != nil {
		return nil, nil, err
	}
	if _, err := prod.LookupCommonICC(destinationID); err != nil {
		return nil, nil, err
	}
	sourceMeta, sourcePath, err := iccProfiles.Get(sourceID)
	if err != nil {
		return nil, nil, err
	}
	destMeta, destPath, err := iccProfiles.Get(destinationID)
	if err != nil {
		return nil, nil, err
	}
	tmp, err := os.MkdirTemp("", "printstudio-icc-*")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(tmp)
	input := filepath.Join(tmp, "input.png")
	output := filepath.Join(tmp, "output.png")
	if err := os.WriteFile(input, pngData, 0o600); err != nil {
		return nil, nil, err
	}
	if err := tools.ICCTransform(ctx, input, output, sourcePath, destPath, intent); err != nil {
		return nil, nil, err
	}
	out, err := os.ReadFile(output)
	if err != nil {
		return nil, nil, err
	}
	return out, map[string]any{
		"sourceProfile":      sourceMeta,
		"destinationProfile": destMeta,
		"intent":             intentOrDefault(intent),
		"engine":             "libvips+littlecms",
	}, nil
}

func intentOrDefault(intent string) string {
	if strings.TrimSpace(intent) == "" {
		return "relative"
	}
	return intent
}
