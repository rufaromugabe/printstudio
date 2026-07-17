package production

import (
	"fmt"
	"sort"
)

type Sheet struct {
	WidthMM  float64 `json:"widthMm"`
	HeightMM float64 `json:"heightMm"`
	MarginMM float64 `json:"marginMm"`
	GapMM    float64 `json:"gapMm"`
}
type Item struct {
	ID          string  `json:"id"`
	WidthMM     float64 `json:"widthMm"`
	HeightMM    float64 `json:"heightMm"`
	Quantity    int     `json:"quantity"`
	AllowRotate bool    `json:"allowRotate"`
}
type Placement struct {
	ID       string  `json:"id"`
	Copy     int     `json:"copy"`
	XMM      float64 `json:"xMm"`
	YMM      float64 `json:"yMm"`
	WidthMM  float64 `json:"widthMm"`
	HeightMM float64 `json:"heightMm"`
	Rotated  bool    `json:"rotated"`
}
type rect struct{ x, y, w, h float64 }

// Nest applies deterministic MaxRects best-short-side-fit placement.
func Nest(sheet Sheet, items []Item) ([]Placement, error) {
	if sheet.WidthMM <= 0 || sheet.HeightMM <= 0 {
		return nil, fmt.Errorf("sheet dimensions must be positive")
	}
	var expanded []Placement
	for _, item := range items {
		if item.ID == "" || item.WidthMM <= 0 || item.HeightMM <= 0 || item.Quantity < 1 {
			return nil, fmt.Errorf("invalid gang item")
		}
		for i := 0; i < item.Quantity; i++ {
			expanded = append(expanded, Placement{ID: item.ID, Copy: i + 1, WidthMM: item.WidthMM + sheet.GapMM, HeightMM: item.HeightMM + sheet.GapMM, Rotated: item.AllowRotate})
		}
	}
	sort.SliceStable(expanded, func(i, j int) bool {
		ai := expanded[i].WidthMM * expanded[i].HeightMM
		aj := expanded[j].WidthMM * expanded[j].HeightMM
		if ai == aj {
			return expanded[i].ID < expanded[j].ID
		}
		return ai > aj
	})
	free := []rect{{sheet.MarginMM, sheet.MarginMM, sheet.WidthMM - 2*sheet.MarginMM, sheet.HeightMM - 2*sheet.MarginMM}}
	result := make([]Placement, 0, len(expanded))
	for _, item := range expanded {
		best, rotated, index, short, long := rect{}, false, -1, 1e30, 1e30
		for i, f := range free {
			try := func(w, h float64, r bool) {
				if w > f.w || h > f.h {
					return
				}
				s := min(f.w-w, f.h-h)
				l := max(f.w-w, f.h-h)
				if s < short || s == short && l < long {
					best = rect{f.x, f.y, w, h}
					rotated = r
					index = i
					short = s
					long = l
				}
			}
			try(item.WidthMM, item.HeightMM, false)
			if item.Rotated {
				try(item.HeightMM, item.WidthMM, true)
			}
		}
		if index < 0 {
			return nil, fmt.Errorf("item %s copy %d does not fit", item.ID, item.Copy)
		}
		free = splitAndPrune(free, best)
		w, h := best.w-sheet.GapMM, best.h-sheet.GapMM
		result = append(result, Placement{ID: item.ID, Copy: item.Copy, XMM: best.x, YMM: best.y, WidthMM: w, HeightMM: h, Rotated: rotated})
	}
	return result, nil
}
func splitAndPrune(free []rect, used rect) []rect {
	var out []rect
	for _, f := range free {
		if !intersects(f, used) {
			out = append(out, f)
			continue
		}
		if used.x > f.x {
			out = append(out, rect{f.x, f.y, used.x - f.x, f.h})
		}
		if used.x+used.w < f.x+f.w {
			out = append(out, rect{used.x + used.w, f.y, f.x + f.w - (used.x + used.w), f.h})
		}
		if used.y > f.y {
			out = append(out, rect{f.x, f.y, f.w, used.y - f.y})
		}
		if used.y+used.h < f.y+f.h {
			out = append(out, rect{f.x, used.y + used.h, f.w, f.y + f.h - (used.y + used.h)})
		}
	}
	for i := 0; i < len(out); i++ {
		if out[i].w <= 0 || out[i].h <= 0 {
			out = append(out[:i], out[i+1:]...)
			i--
			continue
		}
		for j := 0; j < len(out); j++ {
			if i != j && contains(out[j], out[i]) {
				out = append(out[:i], out[i+1:]...)
				i--
				break
			}
		}
	}
	return out
}
func intersects(a, b rect) bool {
	return b.x < a.x+a.w && b.x+b.w > a.x && b.y < a.y+a.h && b.y+b.h > a.y
}
func contains(a, b rect) bool {
	return b.x >= a.x && b.y >= a.y && b.x+b.w <= a.x+a.w && b.y+b.h <= a.y+a.h
}
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
