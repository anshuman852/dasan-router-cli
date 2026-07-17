"""Advanced setup: WAN connection detail, ARP, DDNS, static routing."""
from typing import Optional

import typer

from . import core

app = typer.Typer(help="Advanced setup: WAN detail, ARP, DDNS, static routing")

ARP_STATUS_PAGE = "StatusPage-ARP"
ARP_SETTING_PAGE = "AdvancedSetupPage-ARP"
DDNS_PAGE = "AdvancedSetupPage-DDNS"
STATIC_ROUTING_PAGE = "AdvancedSetupPage-StaticRouting"
LAN_SETUP_PAGE = "AdvancedSetupPage-LANSetup"


def _get_api(client, name, page):
    """StaticRouting* objects are served under a `?api=` query param instead of
    the usual `?objs=` that core.DasanClient.get() builds."""
    payload, _ = client._request("GET", f"/dm/tr98/?api={name}&page={page}")
    entry = payload.get(name, {})
    if entry.get("status_code") != 200:
        raise core.RouterError(f"{name}: {entry.get('error')}")
    return entry.get("data")


def _post_api(client, name, data, page, method="POST"):
    """Read-then-write counterpart to _get_api, mirroring core.DasanClient.post
    but for the `?api=` query scheme used by StaticRouting* objects."""
    path = f"/dm/tr98/?api={name}&page={page}"
    _, csrf = client._request("GET", path)
    if not csrf:
        raise core.RouterError("did not receive a CSRF token from the router")
    payload, _ = client._request(method, path, body={name: {"data": data}}, extra_headers={"X-Csrf-Token": csrf})
    entry = payload.get(name, {})
    if entry.get("status_code") != 200:
        raise core.RouterError(f"{name}: {entry.get('error')}")
    return entry.get("data")


def _delete_api(client, name, data, page):
    return _post_api(client, name, data, page, method="DELETE")


@app.command("wan-connections")
def wan_connections(
    ctx: typer.Context,
    show_password: bool = typer.Option(False, "--show-password", help="reveal PPPoE username/password"),
):
    """Detailed WAN connection view: WANObject slots plus IP/PPPoE connection data."""
    client = ctx.obj

    objects = client.get("WANObject")
    rows = [[o["iid"], o["ServiceList"] or "-", o["ConnectionType"], o["Active"], o["IsDefault"]] for o in objects]
    core.render_table(rows, ["iid", "service", "type", "active", "default"], title="WAN Objects")

    ip_conns = client.get("WANIPConnection")
    rows = []
    for c in ip_conns:
        rows.append([
            c["iid"], c["ServiceList"] or "-", c["connectionType"], c["enable"], c["connectionStatus"],
            c["externalIPAddress"] or "-", c["defaultGateway"] or "-", c["VLANId"],
            c["ip6Mode"] or "-", c["ip6AddrGlobal"] or c["ip6AddrLocal"] or "-",
        ])
    core.render_table(
        rows,
        ["iid", "service", "type", "enable", "status", "ipv4", "gateway", "vlan", "ipv6-mode", "ipv6-addr"],
        title="WAN IP Connections",
    )

    ppp_conns = client.get("WANPPPConnection")
    rows = []
    for c in ppp_conns:
        user = c["username"] if show_password else "*" * min(len(c["username"]), 8) if c["username"] else "-"
        pw = c["password"] if show_password else "*" * min(len(c["password"]), 8) if c["password"] else "-"
        rows.append([
            c["iid"], c["ServiceList"] or "-", c["enable"], c["connectionStatus"],
            c["externalIPAddress"] or "-", c["defaultGateway"] or "-", c["VLANId"], user, pw,
        ])
    core.render_table(
        rows, ["iid", "service", "enable", "status", "ipv4", "gateway", "vlan", "username", "password"],
        title="WAN PPPoE Connections",
    )


