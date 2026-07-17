"""Firewall & NAT: port forwarding, DMZ, port triggering, filters, etc."""
import typer
from typing import Optional

from . import core

app = typer.Typer(help="Firewall/NAT rules: port forwarding, DMZ, filters, UPnP")

PF_PAGE = "FirewallSetupPage-PortForwarding"


@app.command("port-forwarding")
def port_forwarding(
    ctx: typer.Context,
    wan: int = typer.Option(2, help="WAN connection iid to inspect (see `dasan status wan`; "
                                     "2 is typically the INTERNET_TR069 connection)"),
):
    """List port forwarding rules for a WAN connection."""
    client = ctx.obj
    rules = client.get(f"PortForwarding.{wan}", page=PF_PAGE)
    rows = [[r["eid"], "on" if r["active"] else "off", r["Protocol"], f"{r['StartPort']}-{r['EndPort']}",
             r["LocalIP"], f"{r['LocalSPort']}-{r['LocalEPort']}", r["Comment"]] for r in rules]
    core.render_table(rows, ["eid", "active", "proto", "ext-port", "local-ip", "local-port", "comment"],
                       title=f"Port Forwarding (WAN iid={wan})")


@app.command("port-forwarding-add")
def port_forwarding_add(
    ctx: typer.Context,
    ext_port: int = typer.Option(..., help="external (start) port"),
    local_ip: str = typer.Option(..., help="internal device IP"),
    local_port: Optional[int] = typer.Option(None, help="internal port (defaults to ext-port)"),
    ext_port_end: Optional[int] = typer.Option(None, help="external end port for a range"),
    protocol: str = typer.Option("TCP/UDP", help="TCP, UDP, TCP/UDP, or GRE"),
    comment: str = typer.Option("", help="optional label"),
    wan: int = typer.Option(2, help="WAN connection iid"),
):
    """Add a port forwarding rule."""
    client = ctx.obj
    local_port = local_port or ext_port
    ext_end = ext_port_end or ext_port
    local_end = local_port + (ext_end - ext_port)
    rules = client.get(f"PortForwarding.{wan}", page=PF_PAGE)
    next_eid = max([r["eid"] for r in rules], default=0) + 1
    entry = {
        "iid": wan, "eid": next_eid, "active": True, "Protocol": protocol,
        "StartPort": ext_port, "EndPort": ext_end,
        "LocalIP": local_ip, "LocalSPort": local_port, "LocalEPort": local_end,
        "Comment": comment,
    }
    # The API expects the entry wrapped in a list, even for a single record.
    client.post(f"PortForwarding.{wan}", [entry], page=PF_PAGE)
    core.console.print(f"[green]Added port forwarding rule eid={next_eid}[/green]")


@app.command("port-forwarding-delete")
def port_forwarding_delete(
    ctx: typer.Context,
    eid: int = typer.Argument(..., help="rule id (see `dasan firewall port-forwarding`)"),
    wan: int = typer.Option(2, help="WAN connection iid"),
):
    """Delete a port forwarding rule."""
    client = ctx.obj
    rules = client.get(f"PortForwarding.{wan}", page=PF_PAGE)
    target = next((r for r in rules if r["eid"] == eid), None)
    if not target:
        raise core.RouterError(f"no rule with eid={eid} on WAN iid={wan}")
    # Delete requires echoing back the full record, not just its identifiers.
    client.delete(f"PortForwarding.{wan}", [target], page=PF_PAGE)
    core.console.print(f"[green]Deleted port forwarding rule eid={eid}[/green]")


# ---- DMZ ----

DMZ_PAGE = "FirewallSetupPage-Dmz"


def _dmz_row(client, wan):
    rows = client.get("DmzHostConfig", page=DMZ_PAGE)
    intf = f"WAN{wan}"
    return rows, intf, next((r for r in rows if r["intfName"] == intf), None)


@app.command("dmz")
def dmz_show(ctx: typer.Context):
    """Show DMZ host configuration for all WAN interfaces."""
    client = ctx.obj
    rows = client.get("DmzHostConfig", page=DMZ_PAGE)
    out = [[r["intfName"], "on" if r["enable"] else "off", r.get("IPAddress", "")] for r in rows]
    core.render_table(out, ["interface", "enabled", "host-ip"], title="DMZ Host Configuration")


