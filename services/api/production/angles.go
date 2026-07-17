package production

import (
	"fmt"
	"math"
	"sort"
)

// ScreenAngleSet describes a multi-channel AM screen configuration.
type ScreenAngleSet struct {
	Cyan    float64 `json:"cyan"`
	Magenta float64 `json:"magenta"`
	Yellow  float64 `json:"yellow"`
	Black   float64 `json:"black"`
}

func DefaultScreenAngles() ScreenAngleSet {
	return ScreenAngleSet{Cyan: 15, Magenta: 75, Yellow: 0, Black: 45}
}

type AngleConflict struct {
	ChannelA string  `json:"channelA"`
	ChannelB string  `json:"channelB"`
	DeltaDeg float64 `json:"deltaDegrees"`
	Severity string  `json:"severity"`
	Message  string  `json:"message"`
}

// DetectScreenAngleConflicts flags moiré-prone pairs (<22.5° apart, or yellow within 7.5° of another channel).
func DetectScreenAngleConflicts(angles ScreenAngleSet) []AngleConflict {
	channels := []struct {
		name  string
		angle float64
	}{
		{"cyan", normalizeAngle(angles.Cyan)},
		{"magenta", normalizeAngle(angles.Magenta)},
		{"yellow", normalizeAngle(angles.Yellow)},
		{"black", normalizeAngle(angles.Black)},
	}
	var conflicts []AngleConflict
	for i := 0; i < len(channels); i++ {
		for j := i + 1; j < len(channels); j++ {
			delta := angularSeparation(channels[i].angle, channels[j].angle)
			limit := 22.5
			severity := "warning"
			if channels[i].name == "yellow" || channels[j].name == "yellow" {
				limit = 7.5
			}
			if delta+1e-9 < limit {
				if delta < 5 {
					severity = "error"
				}
				conflicts = append(conflicts, AngleConflict{
					ChannelA: channels[i].name, ChannelB: channels[j].name, DeltaDeg: math.Round(delta*10) / 10, Severity: severity,
					Message: fmt.Sprintf("%s and %s screens are only %.1f° apart (minimum %.1f°)", channels[i].name, channels[j].name, delta, limit),
				})
			}
		}
	}
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Severity != conflicts[j].Severity {
			return conflicts[i].Severity == "error"
		}
		return conflicts[i].DeltaDeg < conflicts[j].DeltaDeg
	})
	return conflicts
}

func RejectScreenAngleConflicts(angles ScreenAngleSet) error {
	for _, conflict := range DetectScreenAngleConflicts(angles) {
		if conflict.Severity == "error" {
			return fmt.Errorf("screen-angle conflict: %s", conflict.Message)
		}
	}
	return nil
}

func normalizeAngle(degrees float64) float64 {
	v := math.Mod(degrees, 90)
	if v < 0 {
		v += 90
	}
	return v
}

func angularSeparation(a, b float64) float64 {
	d := math.Abs(a - b)
	if d > 45 {
		d = 90 - d
	}
	return d
}