@app.command()
def arp(ctx: typer.Context):
    """ARP table (IPv4) and ARP timeout setting."""
    client = ctx.obj
    entries = client.get("ARPStatus", page=ARP_STATUS_PAGE)
    rows = [[e["Index"], e["IP"], e["MAC"], e["IntfType"]] for e in entries]
    core.render_table(rows, ["index", "ip", "mac", "interface"], title="ARP Table")

    setting = client.get("ARPSetting", page=ARP_SETTING_PAGE)
    core.console.print(f"[bold]ARP timeout:[/bold] {setting['timeout']}s")

    try:
        client.get("ARP6Status", page=ARP_STATUS_PAGE)
    except core.RouterError:
        core.console.print("[dim]IPv6 ARP (neighbor) table not accessible with this account.[/dim]")


@app.command("arp-set-timeout")
def arp_set_timeout(
    ctx: typer.Context,
    timeout: int = typer.Argument(..., help="ARP cache timeout in seconds (typically 100-1800)"),
):
    """Set the ARP cache timeout."""
    client = ctx.obj
    client.post("ARPSetting", {"timeout": timeout}, page=ARP_SETTING_PAGE)
    core.console.print(f"[green]ARP timeout set to {timeout}s[/green]")


@app.command()
def ddns(ctx: typer.Context, show_password: bool = typer.Option(False, "--show-password", help="reveal DDNS password")):
    """Current DDNS (dynamic DNS) configuration."""
    client = ctx.obj
    entries = client.get("DDnsCfg", page=DDNS_PAGE)
    rows = []
    for e in entries:
        pw = e["Password"] if show_password else "*" * min(len(e["Password"]), 8) if e["Password"] else "-"
        rows.append([
            e["iid"], e["Enable"], e["ProviderName"], e["MyHostName"] or "-", e["Username"] or "-",
            pw, e["Ifname"], e["Status"],
        ])
    core.render_table(rows, ["iid", "enabled", "provider", "hostname", "username", "password", "interface", "status"])

    try:
        client.get("DDnsV6Cfg", page=DDNS_PAGE)
    except core.RouterError:
        core.console.print("[dim]IPv6 DDNS config not accessible with this account.[/dim]")


@app.command("ddns-set")
def ddns_set(
    ctx: typer.Context,
    enable: Optional[bool] = typer.Option(None, "--enable/--disable", help="turn DDNS updates on/off"),
    provider: Optional[str] = typer.Option(None, help="DDNS provider domain, e.g. www.no-ip.com, www.dyn.com"),
    hostname: Optional[str] = typer.Option(None, help="hostname to keep updated, e.g. myhome.no-ip.com"),
    username: Optional[str] = typer.Option(None, help="DDNS account username"),
    password: Optional[str] = typer.Option(None, help="DDNS account password"),
    interface: Optional[str] = typer.Option(None, help="WAN interface DDNS tracks, e.g. WAN0, WAN2"),
    wildcard: Optional[bool] = typer.Option(None, help="enable wildcard DNS support"),
):
    """Configure DDNS settings (read-modify-write; only touches fields you pass).

    Note: this router's DDnsCfg validation rejects an empty MyHostName outright
    (error 9006 "Choose IP address value") and silently drops an empty Username
    (200 OK, but the old value sticks) -- once set, those two fields can't be
    cleared back to blank through this API. Pass a real replacement value if
    you need to change them.
    """
    client = ctx.obj
    entries = client.get("DDnsCfg", page=DDNS_PAGE)
    target = entries[0]
    if enable is not None:
        target["Enable"] = enable
    if provider is not None:
        target["ProviderName"] = provider
    if hostname is not None:
        target["MyHostName"] = hostname
    if username is not None:
        target["Username"] = username
    if password is not None:
        target["Password"] = password
    if interface is not None:
        target["Ifname"] = interface
    if wildcard is not None:
        target["Wildcard"] = wildcard
    client.post("DDnsCfg", [target], page=DDNS_PAGE)
    core.console.print(f"[green]DDNS config updated (iid={target['iid']})[/green]")


