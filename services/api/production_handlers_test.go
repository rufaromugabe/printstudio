package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
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
