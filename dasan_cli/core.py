"""Shared HTTP/auth/CSRF client for the Dasan/Airtel router's internal JSON API.

The router's web UI is a Vue SPA that talks to /dm/tr98/, /dm/sys/ and /bin/
JSON endpoints. See README.md for how this was reverse-engineered.
"""
import getpass
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.request

from rich.console import Console
from rich.table import Table

# Windows terminals often default to a legacy codepage (437/1252) that can't
# render the unicode ellipsis rich uses when truncating wide table cells.
for _stream in (sys.stdout, sys.stderr):
    try:
        _stream.reconfigure(encoding="utf-8")
    except (AttributeError, ValueError):
        pass

DEFAULT_HOST = os.environ.get("DASAN_HOST", "192.168.1.1")
SESSION_FILE = os.path.join(os.path.expanduser("~"), ".dasan-cli-session.json")

SSL_CTX = ssl.create_default_context()
SSL_CTX.check_hostname = False
SSL_CTX.verify_mode = ssl.CERT_NONE

console = Console()
err_console = Console(stderr=True, style="red")

# object name -> page id, used by the router for a per-page permission check.
# Keys may be a plain object name ("DeviceInfo") or an object name plus a
# dotted suffix used verbatim in the query string ("PortForwarding.<wanIid>").
PAGES = {
    "DeviceInfo": "StatusPage-DeviceInfo",
    "HWInfo": "StatusPage-DeviceInfo",
    "PonPortStatus": "StatusPage-DeviceInfo",
    "LANPortStatus": "StatusPage-DeviceInfo",
    "WANObject": "AdvancedSetupPage-WANConnection",
    "WANIPConnection": "AdvancedSetupPage-WANConnection",
    "WANPPPConnection": "AdvancedSetupPage-WANConnection",
    "DhcpLease": "StatusPage-DHCPLease",
    "WLANConfiguration": "WifiSetupPage-WirelessSetting",
    "WLANCommon": "WifiSetupPage-WirelessSetting",
    "WLANOnOff": "WifiSetupPage-WirelessSetting",
    "Reboot": "MaintainancePage-Reboot",
    "RestoreFactory": "MaintainancePage-Reboot",
    "PortForwarding": "FirewallSetupPage-PortForwarding",
    "FireWallCfg": "FirewallSetupPage-PortForwarding",
    "DmzHostConfig": "FirewallSetupPage-Dmz",
    "PortTriggering": "FirewallSetupPage-PortTriggering",
    "URLCommon": "FirewallSetupPage-UrlFilter",
    "URLFilterObject": "FirewallSetupPage-UrlFilter",
    "ParentalControlObj": "FirewallSetupPage-ParentalControl",
    "ParentalCommonObj": "FirewallSetupPage-ParentalControl",
    "UPnPCfg": "FirewallSetupPage-UPnP",
    "UPnPRules": "FirewallSetupPage-UPnP",
    "MacAntiSpoofingCfg": "FirewallSetupPage-MACAntiSpoofing",
    "MacAntiSpoofingTable": "FirewallSetupPage-MACAntiSpoofing",
    "IPAntiSpoofingCfg": "FirewallSetupPage-IPAntiSpoofing",
    "IPAntiSpoofingCommon": "FirewallSetupPage-IPAntiSpoofing",
    "IPAntiSpoofingTable": "FirewallSetupPage-IPAntiSpoofing",
    "DHCPStaticLease": "AdvancedSetupPage-LANSetup",
    "DhcpServerConfiguration": "AdvancedSetupPage-LANSetup",
}


class RouterError(Exception):
    pass