@app.command("static-routing")
def static_routing(ctx: typer.Context):
    """List configured static routes (IPv4 and IPv6)."""
    client = ctx.obj
    v4 = _get_api(client, "StaticRoutingObject", STATIC_ROUTING_PAGE)
    if v4:
        rows = [[r.get("iid"), r.get("DestIp"), r.get("Netmask"), r.get("Gateway"), r.get("Metric"), r.get("Interface")] for r in v4]
        core.render_table(rows, ["iid", "destination", "netmask", "gateway", "metric", "interface"], title="Static Routes (IPv4)")
    else:
        core.console.print("[dim]No IPv4 static routes configured.[/dim]")

    v6 = _get_api(client, "StaticRoutingIpv6Object", STATIC_ROUTING_PAGE)
    if v6:
        rows = [[r.get("iid"), r.get("dstIp"), r.get("prefixLen"), r.get("gateway"), r.get("intfName")] for r in v6]
        core.render_table(rows, ["iid", "destination", "prefix-len", "gateway", "interface"], title="Static Routes (IPv6)")
    else:
        core.console.print("[dim]No IPv6 static routes configured.[/dim]")

    try:
        _get_api(client, "RouteTable", STATIC_ROUTING_PAGE)
    except core.RouterError:
        core.console.print("[dim]Kernel route table (RouteTable) not accessible with this account.[/dim]")


@app.command("static-routing-add")
def static_routing_add(
    ctx: typer.Context,
    destination: str = typer.Option(..., help="destination network IP, e.g. 10.99.99.0"),
    netmask: str = typer.Option(..., help="subnet mask, e.g. 255.255.255.0"),
    interface: str = typer.Option(..., help="egress interface, e.g. LAN, WAN0, WAN2"),
    gateway: str = typer.Option("0.0.0.0", help="gateway IP (0.0.0.0 to route directly via interface)"),
    metric: int = typer.Option(0, help="route metric"),
):
    """Add a static IPv4 route."""
    client = ctx.obj
    routes = _get_api(client, "StaticRoutingObject", STATIC_ROUTING_PAGE)
    next_iid = max([r["iid"] for r in routes], default=0) + 1
    entry = {
        "iid": next_iid, "DestIp": destination, "Netmask": netmask,
        "Gateway": gateway, "Interface": interface, "Metric": metric,
    }
    _post_api(client, "StaticRoutingObject", [entry], STATIC_ROUTING_PAGE)
    core.console.print(f"[green]Added static route iid={next_iid}: {destination}/{netmask} via {interface}[/green]")


@app.command("static-routing-delete")
def static_routing_delete(
    ctx: typer.Context,
    iid: int = typer.Argument(..., help="route id (see `dasan advanced static-routing`)"),
):
    """Delete a static IPv4 route."""
    client = ctx.obj
    routes = _get_api(client, "StaticRoutingObject", STATIC_ROUTING_PAGE)
    target = next((r for r in routes if r["iid"] == iid), None)
    if not target:
        raise core.RouterError(f"no static route with iid={iid}")
    _delete_api(client, "StaticRoutingObject", [target], STATIC_ROUTING_PAGE)
    core.console.print(f"[green]Deleted static route iid={iid}[/green]")


@app.command("static-routing-add-v6")
def static_routing_add_v6(
    ctx: typer.Context,
    destination: str = typer.Option(..., help="destination IPv6 network, e.g. fd00:1::"),
    prefix_len: int = typer.Option(..., help="prefix length, e.g. 64"),
    interface: str = typer.Option(..., help="egress interface, e.g. LAN, WAN0, WAN2"),
    gateway: str = typer.Option(..., help="gateway IPv6 address"),
):
    """Add a static IPv6 route."""
    client = ctx.obj
    routes = _get_api(client, "StaticRoutingIpv6Object", STATIC_ROUTING_PAGE)
    next_iid = max([r["iid"] for r in routes], default=0) + 1
    entry = {"iid": next_iid, "dstIp": destination, "prefixLen": prefix_len, "gateway": gateway, "intfName": interface}
    _post_api(client, "StaticRoutingIpv6Object", [entry], STATIC_ROUTING_PAGE)
    core.console.print(f"[green]Added static IPv6 route iid={next_iid}: {destination}/{prefix_len} via {interface}[/green]")


