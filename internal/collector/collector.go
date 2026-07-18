// Package collector implements Prometheus metric collection from the
// Dasan/Airtel router API.
package collector

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/anshuman852/dasan/internal/client"
)

// ---------------------------------------------------------------------------
// Helper functions for extracting values from the router's JSON responses
// ---------------------------------------------------------------------------

// routerData is returned by DasanClient.Get — either map[string]any or []any.
type routerData = any

// getStr returns the string value for key from m, or "".
func getStr(m map[string]any, key string) string {
	return client.GetStr(m, key)
}

// getFloat returns the numeric value for key from m, handling string, float,
// int, and bool representations. Returns 0 on failure.
func getFloat(m map[string]any, key string) float64 {
	return client.GetFloat(m, key)
}

// getBool returns true for truthy values (bool true, string "true"/"True"/"Up", int 1).
func getBool(m map[string]any, key string) bool {
	return client.GetBool(m, key)
}

// parseSpeed extracts the numeric Mbps from a LAN mode string like "1000M-Full".
func parseSpeed(mode string) float64 {
	if mode == "" {
		return 0
	}
	i := strings.IndexByte(mode, 'M')
	if i <= 0 {
		return 0
	}
	f, err := strconv.ParseFloat(mode[:i], 64)
	if err != nil {
		return 0
	}
	return f
}

// ---------------------------------------------------------------------------
// Prometheus metrics
// ---------------------------------------------------------------------------

var (
	// ---- Device Health ----
	deviceUptimeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_device_uptime_seconds", Help: "Router uptime in seconds.",
	})
	deviceCpuLoadPercent = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_device_cpu_load_percent", Help: "CPU load percentage.",
	})
	deviceMemoryUsagePercent = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_device_memory_usage_percent", Help: "Memory usage percentage.",
	})
	deviceTemperatureCelsius = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_device_temperature_celsius", Help: "Device temperature in Celsius.",
	})

	// ---- PON Optical ----
	ponLinkState = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_pon_link_state", Help: "PON link state (1=Up, 0=Down).",
	})
	ponLinkUptimeSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_pon_link_uptime_seconds", Help: "PON link uptime in seconds.",
	})
	ponRxPowerDbm = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_pon_rx_power_dbm", Help: "PON RX optical power in dBm.",
	})
	ponTxPowerDbm = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_pon_tx_power_dbm", Help: "PON TX optical power in dBm.",
	})
	ponTemperatureCelsius = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_pon_temperature_celsius", Help: "PON module temperature in Celsius.",
	})

	// ---- WAN ----
	wanIPConnectionStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_wan_ip_connection_status",
		Help: "WAN IP connection status (0=Disconnected, 1=Connected, 2=Connecting).",
	}, []string{"service", "vlan"})
	wanPPPoEConnectionStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_wan_pppoe_connection_status",
		Help: "WAN PPPoE connection status (0=Disconnected, 1=Connected, 2=Connecting).",
	}, []string{"service", "vlan"})

	// ---- LAN ----
	lanPortAdminUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_lan_port_admin_up", Help: "LAN port admin state (1=Up, 0=Down).",
	}, []string{"port"})
	lanPortLinkUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_lan_port_link_up", Help: "LAN port link state (1=Up, 0=Down).",
	}, []string{"port"})
	lanPortSpeedMbps = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_lan_port_speed_mbps", Help: "LAN port speed in Mbps.",
	}, []string{"port"})

	// ---- DHCP / ARP ----
	dhcpActiveLeasesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_dhcp_active_leases_total", Help: "Number of active DHCP leases.",
	})
	arpEntriesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_arp_entries_total", Help: "Number of ARP table entries.",
	})

	// ---- WiFi ----
	wifiRadioEnabled = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_wifi_radio_enabled", Help: "WiFi radio enabled (1=Yes, 0=No).",
	}, []string{"iid", "ssid"})

	// ---- Firewall ---
	portForwardingRulesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_port_forwarding_rules_total", Help: "Number of port forwarding rules.",
	})
	dmzEnabled = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dasan_dmz_enabled", Help: "DMZ enabled (1=Yes, 0=No).",
	}, []string{"interface"})
	upnpEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_upnp_enabled", Help: "UPnP enabled (1=Yes, 0=No).",
	})
	urlFilterRulesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_url_filter_rules_total", Help: "Number of URL filter rules.",
	})

	// ---- System ----
	ntpSyncStatus = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_ntp_sync_status", Help: "NTP sync status (0=unsync, 1=synced, 2=error).",
	})
	autoRebootEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dasan_auto_reboot_enabled", Help: "Auto-reboot enabled (1=Yes, 0=No).",
	})

	// ---- Exporter self-monitoring ----
	apiRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dasan_api_requests_total", Help: "Total API requests to the router.",
	}, []string{"object", "status"})
	apiRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dasan_api_request_duration_seconds",
		Help:    "API request duration in seconds.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"object"})
)

