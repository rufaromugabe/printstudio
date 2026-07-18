# Vectorize engine

## Purpose

Advanced vectorization turns image-layer artwork into high-quality polygon contours before vinyl cut, embroidery compile, or screen separation. Editable text stays on the glyph/canvas tracer. There is no browser mask fallback for images.

## Pipeline

1. **ML prep (optional)** — provider-neutral gateway cleans the raster (background harden / optional remove / upscale). It never emits cut or DST coordinates.
2. **Alpha → PBM** — opaque pixels become a binary mask.
3. **Potrace** — server `NativeTools.TracePBM` emits SVG paths (`POTRACE_BIN` or `potrace` on PATH).
4. **SVG → rings** — path data is sampled into closed polygons; Potrace group transforms are applied.
5. **Clipper2 cleanup** — `offset(0)` when the Clipper2 build tag is enabled.
6. **Quality gates** — path/point caps, min-feature checks, hole diagnostics.
7. **Canonical IR** — `VectorContourSet` with rings (px or mm), `sourceHash`, tracer (`potrace` | `ml-assisted`), and metrics.

Same prepared raster + settings → same `sourceHash` and reproducible Potrace/Clipper reruns.

## API

```text
POST /v1/production/vectorize?method=vinyl|embroidery|screen&mlPrep=true|false&alphaCutoff=32
```

- **PNG/JPEG body** — optional `X-PrintStudio-Placement` JSON maps image-local pixels into centered print-area millimetres.
- **JSON body** — `{ assetId | imageBase64, method, mlPrep, alphaCutoff, placement }`.

Fails closed with HTTP 501 when Potrace is missing.

## ML prep env

| Variable | Default | Role |
|---|---|---|
| `PRINTSTUDIO_AI_PROVIDER` | `stub` | `stub` / `none` |
| `PRINTSTUDIO_AI_VECTOR_UPSCALE` | `1` | Integer scale (>1 enables upscale) |
| `PRINTSTUDIO_AI_VECTOR_REMOVE_BG` | `false` | Stub background cleanup when `true` |

Credit metering is stubbed via `CreditHook` for a future ledger.

## Client

- Image layers always call `/v1/production/vectorize` (requires `vectorTrace`)
- Text layers use glyph/canvas trace
- Studio setting **Advanced vectorize (ML)** toggles optional ML prep only
- Potrace unavailable + any image layer → hard error