@app.command("dmz-enable")
def dmz_enable(
    ctx: typer.Context,
    ip: Optional[str] = typer.Option(None, help="DMZ host IP (required if not already set for this interface)"),
    wan: int = typer.Option(2, help="WAN connection iid"),
):
    """Enable DMZ, forwarding all unmatched traffic to the host IP."""
    client = ctx.obj
    rows, intf, existing = _dmz_row(client, wan)
    host_ip = ip or (existing.get("IPAddress") if existing else None)
    if not host_ip:
        raise core.RouterError(f"no DMZ host IP set for {intf} yet; pass --ip")
    entry = dict(existing) if existing else {"intfName": intf}
    entry["enable"] = True
    entry["IPAddress"] = host_ip
    client.post("DmzHostConfig", [entry], page=DMZ_PAGE)
    core.console.print(f"[green]DMZ enabled on {intf} -> {host_ip}[/green]")


@app.command("dmz-disable")
def dmz_disable(ctx: typer.Context, wan: int = typer.Option(2, help="WAN connection iid")):
    """Disable DMZ for a WAN interface."""
    client = ctx.obj
    rows, intf, existing = _dmz_row(client, wan)
    entry = dict(existing) if existing else {"intfName": intf, "IPAddress": ""}
    entry["enable"] = False
    client.post("DmzHostConfig", [entry], page=DMZ_PAGE)
    core.console.print(f"[green]DMZ disabled on {intf}[/green]")


@app.command("dmz-set-host")
def dmz_set_host(
    ctx: typer.Context,
    ip: str = typer.Argument(..., help="internal device IP to expose as the DMZ host"),
    wan: int = typer.Option(2, help="WAN connection iid"),
):
    """Set the DMZ host IP without changing whether DMZ is enabled."""
    client = ctx.obj
    rows, intf, existing = _dmz_row(client, wan)
    entry = dict(existing) if existing else {"intfName": intf, "enable": False}
    entry["IPAddress"] = ip
    client.post("DmzHostConfig", [entry], page=DMZ_PAGE)
    core.console.print(f"[green]DMZ host on {intf} set to {ip}[/green]")


# ---- Port triggering ----

PT_PAGE = "FirewallSetupPage-PortTriggering"


@app.command("port-triggering")
def port_triggering(ctx: typer.Context):
    """List port triggering rules."""
    client = ctx.obj
    rules = client.get("PortTriggering", page=PT_PAGE)
    rows = [[r["iid"], r["Name"], r["TProtocol"], f"{r['TSPort']}-{r['TEPort']}",
             r["OProtocol"], f"{r['OSPort']}-{r['OEPort']}"] for r in rules]
    core.render_table(rows, ["iid", "name", "trigger-proto", "trigger-port", "open-proto", "open-port"],
                       title="Port Triggering")


@app.command("port-triggering-add")
def port_triggering_add(
    ctx: typer.Context,
    name: str = typer.Option(..., help="rule label"),
    trigger_port: int = typer.Option(..., help="trigger (outbound) start port"),
    trigger_port_end: Optional[int] = typer.Option(None, help="trigger end port (defaults to trigger-port)"),
    trigger_protocol: str = typer.Option("TCP", help="TCP, UDP, or TCP/UDP"),
    open_port: int = typer.Option(..., help="opened (inbound) start port"),
    open_port_end: Optional[int] = typer.Option(None, help="opened end port (defaults to open-port)"),
    open_protocol: str = typer.Option("TCP", help="TCP, UDP, or TCP/UDP"),
):
    """Add a port triggering rule."""
    client = ctx.obj
    rules = client.get("PortTriggering", page=PT_PAGE)
    next_iid = max([r["iid"] for r in rules], default=0) + 1
    entry = {
        "iid": next_iid, "Name": name,
        "TProtocol": trigger_protocol, "TSPort": trigger_port, "TEPort": trigger_port_end or trigger_port,
        "OProtocol": open_protocol, "OSPort": open_port, "OEPort": open_port_end or open_port,
    }
    client.post("PortTriggering", [entry], page=PT_PAGE)
    core.console.print(f"[green]Added port triggering rule iid={next_iid}[/green]")


