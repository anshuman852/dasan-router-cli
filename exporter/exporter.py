"""Prometheus metrics collector for the Dasan H660GM-A GPON router.

Collects device health, PON optical, WAN, LAN, WiFi, DHCP, firewall, and
system metrics by querying the router's internal JSON API via DasanClient.
"""
import logging
import re
import time

from prometheus_client import Gauge, Counter, Histogram, Info

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Scrape intervals (seconds)
# ---------------------------------------------------------------------------
# Objects whose data changes slowly (ARP table, DHCP leases) are collected
# less frequently to reduce load on the router.
SLOW_INTERVAL = 300   # 5 minutes
FAST_INTERVAL = 0     # every scrape


class DasanMetricsCollector:
    """Holds all Prometheus metrics and knows how to populate them from the
    Dasan router API.

    Usage::

        client = DasanClient("192.168.1.1")
        client.login("admin", "password")
        collector = DasanMetricsCollector(client)
        collector.collect()          # populate all metrics
        # ... serve /metrics via prometheus_client ...
    """

    _CONN_STATUS_MAP = {"Connected": 1, "Disconnected": 0, "Connecting": 2}
    _PON_LINK_MAP = {"Up": 1, "Down": 0}

    def __init__(self, client):
        self.client = client
        self._router_host = client.host
        self._last_collected = {}  # obj_key -> last timestamp (float)

        # ---- Device Health ----
        self.device_uptime_seconds = Gauge(
            "dasan_device_uptime_seconds", "Router uptime", ["router_host"])
        self.device_cpu_load_percent = Gauge(
            "dasan_device_cpu_load_percent", "CPU load", ["router_host"])
        self.device_memory_usage_percent = Gauge(
            "dasan_device_memory_usage_percent", "Memory usage", ["router_host"])
        self.device_temperature_celsius = Gauge(
            "dasan_device_temperature_celsius", "Temperature", ["router_host"])

        # ---- PON Optical ----
        self.pon_link_state = Gauge(
            "dasan_pon_link_state", "PON link (1=Up, 0=Down)", ["router_host"])
        self.pon_link_uptime_seconds = Gauge(
            "dasan_pon_link_uptime_seconds", "PON link uptime", ["router_host"])
        self.pon_rx_power_dbm = Gauge(
            "dasan_pon_rx_power_dbm", "PON RX optical power", ["router_host"])
        self.pon_tx_power_dbm = Gauge(
            "dasan_pon_tx_power_dbm", "PON TX optical power", ["router_host"])
        self.pon_temperature_celsius = Gauge(
            "dasan_pon_temperature_celsius", "PON module temperature", ["router_host"])

        # ---- WAN ----
        self.wan_ip_connection_status = Gauge(
            "dasan_wan_ip_connection_status",
            "WAN connection (0=Disconnected,1=Connected,2=Connecting)",
            ["router_host", "service", "vlan"])
        self.wan_pppoe_connection_status = Gauge(
            "dasan_wan_pppoe_connection_status",
            "WAN PPPoE connection",
            ["router_host", "service", "vlan"])

        # ---- LAN ----
        self.lan_port_admin_up = Gauge(
            "dasan_lan_port_admin_up", "LAN port admin state", ["router_host", "port"])
        self.lan_port_link_up = Gauge(
            "dasan_lan_port_link_up", "LAN port link state", ["router_host", "port"])
        self.lan_port_speed_mbps = Gauge(
            "dasan_lan_port_speed_mbps", "LAN port speed", ["router_host", "port"])

        # ---- DHCP ----
        self.dhcp_active_leases_total = Gauge(
            "dasan_dhcp_active_leases_total", "Active DHCP leases", ["router_host"])

        # ---- WiFi ----
        self.wifi_radio_enabled = Gauge(
            "dasan_wifi_radio_enabled", "WiFi radio state", ["router_host", "iid", "ssid"])

        # ---- Firewall counts ----
        self.port_forwarding_rules_total = Gauge(
            "dasan_port_forwarding_rules_total", "Port forwarding rules", ["router_host"])
        self.dmz_enabled = Gauge(
            "dasan_dmz_enabled", "DMZ state", ["router_host", "interface"])
        self.upnp_enabled = Gauge(
            "dasan_upnp_enabled", "UPnP state", ["router_host"])
        self.url_filter_rules_total = Gauge(
            "dasan_url_filter_rules_total", "URL filter rules", ["router_host"])

        # ---- System ----
        self.ntp_sync_status = Gauge(
            "dasan_ntp_sync_status",
            "NTP sync (0=unsync,1=synced,2=error)",
            ["router_host"])
        self.arp_entries_total = Gauge(
            "dasan_arp_entries_total", "ARP table entries", ["router_host"])
        self.auto_reboot_enabled = Gauge(
            "dasan_auto_reboot_enabled", "Auto-reboot state", ["router_host"])

        # ---- API errors (for monitoring the exporter itself) ----
        self.api_requests_total = Counter(
            "dasan_api_requests_total",
            "Total API requests",
            ["router_host", "object", "status"])
        self.api_request_duration_seconds = Histogram(
            "dasan_api_request_duration_seconds",
            "API request duration",
            ["router_host", "object"])

        # ---- Exporter info ----
        self.exporter_info = Info("dasan_exporter", "Exporter metadata")
        self.exporter_info.info({"version": "1.0.0"})

    # ------------------------------------------------------------------
    # helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _bool_val(v):
        """Normalise a value that may be a Python bool, a string, or an int
        to a numeric 0/1."""
        if isinstance(v, bool):
            return 1 if v else 0
        if isinstance(v, str):
            return 1 if v.lower() in ("true", "1", "yes", "up") else 0
        if isinstance(v, (int, float)):
            return 1 if v != 0 else 0
        return 0

    @staticmethod
    def _parse_float(v, default=0.0):
        """Parse *v* as a float, returning *default* on failure."""
        if v is None:
            return default
        if isinstance(v, (int, float)):
            return float(v)
        try:
            return float(v)
        except (ValueError, TypeError):
            return default

    def _should_collect(self, key, interval):
        """Return True when *interval* seconds have elapsed since the last
        fetch identified by *key*."""
        if interval == 0:
            return True
        last = self._last_collected.get(key, 0.0)
        return (time.time() - last) >= interval

    def _mark_collected(self, key):
        """Record that *key* was just fetched (so the interval gate resets)."""
        self._last_collected[key] = time.time()

    def _safe_get(self, objs, page=None, interval=0):
        """Fetch *objs* from the router via ``DasanClient.get()``, but only
        if the *interval* gate allows it.

        Returns the parsed data on success, ``None`` when skipped or on error.
        On error a warning is logged and ``api_requests_total`` is incremented
        with ``status="error"``.
        """
        if not self._should_collect(objs, interval):
            return None

        start = time.time()
        api_name = objs.split(".")[0]
        try:
            data = self.client.get(objs, page=page)
            self.api_requests_total.labels(
                router_host=self._router_host, object=api_name,
                status="success",
            ).inc()
            self.api_request_duration_seconds.labels(
                router_host=self._router_host, object=api_name,
            ).observe(time.time() - start)
            self._mark_collected(objs)
            return data
        except Exception as e:
            logger.warning("Failed to collect %s: %s", objs, e)
            self.api_requests_total.labels(
                router_host=self._router_host, object=api_name,
                status="error",
            ).inc()
            self._mark_collected(objs)
            return None

    # ------------------------------------------------------------------
    # per-object collection methods
    # ------------------------------------------------------------------

    def _collect_device_info(self):
        data = self._safe_get("DeviceInfo", "StatusPage-DeviceInfo", FAST_INTERVAL)
        if data is None:
            return
        rh = self._router_host
        self.device_uptime_seconds.labels(router_host=rh).set(
            int(data.get("UpTime", 0)))
        self.device_cpu_load_percent.labels(router_host=rh).set(
            self._parse_float(data.get("CPU_Load", 0)))
        self.device_memory_usage_percent.labels(router_host=rh).set(
            self._parse_float(data.get("MemoryUsage", 0)))
        self.device_temperature_celsius.labels(router_host=rh).set(
            self._parse_float(data.get("Temperature", 0)))

    def _collect_pon_status(self):
        data = self._safe_get("PonPortStatus", "StatusPage-DeviceInfo", FAST_INTERVAL)
        if data is None:
            return
        rh = self._router_host
        link_str = data.get("ponLinkState", "Down")
        self.pon_link_state.labels(router_host=rh).set(
            self._PON_LINK_MAP.get(link_str, 0))
        self.pon_link_uptime_seconds.labels(router_host=rh).set(
            int(data.get("ponLinkUptime", 0)))
        self.pon_rx_power_dbm.labels(router_host=rh).set(
            self._parse_float(data.get("ponRxPower", 0)))
        self.pon_tx_power_dbm.labels(router_host=rh).set(
            self._parse_float(data.get("ponTxPower", 0)))
        self.pon_temperature_celsius.labels(router_host=rh).set(
            self._parse_float(data.get("ponTemp", 0)))

    def _collect_wan(self):
        rh = self._router_host

        # -- IP connections --
        ip_data = self._safe_get(
            "WANIPConnection", "AdvancedSetupPage-WANConnection", FAST_INTERVAL)
        if ip_data is not None:
            for conn in ip_data:
                svc = conn.get("ServiceList", "")
                vlan = str(conn.get("VLANId", ""))
                status_val = self._CONN_STATUS_MAP.get(
                    conn.get("connectionStatus", "Disconnected"), 0)
                self.wan_ip_connection_status.labels(
                    router_host=rh, service=svc, vlan=vlan).set(status_val)

        # -- PPPoE connections --
        ppp_data = self._safe_get(
            "WANPPPConnection", "AdvancedSetupPage-WANConnection", FAST_INTERVAL)
        if ppp_data is not None:
            for conn in ppp_data:
                svc = conn.get("ServiceList", "")
                vlan = str(conn.get("VLANId", ""))
                status_val = self._CONN_STATUS_MAP.get(
                    conn.get("connectionStatus", "Disconnected"), 0)
                self.wan_pppoe_connection_status.labels(
                    router_host=rh, service=svc, vlan=vlan).set(status_val)

    def _collect_lan_ports(self):
        data = self._safe_get("LANPortStatus", "StatusPage-DeviceInfo", FAST_INTERVAL)
        if data is None:
            return
        rh = self._router_host
        for port in data:
            pid = str(port.get("iid", ""))
            admin_str = port.get("Admin", "Down")
            status_str = port.get("Status", "Down")
            mode_str = port.get("Mode", "")

            self.lan_port_admin_up.labels(router_host=rh, port=pid).set(
                1 if admin_str.lower() == "up" else 0)
            self.lan_port_link_up.labels(router_host=rh, port=pid).set(
                1 if status_str.lower() == "up" else 0)

            # Extract numeric speed from e.g. "1000M-Full", "100M-Half"
            speed = 0
            if mode_str:
                m = re.search(r"(\d+)M", mode_str)
                if m:
                    speed = int(m.group(1))
            self.lan_port_speed_mbps.labels(router_host=rh, port=pid).set(speed)

    def _collect_dhcp(self):
        data = self._safe_get("DhcpLease", "StatusPage-DHCPLease", SLOW_INTERVAL)
        if data is None:
            return
        self.dhcp_active_leases_total.labels(
            router_host=self._router_host).set(len(data))

    def _collect_wifi(self):
        data = self._safe_get(
            "WLANConfiguration", "WifiSetupPage-WirelessSetting", FAST_INTERVAL)
        if data is None:
            return
        rh = self._router_host
        for radio in data:
            iid = str(radio.get("iid", ""))
            ssid = radio.get("SSID", "")
            enabled = self._bool_val(radio.get("RadioEnabled", False))
            self.wifi_radio_enabled.labels(
                router_host=rh, iid=iid, ssid=ssid).set(enabled)

    def _collect_firewall(self):
        rh = self._router_host

        # -- Port forwarding (needs WAN iids from WANObject) --
        total_pf = 0
        wan_data = self._safe_get(
            "WANObject", "AdvancedSetupPage-WANConnection", FAST_INTERVAL)
        if wan_data is not None:
            for wan in wan_data:
                wan_iid = wan.get("iid")
                if wan_iid is not None:
                    pf_data = self._safe_get(
                        f"PortForwarding.{wan_iid}",
                        "FirewallSetupPage-PortForwarding",
                        FAST_INTERVAL)
                    if pf_data is not None:
                        total_pf += len(pf_data)
        self.port_forwarding_rules_total.labels(router_host=rh).set(total_pf)

        # -- DMZ --
        dmz_data = self._safe_get(
            "DmzHostConfig", "FirewallSetupPage-Dmz", FAST_INTERVAL)
        if dmz_data is not None:
            for entry in dmz_data:
                intf = entry.get("intfName", "")
                enabled = self._bool_val(entry.get("enable", False))
                self.dmz_enabled.labels(router_host=rh, interface=intf).set(enabled)

        # -- UPnP --
        upnp_data = self._safe_get(
            "UPnPCfg", "FirewallSetupPage-UPnP", FAST_INTERVAL)
        if upnp_data is not None:
            enabled = self._bool_val(upnp_data.get("enable", False))
            self.upnp_enabled.labels(router_host=rh).set(enabled)

        # -- URL filter --
        url_data = self._safe_get(
            "URLFilterObject", "FirewallSetupPage-UrlFilter", FAST_INTERVAL)
        if url_data is not None:
            self.url_filter_rules_total.labels(router_host=rh).set(len(url_data))

    def _collect_ntp(self):
        data = self._safe_get("TimeServer", "MaintainancePage-NTP", FAST_INTERVAL)
        if data is None:
            return
        status_str = data.get("status", "")
        if status_str.lower() == "synced":
            val = 1
        elif status_str.lower() in ("unsync", "unsynchronized"):
            val = 0
        else:
            val = 2
        self.ntp_sync_status.labels(router_host=self._router_host).set(val)

    def _collect_arp(self):
        data = self._safe_get("ARPStatus", "StatusPage-ARP", SLOW_INTERVAL)
        if data is None:
            return
        self.arp_entries_total.labels(
            router_host=self._router_host).set(len(data))

    def _collect_auto_reboot(self):
        data = self._safe_get(
            "AutoRebootObj", "MaintainancePage-AutoReboot", FAST_INTERVAL)
        if data is None:
            return
        enabled = self._bool_val(data.get("Enable", False))
        self.auto_reboot_enabled.labels(
            router_host=self._router_host).set(enabled)

    # ------------------------------------------------------------------
    # public API
    # ------------------------------------------------------------------

    def collect(self):
        """Run a full collection cycle.

        Each per-object method handles its own interval check, so slow-moving
        data (ARP, DHCP) is only queried every *SLOW_INTERVAL* seconds while
        everything else is fetched on every call.
        """
        self._collect_device_info()
        self._collect_pon_status()
        self._collect_wan()
        self._collect_lan_ports()
        self._collect_dhcp()
        self._collect_wifi()
        self._collect_firewall()
        self._collect_ntp()
        self._collect_arp()
        self._collect_auto_reboot()
