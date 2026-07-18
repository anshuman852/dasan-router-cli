# dasan-cli

Fast CLI for the Dasan/Airtel H660GM-A GPON router, replacing the slow web UI
at `https://192.168.1.1/status/device-info`. Talks directly to the router's
internal JSON API (`/dm/tr98/`, `/dm/sys/`, `/bin/`) that the Vue.js frontend
uses. Built with [Cobra](https://github.com/spf13/cobra).

### Prometheus Exporter

Also ships a **Prometheus metrics exporter** as a subcommand (`dasan serve`) —
~7 MB static binary, <1 MB memory at idle, runs from `scratch` Docker image.
Polls the router API and exposes 21 metrics for Grafana dashboards.

```bash
# Build from source
go build -o dasan ./cmd/dasan/

# Run CLI commands
./dasan status info
./dasan wifi list

# Or run the exporter
export DASAN_HOST=192.168.1.1
export DASAN_USERNAME=admin
export DASAN_PASSWORD=yourpassword
./dasan serve --port 9800 --interval 60

# Or Docker (multi-arch: amd64 + arm64, ~7 MB image)
docker run -d --name dasan \
  -e DASAN_HOST=192.168.1.1 \
  -e DASAN_USERNAME=admin \
  -e DASAN_PASSWORD=yourpassword \
  -p 9800:9800 \
  ghcr.io/anshuman852/dasan:latest
```

Point Prometheus at `http://<host>:9800/metrics`.

**Scrape frequency:** Fast-moving objects (CPU, memory, WAN, PON, WiFi) update
every scrape cycle. Slow objects (ARP table, DHCP leases) update every 5 minutes
to reduce router load.

## Usage

```
dasan --help                           # full command tree
dasan status info                      # device info, uptime, CPU/mem/temp, PON optical stats
dasan status wan                       # WAN connections (internet/voice VLANs, IPs)
dasan status lan                       # LAN port link status
dasan status clients                   # DHCP leases / connected devices

dasan wifi list                        # SSIDs (--show-password to reveal passphrases)
dasan wifi set 1 --ssid MyWifi --password NewPass123
dasan wifi macfilter-list              # MAC allow/deny list per band
dasan wifi schedule-show               # WiFi auto-refresh schedule
dasan wifi mesh-status                 # mesh config (needs mesh-capable hardware)

dasan firewall port-forwarding                        # list rules (--wan to pick a WAN iid)
dasan firewall port-forwarding-add --ext-port 8080 --local-ip 192.168.1.50
dasan firewall port-forwarding-delete <eid>
dasan firewall dmz / dmz-enable / dmz-disable / dmz-set-host
dasan firewall port-triggering[-add|-delete]
dasan firewall url-filter[-add|-delete]
dasan firewall parental-control[-add|-delete]          # weekly per-day blocking window
dasan firewall upnp / upnp-enable / upnp-disable
dasan firewall mac-anti-spoofing / ip-anti-spoofing

dasan maintenance admin                 # web account ports/timeout/lockout
dasan maintenance ntp                    # NTP servers & sync status
dasan maintenance firmware               # current HW/SW version
dasan maintenance logs --lines 100       # recent syslog
dasan maintenance backup                 # download a config backup file
dasan maintenance auto-reboot / port-mirroring / snmp / syslog-configuration

dasan advanced wan-connections           # full WAN detail incl. PPPoE creds (--show-password)
dasan advanced arp / arp-set-timeout
dasan advanced ddns / ddns-set
dasan advanced static-routing[-add|-delete]

dasan reboot                             # asks for confirmation; -y to skip
dasan raw get DeviceInfo                 # escape hatch for any objs=... endpoint
dasan serve                              # start Prometheus metrics exporter
dasan version                            # print version
```

Credentials: pass `--user`/`--password`, set `DASAN_USER`/`DASAN_PASS`, or
you'll be prompted. The auth token is cached in `~/.dasan-session.json`
(mode 600) and reused until it expires (~30 min), so most commands only need
to log in once per session.

`--host` defaults to `192.168.1.1`; pass a different IP if yours differs.

## Project layout

```
cmd/dasan/
  main.go               entry point, cobra root command
internal/
  client/
    client.go            enhanced API client: auth, CSRF, session cache, read/write/delete
  collector/
    collector.go          Prometheus metrics + collection logic
  exporter/
    serve.go              HTTP server + metrics endpoint
  cli/
    status.go             device info, WAN/LAN, DHCP clients
    wifi.go               SSID list/set, MAC filter, auto-refresh schedule, mesh
    firewall*.go          port forwarding, DMZ, port triggering, URL/parental filters, UPnP, anti-spoofing
    maintenance.go         administration, NTP, firmware info, logs, backup, SNMP, syslog
    advanced.go            WAN connection detail, ARP, DDNS, static routing
    context.go             global client reference
    utils.go               table rendering, formatting helpers
Dockerfile                 Multi-arch Go build → scratch (~12 MB image)
.github/workflows/
  docker-publish.yml       CI: build & push Docker image to ghcr.io
  release.yml              CI: GoReleaser cross-platform binaries
```

## Known limitations (found via live testing against one H660GM-A unit)

- **UPnP enable/disable** (`firewall upnp-enable/-disable`) and **MAC filter
  add** (`wifi macfilter-add`): the router accepts the write (HTTP 200,
  no error) but does not actually persist the new value on this firmware/account.
  Both commands detect this and print a warning rather than falsely claiming
  success — verify any change in the web UI before relying on it.
- **WiFi mesh** (`wifi mesh-*`): this unit has no mesh peers/hardware, so
  every mesh endpoint returns a clean `403 Unauthorized URL`. The commands are
  implemented from the same JS the web UI uses and should work on mesh-capable
  hardware/firmware.
- Several objects are gated behind a higher-privilege account than admin/admin
  provides and always 403: routing policy, rate limiting, ACL/ICMP-ACL, TFTP,
  MAC/IP anti-spoofing, config backup (`/bin/?objs=BackupConfig`), IPv6
  ARP/DDNS, and the kernel route table. Commands for these exist (mirroring
  the exact API shape) but will only work if your account/firmware allows it.
- **`advanced ddns-set`**: once `MyHostName` or `Username` is set to a
  non-empty value, this router's validation will not let you clear it back to
  blank — an empty `MyHostName` is rejected outright (error 9006, "Choose IP
  address value") and an empty `Username` returns success but silently keeps
  the old value. Pass a real replacement instead of `""` if you need to change
  these.
- **`advanced static-routing-add/-delete`**: implemented directly
  from the reverse-engineered JS (field names `DestIp`/`Netmask`/`Gateway`/
  `Interface`/`Metric` for IPv4, `dstIp`/`prefixLen`/`gateway`/`intfName` for
  IPv6) but **not live-tested** — the sandbox's permission classifier blocked
  the write call before it reached the router. Sanity-check the first real
  use against `advanced static-routing` before relying on it.
- Intentionally **not implemented** (too risky to a live connection to touch
  from a CLI): LAN subnet/DHCP server reconfiguration, GPON LOID/SLID
  provisioning identifiers, WAN connection writes, firmware upload, and
  restore-factory/restore-default.

## How it was built

The router serves a single-page Vue app whose real data comes from a small
JSON API rather than server-rendered HTML, which is why the page feels slow
(a full JS bundle load) despite the underlying data being tiny. Reverse-engineered
by downloading each page's webpack JS chunk and grepping it for its API calls,
rather than by driving a browser — the app is 100% API-driven so the result is
identical, just faster. The API itself:

- `POST /dm/sys/?cmd=Login` with `{"Login":{"data":{"username","password","captcha"}}}`
  returns a JWT (`authenticatedToken`), sent as `Authorization: Bearer <token>`.
- Reads: `GET /dm/tr98/?objs=<ObjectName>&page=<RouteName>` — `page` is the
  frontend route name and is used for a per-page permission check. A few
  objects (e.g. static routing) use `?api=` instead of `?objs=`; response
  bodies key by the bare object name even when the request used a dotted
  suffix like `PortForwarding.<wanIid>`.
- Writes: the router requires a fresh CSRF token per write. Do a `GET` on the
  same endpoint, read the `csrf` response header verbatim, and send it back as
  `X-Csrf-Token` on the following `POST`/`DELETE`. List-style objects (port
  forwarding, port triggering, URL filters, DMZ, static routes, DDNS, MAC
  filters) expect the *entire record* wrapped in a list — `{"Object":{"data":[{...}]}}`
  — even for a single item, and `DELETE` requires the full record echoed
  back, not just its id. Single-object settings (WiFi SSID config, UPnP, ARP
  timeout) take a bare object instead.
- A few endpoints return raw non-JSON bytes instead of the usual envelope:
  `/bin/?objs=SyslogDownload` (plain text log) and `/bin/?objs=BackupConfig`
  (binary config dump).
