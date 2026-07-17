"""MAC filtering, WiFi auto-refresh schedule, and mesh status/actions.

MAC filtering has no dedicated API object of its own: the router's
WifiSetupPage-MacFilter page reads/writes the MacAclPolicy and MacAcl fields
directly on WLANConfiguration (2.4GHz) / WLAN11acConfiguration (5GHz) -- the
same objects wifi.py's `set` command uses for SSID/password. Every command
here does a read-modify-write that touches only those two fields and leaves
everything else (SSID, password, radio state, ...) untouched.
"""
import typer
from typing import Optional

from . import core

app = typer.Typer(help="MAC filter, WiFi auto-refresh schedule, and mesh status")

MACFILTER_PAGE = "WifiSetupPage-MacFilter"
SCHEDULE_PAGE = "WifiSetupPage-WifiSchedule"
MESH_SETTING_PAGE = "WifiMeshPage-Setting"
MESH_BSS_PAGE = "WifiMeshPage-BSSConfiguration"
MESH_ACTION_PAGE = "WifiMeshPage-Action"
# The router's own Topo page JS calls the API with page="WifiMeshPage-Status",
# not "WifiMeshPage-Topo" (the route name) -- matched here for the permission check.
MESH_TOPO_PAGE = "WifiMeshPage-Status"

BAND_OBJECTS = {"2.4ghz": "WLANConfiguration", "5ghz": "WLAN11acConfiguration"}
POLICY_LABELS = {0: "disabled", 1: "allow-list", 2: "deny-list"}
SCHEDULE_DAYS = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"]


def _band_object(band: str) -> str:
    band = band.lower()
    if band not in BAND_OBJECTS:
        raise core.RouterError("band must be 2.4ghz or 5ghz")
    return BAND_OBJECTS[band]


def _find_iid(entries, iid):
    target = next((e for e in entries if e["iid"] == iid), None)
    if not target:
        raise core.RouterError(f"no WLAN with iid={iid}")
    return target


@app.command("macfilter-list")
def macfilter_list(ctx: typer.Context, band: str = typer.Option("2.4ghz", help="2.4ghz or 5ghz")):
    """Show MAC filter policy and address list for each WLAN interface."""
    client = ctx.obj
    obj = _band_object(band)
    entries = client.get(obj, page=MACFILTER_PAGE)
    rows = []
    for e in entries:
        macs = e.get("MacAcl") or []
        rows.append([e["iid"], e.get("SSID", ""), POLICY_LABELS.get(e.get("MacAclPolicy", 0), e.get("MacAclPolicy")),
                     len(macs), ", ".join(macs)])
    core.render_table(rows, ["iid", "ssid", "policy", "count", "mac addresses"], title=f"MAC Filter ({band})")


@app.command("macfilter-add")
def macfilter_add(
    ctx: typer.Context,
    iid: int = typer.Argument(..., help="WLAN interface iid (see macfilter-list)"),
    mac: str = typer.Argument(..., help="MAC address to add, e.g. AA:BB:CC:DD:EE:FF"),
    band: str = typer.Option("2.4ghz", help="2.4ghz or 5ghz"),
):
    """Add a MAC address to an interface's filter list (max 8 entries).

    Only appends to MacAcl -- never changes MacAclPolicy, so this cannot by
    itself start enforcing allow/deny rules against connected devices.
    """
    client = ctx.obj
    obj = _band_object(band)
    entries = client.get(obj, page=MACFILTER_PAGE)
    target = _find_iid(entries, iid)
    macs = list(target.get("MacAcl") or [])
    mac_u = mac.upper()
    if mac_u in [m.upper() for m in macs]:
        raise core.RouterError(f"{mac_u} is already in the filter list")
    if len(macs) >= 8:
        raise core.RouterError("filter list is full (max 8 entries)")
    macs.append(mac_u)
    target["MacAcl"] = macs
    client.post(obj, target, page=MACFILTER_PAGE)
    after = client.get(obj, page=MACFILTER_PAGE)
    if mac_u not in [m.upper() for m in (_find_iid(after, iid).get("MacAcl") or [])]:
        core.console.print(
            "[yellow]Warning: the router accepted the request but did not persist it "
            "(known firmware quirk on this model/account) -- verify in the web UI before relying on this.[/yellow]"
        )
        return
    core.console.print(f"[green]Added {mac_u} to iid={iid} ({band}) filter list[/green]")


@app.command("macfilter-delete")
def macfilter_delete(
    ctx: typer.Context,
    iid: int = typer.Argument(..., help="WLAN interface iid (see macfilter-list)"),
    mac: str = typer.Argument(..., help="MAC address to remove"),
    band: str = typer.Option("2.4ghz", help="2.4ghz or 5ghz"),
):
    """Remove a MAC address from an interface's filter list."""
    client = ctx.obj
    obj = _band_object(band)
    entries = client.get(obj, page=MACFILTER_PAGE)
    target = _find_iid(entries, iid)
    macs = list(target.get("MacAcl") or [])
    mac_u = mac.upper()
    if mac_u not in [m.upper() for m in macs]:
        raise core.RouterError(f"{mac_u} is not in the filter list")
    target["MacAcl"] = [m for m in macs if m.upper() != mac_u]
    client.post(obj, target, page=MACFILTER_PAGE)
    core.console.print(f"[green]Removed {mac_u} from iid={iid} ({band}) filter list[/green]")


