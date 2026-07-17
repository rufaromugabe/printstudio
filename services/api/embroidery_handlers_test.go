package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const embroideryFixture = `{"name":"API TEST","regions":[{"id":"panel","threadId":"black","geometry":{"rings":[[{"x":-10,"y":-5},{"x":10,"y":-5},{"x":10,"y":5},{"x":-10,"y":5}]]},"kind":"tatami","spacingMm":1,"stitchLengthMm":3,"edgeUnderlay":true}],"machine":{}}`

func TestCompileEmbroideryHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/embroidery/compile", strings.NewReader(embroideryFixture))
	res := httptest.NewRecorder()
	compileEmbroidery(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"compilerVersion":"0.2.0"`) || !strings.Contains(res.Body.String(), `"svg":"`) {
		t.Fatalf("missing compiler response: %s", res.Body.String())
	}
}

func TestExportEmbroideryHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/embroidery/export/dst", strings.NewReader(embroideryFixture))
	res := httptest.NewRecorder()
	exportEmbroidery(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	data := res.Body.Bytes()
	if len(data) <= 515 || data[511] != 0x1a || !bytes.HasPrefix(data, []byte("LA:")) {
		t.Fatal("handler returned an invalid DST envelope")
	}
	if res.Header().Get("X-Embroidery-Source-Hash") == "" {
		t.Fatal("missing reproducibility hash")
	}
}
