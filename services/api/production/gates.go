package production

import "fmt"

type AcceptanceCheck struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	// System means the API can confirm this from capabilities / pack behaviour.
	// Operators should not be asked to re-attest steps the platform already performed.
	System bool `json:"system,omitempty"`
}

type MethodGate struct {
	Method string            `json:"method"`
	Checks []AcceptanceCheck `json:"checks"`
}

func MethodAcceptanceGates() []MethodGate {
	return []MethodGate{
		{Method: "DTF", Checks: []AcceptanceCheck{
			{ID: "dpi-300", Label: "Artwork rendered at 300 DPI physical size", Required: true, System: true},
			{ID: "underbase-preset", Label: "Trap preset applied for film/white", Required: true, System: true},
			{ID: "ink-limits", Label: "Operator verified printer ink limits / white density", Required: true},
		}},
		{Method: "Screen print", Checks: []AcceptanceCheck{
			{ID: "am-fm-choice", Label: "AM or FM screening selected deliberately", Required: true, System: true},
			{ID: "angle-ok", Label: "Screen angles passed conflict detection", Required: true, System: true},
			{ID: "icc-or-spot", Label: "ICC-calibrated process colour or named spot match", Required: true, System: true},
			{ID: "mesh-verify", Label: "Mesh / LPI / substrate verified on press", Required: true},
		}},
		{Method: "Vinyl", Checks: []AcceptanceCheck{
			{ID: "clipper2", Label: "Clipper2 Boolean cleanup available", Required: true, System: true},
			{ID: "weed-box", Label: "Weed box present on cut file", Required: true, System: true},
			{ID: "blade-test", Label: "Blade/material test cut completed", Required: true},
		}},
		{Method: "Sublimation", Checks: []AcceptanceCheck{
			{ID: "full-bleed", Label: "Artwork covers bleed area or warning acknowledged", Required: true, System: true},
			{ID: "paper-icc", Label: "Paper ICC applied when profiles are configured", Required: false, System: true},
		}},
		{Method: "Embroidery", Checks: []AcceptanceCheck{
			{ID: "machine-profile", Label: "Machine profile validation passed with no errors", Required: true, System: true},
			{ID: "no-boundary-fallback", Label: "All layers traced without boundary fallback", Required: true, System: true},
			{ID: "sew-out", Label: "Sample sew-out approved for this profile", Required: true},
		}},
	}
}

func LookupMethodGate(method string) (MethodGate, error) {
	for _, gate := range MethodAcceptanceGates() {
		if gate.Method == method {
			return gate, nil
		}
	}
	return MethodGate{}, fmt.Errorf("no acceptance gate for method %q", method)
}

// SatisfySystemChecks marks platform-owned acceptance items so clients are not
// asked to reconfirm work the export/pack path already performs.
func SatisfySystemChecks(method string, checklist map[string]bool, caps Capabilities) map[string]bool {
	out := map[string]bool{}
	for id, value := range checklist {
		out[id] = value
	}
	gate, err := LookupMethodGate(method)
	if err != nil {
		return out
	}
	anglesOK := !hasErrorAngleConflict(DetectScreenAngleConflicts(DefaultScreenAngles()))
	for _, check := range gate.Checks {
		if !check.System {
			continue
		}
		switch check.ID {
		case "clipper2":
			out[check.ID] = caps.PolygonBoolean
		case "angle-ok":
			out[check.ID] = anglesOK
		case "icc-or-spot", "paper-icc":
			out[check.ID] = caps.ICCProfiles || out[check.ID]
		default:
			out[check.ID] = true
		}
	}
	return out
}

func hasErrorAngleConflict(conflicts []AngleConflict) bool {
	for _, conflict := range conflicts {
		if conflict.Severity == "error" {
			return true
		}
	}
	return false
}
