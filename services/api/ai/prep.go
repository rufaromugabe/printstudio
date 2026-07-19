package ai

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"

	xdraw "golang.org/x/image/draw"
)

const (
	TracerPotrace    = "potrace"
	TracerMLAssisted = "ml-assisted"
)

// VectorPrep cleans a raster before deterministic Potrace vectorization.
// Implementations must not emit cut/DST coordinates — only improved PNG-like rasters.
type VectorPrep interface {
	PrepareForVectorize(img image.Image) (image.Image, string, error)
}

// CreditHook is a metering stub for AI prep usage.
type CreditHook interface {
	ChargeVectorPrep(units int, reason string) error
}

type noopCredits struct{}

func (noopCredits) ChargeVectorPrep(int, string) error { return nil }

// Provider exposes optional ML operations used by prep adapters.
type Provider interface {
	BackgroundRemove(img image.Image) (image.Image, error)
	Upscale(img image.Image, scale int) (image.Image, error)
}

// StubProvider is the local/dev default: soft alpha cleanup only, no remote calls.
type StubProvider struct{}

func (StubProvider) BackgroundRemove(img image.Image) (image.Image, error) {
	return softAlphaCleanup(img, 24), nil
}

func (StubProvider) Upscale(img image.Image, scale int) (image.Image, error) {
	if scale <= 1 {
		return img, nil
	}
	return reconstructionUpscale(img, scale), nil
}

// EnvProviderAdapter routes to a configured remote provider name.
// v1 only wires the stub unless PRINTSTUDIO_AI_PROVIDER=stub (explicit) or unset.
type EnvProviderAdapter struct {
	Name     string
	Provider Provider
}

func (a EnvProviderAdapter) BackgroundRemove(img image.Image) (image.Image, error) {
	if a.Provider == nil {
		return nil, fmt.Errorf("ai provider %q is not configured", a.Name)
	}
	return a.Provider.BackgroundRemove(img)
}

func (a EnvProviderAdapter) Upscale(img image.Image, scale int) (image.Image, error) {
	if a.Provider == nil {
		return nil, fmt.Errorf("ai provider %q is not configured", a.Name)
	}
	return a.Provider.Upscale(img, scale)
}

// Gateway is the thin prep stage used by /v1/production/vectorize.
type Gateway struct {
	Provider Provider
	Credits  CreditHook
	Upscale  int // 1 = off
	RemoveBG bool
}

// NewGatewayFromEnv builds a provider-neutral prep gateway.
// PRINTSTUDIO_AI_PROVIDER: stub (default) | none
// PRINTSTUDIO_AI_VECTOR_UPSCALE: integer scale (default 1)
// PRINTSTUDIO_AI_VECTOR_REMOVE_BG: true|false (default false for determinism)
func NewGatewayFromEnv() *Gateway {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("PRINTSTUDIO_AI_PROVIDER")))
	if name == "" || name == "stub" {
		name = "stub"
	}
	var provider Provider
	switch name {
	case "none", "off", "disabled":
		provider = nil
	case "stub":
		provider = StubProvider{}
	default:
		// Unknown providers fall back to stub so local/dev never hard-depends on a vendor.
		provider = StubProvider{}
	}
	upscale := 1
	if v := strings.TrimSpace(os.Getenv("PRINTSTUDIO_AI_VECTOR_UPSCALE")); v != "" {
		if n, err := fmt.Sscanf(v, "%d", &upscale); n != 1 || err != nil || upscale < 1 {
			upscale = 1
		}
	}
	removeBG := strings.EqualFold(os.Getenv("PRINTSTUDIO_AI_VECTOR_REMOVE_BG"), "true")
	return &Gateway{Provider: provider, Credits: noopCredits{}, Upscale: upscale, RemoveBG: removeBG}
}

// PrepareForVectorize implements production.ImagePrep.
func (g *Gateway) PrepareForVectorize(img image.Image) (image.Image, string, error) {
	if g == nil || g.Provider == nil {
		return img, TracerPotrace, nil
	}
	out := img
	tracer := TracerPotrace
	charged := 0
	if g.RemoveBG {
		cleaned, err := g.Provider.BackgroundRemove(out)
		if err != nil {
			return nil, "", fmt.Errorf("background remove: %w", err)
		}
		out = cleaned
		tracer = TracerMLAssisted
		charged++
	}
	if g.Upscale > 1 {
		scaled, err := g.Provider.Upscale(out, g.Upscale)
		if err != nil {
			return nil, "", fmt.Errorf("upscale: %w", err)
		}
		out = scaled
		tracer = TracerMLAssisted
		charged++
	}
	if charged > 0 && g.Credits != nil {
		if err := g.Credits.ChargeVectorPrep(charged, "vectorize-prep"); err != nil {
			return nil, "", fmt.Errorf("ai credits: %w", err)
		}
	}
	// Always apply a cheap edge cleanup so “advanced” mode has a stable prep stage locally.
	if tracer == TracerPotrace {
		out = softAlphaCleanup(out, 20)
	}
	return out, tracer, nil
}

func softAlphaCleanup(img image.Image, cutoff uint8) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			if c.A < cutoff {
				c = color.NRGBA{}
			} else if c.A < 250 {
				// Harden semi-transparent fringe into solid ink for cleaner Potrace masks.
				c.A = 255
			}
			out.SetNRGBA(x-b.Min.X, y-b.Min.Y, c)
		}
	}
	return out
}

func reconstructionUpscale(img image.Image, scale int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w*scale, h*scale))
	// Catmull-Rom avoids the block stair-steps produced by nearest-neighbour
	// scaling. The deterministic mask stage hardens the resulting edge after it
	// applies its method-specific threshold.
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, b, xdraw.Src, nil)
	return out
}
