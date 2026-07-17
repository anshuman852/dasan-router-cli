"""Maintenance: administration, logs, NTP, backup, firmware, and misc read-only pages."""
import urllib.error
import urllib.request

import typer

from . import core

app = typer.Typer(help="Administration, logs, NTP, backup, firmware, and misc maintenance pages")

ADMIN_PAGE = "MaintainancePage-Administration"
LOGS_PAGE = "MaintainancePage-SystemLog"
NTP_PAGE = "MaintainancePage-NTP"
BACKUP_PAGE = "MaintainancePage-BackupRestore"
FIRMWARE_PAGE = "MaintainancePage-FirmwareUpgrade"
AUTO_REBOOT_PAGE = "MaintainancePage-AutoReboot"
PORT_MIRRORING_PAGE = "MaintainancePage-PortMirroring"
SNMP_PAGE = "MaintainancePage-Snmp"
SYSLOG_CONF_PAGE = "MaintainancePage-SyslogConfiguration"


def _raw_get(client, path):
    """GET a non-JSON (plain text / binary) endpoint the /dm/ JSON wrapper can't handle."""
    req = urllib.request.Request(client.base + path, method="GET")
    if client.token:
        req.add_header("Authorization", f"Bearer {client.token}")
    try:
        with urllib.request.urlopen(req, context=core.SSL_CTX, timeout=15) as resp:
            return resp.read(), resp.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        raise core.RouterError(f"HTTP {e.code} on {path}: {e.read()[:200]}")


@app.command()
def administration(ctx: typer.Context):
    """Web account settings: admin ports, idle timeout, login lockout (read-only)."""
    client = ctx.obj
    d = client.get("Administration", page="", namespace="sys")
    core.console.print(f"[bold]HTTP port:[/bold]        {d['WebPort']} (enabled: {d['HttpEnable']})")
    core.console.print(f"[bold]HTTPS port:[/bold]       {d['HttpsPort']}")
    core.console.print(f"[bold]Access from WAN:[/bold]  {d['WebOnWan']}")
    core.console.print(f"[bold]Idle timeout:[/bold]     {d['timeout']}s")
    core.console.print(f"[bold]Max login trials:[/bold] {d['MaxTrial']} (lockout {d['BannedTime']}s)")


@app.command()
def ntp(ctx: typer.Context):
    """NTP server configuration and sync status (read-only)."""
    client = ctx.obj
    d = client.get("TimeServer", page=NTP_PAGE)
    core.console.print(f"[bold]Enabled:[/bold]      {d['enable']}")
    core.console.print(f"[bold]Status:[/bold]       {d['status']}")
    core.console.print(f"[bold]Current time:[/bold] {d['currentLocalTime']} ({d['localTimeZone']})")
    servers = [d.get(f"NTPServer{i}") for i in range(1, 5) if d.get(f"NTPServer{i}")]
    core.console.print(f"[bold]NTP servers:[/bold]  {', '.join(servers)}")
    core.console.print(f"[bold]DST:[/bold]          {d['daylightSavingsUsed']}")


@app.command()
def firmware(ctx: typer.Context):
    """Current firmware/hardware version (read-only; upgrade must be done via the web UI)."""
    client = ctx.obj
    d = client.get("DeviceInfo")
    core.console.print(f"[bold]Manufacturer:[/bold]     {d['manufacturer']}")
    core.console.print(f"[bold]Model:[/bold]            {d['modelName']}")
    core.console.print(f"[bold]Hardware version:[/bold] {d['hardwareVersion']}")
    core.console.print(f"[bold]Software version:[/bold] {d['softwareVersion']}")


@app.command()
def logs(
    ctx: typer.Context,
    lines: int = typer.Option(50, help="number of most recent log lines to show"),
):
    """Show recent system log entries (read-only)."""
    client = ctx.obj
    raw, _ = _raw_get(client, f"/bin/?objs=SyslogDownload&page={LOGS_PAGE}")
    text = raw.decode("utf-8", errors="replace")
    all_lines = [l for l in text.splitlines() if l.strip()]
    for line in all_lines[-lines:]:
        core.console.print(line)


