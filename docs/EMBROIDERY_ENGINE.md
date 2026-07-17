# PrintStudio Embroidery Compiler

## Product vision

PrintStudio Embroidery Compiler converts editable vector artwork and embroidery-aware text into auditable stitch plans. It is not a file-extension converter and must never imply that arbitrary artwork is production-safe merely because a DST or PES file was produced.

The system is designed like a compiler:

```text
SVG / text / approved raster trace
        |
        v
Canonical geometry and semantic regions
        |
        v
Stitch-type planning and region decomposition
        |
        v
Underlay, compensation and density planning
        |
        v
Stitch generation
        |
        v
Constraint-aware sequencing and travel routing
        |
        v
Machine-profile validation and simulation
        |
        v
DST / PES adapter + production report
```

Success is measured by sew quality, repeatability and operator trust—not by stitch count alone.

## Non-negotiable principles

1. Preserve editable source geometry separately from generated stitches.
2. Use millimetres internally; quantize only inside machine encoders.
3. Make every compiler pass deterministic and versioned.
4. Treat fabric, stabilizer, thread, needle, hoop and machine as inputs.
5. Emit warnings instead of silently repairing ambiguous artwork.
6. Keep learned models optional and bounded by hard safety constraints.
7. Require sew-out calibration before claiming production readiness for a new profile.
8. Store the exact compiler, profile and source versions used for every order.

## Supported initial scope

### Inputs

- Sanitized SVG paths
- Editable text converted through licensed font outlines
- Simple PNG/JPEG artwork after an explicit trace-and-review step
- Existing canonical embroidery documents for regeneration

### Stitch families

- Running stitch
- Triple/bean stitch
- Satin columns
- Tatami fills
- Contour fills
- Manual stitch blocks
- Underlay: centre walk, edge walk, zigzag and lattice

### Outputs

- Canonical JSON stitch plan
- SVG diagnostic preview
- PNG production simulation
- Tajima DST as the first machine encoder
- PES only after version-specific compatibility fixtures are available
- Human-readable production report

Brother documents that machine compatibility depends on supported formats, hoop dimensions, stitch count and colour limits. Output validation therefore uses a selected machine profile rather than a generic “valid PES” flag.

## Canonical intermediate representation

Machine formats are lossy targets. The canonical document retains semantic information that DST cannot represent.

```go
type Document struct {
    Version         int
    Units           string // always "mm"
    SourceHash      string
    CompilerVersion string
    Hoop            Hoop
    Material        MaterialProfile
    ThreadPalette   []Thread
    Regions         []Region
    Plan            []Block
    Diagnostics     []Diagnostic
}

type Region struct {
    ID          string
    Geometry    PolygonSet
    Role        RegionRole
    Stitch      StitchPolicy
    Constraints RegionConstraints
}

type Block struct {
    ID         string
    RegionID   string
    ThreadID   string
    Kind       BlockKind
    Underlay   []Stitch
    Stitches   []Stitch
    Entry      Point
    Exit       Point
    Bounds     Bounds
    DependsOn  []string
}

type Stitch struct {
    Position Point
    Command  Command // stitch, jump, trim, color_change, stop, end
    Source   string  // compiler pass responsible for this command
}
```

The IR must serialize with a schema version and pass round-trip tests. Generated files reference an immutable IR version.

## Compiler passes

### 1. Input normalization

- Sanitize SVG and reject scripts, external resources and unsupported filters.
- Resolve transforms and convert shapes to paths.
- Flatten curves according to physical tolerance, not screen pixels.
- Normalize winding and repair only well-defined self-intersections.
- Identify holes and nested regions.
- Convert all coordinates to millimetres.
- Reject geometry smaller than the active material/profile limits.

### 2. Semantic classification

Classify regions using explainable features:

- Local width distribution
- Area and perimeter
- Curvature
- Aspect ratio
- Hole topology
- Neighbouring colours and overlaps
- User intent and selected production profile

Rules choose a candidate stitch family. The UI shows the choice and permits an expert override. A confidence score is informational and never bypasses hard constraints.

### 3. Satin-column planning

Medial-axis geometry is a candidate centreline generator, not an automatic guarantee of a good satin column.

Pipeline:

1. Sample and simplify the polygon boundary.
2. Compute a distance field or constrained Voronoi skeleton.
3. Prune unstable branches using local feature size.
4. Split at junctions and high-curvature points.
5. Construct paired rails with consistent correspondence.
6. Check maximum satin length and acute-turn risk.
7. Generate perpendicular or smoothly varying rungs.
8. Insert corner strategies: capped, mitered, split or turning satin.

