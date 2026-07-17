package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorsPreflight(t *testing.T) {
	h := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	r := httptest.NewRequest(http.MethodOptions, "/v1/designs", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("missing CORS header")
	}
}

func TestValidateAssetRequest(t *testing.T) {
	tests := []struct {
		name string
		in   AssetRequest
		ok   bool
	}{{"png", AssetRequest{"art.png", "image/png", 1024, strings.Repeat("a", 64)}, true}, {"mismatch", AssetRequest{"art.jpg", "image/png", 1024, strings.Repeat("a", 64)}, false}, {"svg rejected", AssetRequest{"art.svg", "image/svg+xml", 1024, strings.Repeat("a", 64)}, false}, {"too large", AssetRequest{"art.png", "image/png", maxAssetBytes + 1, strings.Repeat("a", 64)}, false}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAssetRequest(tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("valid=%v error=%v", tt.ok, err)
			}
		})
	}
}
func TestFileSignatures(t *testing.T) {
	png := append([]byte{137, 80, 78, 71, 13, 10, 26, 10}, make([]byte, 4)...)
	if got := httpDetect(png); got != "image/png" {
		t.Fatalf("got %s", got)
	}
	if got := httpDetect([]byte("<svg></svg>")); !strings.Contains(got, "octet") {
		t.Fatalf("unexpected %s", got)
	}
}
func TestCleanFileName(t *testing.T) {
	got := cleanFileName("../../my unsafe art.PNG")
	if strings.Contains(got, "/") || strings.Contains(got, " ") {
		t.Fatalf("unsafe name %q", got)
	}
}
