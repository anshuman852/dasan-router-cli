// Package client implements the HTTP client for the Dasan/Airtel router's
// internal JSON API (/dm/tr98/, /dm/sys/, /bin/).
package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

// PAGES maps object names to page IDs used by the router for per-page
// permission checks.
var PAGES = map[string]string{
	"DeviceInfo":             "StatusPage-DeviceInfo",
	"HWInfo":                 "StatusPage-DeviceInfo",
	"PonPortStatus":          "StatusPage-DeviceInfo",
	"LANPortStatus":          "StatusPage-DeviceInfo",
	"WANObject":              "AdvancedSetupPage-WANConnection",
	"WANIPConnection":        "AdvancedSetupPage-WANConnection",
	"WANPPPConnection":       "AdvancedSetupPage-WANConnection",
	"DhcpLease":              "StatusPage-DHCPLease",
	"WLANConfiguration":      "WifiSetupPage-WirelessSetting",
	"WLANCommon":             "WifiSetupPage-WirelessSetting",
	"WLANOnOff":              "WifiSetupPage-WirelessSetting",
	"Reboot":                 "MaintainancePage-Reboot",
	"RestoreFactory":         "MaintainancePage-Reboot",
	"PortForwarding":         "FirewallSetupPage-PortForwarding",
	"FireWallCfg":            "FirewallSetupPage-PortForwarding",
	"DmzHostConfig":          "FirewallSetupPage-Dmz",
	"PortTriggering":         "FirewallSetupPage-PortTriggering",
	"URLCommon":              "FirewallSetupPage-UrlFilter",
	"URLFilterObject":        "FirewallSetupPage-UrlFilter",
	"ParentalControlObj":     "FirewallSetupPage-ParentalControl",
	"ParentalCommonObj":      "FirewallSetupPage-ParentalControl",
	"UPnPCfg":                "FirewallSetupPage-UPnP",
	"UPnPRules":              "FirewallSetupPage-UPnP",
	"MacAntiSpoofingCfg":     "FirewallSetupPage-MACAntiSpoofing",
	"MacAntiSpoofingTable":   "FirewallSetupPage-MACAntiSpoofing",
	"IPAntiSpoofingCfg":      "FirewallSetupPage-IPAntiSpoofing",
	"IPAntiSpoofingCommon":   "FirewallSetupPage-IPAntiSpoofing",
	"IPAntiSpoofingTable":    "FirewallSetupPage-IPAntiSpoofing",
	"DHCPStaticLease":        "AdvancedSetupPage-LANSetup",
	"DhcpServerConfiguration":"AdvancedSetupPage-LANSetup",
}

// DasanClient holds the router connection state and JWT token.
type DasanClient struct {
	host    string
	token   string
	hc      *http.Client
	verbose bool
}

