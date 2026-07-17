# Production Export Engine

PrintStudio dispatches the studio Export action according to the selected decoration method. Every output uses the active product view's physical dimensions rather than browser pixels.

## DTF

Current output:

- Transparent 300-DPI PNG
- Exact physical print-area dimensions
- Styled, rotated and circular text
- Original transparent artwork compositing
- Effective-DPI calculation at placed size
- Warning below 300 DPI and stronger warning below 150 DPI
- 36-megapixel browser safety ceiling

The PNG is artwork-ready, but printer-specific white-underbase generation, ink limiting, ICC conversion and RIP screening remain responsibilities of the DTF RIP. PrintStudio must not claim that a generic PNG replaces those device-specific stages.

## Heat-transfer vinyl

Current output:

- Millimetre-based SVG cut paths
- Transparent-hole preservation using even-odd paths
- Mirroring enabled by default and switchable in the review dialog
- Multiple disconnected contours
- Warning for details below 1 mm
- Boundary-fallback disclosure when source pixels cannot be read

Remaining specialist work includes path-union cleanup, overlap removal, automatic weed boxes, multi-colour registration marks and blade/material profiles.

## Screen printing

Current output:

- Layered SVG grouped by ink colour
- Physical millimetre dimensions
- Solid vector silhouettes for traced elements
- Colour-count warning above eight inks
- Explicit warning when raster artwork would require halftone separation

The current output is appropriate for solid spot-colour artwork. Process-colour separation, simulated process, underbase choking, trapping, halftone frequency/angle and device-specific RIP output are not yet generated.

## Sublimation

Current output:

- Transparent 300-DPI PNG
- Product-configured bleed on every edge
- Effective raster-DPI validation
- Full-coverage warning when artwork does not span the canvas
- Styled, rotated and circular text

Printer/paper ICC conversion and product-specific seam warping remain downstream production stages.

## Safety and reproducibility

- Output filenames are sanitized.
- Browser rendering is rejected over 36 megapixels to prevent memory exhaustion.
- Cross-origin artwork that cannot be decoded fails raster export instead of silently disappearing.
- Vector tracing reports any boundary fallback.
- The review dialog shows dimensions, output format, warnings and method-specific controls before download.
- Every downloaded artifact is stored in IndexedDB with its SHA-256 digest, physical dimensions, method, MIME type and creation time.
- Recent artifacts can be downloaded again without regenerating the design.
- Production ZIP packages contain the primary file, `manifest.json` and operator instructions.

## Alternate formats

- PDF uses the exact physical page dimensions and embeds the reviewed production artwork.
- TIFF is encoded from the reviewed artwork at its production pixel dimensions with 300-DPI resolution metadata.
- ZIP packages use DEFLATE compression and include a SHA-256 manifest for integrity checking.
- DTF gang sheets use original-size tiles, configurable sheet dimensions, copy count and 5 mm spacing.
- Gang sheets that exceed the 36-megapixel browser ceiling are rejected instead of being silently downsampled.
- Vinyl SVG automatically removes duplicate traced paths and adds a weed box.

## Next production milestones

1. Server-side rendering workers for larger than 36-megapixel files.
2. Irregular DTF gang-sheet nesting, automatic rotation and cut-line generation.
3. Geometric vinyl path union, overlap subtraction and configurable registration marks.
4. Screen-print spot separation, trapping and configurable halftones.
5. ICC-aware colour-conversion pipeline with embedded profile metadata.
6. Sublimation panel splitting and product-specific seam templates.
7. Server-backed export history shared across devices and team members.
