package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

// DasanClient holds the router connection state and JWT token.
type DasanClient struct {
	host  string
	token string
	hc    *http.Client
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

// Login authenticates against the router and stores the JWT bearer token.
func (c *DasanClient) Login(username, password string) error {
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
	return nil
}

// routerResponse is the envelope the router wraps every reply in.
type routerResponse struct {
	StatusCode float64         `json:"status_code"`
	Error      string          `json:"error"`
	Data       json.RawMessage `json:"data"`
}

// Get fetches an object from the router's tr98 API and returns the parsed
// "data" field.  For single objects the value is a map[string]any; for list
// objects it is a []any.
func (c *DasanClient) Get(objs, page string) (any, error) {
	url := fmt.Sprintf("https://%s/dm/tr98/?objs=%s&page=%s", c.host, objs, page)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", objs, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", objs, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	// The router keys the response by the bare object name.
	topKey := strings.Split(objs, ".")[0]

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
