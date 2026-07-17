"""Fast modular CLI for the Dasan/Airtel H660GM-A GPON router."""
import typer

from . import core, status, wifi, wifi_extra, firewall, maintenance, advanced

app = typer.Typer(
    help="Fast CLI for the Dasan/Airtel router (replaces the slow web UI)",
    pretty_exceptions_enable=False,
)

wifi.app.add_typer(wifi_extra.app, name="extra", help="MAC filter, auto-refresh schedule, mesh")

app.add_typer(status.app, name="status")
app.add_typer(wifi.app, name="wifi")
app.add_typer(firewall.app, name="firewall")
app.add_typer(maintenance.app, name="maintenance")
app.add_typer(advanced.app, name="advanced")


@app.callback()
def main(
    ctx: typer.Context,
    host: str = typer.Option(core.DEFAULT_HOST, help="router IP"),
    user: str = typer.Option(None, help="username (default: prompt / $DASAN_USER)"),
    password: str = typer.Option(None, help="password (default: prompt / $DASAN_PASS)"),
    relogin: bool = typer.Option(False, help="ignore cached session and log in again"),
    verbose: bool = typer.Option(False, "-v", "--verbose", help="print requests being made"),
):
    try:
        ctx.obj = core.get_client(host, user=user, password=password, relogin=relogin, verbose=verbose)
    except core.RouterError as e:
        core.err_console.print(f"Error: {e}")
        raise typer.Exit(1)


@app.command()
def reboot(ctx: typer.Context, yes: bool = typer.Option(False, "-y", "--yes", help="skip confirmation")):
    """Reboot the router."""
    client = ctx.obj
    if not yes:
        confirmed = typer.confirm(f"Reboot the router at {client.host}? This will drop your connection.")
        if not confirmed:
            core.console.print("Aborted.")
            raise typer.Exit()
    client.post("Reboot", {"rebootReason": "reboot"}, namespace="sys")
    core.console.print("[green]Reboot command sent. The router is restarting.[/green]")


@app.command()
def raw(
    ctx: typer.Context,
    method: str = typer.Argument(..., help="get, post, or delete"),
    objs: str = typer.Argument(..., help="object name, e.g. DeviceInfo, WLANConfiguration, PortForwarding.2"),
    namespace: str = typer.Option("tr98", help="tr98, sys, or bin"),
    page: str = typer.Option(None, help="page id for the permission check (auto-guessed if omitted)"),
    data: str = typer.Option(None, help='JSON body for post/delete, e.g. \'{"iid":1,"SSID":"foo"}\''),
):
    """Escape hatch: call any objs/cmd endpoint directly."""
    import json
    client = ctx.obj
    method = method.lower()
    if method == "get":
        result = client.get(objs, page=page, namespace=namespace)
    else:
        body = json.loads(data) if data else {}
        fn = client.post if method == "post" else client.delete
        result = fn(objs, body, page=page, namespace=namespace)
    core.console.print_json(data=result)


def entrypoint():
    try:
        app()
    except core.RouterError as e:
        core.err_console.print(f"Error: {e}")
        raise SystemExit(1)


if __name__ == "__main__":
    entrypoint()
