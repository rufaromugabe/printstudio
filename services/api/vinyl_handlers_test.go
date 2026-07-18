package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const vinylReviewFixture = `{"materialClass":"htv-smooth","mirrored":true,"paths":[[{"x":0,"y":0},{"x":20,"y":0},{"x":20,"y":20},{"x":0,"y":20}]]}`

func TestReviewVinylHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/vinyl/review", strings.NewReader(vinylReviewFixture))
	res := httptest.NewRecorder()
	reviewVinyl(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"htv-smooth"`) || !strings.Contains(body, `"review"`) || !strings.Contains(body, `"mirrorRecommended":true`) {
		t.Fatalf("missing vinyl review response: %s", body)
	}
}

func TestReviewVinylHandlerRejectsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/vinyl/review", strings.NewReader(`{"materialClass":"htv-smooth","paths":[]}`))
	res := httptest.NewRecorder()
	reviewVinyl(res, req)
	if res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
}

func TestReviewVinylHandlerHardStop(t *testing.T) {
	body := `{"materialClass":"htv-glitter","mirrored":true,"paths":[[{"x":0,"y":0},{"x":0.4,"y":0},{"x":0.4,"y":0.4},{"x":0,"y":0.4}]]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/vinyl/review", strings.NewReader(body))
	res := httptest.NewRecorder()
	reviewVinyl(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	out := res.Body.String()
	if !strings.Contains(out, `"FEATURE_TOO_SMALL"`) || !strings.Contains(out, `"blocked"`) {
		t.Fatalf("expected hard-stop review, got %s", out)
	}
}