@app.command("port-triggering-delete")
def port_triggering_delete(
    ctx: typer.Context,
    iid: int = typer.Argument(..., help="rule id (see `dasan firewall port-triggering`)"),
):
    """Delete a port triggering rule."""
    client = ctx.obj
    rules = client.get("PortTriggering", page=PT_PAGE)
    target = next((r for r in rules if r["iid"] == iid), None)
    if not target:
        raise core.RouterError(f"no port triggering rule with iid={iid}")
    client.delete("PortTriggering", [target], page=PT_PAGE)
    core.console.print(f"[green]Deleted port triggering rule iid={iid}[/green]")


# ---- URL filter ----

UF_PAGE = "FirewallSetupPage-UrlFilter"


@app.command("url-filter")
def url_filter(ctx: typer.Context):
    """List URL filter rules."""
    client = ctx.obj
    rules = client.get("URLFilterObject", page=UF_PAGE)
    rows = [[r["Index"], r["URL"], "active" if r["Activate"] else "inactive"] for r in rules]
    core.render_table(rows, ["index", "url", "state"], title="URL Filter")


@app.command("url-filter-add")
def url_filter_add(
    ctx: typer.Context,
    url: str = typer.Option(..., help="URL or keyword to block"),
    active: bool = typer.Option(True, help="activate the rule immediately"),
):
    """Add a URL filter rule."""
    client = ctx.obj
    rules = client.get("URLFilterObject", page=UF_PAGE)
    next_index = max([r["Index"] for r in rules], default=0) + 1
    entry = {"Index": next_index, "URL": url, "Activate": active}
    client.post("URLFilterObject", [entry], page=UF_PAGE)
    core.console.print(f"[green]Added URL filter rule index={next_index}[/green]")


@app.command("url-filter-delete")
def url_filter_delete(
    ctx: typer.Context,
    index: int = typer.Argument(..., help="rule index (see `dasan firewall url-filter`)"),
):
    """Delete a URL filter rule."""
    client = ctx.obj
    rules = client.get("URLFilterObject", page=UF_PAGE)
    target = next((r for r in rules if r["Index"] == index), None)
    if not target:
        raise core.RouterError(f"no URL filter rule with index={index}")
    client.delete("URLFilterObject", [target], page=UF_PAGE)
    core.console.print(f"[green]Deleted URL filter rule index={index}[/green]")


# ---- Parental control ----
# ParentalControlObj is a fixed 7-entry weekly schedule (iid 0=Mon..6=Sun) with a
# StartTime/EndTime blocking window and an Enable flag per day. The router expects
# the *entire* list resubmitted on every save, not a single appended/removed record.

PC_PAGE = "FirewallSetupPage-ParentalControl"
_PC_DAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]


@app.command("parental-control")
def parental_control(ctx: typer.Context):
    """Show the weekly parental control blocking schedule."""
    client = ctx.obj
    rows = client.get("ParentalControlObj", page=PC_PAGE)
    out = [[_PC_DAYS[r["iid"]] if r["iid"] < len(_PC_DAYS) else r["iid"],
            "on" if r["Enable"] else "off", r["StartTime"], r["EndTime"]] for r in rows]
    core.render_table(out, ["day", "enabled", "start", "end"], title="Parental Control Schedule")


@app.command("parental-control-add")
def parental_control_add(
    ctx: typer.Context,
    day: int = typer.Argument(..., help="day index, 0=Mon .. 6=Sun"),
    start: str = typer.Option(..., help="start time, HH:MM (5-minute increments)"),
    end: str = typer.Option(..., help="end time, HH:MM (5-minute increments)"),
):
    """Enable the blocking window for a day of the week."""
    client = ctx.obj
    rows = client.get("ParentalControlObj", page=PC_PAGE)
    target = next((r for r in rows if r["iid"] == day), None)
    if not target:
        raise core.RouterError(f"no parental control schedule entry for day iid={day}")
    target["Enable"] = True
    target["StartTime"] = start
    target["EndTime"] = end
    client.post("ParentalControlObj", rows, page=PC_PAGE)
    core.console.print(f"[green]Parental control enabled on {_PC_DAYS[day]} {start}-{end}[/green]")