@app.command("static-routing-delete-v6")
def static_routing_delete_v6(
    ctx: typer.Context,
    iid: int = typer.Argument(..., help="route id (see `dasan advanced static-routing`)"),
):
    """Delete a static IPv6 route."""
    client = ctx.obj
    routes = _get_api(client, "StaticRoutingIpv6Object", STATIC_ROUTING_PAGE)
    target = next((r for r in routes if r["iid"] == iid), None)
    if not target:
        raise core.RouterError(f"no static IPv6 route with iid={iid}")
    _delete_api(client, "StaticRoutingIpv6Object", [target], STATIC_ROUTING_PAGE)
    core.console.print(f"[green]Deleted static IPv6 route iid={iid}[/green]")


# ---- DHCP static reservations (pin a device's IP by MAC) ----


@app.command("dhcp-reservations")
def dhcp_reservations(ctx: typer.Context):
    """List DHCP static IP reservations, plus the server's pool range."""
    client = ctx.obj
    pool = client.get("DhcpServerConfiguration", page=LAN_SETUP_PAGE)
    core.console.print(f"[bold]DHCP pool:[/bold] {pool['MinAddress']} - {pool['MaxAddress']} "
                        f"(lease {pool['DHCPLeaseTime']}s, gateway {pool['DefaultGateway']})")
    reservations = client.get("DHCPStaticLease", page=LAN_SETUP_PAGE)
    rows = [[r["Index"], r["IP"], r["MAC"]] for r in reservations]
    core.render_table(rows, ["index", "ip", "mac"], title="DHCP Static Reservations")


@app.command("dhcp-reservation-add")
def dhcp_reservation_add(
    ctx: typer.Context,
    ip: str = typer.Option(..., help="IP address to always assign, e.g. 192.168.1.2"),
    mac: str = typer.Option(..., help="device MAC address, e.g. AC:E2:D3:0E:49:11 "
                                       "(see `dasan status clients` or `dasan advanced arp`)"),
):
    """Pin a device to a fixed IP via a DHCP static reservation (by MAC address)."""
    client = ctx.obj
    reservations = client.get("DHCPStaticLease", page=LAN_SETUP_PAGE)
    mac_u = mac.upper()
    if any(r["IP"] == ip for r in reservations):
        raise core.RouterError(f"{ip} is already reserved")
    if any(r["MAC"].upper() == mac_u for r in reservations):
        raise core.RouterError(f"{mac_u} already has a reservation")
    next_index = max([r["Index"] for r in reservations], default=0) + 1
    entry = {"Index": next_index, "IP": ip, "MAC": mac_u}
    client.post("DHCPStaticLease", [entry], page=LAN_SETUP_PAGE)
    core.console.print(f"[green]Reserved {ip} for {mac_u} (index={next_index})[/green]")
    core.console.print("[dim]Takes effect on the device's next DHCP renewal -- "
                        "release/renew its lease now if you want it immediately.[/dim]")


@app.command("dhcp-reservation-delete")
def dhcp_reservation_delete(
    ctx: typer.Context,
    index: int = typer.Argument(..., help="reservation index (see `dasan advanced dhcp-reservations`)"),
):
    """Remove a DHCP static reservation."""
    client = ctx.obj
    reservations = client.get("DHCPStaticLease", page=LAN_SETUP_PAGE)
    target = next((r for r in reservations if r["Index"] == index), None)
    if not target:
        raise core.RouterError(f"no reservation with index={index}")
    client.delete("DHCPStaticLease", [target], page=LAN_SETUP_PAGE)
    core.console.print(f"[green]Deleted reservation index={index}[/green]")
