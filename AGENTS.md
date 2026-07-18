# AGENTS.md

## Project overview

`dasan-cli` is a CLI tool and Prometheus exporter for the Dasan/Airtel H660GM-A GPON router. It talks directly to the router's internal JSON API (`/dm/tr98/`, `/dm/sys/`, `/bin/`) — the same API the Vue.js web UI uses. Two artifacts ship from this repo:

1. **Python CLI** (`dasan_cli/`) — read/write router configuration from the terminal (status, WiFi, firewall, DHCP, etc.)
2. **Go Prometheus exporter** (`go-exporter/`) — scrape the router API and expose metrics at `/metrics`

## Technology stack

| Component | Language | Key deps |
|-----------|----------|----------|
| CLI | Python 3 | `typer`, `rich` |
| Exporter | Go 1.23+ | `prometheus/client_golang` |
| CI | GitHub Actions | QEMU, Buildx, semantic-release |
| Registry | GHCR | `ghcr.io/anshuman852/dasan-exporter` |

## Directory structure

```
dasan.py                          # CLI entry point
dasan_cli/
  core.py                         # HTTP client, auth (JWT), CSRF, session cache
  status.py                       # Device info, WAN, LAN, DHCP clients
  wifi.py / wifi_extra.py         # SSID config, MAC filter, mesh
  firewall.py                     # Port forwarding, DMZ, UPnP, URL filter, anti-spoofing
  maintenance.py                  # Admin, NTP, logs, firmware, SNMP
  advanced.py                     # WAN detail, ARP, DDNS, static routes
  main.py                         # Typer app wiring
go-exporter/
  main.go                         # Entry point, HTTP server, scrape loop
  client.go                       # Dasan API client (JWT login, GET, TLS skip)
  collector.go                    # 21 Prometheus metrics, interval-gated collection
  dasan-dashboard.json            # Pre-built Grafana dashboard
Dockerfile                        # Multi-stage Go build → scratch (~12 MB)
.github/workflows/
  docker-publish.yml              # Build + push multi-arch to GHCR
  release.yml                     # semantic-release (auto version bumps)
.releaserc.json                   # semantic-release config
```

## Router API patterns

**Auth:** `POST /dm/sys/?cmd=Login` returns a JWT. Send as `Authorization: Bearer <token>`. Session cached in `~/.dasan-cli-session.json` (~30 min expiry).

**Reads:** `GET /dm/tr98/?objs=<ObjectName>&page=<PageName>`. Response: `{"<ObjectName>":{"status_code":200,"data":{...}}}`. For list objects, `data` is an array. The response key is always the bare object name (e.g. `PortForwarding` even when querying `PortForwarding.2`).

**Writes:** Require a CSRF token from a preceding GET. Read `csrf` response header, send as `X-Csrf-Token` on the POST/DELETE. List objects expect the full record wrapped in an array `{"Object":{"data":[{...}]}}` — even for single items. DELETE echoes the record back.

**TLS:** Router uses a self-signed certificate. Skip verification (`InsecureSkipVerify: true` in Go, `ssl.CERT_NONE` in Python).

**Page names:** Required by the router for per-page permission checks. Known pages are in `dasan_cli/core.py` `PAGES` dict. Some objects (static routing) use `?api=` instead of `?objs=`.

**Namespaces:** `/dm/tr98/` (config objects), `/dm/sys/` (system commands), `/bin/` (binary/text downloads).

## Exporter collection logic

- **Fast objects** (every 60s): DeviceInfo, PonPortStatus, WAN connections, LAN ports, WiFi, firewall, NTP, auto-reboot
- **Slow objects** (every 300s): DHCP leases, ARP table
- Each object query is independent — a single failure doesn't block the rest
- Self-monitoring: `dasan_api_requests_total` (per object, success/error), `dasan_api_request_duration_seconds` (histogram)

## CI/CD

Push to `main` → Docker build (`linux/amd64`, `linux/arm64`) → push to GHCR with `latest` + `sha-*` tags.

Push `v*.*.*` tag → same Docker build with semver tags (`1.2.3`, `1.2`, `1`). Tags are created automatically by `semantic-release` on every `main` push — no manual tagging needed.

### Conventional commits

Semantic-release determines version bumps from commit messages:
- `feat: add GPU metrics` → **minor** bump (new feature)
- `fix: handle expired token` → **patch** bump (bug fix)
- `feat!: drop Python exporter` or `BREAKING CHANGE:` in body → **major** bump

## Running locally

```bash
# CLI
python dasan.py status info

# Exporter (Go)
cd go-exporter && go build -o dasan-exporter .
DASAN_HOST=192.168.1.1 DASAN_USERNAME=admin DASAN_PASSWORD=pass ./dasan-exporter

# Docker
docker buildx build --platform linux/amd64 --load -t dasan-exporter .
docker run -e DASAN_HOST=192.168.1.1 -e DASAN_USERNAME=admin -e DASAN_PASSWORD=pass -p 9800:9800 dasan-exporter
```

## Boolean quirks

The router returns booleans inconsistently — sometimes `true`/`false`, sometimes `"true"`/`"false"`, sometimes `1`/`0`. Always normalize with a helper that handles all cases.

## Firmware quirks

- UPnP enable/disable and MAC filter add silently fail (accepted but not persisted) — CLI detects and warns
- Mesh endpoints 403 on non-mesh hardware
- Several objects 403 on low-privilege accounts (routing policy, rate limiting, backup config)
- DDNS won't accept empty values for `MyHostName`/`Username` once set
