package embroidery

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func rectangle(x, y, w, h float64) Polygon {
	return Polygon{Rings: [][]Point{{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}}}}
}

func TestCompileIsDeterministic(t *testing.T) {
	regions := []Region{{ID: "panel", ThreadID: "black", Geometry: rectangle(-20, -10, 40, 20), Kind: Tatami, SpacingMM: 1, StitchLengthMM: 3, AngleDegrees: 15, EdgeUnderlay: true}}
	a, e := Compile(regions, DefaultProfile())
	if e != nil {
		t.Fatal(e)
	}
	b, e := Compile(regions, DefaultProfile())
	if e != nil {
		t.Fatal(e)
	}
	da, _ := EncodeDST(a, "TEST")
	db, _ := EncodeDST(b, "TEST")
	if !bytes.Equal(da, db) {
		t.Fatal("same input produced different DST output")
	}
	if a.SourceHash != b.SourceHash {
		t.Fatal("source hashes differ")
	}
}

func TestTatamiExcludesHole(t *testing.T) {
	p := Polygon{Rings: [][]Point{{{0, 0}, {20, 0}, {20, 20}, {0, 20}}, {{8, 8}, {12, 8}, {12, 12}, {8, 12}}}}
	d, e := Compile([]Region{{ID: "with-hole", ThreadID: "red", Geometry: p, Kind: Tatami, SpacingMM: 1, StitchLengthMM: 2}}, DefaultProfile())
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range d.Plan[0].Stitches {
		if s.Command == CommandStitch && s.Position.X > 8 && s.Position.X < 12 && s.Position.Y > 8 && s.Position.Y < 12 {
			t.Fatalf("stitch entered hole: %#v", s.Position)
		}
	}
}

func TestValidationRejectsOutsideHoop(t *testing.T) {
	p := DefaultProfile()
	p.HoopWidthMM = 30
	d, e := Compile([]Region{{ID: "wide", ThreadID: "blue", Geometry: rectangle(-20, -5, 40, 10), Kind: Running}}, p)
	if e != nil {
		t.Fatal(e)
	}
	if !HasErrors(d.Diagnostics) {
		t.Fatal("expected hoop validation error")
	}
	if _, e = EncodeDST(d, "INVALID"); e == nil {
		t.Fatal("invalid document was exported")
	}
}

func TestDSTAndSVG(t *testing.T) {
	d, e := Compile([]Region{{ID: "outline", ThreadID: "green", Geometry: rectangle(0, 0, 10, 10), Kind: Running, StitchLengthMM: 2}}, DefaultProfile())
	if e != nil {
		t.Fatal(e)
	}
	dst, e := EncodeDST(d, "OUTLINE")
	if e != nil {
		t.Fatal(e)
	}
	if len(dst) <= 515 || dst[511] != 0x1a {
		t.Fatal("invalid DST envelope")
	}
	decoded, e := DecodeDST(dst)
	if e != nil {
		t.Fatal(e)
	}
	if len(decoded) == 0 {
		t.Fatal("DST round trip produced no commands")
	}
	last := decoded[len(decoded)-1].Position
	if math.Abs(last.X) > 0.0001 || math.Abs(last.Y) > 0.0001 {
		t.Fatalf("closed outline decoded to unexpected endpoint: %#v", last)
	}
	svg := DiagnosticSVG(d, 300, 400)
	if !strings.Contains(svg, "data-region=\"outline\"") {
		t.Fatal("SVG lacks region diagnostics")
	}
	if !strings.Contains(svg, `data-print-width-mm="300"`) || !strings.Contains(svg, `class="print-area"`) {
		t.Fatal("SVG should frame the print area, not crop to stitches")
	}
}

func TestEveryDSTDeltaRoundTrips(t *testing.T) {
	for x := -121; x <= 121; x++ {
		for y := -121; y <= 121; y++ {
			encoded := encodeDelta(x, y, false)
			dx, dy := decodeDelta(encoded)
			if dx != x || dy != y {
				t.Fatalf("delta (%d,%d) decoded as (%d,%d): %x", x, y, dx, dy, encoded)
			}
		}
	}
}

