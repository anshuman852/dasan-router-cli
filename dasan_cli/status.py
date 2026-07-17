"""Status & network overview: device info, WAN, LAN, DHCP clients."""
import typer
from typing_extensions import Annotated

from . import core

app = typer.Typer(help="Device status, WAN/LAN, and connected clients")


@app.command()
def info(ctx: typer.Context):
    """Device info, uptime, CPU/mem/temp, PON optical stats."""
    client = ctx.obj
    info = client.get("DeviceInfo")
    pon = client.get("PonPortStatus")
    core.console.print(f"[bold]Model:[/bold]        {info['manufacturer']} {info['modelName']}")
    core.console.print(f"[bold]Serial:[/bold]       {info['serialNumber']}")
    core.console.print(f"[bold]HW/SW ver:[/bold]    {info['hardwareVersion']} / {info['softwareVersion']}")
    core.console.print(f"[bold]Uptime:[/bold]       {info['UpTime']}s ({info['UpTime']//3600}h{(info['UpTime']%3600)//60}m)")
    core.console.print(f"[bold]CPU / Mem:[/bold]    {info['CPU_Load']}% / {info['MemoryUsage']}%")
    core.console.print(f"[bold]Temperature:[/bold]  {info['Temperature']} C")
    core.console.print(f"[bold]MAC:[/bold]          {info['MACAddress']}")
    core.console.print()
    core.console.print(f"[bold]PON:[/bold]          {pon['ponMode']} link {pon['ponLinkState']} "
                        f"(uptime {pon['ponLinkUptime']}s), OLT {pon['oltType']}")
    core.console.print(f"[bold]Optical:[/bold]      Rx {pon['ponRxPower']} dBm / Tx {pon['ponTxPower']} dBm, "
                        f"temp {pon['ponTemp']} C, FEC {pon['fecStatus']}")


@app.command()
def wan(ctx: typer.Context):
    """WAN connection summary (internet/voice VLANs, IPs, status)."""
    client = ctx.obj
    conns = client.get("WANIPConnection")
    rows = []
    for c in conns:
        if not c.get("enable") and c.get("connectionType") == "Unconfigured":
            continue
        rows.append([
            c["iid"], c["ServiceList"], c["connectionType"], c["connectionStatus"],
            c["externalIPAddress"] or "-", c["defaultGateway"] or "-", c["VLANId"],
        ])
    core.render_table(rows, ["iid", "service", "type", "status", "ip", "gateway", "vlan"])


@app.command()
def lan(ctx: typer.Context):
    """LAN port link status."""
    client = ctx.obj
    ports = client.get("LANPortStatus")
    rows = [[p["iid"], p["Admin"], p["Status"], p["Mode"]] for p in ports]
    core.render_table(rows, ["port", "admin", "status", "mode"])


@app.command()
def clients(ctx: typer.Context):
    """DHCP leases / connected devices."""
    client = ctx.obj
    leases = client.get("DhcpLease")
    rows = [[l["ClientName"], l["IP"], l["MAC"], l["ExpireTime"]] for l in leases]
    core.render_table(rows, ["name", "ip", "mac", "lease-expires"])
