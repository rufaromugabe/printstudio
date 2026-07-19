package production

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// TraceOptions selects a deterministic Potrace profile for the production
// method instead of relying on one set of generic command defaults.
type TraceOptions struct {
	TurdSize     int
	AlphaMax     float64
	OptTolerance float64
	TurnPolicy   string
}

type NativeTools struct{ Vips, Potrace, Tesseract string }
type Capabilities struct {
	ICC             bool                `json:"icc"`
	VectorTrace     bool                `json:"vectorTrace"`
	VipsPath        string              `json:"vipsPath"`
	PotracePath     string              `json:"potracePath"`
	OCR             bool                `json:"ocr"`
	TesseractPath   string              `json:"tesseractPath"`
	PolygonBoolean  bool                `json:"polygonBoolean"`
	MaxRenderPixels int64               `json:"maxRenderPixels"`
	ScreeningModes  []string            `json:"screeningModes"`
	TrapPresets     []TrapPreset        `json:"trapPresets"`
	NamedInks       []NamedInk          `json:"namedInks"`
	ICCProfiles     bool                `json:"iccProfiles"`
	QualityPolicy   string              `json:"qualityPolicy"`
	ProductionReady bool                `json:"productionReady"`
	RequireICC      bool                `json:"requireIcc"`
	RequireApproval bool                `json:"requireApproval"`
	AcceptanceGates []MethodGate        `json:"acceptanceGates"`
	CommonICC       []CommonICCProfile  `json:"commonIccProfiles"`
	ICCCombinations []map[string]string `json:"iccCombinations"`
}

func (n NativeTools) Probe() Capabilities {
	v := resolve(n.Vips, "vips")
	p := resolve(n.Potrace, "potrace")
	t := resolve(n.Tesseract, "tesseract")
	polygon := Clipper2Available()
	icc := v != ""
	trace := p != ""
	return Capabilities{
		ICC: icc, VectorTrace: trace, VipsPath: v, PotracePath: p, OCR: t != "", TesseractPath: t, PolygonBoolean: polygon,
		ScreeningModes:  []string{string(ScreeningAM), string(ScreeningFM)},
		TrapPresets:     TrapPresets(),
		NamedInks:       DefaultNamedInks(),
		QualityPolicy:   "fail-closed: common ICC profiles only (sRGB, Display P3, Gray); no custom profile uploads",
		ProductionReady: icc && trace && polygon,
		AcceptanceGates: MethodAcceptanceGates(),
		CommonICC:       CommonICCProfiles(),
		ICCCombinations: CommonICCCombinations(),
	}
}
func (n NativeTools) ICCTransform(ctx context.Context, input, output, sourceProfile, destinationProfile, stringIntent string) error {
	v := resolve(n.Vips, "vips")
	if v == "" {
		return fmt.Errorf("libvips is unavailable")
	}
	for _, path := range []string{input, sourceProfile, destinationProfile} {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return fmt.Errorf("required input does not exist: %s", filepath.Base(path))
		}
	}
	if stringIntent == "" {
		stringIntent = "relative"
	}
	switch stringIntent {
	case "perceptual", "relative", "saturation", "absolute":
	default:
		return fmt.Errorf("unsupported ICC intent %q", stringIntent)
	}
	cmd := exec.CommandContext(ctx, v, "icc_transform", input, output, destinationProfile, "--input-profile", sourceProfile, "--intent", stringIntent)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("libvips ICC transform: %w: %s", err, data)
	}
	return nil
}
func (n NativeTools) TracePBM(ctx context.Context, input, output string) error {
	return n.TracePBMWithOptions(ctx, input, output, TraceOptions{})
}
func (n NativeTools) TracePBMWithOptions(ctx context.Context, input, output string, options TraceOptions) error {
	p := resolve(n.Potrace, "potrace")
	if p == "" {
		return fmt.Errorf("potrace is unavailable")
	}
	if info, err := os.Stat(input); err != nil || info.IsDir() {
		return fmt.Errorf("trace input does not exist")
	}
	args := tracePBMArgs(input, output, options)
	cmd := exec.CommandContext(ctx, p, args...)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("potrace: %w: %s", err, data)
	}
	return nil
}

func tracePBMArgs(input, output string, options TraceOptions) []string {
	args := []string{"--svg"}
	if options.TurdSize > 0 {
		args = append(args, "--turdsize", fmt.Sprintf("%d", options.TurdSize))
	}
	if options.AlphaMax > 0 {
		args = append(args, "--alphamax", fmt.Sprintf("%.3f", options.AlphaMax))
	}
	if options.OptTolerance > 0 {
		args = append(args, "--opttolerance", fmt.Sprintf("%.3f", options.OptTolerance))
	}
	switch options.TurnPolicy {
	case "black", "white", "left", "right", "minority", "majority", "random":
		args = append(args, "--turnpolicy", options.TurnPolicy)
	}
	args = append(args, "--output", output, input)
	return args
}
func resolve(configured, name string) string {
	if configured != "" {
		if p, err := exec.LookPath(configured); err == nil {
			return p
		}
		return ""
	}
	p, _ := exec.LookPath(name)
	return p
}
