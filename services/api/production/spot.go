package production

import (
	"fmt"
	"math"
	"strings"
)

// Lab is CIE L*a*b* under D65 / 2° observer.
type Lab struct {
	L, A, B float64
}

type NamedInk struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Hex      string  `json:"hex"`
	Lab      Lab     `json:"lab"`
	Family   string  `json:"family"`
	Opacity  float64 `json:"opacity"`
	Washable bool    `json:"washable"`
}

type SpotMatch struct {
	Ink      NamedInk `json:"ink"`
	DeltaE00 float64  `json:"deltaE00"`
	Exact    bool     `json:"exact"`
}

// DefaultNamedInks is a starter spot library for matching artwork colours.
func DefaultNamedInks() []NamedInk {
	return []NamedInk{
		{ID: "process-black", Name: "Process Black", Hex: "#000000", Lab: HexToLab("#000000"), Family: "process", Opacity: 1},
		{ID: "process-white", Name: "Opaque White", Hex: "#FFFFFF", Lab: HexToLab("#FFFFFF"), Family: "underbase", Opacity: 1},
		{ID: "spot-red", Name: "Spot Red", Hex: "#C8102E", Lab: HexToLab("#C8102E"), Family: "spot", Opacity: 1},
		{ID: "spot-navy", Name: "Spot Navy", Hex: "#00205B", Lab: HexToLab("#00205B"), Family: "spot", Opacity: 1},
		{ID: "spot-royal", Name: "Spot Royal", Hex: "#0033A0", Lab: HexToLab("#0033A0"), Family: "spot", Opacity: 1},
		{ID: "spot-green", Name: "Spot Green", Hex: "#00A651", Lab: HexToLab("#00A651"), Family: "spot", Opacity: 1},
		{ID: "spot-gold", Name: "Metallic Gold", Hex: "#C5A572", Lab: HexToLab("#C5A572"), Family: "metallic", Opacity: .95},
		{ID: "spot-silver", Name: "Metallic Silver", Hex: "#A8A9AD", Lab: HexToLab("#A8A9AD"), Family: "metallic", Opacity: .95},
		{ID: "fluoro-pink", Name: "Fluorescent Pink", Hex: "#FF2BD6", Lab: HexToLab("#FF2BD6"), Family: "fluorescent", Opacity: .9},
		{ID: "fluoro-yellow", Name: "Fluorescent Yellow", Hex: "#DFFF00", Lab: HexToLab("#DFFF00"), Family: "fluorescent", Opacity: .9},
	}
}

func HexToLab(hex string) Lab {
	r, g, b, err := parseHexRGB(hex)
	if err != nil {
		return Lab{}
	}
	return RGBToLab(r, g, b)
}

func RGBToLab(r, g, b float64) Lab {
	x, y, z := srgbToXYZ(r, g, b)
	return xyzToLab(x, y, z)
}

// DeltaE2000 implements CIEDE2000 colour difference.
func DeltaE2000(a, b Lab) float64 {
	const deg = math.Pi / 180
	L1, a1, b1 := a.L, a.A, a.B
	L2, a2, b2 := b.L, b.A, b.B
	C1 := math.Hypot(a1, b1)
	C2 := math.Hypot(a2, b2)
	Cm := (C1 + C2) / 2
	G := 0.5 * (1 - math.Sqrt(math.Pow(Cm, 7)/(math.Pow(Cm, 7)+math.Pow(25, 7))))
	a1p, a2p := (1+G)*a1, (1+G)*a2
	C1p, C2p := math.Hypot(a1p, b1), math.Hypot(a2p, b2)
	h1p := hueRad(a1p, b1)
	h2p := hueRad(a2p, b2)
	dLp := L2 - L1
	dCp := C2p - C1p
	var dhp float64
	switch {
	case C1p*C2p == 0:
		dhp = 0
	case math.Abs(h2p-h1p) <= math.Pi:
		dhp = h2p - h1p
	case h2p > h1p:
		dhp = h2p - h1p - 2*math.Pi
	default:
		dhp = h2p - h1p + 2*math.Pi
	}
	dHp := 2 * math.Sqrt(C1p*C2p) * math.Sin(dhp/2)
	Lm := (L1 + L2) / 2
	CmP := (C1p + C2p) / 2
	var hm float64
	switch {
	case C1p*C2p == 0:
		hm = h1p + h2p
	case math.Abs(h1p-h2p) <= math.Pi:
		hm = (h1p + h2p) / 2
	case h1p+h2p < 2*math.Pi:
		hm = (h1p + h2p + 2*math.Pi) / 2
	default:
		hm = (h1p + h2p - 2*math.Pi) / 2
	}
	T := 1 - 0.17*math.Cos(hm-30*deg) + 0.24*math.Cos(2*hm) + 0.32*math.Cos(3*hm+6*deg) - 0.20*math.Cos(4*hm-63*deg)
	dTheta := 30 * deg * math.Exp(-math.Pow((hm/deg-275)/25, 2))
	Rc := 2 * math.Sqrt(math.Pow(CmP, 7)/(math.Pow(CmP, 7)+math.Pow(25, 7)))
	Sl := 1 + (0.015*math.Pow(Lm-50, 2))/math.Sqrt(20+math.Pow(Lm-50, 2))
	Sc := 1 + 0.045*CmP
	Sh := 1 + 0.015*CmP*T
	Rt := -math.Sin(2*dTheta) * Rc
	return math.Sqrt(math.Pow(dLp/Sl, 2) + math.Pow(dCp/Sc, 2) + math.Pow(dHp/Sh, 2) + Rt*(dCp/Sc)*(dHp/Sh))
}

