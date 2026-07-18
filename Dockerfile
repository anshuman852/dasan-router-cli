# =============================================================================
# Multi-arch Go Docker image for the Dasan H660GM-A CLI + Prometheus exporter
# =============================================================================
# Build with:
#   docker buildx build --platform linux/amd64,linux/arm64 -t dasan-cli .
# =============================================================================

# ---------- Stage 1 : builder -----------------------------------------------
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

COPY . ./

# Static build, stripped, with target arch support
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /dasan ./cmd/dasan/

# ---------- Stage 2 : runtime -----------------------------------------------
FROM scratch

COPY --from=builder /dasan /dasan

EXPOSE 9800

ENTRYPOINT ["/dasan", "serve"]
