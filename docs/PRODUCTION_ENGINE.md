# Go Production Engine

## Purpose

The Go production engine is the authoritative prepress layer. Browser output remains useful for interactive preview, while server algorithms provide deterministic, bounded and testable production transformations.

## Implemented algorithms

### DTF white underbase and choke/spread

`production/morphology.go` computes an exact squared Euclidean distance transform in linear time with respect to pixel count. The radius does not multiply runtime.

- Positive radius spreads/dilates the alpha mask.
- Negative radius chokes/erodes the alpha mask.
- Alpha threshold is configurable.
- Empty and fully opaque masks have defined behavior.
- The output is an 8-bit alpha PNG suitable for an underbase separation.

This avoids the shape bias introduced by repeated square or cross kernels.

### AM and FM screening

`production/halftone.go` generates deterministic rotated round-dot AM screens and a stochastic FM screen.

- Configurable DPI / LPI / screen angle / tone gamma for AM
- FM mode uses a fixed dispersed-dot threshold tile (no angle set required)
- Alpha-aware coverage
- Monotonic tone coverage tests

Default studio AM action: 300 DPI, 45 LPI, 22.5 degrees.

Screen-angle conflict detection rejects colliding AM sets before packaging.

### CMYK separations

The engine produces four 8-bit separation masks with gray-component replacement. This conversion is explicitly labelled device-independent and uncalibrated. Calibrated production must use ICC transforms.

### Gang-sheet optimization

`production/nesting.go` implements deterministic MaxRects best-short-side-fit placement.

- Quantity expansion
- Optional 90-degree rotation
- Sheet margins and inter-item gaps
- Stable area-first ordering
- Free-rectangle splitting and containment pruning
- Hard failure when an item cannot fit
- Non-overlap and boundary regression tests
currently does expot 
`production/packs.go` uses those placements to composite transparent PNG sheets on the server. It caches normal and rotated source tiles, uses premultiplied-alpha bilinear interpolation when pixel rounding requires resampling, and enforces the `MAX_RENDER_PIXELS` ceiling (100 megapixels by default).

### Production packages

The DTF package contains the original validated colour PNG and an exact-Euclidean white-underbase plate. The screen package contains continuous and screened C/M/Y/K plates plus a choked underbase. Every generated plate is SHA-256 hashed in a versioned manifest.

Physical dimensions, DPI and PNG pixel dimensions are cross-checked before packaging. The engine rejects inconsistent metadata rather than silently resampling production artwork.

## Native production capabilities

### libvips and LittleCMS

The `NativeTools` adapter executes libvips with argument arrays rather than shell strings. It validates input and ICC profile paths and permits only the four known rendering intents.

Set `VIPS_BIN` when `vips` is not on the service path. The capability endpoint reports whether ICC processing is genuinely available.

libvips is used because it provides streaming, low-memory image processing and wraps LittleCMS for ICC import, export and device-profile transformation.

### Potrace

The adapter accepts PBM input and produces SVG using Potrace. Set `POTRACE_BIN` when the executable is not on the service path. The engine reports vector tracing as unavailable if the binary cannot be resolved.

### Polygon Boolean operations

The API has an optional native Clipper2 backend through `github.com/epit3d/goclipper2`, pinned to `v0.0.9`. It provides fixed-point polygon union, difference, intersection, XOR and closed-polygon offsets. Coordinates are quantized to one-micron integer units and converted back to millimetres.

The adapter is isolated behind the `clipper2` build tag because the Go module is a CGO wrapper and needs a C++ compiler plus the correct x64 Clipper2 runtime library. A standard build remains CGO-free and reports `polygonBoolean: false`; it returns HTTP 501 for Boolean and offset requests instead of silently using a less capable algorithm. See `services/api/CLIPPER2.md` for activation instructions.

Native object lifetimes are explicitly released and incoming paths are checked for valid finite coordinates and at least three points. The Clipper2-enabled build still requires native compilation, memory-safety and geometry-corpus verification in the deployment environment before it should be promoted as a production capability.

## API

Authenticated routes:

```text
GET  /v1/production/capabilities
POST /v1/production/dtf/underbase?spread=2&threshold=1
POST /v1/production/dtf/pack
POST /v1/production/screen/halftone?dpi=300&lpi=45&angle=22.5&gamma=1&mode=am|fm
POST /v1/production/screen/cmyk
POST /v1/production/screen/pack
POST /v1/production/screen/angles
POST /v1/production/spot/match
GET  /v1/production/icc/profiles
POST /v1/production/icc/profiles?id=&label=
POST /v1/production/icc/transform?sourceProfile=&destinationProfile=&intent=
POST /v1/production/gang/nest
POST /v1/production/gang/render
POST /v1/production/vector/boolean
POST /v1/production/vector/offset
```

Raster requests accept PNG or JPEG bodies up to 50 MB and reject decoded images above 100 megapixels.

Set `ICC_PROFILE_DIR` to enable profile upload/versioning. ICC transforms require both that store and a resolvable `vips` binary (`VIPS_BIN`).

## Verification

The test suite covers:

- Euclidean spread and choke geometry
- Empty and fully opaque masks
- Halftone determinism and monotonic coverage
- CMYK primary behavior
- Gang-sheet determinism
- Boundary containment and non-overlap
- Missing native capability reporting
- HTTP PNG decoding and responses
- HTTP nesting contracts
- Capability-gated polygon endpoint contracts in the portable build
- DTF ZIP contents and manifest integrity fields
- Screen plate count and ink-coverage polarity
- Server gang-sheet dimensions, placement count and transparency
- Transparent-edge colour preservation during resampling

## Remaining moat work

1. Compile the tagged Clipper2 backend in a CGO-capable CI runner and add adversarial geometry, fuzz and native leak tests.
2. Install libvips/LittleCMS in production images and load real printer ICC profiles into `ICC_PROFILE_DIR`.
3. Expand the named-ink library with customer Pantone/Lab libraries and press-measured ΔE targets.
4. Add measured print feedback into versioned trap presets and production profiles.
5. Add sublimation panel splitting and product-specific seam templates.
