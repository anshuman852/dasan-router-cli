"""WiFi SSID configuration."""
import typer
from typing import Optional

from . import core

app = typer.Typer(help="WiFi SSIDs (2.4GHz/5GHz radios)")


@app.command("list")
def list_(ctx: typer.Context, show_password: bool = typer.Option(False, "--show-password", help="reveal WiFi passphrases")):
    """Show all configured SSIDs."""
    client = ctx.obj
    ssids = client.get("WLANConfiguration")
    rows = []
    for s in ssids:
        pw = s["KeyPassphrase"] if show_password else "*" * min(len(s["KeyPassphrase"]), 8)
        rows.append([s["iid"], s["SSID"], "up" if s["RadioEnabled"] else "down",
                     s["Security"], pw, "visible" if s["SSIDAdvertisementEnabled"] else "hidden"])
    core.render_table(rows, ["iid", "ssid", "radio", "security", "password", "broadcast"])


@app.command()
def set(
    ctx: typer.Context,
    iid: int = typer.Argument(..., help="WLAN index (see `dasan wifi list`)"),
    ssid: Optional[str] = typer.Option(None, help="new SSID name"),
    password: Optional[str] = typer.Option(None, help="new WiFi passphrase"),
    enable: Optional[bool] = typer.Option(None, "--enable/--disable", help="turn radio on/off"),
):
    """Update an SSID's name/password/radio state (read-modify-write)."""
    client = ctx.obj
    ssids = client.get("WLANConfiguration")
    target = next((s for s in ssids if s["iid"] == iid), None)
    if not target:
        raise core.RouterError(f"no WLAN with iid={iid}")
    if ssid:
        target["SSID"] = ssid
    if password:
        target["KeyPassphrase"] = password
    if enable is not None:
        target["RadioEnabled"] = enable
    client.post("WLANConfiguration", target)
    core.console.print(f"[green]Updated WLAN iid={iid}[/green]")
