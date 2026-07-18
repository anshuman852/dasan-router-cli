# =============================================================================
# Multi-arch Docker image for the Dasan H660GM-A Prometheus exporter
# =============================================================================
# Build with:
#   docker buildx build --platform linux/amd64,linux/arm64 -t dasan-exporter .
# =============================================================================

# ---------- Stage 1 : builder -----------------------------------------------
FROM python:3.11-slim AS builder

# Prevent pip from writing cache & using latest pip/setuptools
ENV PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

WORKDIR /build

# Copy only the packages needed at runtime
COPY dasan_cli/ ./dasan_cli/
COPY exporter/  ./exporter/
COPY dasan.py   ./

# Install runtime dependencies into a temporary prefix so we can copy them
# cleanly into the final stage.
RUN pip install \
        --target=/install \
        rich \
        typer \
        prometheus_client

# ---------- Stage 2 : runtime -----------------------------------------------
FROM python:3.11-alpine

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1

# Create a non-root user
RUN adduser -S -u 1000 dasan

WORKDIR /app

# Copy installed packages from the builder
COPY --from=builder /install /usr/local/lib/python3.11/site-packages

# Copy application code
COPY --from=builder /build/dasan_cli/  ./dasan_cli/
COPY --from=builder /build/exporter/   ./exporter/
COPY --from=builder /build/dasan.py    ./

# Own everything by the non-root user
RUN chown -R dasan:dasan /app

USER dasan

EXPOSE 9800

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:9800/metrics')" || exit 1

ENTRYPOINT ["python", "-m", "exporter.server"]