app.command("system-log")(logs)


@app.command()
def backup(
    ctx: typer.Context,
    output: str = typer.Option(None, help="output file path (default: backup_<timestamp>.bin)"),
):
    """Download the current router configuration backup (GET only, no restore)."""
    client = ctx.obj
    raw, content_type = _raw_get(client, f"/bin/?objs=BackupConfig&page={BACKUP_PAGE}")
    import time
    path = output or f"backup_{time.strftime('%Y%m%d_%H%M%S')}.bin"
    with open(path, "wb") as f:
        f.write(raw)
    core.console.print(f"[green]Saved config backup to {path} ({len(raw)} bytes, {content_type})[/green]")


@app.command("auto-reboot")
def auto_reboot(ctx: typer.Context):
    """Scheduled auto-reboot configuration (read-only)."""
    client = ctx.obj
    d = client.get("AutoRebootObj", page=AUTO_REBOOT_PAGE)
    core.console.print(f"[bold]Enabled:[/bold]        {d['Enable']}")
    core.console.print(f"[bold]Daily at:[/bold]       {d['Hour']:02d}:{d['Minute']:02d}")
    core.console.print(f"[bold]Min uptime:[/bold]     {d['Uptime']}min")
    core.console.print(f"[bold]Uptime-count reboot:[/bold] {d['Enable_ucount']} (threshold {d['Ucount_threshold']})")


@app.command("port-mirroring")
def port_mirroring(ctx: typer.Context):
    """LAN port mirroring configuration (read-only)."""
    client = ctx.obj
    d = client.get("PortMirroring", page=PORT_MIRRORING_PAGE)
    core.console.print(f"[bold]Enabled:[/bold] {d['Enable']}")
    ports = [k.replace("Port_", "") for k in d if k.startswith("Port_") and d[k]]
    core.console.print(f"[bold]Mirrored ports:[/bold] {', '.join(ports) or '-'}")


@app.command()
def snmp(ctx: typer.Context):
    """SNMP agent configuration (read-only; community/password values are masked)."""
    client = ctx.obj
    d = client.get("SnmpCfg", page=SNMP_PAGE)

    def mask(v):
        return "***" if v else "-"

    core.console.print(f"[bold]SNMP v1/v2 enabled:[/bold] {d['snmpActive']}")
    core.console.print(f"[bold]Get community:[/bold]     {mask(d.get('getCommunity'))}")
    core.console.print(f"[bold]Set community:[/bold]     {mask(d.get('setCommunity'))}")
    core.console.print(f"[bold]Trap manager:[/bold]      {d.get('trapManagerIPv4')}")
    core.console.print(f"[bold]sysName/Contact/Loc:[/bold] {d.get('sysName')} / {d.get('sysContact')} / {d.get('sysLocation')}")
    core.console.print(f"[bold]SNMPv3 enabled:[/bold]     {d['snmpV3Active']}")
    if d["snmpV3Active"]:
        core.console.print(f"[bold]SNMPv3 user:[/bold]        {d.get('snmpV3UserName')}")
        core.console.print(f"[bold]SNMPv3 password:[/bold]    {mask(d.get('snmpV3Password'))}")


@app.command("syslog-configuration")
def syslog_configuration(ctx: typer.Context):
    """Remote syslog forwarding configuration (read-only; password is masked)."""
    client = ctx.obj
    d = client.get("RemoteSyslogCfg", page=SYSLOG_CONF_PAGE)
    core.console.print(f"[bold]Enabled:[/bold]  {d['isActive']}")
    core.console.print(f"[bold]Protocol:[/bold] {d['protocol']}")
    core.console.print(f"[bold]Host:[/bold]     {d['host'] or '-'}")
    core.console.print(f"[bold]Level:[/bold]    {d['level']}")
    if d.get("userName"):
        core.console.print(f"[bold]User:[/bold]     {d['userName']} (password: {'***' if d.get('passwd') else '-'})")
