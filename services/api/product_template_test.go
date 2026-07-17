package main

import (
	"encoding/json"
	"testing"
)

func TestValidateProductTemplate(t *testing.T) {
	valid := json.RawMessage(`{"version":1,"category":"apparel","views":[{"id":"left_sleeve","label":"Left sleeve","canvasWidth":240,"canvasHeight":300,"physicalWidthMm":100,"physicalHeightMm":120,"safeMarginMm":5,"bleedMm":2,"mockup":{"kind":"sleeve"}}],"properties":[{"id":"size","label":"Size","type":"select","required":true,"options":[{"value":"m","label":"Medium"}]}],"colors":[{"value":"#000000","label":"Black"}]}`)
	if err := validateProductTemplate(valid); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}
}
func TestRejectDuplicateViews(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"views":[{"id":"front","label":"Front","canvasWidth":200,"canvasHeight":200,"physicalWidthMm":100,"physicalHeightMm":100},{"id":"front","label":"Again","canvasWidth":200,"canvasHeight":200,"physicalWidthMm":100,"physicalHeightMm":100}]}`)
	if validateProductTemplate(raw) == nil {
		t.Fatal("duplicate view accepted")
	}
}
func TestRejectUnboundedCanvas(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"views":[{"id":"front","label":"Front","canvasWidth":99999,"canvasHeight":200,"physicalWidthMm":100,"physicalHeightMm":100}]}`)
	if validateProductTemplate(raw) == nil {
		t.Fatal("unsafe canvas accepted")
	}
}
