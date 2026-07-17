# PrintStudio

PrintStudio is a multi-user product-customisation SaaS foundation. It includes PostgreSQL cloud persistence, workspace isolation, immutable versions, expiring share links, authenticated APIs, resilient autosave, and a verified original-artwork pipeline backed by S3-compatible object storage.

## Run it

Requirements: Node.js 22+ and Go 1.24+.

```bash
npm install
npm run dev
```

Open `http://localhost:3000`. To run the API separately:

```bash
cd services/api
go run .
```

Or run the whole local stack with `docker compose up --build`. Development mode creates a seeded user and workspace; production must set `AUTH_MODE=jwt` and provide a strong `JWT_SECRET` to verify gateway-issued HS256 access tokens. Copy `.env.example` as the configuration reference.

## Repository map

- `src/app/page.tsx` — interactive editor and canonical design document
- `src/app/globals.css` — responsive editor UI and garment mockup
- `services/api` — Go/PostgreSQL multi-tenant API, migrations and tests
- `compose.yaml` — web, API, PostgreSQL and Redis local topology
- `docs/ARCHITECTURE.md` — service boundaries and delivery roadmap
- `docs/PRODUCT_TEMPLATES.md` — configurable products, views, physical sizes and properties
- `docs/EMBROIDERY_ENGINE.md` — production-grade embroidery compiler architecture and roadmap
- `docs/PRODUCTION_EXPORTS.md` — DTF, vinyl, screen-print and sublimation output guarantees and boundaries
- `docs/PRODUCTION_ENGINE.md` — Go prepress algorithms, native colour/vector adapters and verification
- `services/api/embroidery` — deterministic embroidery core with running/tatami generation, validation, DST round trips and SVG diagnostics
- `src/lib/embroidery-digitizer.ts` — browser contour tracing for styled text, circular text and transparent artwork
- `src/lib/production-export.ts` — physical-size DTF, vinyl, screen-print and sublimation rendering
- `src/lib/production-packaging.ts` — PDF, TIFF, ZIP manifests, gang sheets, hashes and local export history

## Current boundaries

The editor automatically saves to the API after 1.2 seconds of inactivity and retains a browser copy when offline. Artwork is uploaded directly through a 15-minute signed PUT URL, then verified server-side for declared size, MIME signature, decodability and safe pixel dimensions before use. Durable designs reference the asset ID; one-hour signed display URLs are refreshed on reopen. PNG and JPEG originals up to 25 MB are currently accepted. SVG is deliberately rejected until a sanitization/rasterization worker is available.

Authentication uses Google Identity Services when `GOOGLE_CLIENT_ID` and `NEXT_PUBLIC_GOOGLE_CLIENT_ID` are configured. The API validates Google signatures, issuer, audience, expiry and verified email before provisioning an internal user and personal workspace. Development mode remains available without Google credentials.

Future integrations intentionally absent from the active codebase are payments, AI generation, transactional email, monitoring providers, Cloudflare-specific storage configuration and external job brokers. The disabled AI control communicates that boundary in the editor.

Embroidery export now traces rendered text and transparent artwork into millimetre-based polygon rings, compiles a diagnostic stitch preview and exports validated DST. If an image cannot be decoded because its host blocks browser canvas access, the preview identifies the affected layer and uses its transformed boundary as a safe, visible fallback. Simple lettering components receive paired-rail satin stitches with center-walk and zigzag underlay. Multi-hole or branching glyphs safely use tatami until full skeleton-assisted satin is available; unsafe satin widths are rejected against the active machine profile.

Single-hole lettering and badge outlines can now use closed-ring satin with resampled and automatically aligned inner/outer rails. Multi-hole or genuinely branching geometry remains an explicit review case. The compiler routes nearest blocks within stable thread groups, emits colour changes only when thread changes, uses jump travel between underlay and top stitching, and writes measured design extents into the DST header. When Embroidery is selected, each editor element exposes expert overrides for stitch family, row spacing, direction and underlay; automatic mode remains the default.

Non-embroidery methods now dispatch through a production review workflow. DTF produces transparent 300-DPI PNG with placed-size DPI warnings; vinyl produces mirrored millimetre SVG cut paths with hole preservation and small-detail warnings; screen printing produces colour-grouped layered SVG for solid separations; sublimation produces a 300-DPI PNG with configured bleed and coverage checks. Device-specific RIP, ICC and halftone stages are clearly identified rather than simulated.

Reviewed exports can also be downloaded as exact-size PDF, 300-DPI TIFF or a production ZIP containing artwork, SHA-256 manifest and operator instructions. DTF includes configurable original-size gang sheets within the browser memory ceiling. Vinyl output deduplicates contours and adds a weed box. Downloads are recorded immutably in browser IndexedDB with hashes and can be downloaded again from the production dialog.

The Go API now supplies the difficult deterministic prepress operations: exact Euclidean underbase spread/choke, configurable AM halftones, CMYK masks and MaxRects gang nesting. The studio exposes DTF underbase, 45-LPI screen and CMYK-package actions. Native libvips/LittleCMS ICC conversion and Potrace vectorization are capability-checked and never reported as available unless their configured binaries resolve.

## Quality checks

```bash
npm run build
cd services/api && CGO_ENABLED=0 go test ./...
```

## Production-ready deploy

The API production image (`services/api/Dockerfile`) builds with Clipper2 (`-tags clipper2`), installs libvips + Potrace, and enables:

- `REQUIRE_PRODUCTION_NATIVES=true`
- `REQUIRE_PRODUCTION_APPROVAL=true`
- `REQUIRE_ICC=true`
- `ICC_PROFILE_DIR=/var/printstudio/icc`

Use the prod compose overlay:

```bash
docker compose -f compose.yaml -f compose.prod.yaml up --build
```

Upload printer ICC profiles via `POST /v1/production/icc/profiles`, then check `GET /health/ready` for `productionReady: true`.

DTF/sublimation colour tiles render on the server (`POST /v1/production/render/scene`). Packaging requires an approved production proof when approval policy is on. Vinyl requires Clipper2. Admin metrics: `GET /v1/production/metrics`.