Ambiguous branches become separate reviewed regions. Wide columns fall back to split satin or fill according to the profile.

### 4. Fill planning

The first dependable fill is scanline tatami:

- Rotate geometry into fill space.
- Intersect each scanline with all polygon rings.
- Pair intersections using the even-odd rule.
- Exclude holes.
- Alternate traversal direction.
- stagger needle penetrations to avoid visible columns.
- Subdivide stitches to satisfy minimum and maximum length.
- Apply edge inset, underlay and tie-in/tie-off policies.

Later fill strategies can share the same region and constraint interfaces:

- Contour-parallel
- Spiral
- Curvilinear vector-field
- Variable-density artistic fills
- Image-guided tonal fills

Variable density must enforce minimum spacing, maximum local penetration count and fabric-profile tension budgets.

### 5. Underlay

Underlay is planned before top stitching and participates in routing.

Inputs:

- Region width and curvature
- Top-stitch angle
- Fabric stretch and pile
- Stabilizer
- Thread weight
- Desired coverage

Outputs:

- Underlay family
- Inset
- Stitch length
- Density
- Direction
- Entry/exit points

The validator checks that underlay stays inside compensated boundaries and does not create excessive penetration clusters.

### 6. Compensation model

Start with calibrated, interpretable compensation:

```text
compensation = base(profile)
             + width_factor(local_span)
             + density_factor(local_density)
             + direction_factor(stitch_angle, fabric_grain)
             + layer_factor(overlap_depth)
```

Compensation is anisotropic: expand primarily across stitch direction, not uniformly around the shape.

An FEA or learned residual model belongs behind the same interface only after collecting paired input/sew-out measurements. It predicts a bounded correction to the calibrated model. Hard limits clamp its output, and the production report records whether it was used.

### 7. Sequencing and travel routing

The routing problem is a precedence-constrained, asymmetric path problem—not a plain travelling-salesman problem.

The graph contains:

- Stitch blocks as nodes
- Candidate entry/exit variants
- Colour-change costs
- Trim costs
- Jump distance and visibility costs
- Travel-stitch costs
- Layering dependencies
- “must sew before” constraints
- Region containment and cover opportunities

Use a practical staged optimizer:

1. Topologically partition by hard dependencies.
2. Group compatible thread colours when layering permits.
3. Choose block orientation and entry/exit candidates.
4. Construct a greedy feasible route.
5. Improve with 2-opt/or-opt moves that preserve dependencies.
6. Replace eligible jumps with travel stitches only when a later block fully covers them.
7. Insert trims according to machine and garment rules.

Zero trims is not a universal goal. The objective is the lowest safe weighted cost without visible travel or unstable long floats.

### 8. Hardware lowering

Each machine profile defines:

- Supported format/version
- Hoop bounds
- Coordinate resolution
- Maximum encoded movement
- Maximum design stitches and colours
- Trim behaviour
- Colour-change semantics
- Needle/thread mapping support

Long movements are subdivided without changing command semantics. Coordinates are quantized with accumulated-error correction so rounding does not distort the overall design.

## Quality validator

Every compile produces machine-readable diagnostics.

### Errors

- Design outside hoop
- Unsupported encoder/profile combination
- Unresolved self-intersection
- Movement cannot be represented safely
- Missing thread or machine profile
- Empty stitch plan
- Corrupt machine-format round trip

### Warnings

- Satin span too wide or too narrow
- Stitch shorter/longer than profile limit
- Excessive local density
- Too many penetrations in a small radius
- Uncovered travel stitch
- Long jump or float
- High trim count
- Small text/detail risk
- Insufficient underlay
- Excessive layer count
- Estimated stitch count beyond machine/profile recommendation

### Metrics

- Stitch, jump, trim and colour-change counts
- Thread length per colour
- Estimated run time
- Bounds and hoop utilization
- Density heat-map extrema
- Penetration-cluster score
- Maximum satin span
- Maximum movement
- Route cost and covered-travel ratio

## Simulation

The preview is diagnostic, not merely decorative.

Render:

- Thread-width strokes with directional shading
- Underlay toggle
- Stitch-by-stitch playback
- Jump and trim visualization
- Needle-penetration heat map
- Density and tension-risk overlays
- Fabric/profile comparison
- Entry, exit and block-order labels

