package main

import (
	"net/http"
	"strings"

	"printstudio/api/embroidery"
)

type embroideryRequest struct {
	Name          string                    `json:"name"`
	FabricClass   string                    `json:"fabricClass"`
	Regions       []embroidery.Region       `json:"regions"`
	Machine       embroidery.MachineProfile `json:"machine"`
	PrintWidthMm  float64                   `json:"printWidthMm"`
	PrintHeightMm float64                   `json:"printHeightMm"`
}

func compileEmbroidery(w http.ResponseWriter, r *http.Request) {
	var in embroideryRequest
	if decode(w, r, &in) != nil {
		return
	}
	document, err := embroidery.CompileWithFabric(in.Regions, in.Machine, embroidery.NormalizeFabric(in.FabricClass))
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{
		"document": document,
		"svg":      embroidery.DiagnosticSVG(document, in.PrintWidthMm, in.PrintHeightMm),
	})
}

func exportEmbroidery(w http.ResponseWriter, r *http.Request) {
	var in embroideryRequest
	if decode(w, r, &in) != nil {
		return
	}
	document, err := embroidery.CompileWithFabric(in.Regions, in.Machine, embroidery.NormalizeFabric(in.FabricClass))
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if embroidery.HasErrors(document.Diagnostics) {
		problem(w, http.StatusUnprocessableEntity, "embroidery failed machine or fabric policy checks; resolve diagnostics before DST export")
		return
	}
	data, err := embroidery.EncodeDST(document, in.Name)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "printstudio-design"
	}
	name = strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == ' ' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			return r
		}
		return -1
	}, name)
	w.Header().Set("Content-Type", "application/x-dst")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.dst"`)
	w.Header().Set("X-Embroidery-Source-Hash", document.SourceHash)
	w.Header().Set("X-Embroidery-Fabric", string(document.Fabric.Class))
	w.Header().Set("X-Embroidery-Review", string(document.Review.Decision))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
