// Package embroidery contains PrintStudio's deterministic embroidery compiler core.
package embroidery

import "fmt"

const SchemaVersion = 1

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Bounds struct {
	MinX float64 `json:"minX"`
	MinY float64 `json:"minY"`
	MaxX float64 `json:"maxX"`
	MaxY float64 `json:"maxY"`
}

type Command string

const (
	CommandStitch      Command = "stitch"
	CommandJump        Command = "jump"
	CommandTrim        Command = "trim"
	CommandColorChange Command = "color_change"
	CommandEnd         Command = "end"
)

type Stitch struct {
	Position Point   `json:"position"`
	Command  Command `json:"command"`
	Source   string  `json:"source"`
}

type StitchKind string

const (
	Running StitchKind = "running"
	Tatami  StitchKind = "tatami"
	Satin   StitchKind = "satin"
)

// Polygon uses the first ring as its exterior and subsequent rings as holes.
type Polygon struct {
	Rings [][]Point `json:"rings"`
}

type Region struct {
	ID             string     `json:"id"`
	ThreadID       string     `json:"threadId"`
	Geometry       Polygon    `json:"geometry"`
	Kind           StitchKind `json:"kind"`
	SpacingMM      float64    `json:"spacingMm,omitempty"`
	StitchLengthMM float64    `json:"stitchLengthMm,omitempty"`
	AngleDegrees   float64    `json:"angleDegrees,omitempty"`
	// WidthMM enables spine satin: Rings[0] is treated as a centerline and
	// expanded to left/right rails at this total column width.
	WidthMM        float64    `json:"widthMm,omitempty"`
	EdgeUnderlay   bool       `json:"edgeUnderlay,omitempty"`
	CenterUnderlay bool       `json:"centerUnderlay,omitempty"`
	ZigzagUnderlay bool       `json:"zigzagUnderlay,omitempty"`
}

type Block struct {
	ID       string     `json:"id"`
	RegionID string     `json:"regionId"`
	ThreadID string     `json:"threadId"`
	Kind     StitchKind `json:"kind"`
	Underlay []Stitch   `json:"underlay"`
	Stitches []Stitch   `json:"stitches"`
	Entry    Point      `json:"entry"`
	Exit     Point      `json:"exit"`
	Bounds   Bounds     `json:"bounds"`
}

type MachineProfile struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	HoopWidthMM  float64 `json:"hoopWidthMm"`
	HoopHeightMM float64 `json:"hoopHeightMm"`
	MaxStitches  int     `json:"maxStitches"`
	MaxColors    int     `json:"maxColors"`
	MinStitchMM  float64 `json:"minStitchMm"`
	MaxStitchMM  float64 `json:"maxStitchMm"`
	MaxJumpMM    float64 `json:"maxJumpMm"`
}

type Document struct {
	Version         int            `json:"version"`
	Units           string         `json:"units"`
	SourceHash      string         `json:"sourceHash"`
	CompilerVersion string         `json:"compilerVersion"`
	Machine         MachineProfile `json:"machine"`
	Regions         []Region       `json:"regions"`
	Plan            []Block        `json:"plan"`
	Diagnostics     []Diagnostic   `json:"diagnostics"`
}

type Severity string

const (
	Error   Severity = "error"
	Warning Severity = "warning"
)

type Diagnostic struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	RegionID string   `json:"regionId"`
}

func DefaultProfile() MachineProfile {
	return MachineProfile{ID: "generic-130x180", Name: "Generic 130 x 180 mm", HoopWidthMM: 130, HoopHeightMM: 180, MaxStitches: 100000, MaxColors: 16, MinStitchMM: .4, MaxStitchMM: 12.1, MaxJumpMM: 12.1}
}

func (p Polygon) Validate() error {
	if len(p.Rings) == 0 || len(p.Rings[0]) < 3 {
		return fmt.Errorf("polygon requires an exterior ring with at least three points")
	}
	for i, ring := range p.Rings {
		if len(ring) < 3 {
			return fmt.Errorf("ring %d has fewer than three points", i)
		}
	}
	return nil
}

// ValidateGeometry accepts either a filled polygon or an open spine centerline
// when WidthMM is set for satin.
func (r Region) ValidateGeometry() error {
	if r.Kind == Satin && r.WidthMM > 0 {
		if len(r.Geometry.Rings) != 1 || len(r.Geometry.Rings[0]) < 2 {
			return fmt.Errorf("spine satin requires a single centerline with at least two points")
		}
		if r.WidthMM > 40 {
			return fmt.Errorf("spine satin width %.2f mm is unrealistically wide", r.WidthMM)
		}
		return nil
	}
	return r.Geometry.Validate()
}
