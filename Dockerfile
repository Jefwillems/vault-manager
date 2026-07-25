# syntax=docker/dockerfile:1

# ---- build stage -------------------------------------------------------------
# Bundles the GitHub Copilot CLI into the binary via the SDK's bundler, then
# builds a static (CGO_ENABLED=0) vault-manager binary.
FROM golang:1.26-bookworm AS build

WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# Embed the Copilot CLI for the target platform. The bundler writes a
# `package main` Go file plus a compressed CLI bundle into cmd/vault-manager/,
# which are then compiled into the binary (no `copilot` needed in PATH at
# runtime). This step requires network access to the npm registry.
RUN go tool bundler --platform linux/amd64 --output cmd/vault-manager

# Static build so the image can be minimal.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/vault-manager ./cmd/vault-manager

# ---- runtime stage -----------------------------------------------------------
# The embedded Copilot CLI is a Node single-executable that is extracted and run
# as a child process at runtime; it needs glibc + libstdc++/libgcc, which the
# distroless "cc" image provides (along with ca-certificates for TLS).
FROM gcr.io/distroless/cc-debian12:nonroot

COPY --from=build /out/vault-manager /usr/local/bin/vault-manager

# Defaults; override HOME/XDG_CACHE_HOME via the pod spec to a writable volume
# when the container runs as a non-default UID (e.g. to match PVC ownership).
ENV HOME=/home/nonroot \
    XDG_CACHE_HOME=/home/nonroot/.cache \
    VAULT_PATH=/app/data/vault \
    BRAINDUMP_DIR=Braindumps

ENTRYPOINT ["/usr/local/bin/vault-manager"]