// ---------------------------------------------------------------------------
// connection / link status maps
// ---------------------------------------------------------------------------

var connStatusMap = map[string]float64{
	"connected":    1,
	"disconnected": 0,
	"connecting":   2,
}

// ---------------------------------------------------------------------------
// Collector — orchestrates metric collection with interval gating.
// ---------------------------------------------------------------------------

const (
	fastInterval = 0   // collect every scrape
	slowInterval = 300 // 5 minutes
)

// Collector holds the DasanClient and last-collected timestamps.
type Collector struct {
	client        *client.DasanClient
	lastCollected map[string]time.Time
	mu            sync.Mutex
}

// NewCollector creates a Collector backed by the given client.
func NewCollector(cl *client.DasanClient) *Collector {
	return &Collector{
		client:        cl,
		lastCollected: make(map[string]time.Time),
	}
}

// Collect runs a full collection cycle. Each per-object method handles its
// own interval check so slow-moving data is only fetched every slowInterval.
func (c *Collector) Collect() {
	c.collectDeviceInfo()
	c.collectPonStatus()
	c.collectWAN()
	c.collectLANPorts()
	c.collectDHCP()
	c.collectWiFi()
	c.collectFirewall()
	c.collectNTP()
	c.collectARP()
	c.collectAutoReboot()
}

// ---------------------------------------------------------------------------
// interval gating helpers
// ---------------------------------------------------------------------------

func (c *Collector) shouldCollect(key string, interval time.Duration) bool {
	if interval == 0 {
		return true
	}
	c.mu.Lock()
	last, ok := c.lastCollected[key]
	c.mu.Unlock()
	if !ok {
		return true
	}
	return time.Since(last) >= interval
}

func (c *Collector) markCollected(key string) {
	c.mu.Lock()
	c.lastCollected[key] = time.Now()
	c.mu.Unlock()
}

// safeGet fetches objs from the router, tracks duration and status, and
// respects the interval gate. Returns nil when skipped or on error.
func (c *Collector) safeGet(objs, page string, interval time.Duration) routerData {
	if !c.shouldCollect(objs, interval) {
		return nil
	}
	topKey := strings.Split(objs, ".")[0]
	start := time.Now()
	data, err := c.client.Get(objs, page)
	dur := time.Since(start).Seconds()
	apiRequestDurationSeconds.WithLabelValues(topKey).Observe(dur)

	if err != nil {
		log.Printf("collect %s: %v", objs, err)
		apiRequestsTotal.WithLabelValues(topKey, "error").Inc()
		c.markCollected(objs)
		return nil
	}
	apiRequestsTotal.WithLabelValues(topKey, "success").Inc()
	c.markCollected(objs)
	return data
}

// ---------------------------------------------------------------------------
// Per-object collection
// ---------------------------------------------------------------------------

func (c *Collector) collectDeviceInfo() {
	data := c.safeGet("DeviceInfo", "StatusPage-DeviceInfo", fastInterval)
	if data == nil {
		return
	}
	obj, err := client.ToObj(data)
	if err != nil {
		log.Printf("DeviceInfo: %v", err)
		return
	}
	deviceUptimeSeconds.Set(getFloat(obj, "UpTime"))
	deviceCpuLoadPercent.Set(getFloat(obj, "CPU_Load"))
	deviceMemoryUsagePercent.Set(getFloat(obj, "MemoryUsage"))
	deviceTemperatureCelsius.Set(getFloat(obj, "Temperature"))
}