func MatchSpot(hex string, library []NamedInk, maxDeltaE float64) (SpotMatch, error) {
	if maxDeltaE <= 0 {
		maxDeltaE = 6
	}
	if len(library) == 0 {
		library = DefaultNamedInks()
	}
	target := HexToLab(hex)
	if _, _, _, err := parseHexRGB(hex); err != nil {
		return SpotMatch{}, err
	}
	best := SpotMatch{DeltaE00: math.Inf(1)}
	for _, ink := range library {
		dE := DeltaE2000(target, ink.Lab)
		if dE < best.DeltaE00 {
			best = SpotMatch{Ink: ink, DeltaE00: dE, Exact: dE < 0.5}
		}
	}
	if best.DeltaE00 > maxDeltaE {
		return SpotMatch{}, fmt.Errorf("no named ink within ΔE00 %.1f of %s (best %s at %.2f)", maxDeltaE, strings.ToUpper(hex), best.Ink.Name, best.DeltaE00)
	}
	return best, nil
}

func MatchSpots(hexes []string, maxDeltaE float64) ([]SpotMatch, error) {
	out := make([]SpotMatch, 0, len(hexes))
	for _, hex := range hexes {
		match, err := MatchSpot(hex, DefaultNamedInks(), maxDeltaE)
		if err != nil {
			return nil, err
		}
		out = append(out, match)
	}
	return out, nil
}

func parseHexRGB(hex string) (r, g, b float64, err error) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("colour must be #RRGGBB")
	}
	var value uint32
	if _, scanErr := fmt.Sscanf(hex, "%06x", &value); scanErr != nil {
		return 0, 0, 0, fmt.Errorf("invalid hex colour")
	}
	return float64(value>>16&0xff) / 255, float64(value>>8&0xff) / 255, float64(value&0xff) / 255, nil
}

func srgbToXYZ(r, g, b float64) (x, y, z float64) {
	linear := func(c float64) float64 {
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	rl, gl, bl := linear(r), linear(g), linear(b)
	x = rl*0.4124564 + gl*0.3575761 + bl*0.1804375
	y = rl*0.2126729 + gl*0.7151522 + bl*0.0721750
	z = rl*0.0193339 + gl*0.1191920 + bl*0.9503041
	return
}

func xyzToLab(x, y, z float64) Lab {
	// D65 reference white
	const xn, yn, zn = 0.95047, 1.00000, 1.08883
	f := func(t float64) float64 {
		if t > 216.0/24389.0 {
			return math.Cbrt(t)
		}
		return (841.0/108.0)*t + 4.0/29.0
	}
	fx, fy, fz := f(x/xn), f(y/yn), f(z/zn)
	return Lab{L: 116*fy - 16, A: 500 * (fx - fy), B: 200 * (fy - fz)}
}

func hueRad(a, b float64) float64 {
	if a == 0 && b == 0 {
		return 0
	}
	h := math.Atan2(b, a)
	if h < 0 {
		h += 2 * math.Pi
	}
	return h
}
