package production

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed iccdata/*.icc
var commonICCFiles embed.FS

// CommonICCProfile is a curated, bundled working-space profile.
// Press-measured CMYK printer profiles are intentionally excluded (licensing + device-specific).
type CommonICCProfile struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Roles       []string `json:"roles"` // source, destination
	FileName    string   `json:"fileName"`
}

func CommonICCProfiles() []CommonICCProfile {
	return []CommonICCProfile{
		{ID: "srgb", Label: "sRGB", Description: "Standard web/studio RGB. Default for most artwork.", Roles: []string{"source", "destination"}, FileName: "srgb.icc"},
		{ID: "display-p3", Label: "Display P3", Description: "Wide-gamut RGB (modern displays). Convert to sRGB for most print paths.", Roles: []string{"source", "destination"}, FileName: "display-p3.icc"},
		{ID: "gray-gamma-22", Label: "Gray Gamma 2.2", Description: "Neutral grayscale for mono artwork.", Roles: []string{"source", "destination"}, FileName: "gray-gamma-22.icc"},
	}
}

func CommonICCCombinations() []map[string]string {
	return []map[string]string{
		{"id": "srgb-to-srgb", "label": "sRGB → sRGB (default)", "sourceProfile": "srgb", "destinationProfile": "srgb"},
		{"id": "p3-to-srgb", "label": "Display P3 → sRGB", "sourceProfile": "display-p3", "destinationProfile": "srgb"},
		{"id": "gray-to-gray", "label": "Gray → Gray", "sourceProfile": "gray-gamma-22", "destinationProfile": "gray-gamma-22"},
	}
}

func IsCommonICCProfile(id string) bool {
	for _, profile := range CommonICCProfiles() {
		if profile.ID == id {
			return true
		}
	}
	return false
}

func LookupCommonICC(id string) (CommonICCProfile, error) {
	for _, profile := range CommonICCProfiles() {
		if profile.ID == id {
			return profile, nil
		}
	}
	return CommonICCProfile{}, fmt.Errorf("unsupported ICC profile %q; only common bundled profiles are allowed", id)
}

// SeedCommonICCProfiles writes the curated embed set into the profile store.
func SeedCommonICCProfiles(store *ICCProfileStore) error {
	if store == nil {
		return fmt.Errorf("ICC profile store is nil")
	}
	for _, profile := range CommonICCProfiles() {
		data, err := fs.ReadFile(commonICCFiles, "iccdata/"+profile.FileName)
		if err != nil {
			return fmt.Errorf("embedded ICC %s: %w", profile.ID, err)
		}
		if _, err := store.Put(profile.ID, profile.Label, profile.Description, data); err != nil {
			return fmt.Errorf("seed ICC %s: %w", profile.ID, err)
		}
	}
	return nil
}
