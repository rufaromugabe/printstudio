# Production Export Engine

PrintStudio dispatches the studio Export action according to the selected decoration method. Dimensions are always physical millimetres (never browser CSS pixels). DTF, vinyl and screen exports size to the inked/cut artwork content; sublimation keeps the full product print area plus bleed.

## DTF

Current output:

- Transparent 300-DPI PNG
- Physical size cropped to opaque artwork (plus a small production margin), not the full garment print area
- Styled, rotated and circular text
- Original transparent artwork compositing
- Effective-DPI calculation at placed size
- Warning below 300 DPI and stronger warning below 150 DPI
- 36-megapixel browser safety ceiling
- One-click server-generated production ZIP containing `color.png`, `white-underbase.png`, `manifest.json` and operator instructions
- Exact Euclidean underbase spread/choke generated from the colour PNG alpha channel
- SHA-256, byte size, MIME type, physical dimensions, pixel dimensions, DPI and underbase settings recorded in the package manifest

The colour and generic white-underbase separations are artwork-ready inputs to a DTF workflow. Printer-specific white density, ink limiting, ICC conversion, curing parameters and final RIP screening remain operator responsibilities. PrintStudio must not claim that a generic package replaces those device-specific stages.

## Heat-transfer vinyl

Current output:

- Millimetre-based SVG cut paths
- Transparent-hole preservation using even-odd paths
- Mirroring enabled by default and switchable in the review dialog
- Multiple disconnected contours
- ViewBox, weed box and declared size fitted to cut contours (plus margin)
- Warning for details below 1 mm
- Clipper2 union cleanup is required; exports fail closed when `polygonBoolean` is unavailable

Remaining specialist work includes multi-colour registration marks and blade/material profiles.

## Screen printing

Current output:

- Layered SVG grouped by ink colour
- Physical millimetre dimensions fitted to inked separations (plus margin)
- Solid vector silhouettes for traced elements
- Colour-count warning above eight inks
- Explicit warning when raster artwork would require halftone separation
- Server production ZIP with continuous-tone C, M, Y and K masks
- Deterministic 45-LPI AM screens using C=15°, M=75°, Y=0° and K=45° defaults
- Configurable DPI, LPI, gamma and underbase choke
- Generic white-underbase plate plus integrity manifest

The layered SVG remains appropriate for solid spot-colour artwork. The process package is explicitly uncalibrated CMYK: mesh, dot gain, ink set, substrate, trapping and screen-angle conflicts require operator verification. ICC-calibrated conversion remains capability-gated on libvips/LittleCMS.

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
- The server gang compositor accepts validated PNG artwork and can build sheets up to the bounded `MAX_RENDER_PIXELS` limit (100 megapixels by default).
- Server pack and gang requests reject pixel dimensions that disagree with declared millimetres and DPI; silent upscaling is not permitted.
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
- DTF gang sheets use deterministic MaxRects placement, original physical size, optional 90-degree rotation, configurable sheet dimensions, copy count, margins and spacing.
- Gang-sheet compositing runs in Go, uses premultiplied-alpha bilinear resampling when rounding requires it, and preserves transparent unused sheet area.
- Vinyl SVG automatically removes duplicate traced paths and adds a weed box.

## Next production milestones

1. Move single-artwork scene reconstruction to asynchronous server workers; the current server ownership covers packaging and gang-sheet pixel compositing, while the initial colour tile is still rendered in the browser at up to 36 megapixels.
2. Add gang-sheet cut contours and multi-artwork irregular nesting.
3. Expand customer Pantone/Lab ink libraries and press-measured ΔE targets.
4. Add sublimation panel splitting and product-specific seam templates.
5. Add server-backed export history shared across devices and team members.
