# dasan-cli

Fast CLI for the Dasan/Airtel H660GM-A GPON router, replacing the slow web UI
at `https://192.168.1.1/status/device-info`. Talks directly to the router's
internal JSON API (`/dm/tr98/`, `/dm/sys/`, `/bin/`) that the Vue.js frontend
uses. Built with [Typer](https://typer.tiangolo.com/) + [Rich](https://rich.readthedocs.io/)
for a discoverable, nicely formatted command tree.

Dependencies: `typer`, `rich` (`pip install typer rich` if not already present).

### Prometheus Exporter (new!)

Also ships a **Prometheus metrics exporter** that polls the router API and exposes
21 metrics for Grafana dashboards. Metrics include CPU/memory/temperature, PON
optical power (RX/TX dBm), WAN connection status, LAN port speeds, DHCP/WiFi
client counts, firewall rule counts, NTP sync, and more.

```bash
# Via env vars
export DASAN_HOST=192.168.1.1
export DASAN_USERNAME=admin
export DASAN_PASSWORD=yourpassword
python -m exporter.server

# Or CLI args
python -m exporter.server --host 192.168.1.1 --username admin --password yourpassword --port 9800

# Or Docker (multi-arch: amd64 + arm64)
docker run -d --name dasan-exporter \
  -e DASAN_HOST=192.168.1.1 \
  -e DASAN_USERNAME=admin \
  -e DASAN_PASSWORD=yourpassword \
  -p 9800:9800 \
  ghcr.io/anshuman852/dasan-router-cli:latest
```

Point Prometheus at `http://<host>:9800/metrics`, then import
`exporter/dasan-dashboard.json` into Grafana for a pre-built dashboard with
gauges, timeseries, and status indicators.

**Exporter options:** `--port` (default 9800), `--interval` seconds between
scrapes (default 60), `--log-level` (DEBUG/INFO/WARNING/ERROR).

**Scrape frequency:** Fast-moving objects (CPU, memory, WAN, PON, WiFi) update
every scrape cycle. Slow objects (ARP table, DHCP leases) update every 5 minutes
to reduce router load.

Dependencies: `typer`, `rich`, `prometheus_client` (`pip install typer rich prometheus_client`).

## Usage

```
python dasan.py --help                       # full command tree
python dasan.py status info                  # device info, uptime, CPU/mem/temp, PON optical stats
python dasan.py status wan                   # WAN connections (internet/voice VLANs, IPs)
python dasan.py status lan                   # LAN port link status
python dasan.py status clients               # DHCP leases / connected devices

python dasan.py wifi list                    # SSIDs (--show-password to reveal passphrases)
python dasan.py wifi set 1 --ssid MyWifi --password NewPass123
python dasan.py wifi extra macfilter-list    # MAC allow/deny list per band
python dasan.py wifi extra schedule-show     # WiFi auto-refresh schedule
python dasan.py wifi extra mesh-status       # mesh config (needs mesh-capable hardware)

python dasan.py firewall port-forwarding                 # list rules (--wan to pick a WAN iid)
python dasan.py firewall port-forwarding-add --ext-port 8080 --local-ip 192.168.1.50
python dasan.py firewall port-forwarding-delete <eid>
python dasan.py firewall dmz / dmz-enable / dmz-disable / dmz-set-host
python dasan.py firewall port-triggering[-add|-delete]
python dasan.py firewall url-filter[-add|-delete]
python dasan.py firewall parental-control[-add|-delete]  # weekly per-day blocking window
python dasan.py firewall upnp / upnp-enable / upnp-disable
python dasan.py firewall mac-anti-spoofing / ip-anti-spoofing

python dasan.py maintenance administration    # web account ports/timeout/lockout
python dasan.py maintenance ntp               # NTP servers & sync status
python dasan.py maintenance firmware          # current HW/SW version
python dasan.py maintenance logs --lines 100  # recent syslog
python dasan.py maintenance backup            # download a config backup file
python dasan.py maintenance auto-reboot / port-mirroring / snmp / syslog-configuration

python dasan.py advanced wan-connections      # full WAN detail incl. PPPoE creds (--show-password)
python dasan.py advanced arp / arp-set-timeout
python dasan.py advanced ddns / ddns-set
python dasan.py advanced static-routing[-add[-v6]|-delete[-v6]]

python dasan.py reboot                        # asks for confirmation; -y to skip
python dasan.py raw get DeviceInfo            # escape hatch for any objs=... endpoint
```

Credentials: pass `--user`/`--password`, set `$DASAN_USER`/`$DASAN_PASS`, or
you'll be prompted. The auth token is cached in `~/.dasan-cli-session.json`
(mode 600) and reused until it expires (~30 min), so most commands only need
to log in once per session.

`--host` defaults to `192.168.1.1`; pass a different IP if yours differs.

## Project layout

```
dasan.py               thin entry point -> dasan_cli.main
  dasan_cli/
  core.py              shared HTTP client: auth, CSRF, session cache, table rendering
  status.py            device info, WAN/LAN, DHCP clients
  wifi.py               SSID list/set (WLANConfiguration)
  wifi_extra.py         MAC filter, auto-refresh schedule, mesh (nested under `wifi extra`)
  firewall.py            port forwarding, DMZ, port triggering, URL/parental filters, UPnP, anti-spoofing
  maintenance.py         administration, NTP, firmware info, logs, backup, SNMP, syslog
  advanced.py            WAN connection detail, ARP, DDNS, static routing
  main.py                wires every module's Typer app together
exporter/
  __init__.py
  exporter.py          Prometheus metrics collector (21 metrics across 10 categories)
  server.py            HTTP server for Prometheus scraping
  dasan-dashboard.json  Grafana dashboard (import into Grafana 10+)
Dockerfile              Multi-arch image (amd64 + arm64)
.github/workflows/docker-publish.yml  CI: build & push to ghcr.io
```

## Known limitations (found via live testing against one H660GM-A unit)

- **UPnP enable/disable** (`firewall upnp-enable/-disable`) and **MAC filter
  add** (`wifi extra macfilter-add`): the router accepts the write (HTTP 200,
  no error) but does not actually persist the new value on this firmware/account.
  Both commands detect this and print a warning rather than falsely claiming
  success — verify any change in the web UI before relying on it.
- **WiFi mesh** (`wifi extra mesh-*`): this unit has no mesh peers/hardware, so
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
  these. (Confirmed live: `MyHostName`/`Username` on this unit currently hold
  test values `myhome.no-ip.org` / `testuser` from write-path verification —
  harmless since `Enable` was left `false` throughout, but not the original
  blank state; reset via the web UI if you want it pristine.)
- **`advanced static-routing-add[-v6]/-delete[-v6]`**: implemented directly
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
