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
// GapMM is spacing between copies only — a single piece may use the full
// usable sheet without reserving an unused trailing gap against the edge.
func Nest(sheet Sheet, items []Item) ([]Placement, error) {
	if sheet.WidthMM <= 0 || sheet.HeightMM <= 0 {
		return nil, fmt.Errorf("sheet dimensions must be positive")
	}
	if sheet.GapMM < 0 || sheet.MarginMM < 0 {
		return nil, fmt.Errorf("sheet margin and gap cannot be negative")
	}
	usableW := sheet.WidthMM - 2*sheet.MarginMM
	usableH := sheet.HeightMM - 2*sheet.MarginMM
	if usableW <= 0 || usableH <= 0 {
		return nil, fmt.Errorf("sheet margins leave no usable area")
	}
	var expanded []Placement
	for _, item := range items {
		if item.ID == "" || item.WidthMM <= 0 || item.HeightMM <= 0 || item.Quantity < 1 {
			return nil, fmt.Errorf("invalid gang item")
		}
		for i := 0; i < item.Quantity; i++ {
			expanded = append(expanded, Placement{ID: item.ID, Copy: i + 1, WidthMM: item.WidthMM, HeightMM: item.HeightMM, Rotated: item.AllowRotate})
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
	free := []rect{{sheet.MarginMM, sheet.MarginMM, usableW, usableH}}
	result := make([]Placement, 0, len(expanded))
	for _, item := range expanded {
		bestArt, bestUsed := rect{}, rect{}
		rotated, index, short, long := false, -1, 1e30, 1e30
		for i, f := range free {
			try := func(w, h float64, r bool) {
				if w > f.w+1e-9 || h > f.h+1e-9 {
					return
				}
				s := min(f.w-w, f.h-h)
				l := max(f.w-w, f.h-h)
				if s < short || s == short && l < long {
					occupiedW := w + sheet.GapMM
					if occupiedW > f.w {
						occupiedW = f.w
					}
					occupiedH := h + sheet.GapMM
					if occupiedH > f.h {
						occupiedH = f.h
					}
					bestArt = rect{f.x, f.y, w, h}
					bestUsed = rect{f.x, f.y, occupiedW, occupiedH}
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
			return nil, fmt.Errorf("item %s copy %d (%.1f×%.1f mm) does not fit on %.1f×%.1f mm sheet", item.ID, item.Copy, item.WidthMM, item.HeightMM, sheet.WidthMM, sheet.HeightMM)
		}
		free = splitAndPrune(free, bestUsed)
		result = append(result, Placement{ID: item.ID, Copy: item.Copy, XMM: bestArt.x, YMM: bestArt.y, WidthMM: bestArt.w, HeightMM: bestArt.h, Rotated: rotated})
	}
	return result, nil
}

// MaxCopiesForSheet finds how many identical pieces fit on the sheet using the
// same MaxRects packer as Nest, so "fill page" matches production placement.
func MaxCopiesForSheet(sheet Sheet, widthMM, heightMM float64, allowRotate bool, limit int) (int, error) {
	if widthMM <= 0 || heightMM <= 0 {
		return 0, fmt.Errorf("source dimensions must be positive")
	}
	if limit < 1 {
		limit = 500
	}
	if limit > 500 {
		limit = 500
	}
	low, high, best := 1, limit, 0
	for low <= high {
		mid := (low + high) / 2
		_, err := Nest(sheet, []Item{{ID: "artwork", WidthMM: widthMM, HeightMM: heightMM, Quantity: mid, AllowRotate: allowRotate}})
		if err == nil {
			best = mid
			low = mid + 1
			continue
		}
		high = mid - 1
	}
	if best < 1 {
		return 0, fmt.Errorf("artwork %.0f×%.0f mm does not fit on the selected sheet %.0f×%.0f mm — choose a larger sheet (or allow rotation)", widthMM, heightMM, sheet.WidthMM, sheet.HeightMM)
	}
	return best, nil
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
	return pruneContained(out)
}

func intersects(a, b rect) bool {
	return a.x < b.x+b.w && a.x+a.w > b.x && a.y < b.y+b.h && a.y+a.h > b.y
}

func pruneContained(rects []rect) []rect {
	out := make([]rect, 0, len(rects))
	for i, a := range rects {
		if a.w <= 0 || a.h <= 0 {
			continue
		}
		contained := false
		for j, b := range rects {
			if i == j || b.w <= 0 || b.h <= 0 {
				continue
			}
			if a.x >= b.x && a.y >= b.y && a.x+a.w <= b.x+b.w && a.y+a.h <= b.y+b.h {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, a)
		}
	}
	return out
}