@app.command("parental-control-delete")
def parental_control_delete(
    ctx: typer.Context,
    day: int = typer.Argument(..., help="day index, 0=Mon .. 6=Sun"),
):
    """Disable the blocking window for a day of the week."""
    client = ctx.obj
    rows = client.get("ParentalControlObj", page=PC_PAGE)
    target = next((r for r in rows if r["iid"] == day), None)
    if not target:
        raise core.RouterError(f"no parental control schedule entry for day iid={day}")
    target["Enable"] = False
    client.post("ParentalControlObj", rows, page=PC_PAGE)
    core.console.print(f"[green]Parental control disabled on {_PC_DAYS[day]}[/green]")


# ---- UPnP ----

UPNP_PAGE = "FirewallSetupPage-UPnP"


@app.command("upnp")
def upnp_show(ctx: typer.Context):
    """Show whether UPnP is enabled."""
    client = ctx.obj
    cfg = client.get("UPnPCfg", page=UPNP_PAGE)
    core.console.print(f"UPnP: {'[green]enabled[/green]' if cfg['enable'] else '[yellow]disabled[/yellow]'}")


def _upnp_set(client, enable):
    client.post("UPnPCfg", {"enable": enable}, page=UPNP_PAGE)
    after = client.get("UPnPCfg", page=UPNP_PAGE)
    if after["enable"] != enable:
        core.console.print(
            "[yellow]Warning: the router accepted the request but did not change state "
            "(known firmware quirk on this model/account) -- verify in the web UI.[/yellow]"
        )


@app.command("upnp-enable")
def upnp_enable(ctx: typer.Context):
    """Enable UPnP. Note: this write is accepted but silently ignored by the
    router's firmware on at least one tested unit/account -- always re-check
    with `dasan firewall upnp` afterward."""
    _upnp_set(ctx.obj, True)
    core.console.print("[green]UPnP enable requested[/green]")


@app.command("upnp-disable")
def upnp_disable(ctx: typer.Context):
    """Disable UPnP. Note: this write is accepted but silently ignored by the
    router's firmware on at least one tested unit/account -- always re-check
    with `dasan firewall upnp` afterward."""
    _upnp_set(ctx.obj, False)
    core.console.print("[green]UPnP disable requested[/green]")


# ---- MAC / IP anti-spoofing (read-only) ----

MAS_PAGE = "FirewallSetupPage-MACAntiSpoofing"
IAS_PAGE = "FirewallSetupPage-IPAntiSpoofing"


@app.command("mac-anti-spoofing")
def mac_anti_spoofing(ctx: typer.Context):
    """List configured MAC anti-spoofing rules."""
    client = ctx.obj
    rules = client.get("MacAntiSpoofingCfg", page=MAS_PAGE)
    rows = [[r["Index"], r["Mac"], r["PortBind"], "active" if r["Active"] else "inactive"] for r in rules]
    core.render_table(rows, ["index", "mac", "port-bind", "state"], title="MAC Anti-Spoofing Rules")


@app.command("mac-anti-spoofing-table")
def mac_anti_spoofing_table(ctx: typer.Context):
    """Show the learned MAC-per-LAN-port binding table."""
    client = ctx.obj
    rows = client.get("MacAntiSpoofingTable", page=MAS_PAGE)
    if not rows:
        core.console.print("(empty)")
        return
    headers = list(rows[0].keys())
    core.render_table([[r.get(h, "") for h in headers] for r in rows], headers,
                       title="MAC Anti-Spoofing Table")


@app.command("ip-anti-spoofing")
def ip_anti_spoofing(ctx: typer.Context):
    """List configured IP anti-spoofing rules."""
    client = ctx.obj
    rules = client.get("IPAntiSpoofingCfg", page=IAS_PAGE)
    rows = [[r["Index"], r["IP"], r["Mask"], r["PortBind"], "active" if r["Active"] else "inactive"]
            for r in rules]
    core.render_table(rows, ["index", "ip", "mask", "port-bind", "state"], title="IP Anti-Spoofing Rules")


@app.command("ip-anti-spoofing-table")
def ip_anti_spoofing_table(ctx: typer.Context):
    """Show the learned IP-per-LAN-port binding table."""
    client = ctx.obj
    rows = client.get("IPAntiSpoofingTable", page=IAS_PAGE)
    if not rows:
        core.console.print("(empty)")
        return
    headers = list(rows[0].keys())
    core.render_table([[r.get(h, "") for h in headers] for r in rows], headers,
                       title="IP Anti-Spoofing Table")