@app.command("schedule-show")
def schedule_show(ctx: typer.Context):
    """Show the WiFi auto-refresh schedule (day/time trigger + unassociated-client threshold)."""
    client = ctx.obj
    cfg = client.get("WifiRefreshObj", page=SCHEDULE_PAGE)
    days = cfg.get("Schedule", "").split(",") if cfg.get("Schedule") else []
    rows = [
        ["enabled", cfg.get("Enable")],
        ["days", ", ".join(days) or "-"],
        ["time", f"{cfg.get('Hour', 0):02d}:{cfg.get('Minute', 0):02d}"],
        ["threshold enabled", cfg.get("Enable_ucount")],
        ["unassociated-client threshold", cfg.get("Ucount_threshold")],
    ]
    core.render_table(rows, ["field", "value"], title="WiFi Auto-Refresh Schedule")


@app.command("schedule-set")
def schedule_set(
    ctx: typer.Context,
    enable: Optional[bool] = typer.Option(None, "--enable/--disable", help="turn the scheduled refresh on/off"),
    days: Optional[str] = typer.Option(None, help="comma-separated days, e.g. mon,wed,fri"),
    hour: Optional[int] = typer.Option(None, min=0, max=23),
    minute: Optional[int] = typer.Option(None, min=0, max=59),
    threshold_enable: Optional[bool] = typer.Option(None, "--enable-threshold/--disable-threshold"),
    threshold: Optional[int] = typer.Option(None, help="unassociated client count that triggers a refresh"),
):
    """Update the WiFi auto-refresh schedule (read-modify-write)."""
    client = ctx.obj
    cfg = client.get("WifiRefreshObj", page=SCHEDULE_PAGE)
    if enable is not None:
        cfg["Enable"] = enable
    if days is not None:
        parsed = [d.strip().lower() for d in days.split(",") if d.strip()]
        bad = [d for d in parsed if d not in SCHEDULE_DAYS]
        if bad:
            raise core.RouterError(f"invalid day(s): {bad}; must be one of {SCHEDULE_DAYS}")
        cfg["Schedule"] = ",".join(parsed)
    if hour is not None:
        cfg["Hour"] = hour
    if minute is not None:
        cfg["Minute"] = minute
    if threshold_enable is not None:
        cfg["Enable_ucount"] = threshold_enable
    if threshold is not None:
        cfg["Ucount_threshold"] = threshold
    client.post("WifiRefreshObj", cfg, page=SCHEDULE_PAGE)
    core.console.print("[green]Updated WiFi auto-refresh schedule[/green]")


@app.command("mesh-status")
def mesh_status(ctx: typer.Context):
    """Show mesh configuration/mode (WifiMeshCfg). Requires mesh-capable hardware."""
    client = ctx.obj
    cfg = client.get("WifiMeshCfg", page=MESH_SETTING_PAGE)
    core.render_table([[k, v] for k, v in cfg.items()], ["field", "value"], title="WiFi Mesh Configuration")


@app.command("mesh-bss")
def mesh_bss(ctx: typer.Context):
    """List mesh BSS (SSID) configuration per band. Requires mesh-capable hardware."""
    client = ctx.obj
    entries = client.get("WifiMeshBssCfg", page=MESH_BSS_PAGE)
    rows = [[e.get("iid"), e.get("SSID"), e.get("auth"), e.get("encryption")] for e in entries]
    core.render_table(rows, ["iid", "ssid", "auth", "encryption"], title="Mesh BSS Configuration")


@app.command("mesh-topo")
def mesh_topo(ctx: typer.Context):
    """Show mesh topology nodes. Requires mesh-capable hardware."""
    client = ctx.obj
    topo = client.get("WifiMeshTopo", page=MESH_TOPO_PAGE)
    nodes = topo.get("nodes", [])
    rows = [[n.get("id"), n.get("type"), n.get("Mac"), n.get("title", "")] for n in nodes]
    core.render_table(rows, ["id", "type", "mac", "info"], title="Mesh Topology")


@app.command("mesh-onboard")
def mesh_onboard(ctx: typer.Context):
    """Trigger mesh onboarding mode so a new mesh node can join. Requires mesh-capable hardware."""
    client = ctx.obj
    client.post("WifiMeshAct", {"action": "TriggerOnboarding"}, page=MESH_ACTION_PAGE)
    core.console.print("[green]Mesh onboarding triggered[/green]")


@app.command("mesh-backhaul-steer")
def mesh_backhaul_steer(
    ctx: typer.Context,
    sta_mac: str = typer.Argument(..., help="backhaul station MAC to steer"),
    target_bssid: str = typer.Argument(..., help="target BSSID to steer the station to"),
):
    """Steer a mesh backhaul station to a target BSSID. Requires mesh-capable hardware."""
    client = ctx.obj
    client.post(
        "WifiMeshAct",
        {"action": "BackhaulSteering", "backhaulStaMac": sta_mac, "targetBssid": target_bssid},
        page=MESH_ACTION_PAGE,
    )
    core.console.print("[green]Backhaul steering requested[/green]")
