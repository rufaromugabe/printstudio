package production

import "fmt"

// TrapPreset is a calibrated choke/spread recipe for a printer/ink/film stack.
type TrapPreset struct {
	ID               string  `json:"id"`
	Label            string  `json:"label"`
	Method           string  `json:"method"`
	SpreadPixels     int     `json:"spreadPixels"`
	UnderbaseChokePX int     `json:"underbaseChokePixels"`
	Threshold        uint8   `json:"threshold"`
	Notes            string  `json:"notes"`
	DPI              float64 `json:"dpi"`
}

func TrapPresets() []TrapPreset {
	return []TrapPreset{
		{ID: "dtf-pet-film-standard", Label: "DTF PET film · standard white", Method: "DTF", SpreadPixels: 2, UnderbaseChokePX: 0, Threshold: 1, Notes: "2 px Euclidean white spread for typical PET film at 300 DPI.", DPI: 300},
		{ID: "dtf-pet-film-fine", Label: "DTF PET film · fine detail", Method: "DTF", SpreadPixels: 1, UnderbaseChokePX: 0, Threshold: 8, Notes: "Tighter white for small type; raise threshold to ignore anti-aliased fringes.", DPI: 300},
		{ID: "dtf-dark-garment-heavy", Label: "DTF dark garment · heavy white", Method: "DTF", SpreadPixels: 3, UnderbaseChokePX: 0, Threshold: 1, Notes: "Extra white coverage for dark fabrics; verify ink limits on press.", DPI: 300},
		{ID: "screen-plastisol-45lpi", Label: "Screen plastisol · 45 LPI", Method: "Screen print", SpreadPixels: 0, UnderbaseChokePX: -2, Threshold: 1, Notes: "2 px underbase choke to hide registration drift on mid-mesh.", DPI: 300},
		{ID: "screen-plastisol-55lpi", Label: "Screen plastisol · 55 LPI", Method: "Screen print", SpreadPixels: 0, UnderbaseChokePX: -3, Threshold: 1, Notes: "Slightly tighter choke for finer mesh / higher LPI sets.", DPI: 300},
		{ID: "screen-waterbase-soft", Label: "Screen water-based · soft hand", Method: "Screen print", SpreadPixels: 0, UnderbaseChokePX: -1, Threshold: 1, Notes: "Minimal choke; water-based sets tolerate less underbase flash.", DPI: 300},
		{ID: "sublimation-paper-standard", Label: "Sublimation paper · standard", Method: "Sublimation", SpreadPixels: 0, UnderbaseChokePX: 0, Threshold: 1, Notes: "No underbase; rely on bleed and full-coverage checks.", DPI: 300},
	}
}

func LookupTrapPreset(id string) (TrapPreset, error) {
	for _, preset := range TrapPresets() {
		if preset.ID == id {
			return preset, nil
		}
	}
	return TrapPreset{}, fmt.Errorf("unknown trap preset %q", id)
}
