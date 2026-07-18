package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
)

func productionPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 7, 7))
	img.SetNRGBA(3, 3, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}
func TestProductionUnderbaseHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/production/dtf/underbase?spread=1", bytes.NewReader(productionPNG(t)))
	res := httptest.NewRecorder()
	productionUnderbase(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	mask, err := png.Decode(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, a := mask.At(2, 3).RGBA()
	if a == 0 {
		t.Fatal("spread underbase omitted adjacent pixel")
	}
}
func TestProductionHalftoneHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/production/screen/halftone?dpi=300&lpi=45&angle=22.5", bytes.NewReader(productionPNG(t)))
	res := httptest.NewRecorder()
	productionHalftone(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d", res.Code)
	}
	if _, err := png.Decode(res.Body); err != nil {
		t.Fatal(err)
	}
}
func TestProductionNestHandler(t *testing.T) {
	body := `{"sheet":{"widthMm":100,"heightMm":100,"marginMm":2,"gapMm":2},"items":[{"id":"logo","widthMm":20,"heightMm":10,"quantity":4,"allowRotate":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/production/gang/nest", strings.NewReader(body))
	res := httptest.NewRecorder()
	productionNest(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"count":4`) {
		t.Fatal("unexpected nesting response")
	}
}

func TestProductionCapabilitiesReportsClipperState(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/production/capabilities", nil)
	res := httptest.NewRecorder()
	productionCapabilities(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"polygonBoolean":false`) {
		t.Fatalf("expected the default build to report Clipper2 unavailable: %s", res.Body.String())
	}
}

func TestProductionBooleanRequiresNativeClipperBuild(t *testing.T) {
	body := `{"subject":[[{"x":0,"y":0},{"x":10,"y":0},{"x":10,"y":10}]],"clip":[],"operation":"union"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/production/vector/boolean", strings.NewReader(body))
	res := httptest.NewRecorder()
	productionBoolean(res, req)
	if res.Code != http.StatusNotImplemented {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "Clipper2 native backend is unavailable") {
		t.Fatalf("unexpected error response: %s", res.Body.String())
	}
}

func TestProductionOffsetRequiresNativeClipperBuild(t *testing.T) {
	body := `{"paths":[[{"x":0,"y":0},{"x":10,"y":0},{"x":10,"y":10}]],"deltaMm":0.5,"join":"round"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/production/vector/offset", strings.NewReader(body))
	res := httptest.NewRecorder()
	productionOffset(res, req)
	if res.Code != http.StatusNotImplemented {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
}

func TestProductionDTFPackHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/production/dtf/pack?name=Shop+Logo&widthMm=0.5927&heightMm=0.5927&spread=1", bytes.NewReader(productionPNG(t)))
	res := httptest.NewRecorder()
	productionDTFPack(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, file := range archive.File {
		found[file.Name] = true
		if file.Name == "manifest.json" {
			reader, openErr := file.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			var manifest struct {
				SchemaVersion int `json:"schemaVersion"`
				Files         []struct {
					SHA256 string `json:"sha256"`
				} `json:"files"`
			}
			if json.NewDecoder(reader).Decode(&manifest) != nil || manifest.SchemaVersion != 2 || len(manifest.Files) != 2 || len(manifest.Files[0].SHA256) != 64 {
				t.Fatal("invalid DTF manifest")
			}
			_ = reader.Close()
		}
	}
	for _, name := range []string{"color.png", "white-underbase.png", "manifest.json", "production-instructions.txt"} {
		if !found[name] {
			t.Fatalf("DTF archive omitted %s", name)
		}
	}
}

func TestProductionScreenPackHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/production/screen/pack?name=Logo&widthMm=0.5927&heightMm=0.5927&lpi=45", bytes.NewReader(productionPNG(t)))
	res := httptest.NewRecorder()
	productionScreenPack(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive.File) != 11 {
		t.Fatalf("expected nine production plates plus manifest and instructions, got %d entries", len(archive.File))
	}
}

func TestProductionGangRenderHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/production/gang/render?name=Logo&sourceWidthMm=1.778&sourceHeightMm=1.778&sheetWidthMm=20&sheetHeightMm=20&copies=2&dpi=100&gapMm=1", bytes.NewReader(productionPNG(t)))
	res := httptest.NewRecorder()
	productionGangRender(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	img, err := png.Decode(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 79 || img.Bounds().Dy() != 79 || res.Header().Get("X-PrintStudio-Placement-Count") != "2" {
		t.Fatal("unexpected server gang-sheet output")
	}
}

func vectorizeFixturePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 8; y < 40; y++ {
		for x := 8; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestProductionVectorizeHandlerPNG(t *testing.T) {
	if _, err := exec.LookPath("potrace"); err != nil {
		t.Skip("potrace not installed")
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/production/vectorize?method=vinyl&mlPrep=false", bytes.NewReader(vectorizeFixturePNG(t)))
	req.Header.Set("Content-Type", "image/png")
	res := httptest.NewRecorder()
	productionVectorizeHandler(nil)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	var out struct {
		SourceHash string `json:"sourceHash"`
		PathCount  int    `json:"pathCount"`
		Tracer     string `json:"tracer"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.SourceHash == "" || out.PathCount < 1 || out.Tracer == "" {
		t.Fatalf("unexpected vectorize response: %+v", out)
	}
}

func TestProductionVectorizeHandlerJSONBase64(t *testing.T) {
	if _, err := exec.LookPath("potrace"); err != nil {
		t.Skip("potrace not installed")
	}
	payload := `{"method":"embroidery","mlPrep":false,"imageBase64":"` + base64.StdEncoding.EncodeToString(vectorizeFixturePNG(t)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/production/vectorize", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	productionVectorizeHandler(nil)(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
}
