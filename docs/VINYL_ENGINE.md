# PrintStudio Vinyl Cut Policy

PrintStudio treats **HTV** and **pressure-sensitive adhesive vinyl** as separate production systems. It exports a Clipper2-cleaned millimetre SVG with a weed box, then scores the cut geometry against a material class before download.

```text
Artwork (text / silhouette trace)
        |
        v
Clipper2 clean (union + hole preservation)
        |
        v
Material-class policy review
        |
        v
Cut SVG + review scorecard
        |
        v
Download only if hard stops clear
```

## Non-negotiable principles

1. HTV jobs default to **mirrored**; adhesive jobs default to **non-mirrored**.
2. Feature warnings and rejects are **material-class specific**, not a single universal millimetre.
3. Counters (holes) are first-class risk zones; tiny interiors get `COUNTER_RISK` warnings.
4. Operators must still **test-cut** when material, blade, software path, or feature scale changes.
5. PrintStudio does not claim machine-brand force/speed numbers as absolute; profiles carry guidance notes only.

## Material classes

| Class | Family | Warn | Reject | Mirror | Guidance |
|---|---|---:|---:|---|---|
| `htv-smooth` | HTV | 1.0 mm | 0.6 mm | on | EasyWeed-class smooth PU; carrier-led weeding |
| `htv-flock` | HTV | 1.2 mm | 0.8 mm | on | Higher force / slower send than smooth |
| `htv-glitter` | HTV | 1.5 mm | 1.0 mm | on | Specialty glitter lane |
| `adhesive-permanent` | adhesive | 1.0 mm | 0.6 mm | off | 651-class; shallow liner score; transfer tape after weed |
| `adhesive-removable` | adhesive | 1.0 mm | 0.6 mm | off | 631-class; release/tack sensitive |
| `adhesive-glitter` | adhesive | 1.5 mm | 1.0 mm | off | 851-class specialty |

These thresholds are **conservative house defaults** until a shop’s own threshold file overrides them.

## File-prep doctrine (export hygiene)

PrintStudio’s vinyl path already:

- Traces a single opaque silhouette (filled contours, not centreline)
- Preserves interior cutouts with even-odd compound paths
- Fits SVG viewBox and weed box to cut content
- Optionally mirrors for HTV

Operators should still convert live text to outlines in source art when sending to cutter ecosystems that drop editable text, flatten clipping constructions for Cricut-bound SVGs, and expand strokes that must become physical cut shapes (especially Roland CutStudio).

## API

`POST /v1/vinyl/review`

Request:

```json
{
  "materialClass": "htv-smooth",
  "mirrored": true,
  "paths": [[{"x": 0, "y": 0}, {"x": 20, "y": 0}, {"x": 20, "y": 20}, {"x": 0, "y": 20}]]
}
```

Response includes `profile`, `diagnostics`, `review` (score / decision / factors), and `mirrorRecommended`.

Diagnostic codes:

- `FEATURE_TOO_SMALL` (error) — min bbox axis below reject threshold
- `FEATURE_BORDERLINE` (warning) — below warn threshold
- `COUNTER_RISK` (warning) — interior hole below warn threshold

Review decisions: `auto`, `semi-auto`, `human`, `blocked`. Download is blocked when decision is `blocked` or any diagnostic severity is `error`.

## What PrintStudio does not do yet

- Per-brand blade force/speed send presets as machine commands
- Cricut / Silhouette / Roland / Graphtec native interchange packs beyond SVG
- Automated weed-line generation beyond the rectangular weed box
- Physical test-protocol UI or measured results worksheet

## Operator checklist

1. Pick the correct **material class**.
2. Confirm **mirror** matches HTV vs adhesive intent.
3. Export and read the **review scorecard**.
4. Resolve hard stops (enlarge detail or change material) before download.
5. Run a **test cut** on the target machine/blade before production volume.