// NewDasanClient creates a client for the router at the given host.
// TLS certificate verification is skipped because the router uses a
// self-signed certificate.
func NewDasanClient(host string) *DasanClient {
	return &DasanClient{
		host: host,
		hc: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// SetVerbose controls verbose request logging to stderr.
func (c *DasanClient) SetVerbose(v bool) { c.verbose = v }

// GetHost returns the router hostname.
func (c *DasanClient) GetHost() string { return c.host }

// ---------------------------------------------------------------------------
// Session cache
// ---------------------------------------------------------------------------

type session struct {
	Host    string `json:"host"`
	Token   string `json:"token"`
	SavedAt int64  `json:"saved_at"`
}

func sessionPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dasan-session.json")
}

// tryCachedToken tries the cached JWT token. Returns true if it works.
func (c *DasanClient) tryCachedToken() bool {
	if c.token != "" {
		return true
	}
	p := sessionPath()
	if p == "" {
		return false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var s session
	if err := json.Unmarshal(raw, &s); err != nil {
		return false
	}
	if s.Host != c.host || s.Token == "" {
		return false
	}
	c.token = s.Token
	// Test the token with a lightweight GET.
	_, err = c.Get("DeviceInfo", "")
	if err != nil {
		c.token = ""
		return false
	}
	return true
}

// saveSession persists the current JWT token to disk.
func (c *DasanClient) saveSession() {
	p := sessionPath()
	if p == "" {
		return
	}
	s := session{Host: c.host, Token: c.token, SavedAt: time.Now().Unix()}
	raw, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := os.WriteFile(p, raw, 0600); err != nil {
		return
	}
	// Ensure permissions are correct (Windows ignores 0600 but no-op is fine).
}

// ---------------------------------------------------------------------------
// Authentication
// ---------------------------------------------------------------------------

// Login authenticates against the router and stores the JWT bearer token.
// The session is cached to disk on success.
func (c *DasanClient) Login(username, password string) error {
	// Try cached token first.
	if c.tryCachedToken() {
		return nil
	}

	body := fmt.Sprintf(
		`{"Login":{"data":{"username":"%s","password":"%s","captcha":""}}}`,
		username, password,
	)
	req, err := http.NewRequest("POST",
		fmt.Sprintf("https://%s/dm/sys/?cmd=Login", c.host),
		strings.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("login http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("login parse: %w (body: %s)", err, string(raw))
	}

	var login struct {
		Data struct {
			Login struct {
				Status             string `json:"status"`
				AuthenticatedToken string `json:"authenticatedToken"`
			} `json:"login"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload["Login"], &login); err != nil {
		return fmt.Errorf("login decode: %w", err)
	}
	if login.Data.Login.Status != "success" {
		return fmt.Errorf("login failed: %s", string(raw))
	}
	c.token = login.Data.Login.AuthenticatedToken
	c.saveSession()
	return nil
}

// Logout expires the session on the router.
func (c *DasanClient) Logout() {
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://%s/dm/sys/?cmd=Logout", c.host), nil)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	c.hc.Do(req)
	c.token = ""
}

// ---------------------------------------------------------------------------
// Low-level request helpers
// ---------------------------------------------------------------------------

type routerResponse struct {
	StatusCode float64         `json:"status_code"`
	Error      string          `json:"error"`
	Data       json.RawMessage `json:"data"`
}

// rawRequest sends an HTTP request and returns the response body + csrf header.
func (c *DasanClient) rawRequest(method, urlStr string, body io.Reader, extraHeaders map[string]string) ([]byte, string, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	if c.verbose {
		fmt.Fprintf(os.Stderr, "  -> %s %s\n", method, urlStr)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	csrf := resp.Header.Get("csrf")
	return raw, csrf, nil
}

// topKey returns the bare object name from a dotted objs string.
// e.g. "PortForwarding.2" -> "PortForwarding"
func topKey(objs string) string {
	return strings.Split(objs, ".")[0]
}

// pageFor returns the page name for the given objs string, falling back to
// the empty string if not found in the PAGES map.
func pageFor(objs string) string {
	key := topKey(objs)
	if p, ok := PAGES[key]; ok {
		return p
	}
	return ""
}

// parseRouterResponse extracts the "data" field from a router response.
func parseRouterResponse(topKey string, raw []byte) (any, error) {
	var payload map[string]routerResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("%s: parse: %w", topKey, err)
	}
	entry, ok := payload[topKey]
	if !ok {
		return nil, fmt.Errorf("%s: missing key in response", topKey)
	}
	if entry.StatusCode != 200 {
		return nil, fmt.Errorf("%s: code=%.0f err=%s", topKey, entry.StatusCode, entry.Error)
	}

	// Try decoding as array first, then as object.
	var arr []any
	if err := json.Unmarshal(entry.Data, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(entry.Data, &obj); err == nil {
		return obj, nil
	}
	return nil, fmt.Errorf("%s: cannot decode data: %s", topKey, string(entry.Data))
}

// ---------------------------------------------------------------------------
// Read operations
// ---------------------------------------------------------------------------

// Get fetches an object from the router's tr98 API and returns the parsed
// "data" field. For single objects the value is map[string]any; for list
// objects it is []any.
func (c *DasanClient) Get(objs, page string) (any, error) {
	return c.GetNS(objs, page, "tr98")
}

// GetNS fetches from a specific namespace (tr98, sys, bin).
func (c *DasanClient) GetNS(objs, page, namespace string) (any, error) {
	if page == "" {
		page = pageFor(objs)
	}
	urlStr := fmt.Sprintf("https://%s/dm/%s/?objs=%s&page=%s", c.host, namespace, objs, page)

	raw, _, err := c.rawRequest("GET", urlStr, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", objs, err)
	}

	return parseRouterResponse(topKey(objs), raw)
}

// GetAPI fetches an object using the ?api= query parameter (used by static
// routing objects).
func (c *DasanClient) GetAPI(name, page string) (any, error) {
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?api=%s&page=%s", c.host, name, page)
	raw, _, err := c.rawRequest("GET", urlStr, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return parseRouterResponse(name, raw)
}

// GetRaw downloads raw bytes from a /bin/ endpoint (syslog, backup config).
func (c *DasanClient) GetRaw(path string) ([]byte, error) {
	urlStr := fmt.Sprintf("https://%s%s", c.host, path)
	raw, _, err := c.rawRequest("GET", urlStr, nil, nil)
	return raw, err
}

// ---------------------------------------------------------------------------
// Write operations
// ---------------------------------------------------------------------------

// csrfGet does a GET to obtain a CSRF token for a subsequent write.
func (c *DasanClient) csrfGet(urlStr string) (string, error) {
	_, csrf, err := c.rawRequest("GET", urlStr, nil, nil)
	if err != nil {
		return "", err
	}
	if csrf == "" {
		return "", fmt.Errorf("did not receive a CSRF token from the router")
	}
	return csrf, nil
}

// writeRequest sends a POST/DELETE with the CSRF token and data body.
// data is wrapped as {"ObjectName":{"data":<data>}}.
func (c *DasanClient) writeRequest(method, objs, page, csrf string, data any) error {
	key := topKey(objs)
	if page == "" {
		page = pageFor(objs)
	}
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?objs=%s&page=%s", c.host, objs, page)

	bodyPayload := map[string]any{
		key: map[string]any{
			"data": data,
		},
	}
	bodyJSON, err := json.Marshal(bodyPayload)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", objs, err)
	}

	raw, _, err := c.rawRequest(method, urlStr, bytes.NewReader(bodyJSON), map[string]string{
		"X-Csrf-Token": csrf,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", objs, err)
	}

	var payload map[string]routerResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("%s: parse response: %w (body: %s)", objs, err, string(raw))
	}
	entry, ok := payload[key]
	if !ok {
		return fmt.Errorf("%s: missing key in response", objs)
	}
	if entry.StatusCode != 200 {
		return fmt.Errorf("%s: code=%.0f err=%s", objs, entry.StatusCode, entry.Error)
	}
	return nil
}

// writeRequestAPI is the ?api= version of writeRequest.
func (c *DasanClient) writeRequestAPI(method, name, page, csrf string, data any) error {
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?api=%s&page=%s", c.host, name, page)
	bodyPayload := map[string]any{
		name: map[string]any{
			"data": data,
		},
	}
	bodyJSON, err := json.Marshal(bodyPayload)
	if err != nil {
		return fmt.Errorf("%s: marshal: %w", name, err)
	}
	raw, _, err := c.rawRequest(method, urlStr, bytes.NewReader(bodyJSON), map[string]string{
		"X-Csrf-Token": csrf,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	var payload map[string]routerResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("%s: parse response: %w (body: %s)", name, err, string(raw))
	}
	entry, ok := payload[name]
	if !ok {
		return fmt.Errorf("%s: missing key in response", name)
	}
	if entry.StatusCode != 200 {
		return fmt.Errorf("%s: code=%.0f err=%s", name, entry.StatusCode, entry.Error)
	}
	return nil
}

// Post writes data to a tr98 object. Does a GET first for the CSRF token.
func (c *DasanClient) Post(objs, page string, data any) error {
	if page == "" {
		page = pageFor(objs)
	}
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?objs=%s&page=%s", c.host, objs, page)
	csrf, err := c.csrfGet(urlStr)
	if err != nil {
		return fmt.Errorf("%s: csrf: %w", objs, err)
	}
	return c.writeRequest("POST", objs, page, csrf, data)
}

// PostAPI writes data using the ?api= query parameter scheme.
func (c *DasanClient) PostAPI(name, page string, data any) error {
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?api=%s&page=%s", c.host, name, page)
	csrf, err := c.csrfGet(urlStr)
	if err != nil {
		return fmt.Errorf("%s: csrf: %w", name, err)
	}
	return c.writeRequestAPI("POST", name, page, csrf, data)
}

// Delete removes a list entry. GETs CSRF first, sends full record back.
func (c *DasanClient) Delete(objs, page string, data any) error {
	if page == "" {
		page = pageFor(objs)
	}
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?objs=%s&page=%s", c.host, objs, page)
	csrf, err := c.csrfGet(urlStr)
	if err != nil {
		return fmt.Errorf("%s: csrf: %w", objs, err)
	}
	return c.writeRequest("DELETE", objs, page, csrf, data)
}

// DeleteAPI removes an entry using the ?api= query parameter scheme.
func (c *DasanClient) DeleteAPI(name, page string, data any) error {
	urlStr := fmt.Sprintf("https://%s/dm/tr98/?api=%s&page=%s", c.host, name, page)
	csrf, err := c.csrfGet(urlStr)
	if err != nil {
		return fmt.Errorf("%s: csrf: %w", name, err)
	}
	return c.writeRequestAPI("DELETE", name, page, csrf, data)
}

// Cmd executes a /dm/sys/ command (reboot, administration writes, etc.).
func (c *DasanClient) Cmd(name string, data any) (any, error) {
	urlStr := fmt.Sprintf("https://%s/dm/sys/?cmd=%s", c.host, name)
	if data == nil {
		// GET-style command
		raw, _, err := c.rawRequest("GET", urlStr, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return parseRouterResponse(name, raw)
	}

	// POST-style command with CSRF
	csrf, err := c.csrfGet(urlStr)
	if err != nil {
		// Some commands work without CSRF
		csrf = ""
	}
	bodyPayload := map[string]any{
		name: map[string]any{
			"data": data,
		},
	}
	bodyJSON, err := json.Marshal(bodyPayload)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal: %w", name, err)
	}
	headers := map[string]string{}
	if csrf != "" {
		headers["X-Csrf-Token"] = csrf
	}
	raw, _, err := c.rawRequest("POST", urlStr, bytes.NewReader(bodyJSON), headers)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return parseRouterResponse(name, raw)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// GetStr returns the string value for a key from a map, or "".
func GetStr(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// GetFloat returns the numeric value for a key, handling string/float/int/bool.
func GetFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case bool:
		if t {
			return 1
		}
		return 0
	case string:
		var f float64
		if _, err := fmt.Sscanf(t, "%f", &f); err == nil {
			return f
		}
		if t == "true" || t == "True" || t == "Up" {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// GetBool returns true for truthy values (bool true, string "true"/"Up"/"1", numeric != 0).
func GetBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t == "true" || t == "True" || t == "Up" || t == "yes" || t == "1"
	default:
		return false
	}
}

// ToObj asserts data is a map[string]any.
func ToObj(d any) (map[string]any, error) {
	m, ok := d.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map, got %T", d)
	}
	return m, nil
}

// ToArr asserts data is a []any.
func ToArr(d any) ([]any, error) {
	a, ok := d.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", d)
	}
	return a, nil
}

// init ensures os.UserHomeDir is available. On Windows, it sets HOME if needed.
func init() {
	if runtime.GOOS == "windows" {
		if os.Getenv("HOME") == "" {
			home := os.Getenv("USERPROFILE")
			if home != "" {
				os.Setenv("HOME", home)
			}
		}
	}
}
