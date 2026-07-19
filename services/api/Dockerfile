# Production API image with Clipper2 (CGO), libvips/LittleCMS and Potrace.
FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc g++ pkg-config libvips-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p /out/lib \
    && CLIPPER_LIB="$(go env GOMODCACHE)/github.com/epit3d/goclipper2@v0.0.9/lib" \
    && cp "$CLIPPER_LIB"/libclipper2c.so* /out/lib/ \
    && CGO_ENABLED=1 go build -tags clipper2 -o /out/api .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    bash ca-certificates libvips42 libvips-tools libstdc++6 potrace tesseract-ocr tesseract-ocr-eng \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /usr/local/bin
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/lib /usr/local/bin/lib
ENV PATH="/usr/local/bin:${PATH}" \
    LD_LIBRARY_PATH="/usr/local/bin/lib" \
    VIPS_BIN=vips \
    POTRACE_BIN=potrace \
    TESSERACT_BIN=tesseract \
    ICC_PROFILE_DIR=/var/printstudio/icc \
    REQUIRE_PRODUCTION_NATIVES=true \
    REQUIRE_PRODUCTION_APPROVAL=true \
    REQUIRE_ICC=false \
    MAX_RENDER_PIXELS=100000000
RUN mkdir -p /var/printstudio/icc
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=5 CMD ["/usr/local/bin/api","healthcheck"]
ENTRYPOINT ["/usr/local/bin/api"]