func (c *Collector) collectPonStatus() {
	data := c.safeGet("PonPortStatus", "StatusPage-DeviceInfo", fastInterval)
	if data == nil {
		return
	}
	obj, err := client.ToObj(data)
	if err != nil {
		log.Printf("PonPortStatus: %v", err)
		return
	}
	linkStr := strings.ToLower(getStr(obj, "ponLinkState"))
	ponLinkState.Set(mapVal(connStatusMap, linkStr, 0))

	ponLinkUptimeSeconds.Set(getFloat(obj, "ponLinkUptime"))
	ponRxPowerDbm.Set(getFloat(obj, "ponRxPower"))
	ponTxPowerDbm.Set(getFloat(obj, "ponTxPower"))
	ponTemperatureCelsius.Set(getFloat(obj, "ponTemp"))
}

func (c *Collector) collectWAN() {
	// IP connections
	ipData := c.safeGet("WANIPConnection", "AdvancedSetupPage-WANConnection", fastInterval)
	if ipData != nil {
		arr, err := client.ToArr(ipData)
		if err == nil {
			for _, item := range arr {
				conn, ok := item.(map[string]any)
				if !ok {
					continue
				}
				svc := getStr(conn, "ServiceList")
				vlan := fmt.Sprintf("%.0f", getFloat(conn, "VLANId"))
				status := connStatus(strings.ToLower(getStr(conn, "connectionStatus")))
				wanIPConnectionStatus.WithLabelValues(svc, vlan).Set(status)
			}
		}
	}

	// PPPoE connections
	pppData := c.safeGet("WANPPPConnection", "AdvancedSetupPage-WANConnection", fastInterval)
	if pppData != nil {
		arr, err := client.ToArr(pppData)
		if err == nil {
			for _, item := range arr {
				conn, ok := item.(map[string]any)
				if !ok {
					continue
				}
				svc := getStr(conn, "ServiceList")
				vlan := fmt.Sprintf("%.0f", getFloat(conn, "VLANId"))
				status := connStatus(strings.ToLower(getStr(conn, "connectionStatus")))
				wanPPPoEConnectionStatus.WithLabelValues(svc, vlan).Set(status)
			}
		}
	}
}

func (c *Collector) collectLANPorts() {
	data := c.safeGet("LANPortStatus", "StatusPage-DeviceInfo", fastInterval)
	if data == nil {
		return
	}
	arr, err := client.ToArr(data)
	if err != nil {
		log.Printf("LANPortStatus: %v", err)
		return
	}
	for _, item := range arr {
		port, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pid := fmt.Sprintf("%.0f", getFloat(port, "iid"))

		if strings.ToLower(getStr(port, "Admin")) == "up" {
			lanPortAdminUp.WithLabelValues(pid).Set(1)
		} else {
			lanPortAdminUp.WithLabelValues(pid).Set(0)
		}
		if strings.ToLower(getStr(port, "Status")) == "up" {
			lanPortLinkUp.WithLabelValues(pid).Set(1)
		} else {
			lanPortLinkUp.WithLabelValues(pid).Set(0)
		}
		lanPortSpeedMbps.WithLabelValues(pid).Set(parseSpeed(getStr(port, "Mode")))
	}
}

func (c *Collector) collectDHCP() {
	data := c.safeGet("DhcpLease", "StatusPage-DHCPLease", slowInterval*time.Second)
	if data == nil {
		return
	}
	arr, err := client.ToArr(data)
	if err != nil {
		log.Printf("DhcpLease: %v", err)
		return
	}
	dhcpActiveLeasesTotal.Set(float64(len(arr)))
}

func (c *Collector) collectWiFi() {
	data := c.safeGet("WLANConfiguration", "WifiSetupPage-WirelessSetting", fastInterval)
	if data == nil {
		return
	}
	arr, err := client.ToArr(data)
	if err != nil {
		log.Printf("WLANConfiguration: %v", err)
		return
	}
	for _, item := range arr {
		radio, ok := item.(map[string]any)
		if !ok {
			continue
		}
		iid := fmt.Sprintf("%.0f", getFloat(radio, "iid"))
		ssid := getStr(radio, "SSID")
		v := 0.0
		if getBool(radio, "RadioEnabled") {
			v = 1
		}
		wifiRadioEnabled.WithLabelValues(iid, ssid).Set(v)
	}
}

