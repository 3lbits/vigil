# syntax=docker/dockerfile:1.7
# Multi-stage: build with full Go toolchain, run on distroless/static (no shell, no libc)

## ── Stage 1: build ──────────────────────────────────────────────────────────
# --platform=$BUILDPLATFORM pins the builder to the native build host (e.g. the
# amd64 GHA runner) for ALL target platforms. The Go build cross-compiles via
# GOARCH=${TARGETARCH} (CGO_ENABLED=0), and the runtime stage only COPYs the
# binary, so no target-arch code is ever executed at build time — QEMU emulation
# is not needed. Digest pinning still works: the digest points at the manifest
# index and $BUILDPLATFORM resolves the host variant from it.
FROM --platform=$BUILDPLATFORM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS builder

# Prevent Go from auto-downloading a newer toolchain at build time
ENV GOTOOLCHAIN=local

WORKDIR /build

ARG VERSION=dev

# Install templ CLI (version pinned to match go.mod: v0.3.1100)
# Placed before source copy so this layer is cached as long as the version is unchanged.
# Now runs natively on the build host for every target -> built once, reused.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1100

# Fetch dependencies first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source tree
COPY . .

# Generate templ components (*_templ.go are gitignored, must be generated here)
RUN templ generate ./...

# Combine the authored stylesheets (output.css is gitignored, must be generated here)
RUN make css-build

# Build a fully static binary (cross-compile via GOARCH — avoids QEMU for this step)
# -trimpath:       remove local filesystem paths from the binary
# -buildvcs=false: skip Git metadata embedding (version injected via CI if needed)
# -ldflags -s -w:  strip debug symbols and DWARF (reduces binary size ~30%)
# -mod=readonly:   fail if the build would require updating go.mod/go.sum
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build \
      -trimpath \
      -buildvcs=false \
      -ldflags="-s -w -X 'main.Version=${VERSION}'" \
      -mod=readonly \
      -o /out/server \
      ./cmd/server

## ── Stage 2: runtime ─────────────────────────────────────────────────────────
# distroless/static: no shell, no package manager, no libc — minimal CVE surface
# :nonroot sets USER 65532:65532 (nonroot) — no privilege escalation path
#
# Pin by digest for supply-chain integrity. Update the digest when intentionally
# bumping the base image (see commands at top of file to obtain the current digest).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

# OCI image labels for supply chain traceability
LABEL org.opencontainers.image.title="vigil" \
      org.opencontainers.image.description="Vigil — GRC toolbox" \
      org.opencontainers.image.source="https://github.com/3lbits/vigil" \
      org.opencontainers.image.licenses="Apache-2.0"

WORKDIR /app

# Binary — owned by nonroot so the running process owns its own executable
COPY --link --chown=65532:65532 --from=builder /out/server /app/server

# Goose reads SQL migration files from the filesystem at startup
# ("db/migrations" is a relative path in main.go — resolves to /app/db/migrations)
COPY --link --chown=65532:65532 --from=builder /build/db/migrations /app/db/migrations

# LICENSE + third-party notices — shipped for distribution compliance (the image is the redistributed artifact)
COPY --link --chown=65532:65532 LICENSE /app/LICENSE
COPY --link --chown=65532:65532 THIRD_PARTY_NOTICES.md /app/THIRD_PARTY_NOTICES.md
COPY --link --chown=65532:65532 THIRD_PARTY_NOTICES_GO.md /app/THIRD_PARTY_NOTICES_GO.md

EXPOSE 8080

# HEALTHCHECK: distroless has no shell, so CMD-based health checks are not possible.
# Use orchestrator-level health checks instead:
#   Kubernetes: livenessProbe/readinessProbe (httpGet on /health or tcpSocket on 8080)
#   Compose:    healthcheck with a sidecar or curl image
USER 65532:65532
ENTRYPOINT ["/app/server"]