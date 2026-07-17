package main

import (
	"encoding/json"
	"errors"
	"regexp"
)

var templateID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

type ProductTemplate struct {
	Version    int               `json:"version"`
	Category   string            `json:"category"`
	Views      []ProductView     `json:"views"`
	Properties []ProductProperty `json:"properties"`
	Colors     []ProductColor    `json:"colors"`
}
type ProductView struct {
	ID               string         `json:"id"`
	Label            string         `json:"label"`
	CanvasWidth      float64        `json:"canvasWidth"`
	CanvasHeight     float64        `json:"canvasHeight"`
	PhysicalWidthMM  float64        `json:"physicalWidthMm"`
	PhysicalHeightMM float64        `json:"physicalHeightMm"`
	SafeMarginMM     float64        `json:"safeMarginMm"`
	BleedMM          float64        `json:"bleedMm"`
	Mockup           map[string]any `json:"mockup"`
}
type ProductProperty struct {
	ID       string          `json:"id"`
	Label    string          `json:"label"`
	Type     string          `json:"type"`
	Required bool            `json:"required"`
	Options  []ProductOption `json:"options"`
}
type ProductOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type ProductColor struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func validateProductTemplate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("product template is required")
	}
	var t ProductTemplate
	if json.Unmarshal(raw, &t) != nil {
		return errors.New("product template must be valid JSON")
	}
	if t.Version != 1 {
		return errors.New("unsupported product template version")
	}
	if len(t.Views) < 1 || len(t.Views) > 20 {
		return errors.New("product template requires 1 to 20 views")
	}
	seen := map[string]bool{}
	for _, v := range t.Views {
		if !templateID.MatchString(v.ID) || v.Label == "" || seen[v.ID] {
			return errors.New("every view needs a unique valid id and label")
		}
		seen[v.ID] = true
		if v.CanvasWidth < 50 || v.CanvasHeight < 50 || v.CanvasWidth > 4000 || v.CanvasHeight > 4000 {
			return errors.New("view canvas dimensions must be between 50 and 4000")
		}
		if v.PhysicalWidthMM <= 0 || v.PhysicalHeightMM <= 0 || v.PhysicalWidthMM > 3000 || v.PhysicalHeightMM > 3000 {
			return errors.New("view physical dimensions are invalid")
		}
		if v.SafeMarginMM < 0 || v.BleedMM < 0 {
			return errors.New("view margins cannot be negative")
		}
	}
	seen = map[string]bool{}
	for _, p := range t.Properties {
		if !templateID.MatchString(p.ID) || p.Label == "" || seen[p.ID] {
			return errors.New("every property needs a unique valid id and label")
		}
		seen[p.ID] = true
		if p.Type != "select" && p.Type != "text" && p.Type != "number" && p.Type != "boolean" {
			return errors.New("unsupported product property type")
		}
		if p.Type == "select" && len(p.Options) == 0 {
			return errors.New("select properties require options")
		}
	}
	return nil
}
