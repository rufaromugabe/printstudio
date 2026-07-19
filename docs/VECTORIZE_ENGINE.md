# Vectorize engine

## Purpose

Advanced vectorization turns image-layer artwork into high-quality polygon contours before vinyl cut, embroidery compile, or screen separation. Editable text stays on the glyph/canvas tracer. There is no browser mask fallback for images.

## Pipeline

1. **ML prep (optional)** — a provider-neutral gateway can remove a background or reconstruct a low-resolution source. It never emits cut or DST coordinates.
2. **Content analysis** — deterministic preflight classifies raster lettering, flat logos/illustrations and continuous-tone pictures. This is structural detection, not OCR; it never guesses or rewrites characters.
3. **Foreground isolation** — real transparency is preserved. Opaque PNG/JPEG logos use border-colour segmentation with a luminance fallback, so a white page does not become one rectangular cut path.
4. **Method polish** — one-pixel defects and speckles are repaired, small sources receive bounded Catmull-Rom edge supersampling, and vinyl, embroidery, screen and DTF select separate production profiles.
5. **Potrace** — server `NativeTools.TracePBMWithOptions` applies profile-specific speckle, corner, curve-optimization and turn-policy values.
6. **SVG → rings** — path data is sampled into closed polygons; Potrace transforms are applied and redundant sampled vertices are removed within a bounded shape tolerance.
7. **Clipper2 cleanup** — `offset(0)` when the Clipper2 build tag is enabled.
8. **Visual similarity QA** — final rings are rasterized back onto a bounded proof canvas and compared with the exact trace mask. IoU, precision, recall, tolerant edge F1, connected components and enclosed counters contribute to a score. Missing text counters or severe shape loss fail closed.
9. **OCR reconstruction (text-like artwork)** — production images use Tesseract TSV OCR when available. The browser renders the recognized text across every studio font family and weight, measures mask IoU against the source, and ranks the candidates. Raster artwork is only replaced after OCR and font-shape confidence pass and the operator explicitly confirms.
10. **Server colour separation** — `mode=color` uses deterministic weighted k-means in CIE Lab, merges near-identical clusters, reports mean/P95 ΔE00 loss, and sends each separation through the same trace and similarity gates.
11. **Canonical IR** — `VectorContourSet` includes rings, `sourceHash`, tracer, diagnostics, an auditable `prep` report, similarity proof, and OCR metadata.

Same polished mask + method/profile → same `sourceHash` and reproducible Potrace/Clipper reruns.

## API

```text
POST /v1/production/vectorize?method=vinyl|embroidery|screen|dtf&mlPrep=true|false&alphaCutoff=32
```

Additional controls:

- `proof=true` includes a PNG overlay: green = matched, red = source detail missing from vectors, blue = extra vector fill.
- `ocr=false` disables OCR for latency-sensitive calls; OCR is otherwise attempted only for text-like artwork.
- `mode=color&maxColors=8` returns server-owned Lab separations and colour-similarity metrics.

- **PNG/JPEG body** — optional `X-PrintStudio-Placement` JSON maps image-local pixels into centered print-area millimetres.
- **JSON body** — `{ assetId | imageBase64, method, mlPrep, alphaCutoff, placement }`.

Fails closed with HTTP 501 when Potrace is missing.

## ML prep env

| Variable | Default | Role |
|---|---|---|
| `PRINTSTUDIO_AI_PROVIDER` | `stub` | `stub` / `none` |
| `PRINTSTUDIO_AI_VECTOR_UPSCALE` | `1` | Integer scale (>1 enables upscale) |
| `PRINTSTUDIO_AI_VECTOR_REMOVE_BG` | `false` | Stub background cleanup when `true` |
| `TESSERACT_BIN` | `tesseract` on production image | Optional real OCR for raster lettering |

Credit metering is stubbed via `CreditHook` for a future ledger.

## Client

- Image layers always call `/v1/production/vectorize` (requires `vectorTrace`)
- Text layers use glyph/canvas trace
- Deterministic content-aware polish always runs; the studio switch adds optional provider prep before it
- Production review displays the visual similarity overlay and OCR confidence
- OCR rebuild is opt-in: high-confidence text can be converted to an editable studio text layer; uncertain characters never overwrite artwork
- Multi-colour image tracing is owned by the Go service using perceptual Lab clustering, not browser RGB buckets
- Final DTF/sublimation raster scaling uses Catmull-Rom reconstruction, and rotated artwork uses inverse bilinear sampling to avoid underbase pinholes
- Potrace unavailable + any image layer → hard error