A later calibrated deformation preview can warp the fabric texture, but it must state its confidence and profile provenance.

## Go package structure

```text
/embroidery
  /model          canonical IR and schema migrations
  /profile        fabric, thread, needle, hoop and machine profiles
  /input/svg      secure SVG normalization
  /input/font     font outline shaping and licensing metadata
  /geometry       polygon operations, offsets, clipping and distance fields
  /classify       explainable stitch-family selection
  /satin          skeleton pruning, rails, rungs and corner strategies
  /fill           tatami, contour and experimental vector-field fills
  /underlay       underlay policies and generators
  /compensate     calibrated anisotropic compensation
  /route          precedence graph and route improvement
  /validate       geometry, stitch, density and machine diagnostics
  /simulate       SVG/raster diagnostic rendering
  /encode/dst     Tajima encoder and decoder
  /encode/pes     isolated, version-specific PES adapters
  /compiler       deterministic pass orchestration
  /fixtures       golden designs and known machine files
```

Geometry packages should be pure where practical. Encoders must not make design decisions; they only validate and lower an approved stitch plan.

## API boundary

```http
POST /v1/embroidery/compile
GET  /v1/embroidery/jobs/{id}
GET  /v1/embroidery/jobs/{id}/diagnostics
GET  /v1/embroidery/jobs/{id}/simulation
POST /v1/embroidery/jobs/{id}/approve
GET  /v1/embroidery/jobs/{id}/files/{format}
```

Compile request:

```json
{
  "designVersionId": "uuid",
  "viewId": "left_chest",
  "hoopProfileId": "hoop-130x180",
  "machineProfileId": "brother-profile-example",
  "materialProfileId": "pique-polo-medium",
  "threadProfileId": "polyester-40wt",
  "intent": "production",
  "overrides": []
}
```

Production files are unavailable until validation succeeds and the user approves the simulation. An approval snapshots the IR, profiles, diagnostics and compiler version.

## Testing strategy

### Unit and property tests

- Polygon intersection and hole handling
- Curve flattening tolerance
- Offset topology
- Stitch-length bounds
- Coordinate quantization
- DST command encoding/decoding
- Determinism: identical inputs produce identical IR and bytes
- No NaN/Inf or out-of-hoop stitches

### Golden fixtures

- Narrow and widening satin columns
- Acute and rounded corners
- Donut and nested-hole fills
- Small lettering
- Multi-colour overlapping regions
- Long jumps and unavoidable trims
- Maximum hoop boundary
- Corrupt and hostile SVG inputs

### Differential checks

- Decode every encoded file and compare canonical stitch commands.
- Import fixtures into independent viewers when licensing permits.
- Test on representative machines before marking a profile verified.

### Physical sew-out programme

Use calibration targets across fabrics, stabilizers, thread weights and angles. Photograph or scan sew-outs with a scale marker, register them to expected geometry and record measured pull, push, gaps, puckering and registration error. Profile changes require regression sew-outs.

## Delivery roadmap

### Milestone 1 — trustworthy stitch core

- Canonical IR
- Running stitch and tatami fill
- Underlay primitives
- Machine profiles
- DST encoder/decoder
- SVG diagnostic simulation
- Determinism and golden tests

### Milestone 2 — professional satin and lettering

- Skeleton-assisted satin columns
- Rail editing and corner strategies
- Font shaping and embroidery lettering profiles
- Anisotropic calibrated compensation
- Density and penetration heat maps

### Milestone 3 — routing and production workflow

- Precedence-aware sequencing
- Covered travel planning
- Trim optimization
- Approval, immutable versions and production reports
- Real machine compatibility matrix

### Milestone 4 — calibrated intelligence

- Sew-out measurement pipeline
- Profile fitting
- Bounded learned compensation residuals
- Confidence reporting

### Milestone 5 — generative stitch art

- Vector-field and contour fills
- Variable-density artistic patterns
- User-authored field maps
- Experimental features isolated from verified production profiles

## Honest market advantage

The defensible advantage is not an unsupported claim that a mathematical trick beats every commercial package. It is a reproducible system that combines:

- Editable semantic source documents
- Explainable automatic digitization
- Profile-calibrated compensation
- Constraint-aware routing
- Transparent diagnostics
- Deterministic regeneration
- Automated machine-format round-trip testing
- Measured physical sew-out feedback

That turns embroidery preparation from a black-box export into an auditable production compiler.