func (c *Collector) collectFirewall() {
	// Port forwarding — need WAN iids from WANObject first
	totalPF := 0.0
	wanData := c.safeGet("WANObject", "AdvancedSetupPage-WANConnection", fastInterval)
	if wanData != nil {
		arr, err := client.ToArr(wanData)
		if err == nil {
			for _, item := range arr {
				wan, ok := item.(map[string]any)
				if !ok {
					continue
				}
				iid := int(getFloat(wan, "iid"))
				pfKey := fmt.Sprintf("PortForwarding.%d", iid)
				pfData := c.safeGet(pfKey, "FirewallSetupPage-PortForwarding", fastInterval)
				if pfData != nil {
					if pfArr, pfErr := client.ToArr(pfData); pfErr == nil {
						totalPF += float64(len(pfArr))
					}
				}
			}
		}
	}
	portForwardingRulesTotal.Set(totalPF)

	// DMZ
	dmzData := c.safeGet("DmzHostConfig", "FirewallSetupPage-Dmz", fastInterval)
	if dmzData != nil {
		arr, err := client.ToArr(dmzData)
		if err == nil {
			for _, item := range arr {
				entry, ok := item.(map[string]any)
				if !ok {
					continue
				}
				intf := getStr(entry, "intfName")
				v := 0.0
				if getBool(entry, "enable") {
					v = 1
				}
				dmzEnabled.WithLabelValues(intf).Set(v)
			}
		}
	}

	// UPnP
	upnpData := c.safeGet("UPnPCfg", "FirewallSetupPage-UPnP", fastInterval)
	if upnpData != nil {
		obj, err := client.ToObj(upnpData)
		if err == nil {
			if getBool(obj, "enable") {
				upnpEnabled.Set(1)
			} else {
				upnpEnabled.Set(0)
			}
		}
	}

	// URL filter
	urlData := c.safeGet("URLFilterObject", "FirewallSetupPage-UrlFilter", fastInterval)
	if urlData != nil {
		arr, err := client.ToArr(urlData)
		if err == nil {
			urlFilterRulesTotal.Set(float64(len(arr)))
		}
	}
}

func (c *Collector) collectNTP() {
	data := c.safeGet("TimeServer", "MaintainancePage-NTP", fastInterval)
	if data == nil {
		return
	}
	obj, err := client.ToObj(data)
	if err != nil {
		log.Printf("TimeServer: %v", err)
		return
	}
	status := strings.ToLower(getStr(obj, "status"))
	switch {
	case status == "synced":
		ntpSyncStatus.Set(1)
	case status == "unsync" || status == "unsynchronized":
		ntpSyncStatus.Set(0)
	default:
		ntpSyncStatus.Set(2)
	}
}

func (c *Collector) collectARP() {
	data := c.safeGet("ARPStatus", "StatusPage-ARP", slowInterval*time.Second)
	if data == nil {
		return
	}
	arr, err := client.ToArr(data)
	if err != nil {
		log.Printf("ARPStatus: %v", err)
		return
	}
	arpEntriesTotal.Set(float64(len(arr)))
}

func (c *Collector) collectAutoReboot() {
	data := c.safeGet("AutoRebootObj", "MaintainancePage-AutoReboot", fastInterval)
	if data == nil {
		return
	}
	obj, err := client.ToObj(data)
	if err != nil {
		log.Printf("AutoRebootObj: %v", err)
		return
	}
	if getBool(obj, "Enable") {
		autoRebootEnabled.Set(1)
	} else {
		autoRebootEnabled.Set(0)
	}
}

// ---------------------------------------------------------------------------
// tiny helpers
// ---------------------------------------------------------------------------

func connStatus(s string) float64 {
	if v, ok := connStatusMap[s]; ok {
		return v
	}
	return 0
}

func mapVal(m map[string]float64, key string, def float64) float64 {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}
