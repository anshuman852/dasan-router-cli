# AGENTS.md

## Project overview

`dasan-cli` is a CLI tool and Prometheus exporter for the Dasan/Airtel H660GM-A GPON router. It talks directly to the router's internal JSON API (`/dm/tr98/`, `/dm/sys/`, `/bin/`) — the same API the Vue.js web UI uses. A single Go binary ships both the CLI and the Prometheus exporter:

- **CLI** (`cmd/dasan/`) — read/write router configuration from the terminal (status, WiFi, firewall, DHCP, etc.)
- **Exporter** (`internal/exporter/`) — scrape the router API and expose metrics at `/metrics`

## Technology stack

| Component | Language | Key deps |
|-----------|----------|----------|
| CLI + Exporter | Go 1.23+ | cobra, prometheus/client_golang |
| CI | GitHub Actions | Docker Buildx, GoReleaser |
| Registry | GHCR | `ghcr.io/anshuman852/dasan` |

## Directory structure

```
cmd/dasan/
  main.go                         # Entry point, cobra root command
internal/
  client/
    client.go                     # Enhanced API client: auth, CSRF, session cache, read/write/delete
  collector/
    collector.go                  # 21 Prometheus metrics, interval-gated collection
  exporter/
    serve.go                      # HTTP server + metrics endpoint
  cli/
    context.go                    # Global client reference
    utils.go                      # Table rendering, formatting helpers
    status.go                     # Device info, WAN, LAN, DHCP clients
    wifi.go                       # SSID config, MAC filter, mesh
    firewall*.go                  # Port forwarding, DMZ, UPnP, URL filter, anti-spoofing
    maintenance.go                # Admin, NTP, logs, firmware, SNMP
    advanced.go                   # WAN detail, ARP, DDNS, static routes
Dockerfile                        # Multi-stage Go build → scratch (~12 MB)
.github/workflows/
  docker-publish.yml              # Build + push multi-arch to GHCR
  release.yml                     # GoReleaser (cross-platform binaries)
```

## Router API patterns

**Auth:** `POST /dm/sys/?cmd=Login` returns a JWT. Send as `Authorization: Bearer <token>`. Session cached in `~/.dasan-session.json` (~30 min expiry).

**Reads:** `GET /dm/tr98/?objs=<ObjectName>&page=<PageName>`. Response: `{"<ObjectName>":{"status_code":200,"data":{...}}}`. For list objects, `data` is an array. The response key is always the bare object name (e.g. `PortForwarding` even when querying `PortForwarding.2`).

**Writes:** Require a CSRF token from a preceding GET. Read `csrf` response header, send as `X-Csrf-Token` on the POST/DELETE. List objects expect the full record wrapped in an array `{"Object":{"data":[{...}]}}` — even for single items. DELETE echoes the record back.

**TLS:** Router uses a self-signed certificate. Skip verification (`InsecureSkipVerify: true`).

**Page names:** Required by the router for per-page permission checks. Known pages are in `internal/client/client.go` `PAGES` map. Some objects (static routing) use `?api=` instead of `?objs=`.

**Namespaces:** `/dm/tr98/` (config objects), `/dm/sys/` (system commands), `/bin/` (binary/text downloads).

## Exporter collection logic

- **Fast objects** (every scrape): DeviceInfo, PonPortStatus, WAN connections, LAN ports, WiFi, firewall, NTP, auto-reboot
- **Slow objects** (every 300s): DHCP leases, ARP table
- Each object query is independent — a single failure doesn't block the rest
- Self-monitoring: `dasan_api_requests_total` (per object, success/error), `dasan_api_request_duration_seconds` (histogram)

## CI/CD

Push to `main` → Docker build (`linux/amd64`, `linux/arm64`) → push to GHCR with `latest` + `sha-*` tags.

Push `v*.*.*` tag → Docker build with semver tags + GoReleaser cross-platform binaries.

## Running locally

```bash
# Build
go build -o dasan ./cmd/dasan/

# CLI
./dasan status info
./dasan wifi list

# Exporter (Go)
DASAN_HOST=192.168.1.1 DASAN_USERNAME=admin DASAN_PASSWORD=pass ./dasan serve

# Docker
docker buildx build --platform linux/amd64 --load -t dasan .
docker run -e DASAN_HOST=192.168.1.1 -e DASAN_USERNAME=admin -e DASAN_PASSWORD=pass -p 9800:9800 dasan
```

## Boolean quirks

The router returns booleans inconsistently — sometimes `true`/`false`, sometimes `"true"`/`"false"`, sometimes `1`/`0`. Always normalize with a helper that handles all cases.

## Firmware quirks

- UPnP enable/disable and MAC filter add silently fail (accepted but not persisted) — CLI detects and warns
- Mesh endpoints 403 on non-mesh hardware
- Several objects 403 on low-privilege accounts (routing policy, rate limiting, backup config)
- DDNS won't accept empty values for `MyHostName`/`Username` once set
