package main

import (
	"net/http"

	"printstudio/api/vinyl"
)

type vinylReviewRequest struct {
	MaterialClass string          `json:"materialClass"`
	Paths         [][]vinyl.Point `json:"paths"`
	Mirrored      bool            `json:"mirrored"`
}

func reviewVinyl(w http.ResponseWriter, r *http.Request) {
	var in vinylReviewRequest
	if decode(w, r, &in) != nil {
		return
	}
	if len(in.Paths) == 0 {
		problem(w, http.StatusUnprocessableEntity, "vinyl review requires at least one cut path")
		return
	}
	profile, diagnostics, review := vinyl.Review(in.MaterialClass, in.Paths, in.Mirrored)
	write(w, http.StatusOK, map[string]any{
		"profile":           profile,
		"diagnostics":       diagnostics,
		"review":            review,
		"mirrorRecommended": profile.MirrorDefault,
	})
}
