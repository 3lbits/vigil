# syntax=docker/dockerfile:1.7
# Multi-stage: build with full Go toolchain, run on distroless/static (no shell, no libc)

## ── Stage 1: build ──────────────────────────────────────────────────────────
FROM golang:1.26.3-bookworm@sha256:386d475a660466863d9f8c766fec64d7fdad3edac2c6a05020c09534d71edb4b AS builder

# Prevent Go from auto-downloading a newer toolchain at build time
ENV GOTOOLCHAIN=local

WORKDIR /build

ARG VERSION=dev

# Install templ CLI (version pinned to match go.mod: v0.3.1001)
# Placed before source copy so this layer is cached as long as the version is unchanged
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1001

# Install Tailwind v4 standalone CLI (no Node.js required)
# SHA256 values from sha256sums.txt in the release — update all three ARGs together when bumping.
ARG TARGETARCH
ARG TAILWIND_VERSION=v4.1.5
ARG TAILWIND_SHA256_amd64=9d258a7786c22f8572aea20ed35848f8cbef1d06277e4e6b97ad8561ffc21e07
ARG TAILWIND_SHA256_arm64=9555c6414617c15cc820679ec65404af92507e7b7718fd035d26df055561dec9
RUN case "${TARGETARCH}" in \
      arm64) ARCH=arm64; SHA256="${TAILWIND_SHA256_arm64}" ;; \
      *)     ARCH=x64;   SHA256="${TAILWIND_SHA256_amd64}" ;; \
    esac \
    && curl -sLo /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${ARCH}" \
    && echo "${SHA256}  /usr/local/bin/tailwindcss" | sha256sum -c - \
    && chmod +x /usr/local/bin/tailwindcss

# Fetch dependencies first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source tree
COPY . .

# Generate templ components (*_templ.go are gitignored, must be generated here)
RUN templ generate ./...

# Generate tree-shaken, minified CSS (output.css is gitignored, must be generated here)
RUN tailwindcss -i cmd/server/public/css/input.css -o cmd/server/public/css/output.css --minify

# Build a fully static binary (cross-compile via GOARCH — avoids QEMU for this step)
# -trimpath:       remove local filesystem paths from the binary
# -buildvcs=false: skip Git metadata embedding (version injected via CI if needed)
# -ldflags -s -w:  strip debug symbols and DWARF (reduces binary size ~30%)
# -mod=readonly:   fail if the build would require updating go.mod/go.sum
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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639

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