func TestSatinBuildsPairedRailsAndUnderlay(t *testing.T) {
	d, err := Compile([]Region{{ID: "letter-stem", ThreadID: "navy", Geometry: rectangle(-2, -12, 4, 24), Kind: Satin, SpacingMM: .5, CenterUnderlay: true, ZigzagUnderlay: true}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	block := d.Plan[0]
	if len(block.Stitches) < 80 || len(block.Stitches)%2 != 0 {
		t.Fatalf("expected paired satin rungs, got %d stitches", len(block.Stitches))
	}
	center, zigzag := false, false
	for _, s := range block.Underlay {
		center = center || s.Source == "satin_center_underlay"
		zigzag = zigzag || s.Source == "satin_zigzag_underlay"
	}
	if !center || !zigzag {
		t.Fatalf("missing satin underlay: center=%v zigzag=%v", center, zigzag)
	}
}

func TestSatinRejectsUnsafeWidth(t *testing.T) {
	_, err := Compile([]Region{{ID: "too-wide", ThreadID: "red", Geometry: rectangle(-10, -10, 20, 20), Kind: Satin, SpacingMM: .5}}, DefaultProfile())
	if err == nil || !strings.Contains(err.Error(), "satin width") {
		t.Fatalf("expected satin width error, got %v", err)
	}
}

func TestSatinRejectsBranchingHole(t *testing.T) {
	p := Polygon{Rings: [][]Point{{{-5, -10}, {5, -10}, {5, 10}, {-5, 10}}, {{-2, -6}, {2, -6}, {2, -3}, {-2, -3}}, {{-2, 3}, {2, 3}, {2, 6}, {-2, 6}}}}
	_, err := Compile([]Region{{ID: "ambiguous", ThreadID: "red", Geometry: p, Kind: Satin, SpacingMM: .5}}, DefaultProfile())
	if err == nil || !strings.Contains(err.Error(), "split it into editable columns") {
		t.Fatalf("expected ambiguous rail error, got %v", err)
	}
}

func TestRingSatinClosesLetterform(t *testing.T) {
	p := Polygon{Rings: [][]Point{{{-6, -8}, {6, -8}, {6, 8}, {-6, 8}}, {{-3, -5}, {-3, 5}, {3, 5}, {3, -5}}}}
	d, err := Compile([]Region{{ID: "letter-o", ThreadID: "black", Geometry: p, Kind: Satin, SpacingMM: .5, ZigzagUnderlay: true}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	block := d.Plan[0]
	if len(block.Stitches) < 100 {
		t.Fatalf("ring satin is unexpectedly sparse: %d", len(block.Stitches))
	}
	if block.Stitches[0].Source != "ring_satin" {
		t.Fatalf("wrong stitch source %q", block.Stitches[0].Source)
	}
	if distance(block.Stitches[0].Position, block.Stitches[len(block.Stitches)-2].Position) > .001 {
		t.Fatal("ring satin did not close")
	}
}

func TestRoutingUsesNearestEntryWithinThread(t *testing.T) {
	regions := []Region{{ID: "far", ThreadID: "black", Geometry: rectangle(40, 40, 4, 10), Kind: Running}, {ID: "near", ThreadID: "black", Geometry: rectangle(2, 2, 4, 10), Kind: Running}}
	d, err := Compile(regions, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if d.Plan[0].RegionID != "near" {
		t.Fatalf("expected nearest block first, got %s", d.Plan[0].RegionID)
	}
}

func TestDSTAvoidsRedundantColorChangesAndReportsExtents(t *testing.T) {
	d, err := Compile([]Region{{ID: "one", ThreadID: "black", Geometry: rectangle(-10, -5, 4, 4), Kind: Running}, {ID: "two", ThreadID: "black", Geometry: rectangle(6, 5, 4, 4), Kind: Running}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	dst, err := EncodeDST(d, "METADATA")
	if err != nil {
		t.Fatal(err)
	}
	header := string(dst[:512])
	if !strings.Contains(header, "CO:  1") {
		t.Fatalf("unexpected color metadata: %q", header[:40])
	}
	decoded, err := DecodeDST(dst)
	if err != nil {
		t.Fatal(err)
	}
	jumps := 0
	colors := 0
	for _, s := range decoded {
		if s.Command == CommandJump {
			jumps++
		}
		if s.Command == CommandColorChange {
			colors++
		}
	}
	if jumps == 0 {
		t.Fatal("missing inter-block jump")
	}
	if colors != 0 {
		t.Fatalf("same-thread blocks emitted %d color changes", colors)
	}
	if strings.Contains(header, "+X:    0") || strings.Contains(header, "+Y:    0") {
		t.Fatal("DST header did not record positive extents")
	}
}

func TestSpineSatinFromCenterline(t *testing.T) {
	spine := Polygon{Rings: [][]Point{{{0, -10}, {0, 0}, {0, 10}}}}
	d, err := Compile([]Region{{ID: "script", ThreadID: "navy", Geometry: spine, Kind: Satin, WidthMM: 3, SpacingMM: .5, CenterUnderlay: true, ZigzagUnderlay: true}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	block := d.Plan[0]
	if len(block.Stitches) < 40 || len(block.Stitches)%2 != 0 {
		t.Fatalf("expected paired spine satin rungs, got %d", len(block.Stitches))
	}
	if block.Stitches[0].Source != "spine_satin" {
		t.Fatalf("wrong source %q", block.Stitches[0].Source)
	}
	width := distance(block.Stitches[0].Position, block.Stitches[1].Position)
	if math.Abs(width-3) > .05 {
		t.Fatalf("expected ~3 mm column width, got %.3f", width)
	}
	center, zigzag := false, false
	for _, s := range block.Underlay {
		center = center || s.Source == "spine_satin_center_underlay"
		zigzag = zigzag || s.Source == "spine_satin_zigzag_underlay"
	}
	if !center || !zigzag {
		t.Fatalf("missing spine underlay: center=%v zigzag=%v", center, zigzag)
	}
}

func TestSpineSatinRejectsMachineWidth(t *testing.T) {
	spine := Polygon{Rings: [][]Point{{{0, 0}, {0, 20}}}}
	_, err := Compile([]Region{{ID: "wide", ThreadID: "red", Geometry: spine, Kind: Satin, WidthMM: 20, SpacingMM: .5}}, DefaultProfile())
	if err == nil || !strings.Contains(err.Error(), "satin width") {
		t.Fatalf("expected width error, got %v", err)
	}
}

func TestTatamiEmptyFillFallsBackToRunning(t *testing.T) {
	// Height below spacing/2 so the scanline loop never runs.
	p := Polygon{Rings: [][]Point{{{0, 0}, {4, 0}, {4, 0.15}, {0, 0.15}}}}
	d, err := Compile([]Region{{ID: "sliver", ThreadID: "black", Geometry: p, Kind: Tatami, SpacingMM: .45, StitchLengthMM: 2}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Plan) != 1 {
		t.Fatalf("expected fallback plan block, got %d", len(d.Plan))
	}
	if d.Plan[0].Kind != Running {
		t.Fatalf("expected running fallback, got %s", d.Plan[0].Kind)
	}
	found := false
	for _, diag := range d.Diagnostics {
		if diag.Code == "TATAMI_FALLBACK_RUNNING" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected TATAMI_FALLBACK_RUNNING diagnostic")
	}
}

func TestTatamiPrefersNearestSegmentInRow(t *testing.T) {
	// Dumbbell so mid-height scanlines contain two fill segments.
	p := Polygon{Rings: [][]Point{{{0, 0}, {4, 0}, {4, 2}, {20, 2}, {20, 0}, {24, 0}, {24, 6}, {20, 6}, {20, 4}, {4, 4}, {4, 6}, {0, 6}}}}
	d, err := Compile([]Region{{ID: "islands", ThreadID: "black", Geometry: p, Kind: Tatami, SpacingMM: 1, StitchLengthMM: 2}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	jumps := 0
	for _, s := range d.Plan[0].Stitches {
		if s.Command == CommandJump {
			jumps++
		}
	}
	if jumps == 0 {
		t.Fatal("expected travel jumps between row segments")
	}
}

func TestCompileAvoidsShortStitchesOnJaggedContour(t *testing.T) {
	// Dense traced-style contour: many sub-millimetre edges that used to become short stitches.
	ring := []Point{{0, 0}}
	for i := 1; i <= 200; i++ {
		ring = append(ring, Point{X: float64(i) * 0.12, Y: math.Sin(float64(i)/8) * 0.08})
	}
	for i := 200; i >= 0; i-- {
		ring = append(ring, Point{X: float64(i) * 0.12, Y: 8 + math.Sin(float64(i)/8)*0.08})
	}
	d, err := Compile([]Region{{
		ID: "jagged", ThreadID: "black", Geometry: Polygon{Rings: [][]Point{ring}},
		Kind: Tatami, SpacingMM: .45, StitchLengthMM: 3, EdgeUnderlay: true,
	}}, DefaultProfile())
	if err != nil {
		t.Fatal(err)
	}
	for _, diag := range d.Diagnostics {
		if diag.Code == "STITCH_TOO_SHORT" {
			t.Fatalf("expected no short-stitch warnings, got %s: %s", diag.Code, diag.Message)
		}
	}
	minLen := DefaultProfile().MinStitchMM
	for _, block := range d.Plan {
		for _, sequence := range [][]Stitch{block.Underlay, block.Stitches} {
			for i := 1; i < len(sequence); i++ {
				if sequence[i].Command != CommandStitch {
					continue
				}
				n := distance(sequence[i-1].Position, sequence[i].Position)
				if n > 0 && n < minLen {
					t.Fatalf("short stitch %.3f mm at index %d source=%s", n, i, sequence[i].Source)
				}
			}
		}
	}
}
