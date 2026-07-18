package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const embroideryFixture = `{"name":"API TEST","fabricClass":"tshirt","regions":[{"id":"panel","threadId":"black","geometry":{"rings":[[{"x":-10,"y":-5},{"x":10,"y":-5},{"x":10,"y":5},{"x":-10,"y":5}]]},"kind":"tatami","spacingMm":0.4,"stitchLengthMm":3,"edgeUnderlay":true}],"machine":{}}`

func TestCompileEmbroideryHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/embroidery/compile", strings.NewReader(embroideryFixture))
	res := httptest.NewRecorder()
	compileEmbroidery(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status %d: %s", res.Code, res.Body.String())
	}
	body := res.Body.String()
	if !strings.Contains(body, `"compilerVersion":"0.3.0"`) || !strings.Contains(body, `"svg":"`) || !strings.Contains(body, `"review"`) || !strings.Contains(body, `"tshirt"`) {
		t.Fatalf("missing compiler response: %s", body)
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
	if res.Header().Get("X-Embroidery-Fabric") != "tshirt" {
		t.Fatalf("fabric header %q", res.Header().Get("X-Embroidery-Fabric"))
	}
}
