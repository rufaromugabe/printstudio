# Optional Clipper2 backend

PrintStudio pins `github.com/epit3d/goclipper2` at `v0.0.9` for robust fixed-point polygon Boolean operations and offsets. The integration is optional so the API continues to build on machines without CGO or a native toolchain.

## Requirements

- A C/C++ compiler available to Go's CGO toolchain
- `CGO_ENABLED=1`
- Build tag `clipper2`
- The goclipper2 native x64 library for the target operating system available to the linker and beside the deployed executable (or on the platform library search path)

The upstream module includes Windows and Linux x64 libraries in its `lib` directory. Those binaries are platform-specific; do not copy a Windows DLL into a Linux deployment or assume ARM support.

## Build

From `services/api`:

```text
go mod download
go test -tags clipper2 ./...
go build -tags clipper2 .
```

On Windows, use a compiler compatible with the supplied library. On Linux, install the normal C/C++ build toolchain before running the tagged build. Copy the matching native library as described by the upstream goclipper2 README.

The untagged command remains the portable build:

```text
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build .
```

## Runtime contract

`GET /v1/production/capabilities` returns `polygonBoolean: true` only in a successfully linked tagged build. The routes are:

```text
POST /v1/production/vector/boolean
POST /v1/production/vector/offset
```

Boolean request:

```json
{
  "subject": [[{"x": 0, "y": 0}, {"x": 20, "y": 0}, {"x": 20, "y": 20}]],
  "clip": [[{"x": 10, "y": 10}, {"x": 30, "y": 10}, {"x": 30, "y": 30}]],
  "operation": "union"
}
```

Supported operations are `union`, `difference`, `intersection`, and `xor`.

Offset request:

```json
{
  "paths": [[{"x": 0, "y": 0}, {"x": 20, "y": 0}, {"x": 20, "y": 20}]],
  "deltaMm": 0.5,
  "join": "round",
  "miterLimit": 2
}
```

Supported joins are `round`, `square`, and `miter`. Positive deltas expand and negative deltas contract closed polygons.

## Release gate

Before enabling this in production, run the tagged test suite on every deployment target and add a corpus covering self-intersections, nested holes, touching edges, degenerate rings, very large coordinates and repeated points. Run native leak and sanitizer checks against the exact shared library shipped with the service.
