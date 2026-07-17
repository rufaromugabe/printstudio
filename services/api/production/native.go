package production

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type NativeTools struct{ Vips, Potrace string }
type Capabilities struct {
	ICC             bool         `json:"icc"`
	VectorTrace     bool         `json:"vectorTrace"`
	VipsPath        string       `json:"vipsPath"`
	PotracePath     string       `json:"potracePath"`
	PolygonBoolean  bool         `json:"polygonBoolean"`
	MaxRenderPixels int64        `json:"maxRenderPixels"`
	ScreeningModes  []string     `json:"screeningModes"`
	TrapPresets     []TrapPreset `json:"trapPresets"`
	NamedInks       []NamedInk   `json:"namedInks"`
	ICCProfiles     bool         `json:"iccProfiles"`
	QualityPolicy   string       `json:"qualityPolicy"`
	ProductionReady bool         `json:"productionReady"`
	RequireICC      bool         `json:"requireIcc"`
	RequireApproval bool         `json:"requireApproval"`
	AcceptanceGates []MethodGate `json:"acceptanceGates"`
}

func (n NativeTools) Probe() Capabilities {
	v := resolve(n.Vips, "vips")
	p := resolve(n.Potrace, "potrace")
	polygon := Clipper2Available()
	icc := v != ""
	trace := p != ""
	return Capabilities{
		ICC: icc, VectorTrace: trace, VipsPath: v, PotracePath: p, PolygonBoolean: polygon,
		ScreeningModes: []string{string(ScreeningAM), string(ScreeningFM)},
		TrapPresets:    TrapPresets(),
		NamedInks:      DefaultNamedInks(),
		QualityPolicy:  "fail-closed: no boundary fallbacks, no browser gang sheets, AM angle conflicts rejected, ICC required when policy enabled",
		ProductionReady: icc && trace && polygon,
		AcceptanceGates: MethodAcceptanceGates(),
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
	p := resolve(n.Potrace, "potrace")
	if p == "" {
		return fmt.Errorf("potrace is unavailable")
	}
	if info, err := os.Stat(input); err != nil || info.IsDir() {
		return fmt.Errorf("trace input does not exist")
	}
	cmd := exec.CommandContext(ctx, p, "--svg", "--output", output, input)
	if data, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("potrace: %w: %s", err, data)
	}
	return nil
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