class DasanClient:
    def __init__(self, host, verbose=False):
        self.host = host
        self.base = f"https://{host}"
        self.token = None
        self.verbose = verbose

    # ---- low level ----

    def _request(self, method, path, body=None, extra_headers=None):
        url = self.base + path
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, method=method)
        req.add_header("Content-Type", "application/json")
        if self.token:
            req.add_header("Authorization", f"Bearer {self.token}")
        for k, v in (extra_headers or {}).items():
            req.add_header(k, v)
        if self.verbose:
            print(f"  -> {method} {path}", file=sys.stderr)
        try:
            with urllib.request.urlopen(req, context=SSL_CTX, timeout=10) as resp:
                raw = resp.read()
                csrf = resp.headers.get("csrf")
                return json.loads(raw), csrf
        except urllib.error.HTTPError as e:
            raw = e.read()
            csrf = e.headers.get("csrf")
            try:
                return json.loads(raw), csrf
            except ValueError:
                raise RouterError(f"HTTP {e.code}: {raw[:200]}")

    def _top_key(self, objs):
        """Response bodies key by the bare object name even when the request
        used a dotted suffix, e.g. objs=PortForwarding.2 -> {"PortForwarding": ...}."""
        return objs.split(".")[0]

    def get(self, objs, page=None, namespace="tr98"):
        page = page if page is not None else PAGES.get(objs, "")
        payload, _ = self._request("GET", f"/dm/{namespace}/?objs={objs}&page={page}")
        entry = payload.get(self._top_key(objs), {})
        if entry.get("status_code") != 200:
            raise RouterError(f"{objs}: {entry.get('error')}")
        return entry.get("data")

    def post(self, objs, data, page=None, namespace="tr98", method="POST"):
        """Read-then-write: GETs first to obtain a fresh CSRF token, then POSTs/DELETEs."""
        page = page if page is not None else PAGES.get(objs, "")
        path = f"/dm/{namespace}/?objs={objs}&page={page}"
        _, csrf = self._request("GET", path)
        if not csrf:
            raise RouterError("did not receive a CSRF token from the router")
        key = self._top_key(objs)
        payload, _ = self._request(
            method, path, body={key: {"data": data}}, extra_headers={"X-Csrf-Token": csrf}
        )
        entry = payload.get(key, {})
        if entry.get("status_code") != 200:
            raise RouterError(f"{objs}: {entry.get('error')}")
        return entry.get("data")

    def delete(self, objs, data, page=None, namespace="tr98"):
        return self.post(objs, data, page=page, namespace=namespace, method="DELETE")

    def cmd(self, name, data=None, method="get"):
        path = f"/dm/sys/?cmd={name}"
        if method == "get":
            payload, _ = self._request("GET", path)
        else:
            _, csrf = self._request("GET", path)
            payload, _ = self._request(
                "POST", path, body={name: {"data": data or {}}},
                extra_headers={"X-Csrf-Token": csrf} if csrf else {},
            )
        entry = payload.get(name, {})
        if entry.get("status_code") != 200:
            raise RouterError(f"{name}: {entry.get('error')}")
        return entry.get("data")

    # ---- auth ----

    def login(self, username, password):
        payload, _ = self._request(
            "POST",
            "/dm/sys/?cmd=Login",
            body={"Login": {"data": {"username": username, "password": password, "captcha": ""}}},
        )
        login = payload.get("Login", {}).get("data", {}).get("login")
        if not login or login.get("status") != "success":
            raise RouterError(f"login failed: {payload}")
        self.token = login["authenticatedToken"]
        return self.token

    def logout(self):
        self._request("GET", "/dm/sys/?cmd=Logout")
        self.token = None


def load_session():
    if os.path.exists(SESSION_FILE):
        try:
            with open(SESSION_FILE) as f:
                return json.load(f)
        except (ValueError, OSError):
            return None
    return None


def save_session(host, token):
    with open(SESSION_FILE, "w") as f:
        json.dump({"host": host, "token": token, "saved_at": time.time()}, f)
    try:
        os.chmod(SESSION_FILE, 0o600)
    except OSError:
        pass


def get_client(host, user=None, password=None, relogin=False, verbose=False):
    client = DasanClient(host, verbose=verbose)
    session = load_session()
    if session and session.get("host") == host and not relogin:
        client.token = session["token"]
        try:
            client.get("DeviceInfo")
            return client
        except RouterError:
            pass  # token expired, fall through to a fresh login

    username = user or os.environ.get("DASAN_USER") or input("Username: ")
    pw = password or os.environ.get("DASAN_PASS") or getpass.getpass("Password: ")
    client.login(username, pw)
    save_session(host, client.token)
    return client


def render_table(rows, headers, title=None):
    table = Table(title=title, show_lines=False)
    for h in headers:
        table.add_column(str(h))
    for r in rows:
        table.add_row(*[str(c) for c in r])
    console.print(table)
