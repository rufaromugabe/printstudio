# Architecture and delivery plan

## Product boundary

Free users can create, preview, save and share editable product designs. Paid capabilities are AI credits, background removal/upscaling, professional mockups, production-ready exports, brand kits and collaboration. Decoration-method rules are separate policy modules: DTF, embroidery, screen print, vinyl and sublimation must never share a fake one-size-fits-all exporter.

## Target services

1. **Web editor** — Next.js/TypeScript, canonical JSON documents, responsive direct manipulation.
2. **Core API** — users, workspaces, products, variants, print areas, projects, immutable design versions, entitlements and orders.
3. **Asset service** — signed multipart uploads, virus/type checks, metadata and R2/S3 objects.
4. **Validation worker** — DPI, physical dimensions, safe/bleed area, colour/detail rules by method.
5. **Render/export worker** — reconstructs from original assets at physical output size; emits PNG/PDF/SVG and method packages.
6. **AI gateway** — provider-neutral generation, removal, enhancement and moderation with metered credits.
7. **Billing** — provider adapters, idempotent webhooks, subscriptions and an append-only credit ledger.
8. **Production workflow** — quote, proof, approval, immutable order snapshot, status timeline and fulfilment.

PostgreSQL is authoritative; Redis is cache/rate-limit/short-lived coordination; object storage holds originals and generated files; a durable queue handles jobs. Start as a modular monolith plus workers and split services only when load or team ownership requires it.

## Canonical design rules

- Coordinates are stored in print-area logical units and mapped to physical millimetres.
- Every element references its original asset; previews are derivatives only.
- Orders reference an immutable design version, never a mutable project.
- Exports are reproducible jobs identified by design-version, print profile and renderer version.
- Embroidery artwork must pass suitability checks and then use a real digitiser/human review; raster-to-DST renaming is prohibited.

## Delivery roadmap

### Release 1 — executable editor (implemented)

Garment front/back, text and image elements, movement, property controls, layers, colour/method selection, undo/redo, local persistence, print-boundary warnings, responsive UI, product and design API contracts.

### Release 2 — multi-user SaaS

OIDC authentication, PostgreSQL migrations/repositories, signed asset upload, cloud save/autosave, share links, immutable versions, product/admin management, tenant/workspace isolation, audit log, quotas and observability.

### Release 3 — paid DTF production

Entitlements/credit ledger, regional payments, 300-DPI transparent PNG/PDF worker, DPI/detail validation, asynchronous jobs, download expiry, DTF gang-sheet compositor and transactional email.

### Release 4 — AI and professional workflow

Moderated image generation, background removal, upscale, vectorisation assistance, brand kits, templates, comments/approvals and commercial licensing metadata.

### Release 5 — additional methods and marketplace

Embroidery suitability plus digitisation fulfilment, screen separations, vinyl cut validation, sublimation/full bleed, printer onboarding, quotes, production tracking, payouts and white-label embeds.

## MVP acceptance gates

- Saved designs reopen with element geometry unchanged.
- Browser-to-physical coordinate tests are deterministic.
- Original asset resolution is never inferred from preview resolution.
- Cross-tenant resource access is integration-tested.
- Payment and job requests are idempotent.
- Export output is validated against expected dimensions, transparency and print profile.
- Orders preserve the exact approved design version.
- Accessibility, touch input, error recovery, security headers, rate limits, backups and restore drills are release requirements.
