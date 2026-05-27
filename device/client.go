// Package device implements clients for direct communication with SolarAssistant units.
package device

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Client performs REST API calls against a SolarAssistant device.
// A single Client reuses its underlying HTTP connection across calls.
type Client struct {
	// Host is the hostname or IP (with optional port) of the device or cloud proxy.
	Host string

	// Password is the web password for HTTP Basic auth (local connections).
	// Set at http://<your-unit>/configuration/system.
	Password string

	// Token is a JWT for Bearer auth. Works for both cloud-proxied and local connections.
	Token string

	// SiteID and SiteKey are required when using a cloud-proxied host.
	SiteID  int
	SiteKey string

	// Scheme is "http" or "https". Defaults to "http".
	Scheme string

	// Verbose logs all HTTP requests and responses to stderr.
	Verbose bool

	http *http.Client
}

// NewClient returns a Client for the given host.
// Set Password or Token (and optionally SiteID/SiteKey) before making calls.
func NewClient(host string) *Client {
	return &Client{
		Host:   host,
		Scheme: "http",
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Metric is a single metric value from a SolarAssistant device.
// Returned by both REST (GetMetrics) and WebSocket (SubscribeMetrics) calls.
type Metric struct {
	Topic  string
	Device string
	Number int
	Group  string
	Name   string
	Value  any
	Unit   string
}

// GetMetrics fetches metrics via GET /api/v1/metrics.
// Pass topic glob patterns to filter (e.g. "battery_1/*", "total/pv_power").
// Pass no topics to fetch all metrics.
// Multiple topics are fetched in separate requests and deduplicated.
func (c *Client) GetMetrics(topics ...string) ([]Metric, error) {
	if len(topics) == 0 {
		return c.fetchMetrics("")
	}
	seen := map[string]bool{}
	var all []Metric
	for _, topic := range topics {
		batch, err := c.fetchMetrics(topic)
		if err != nil {
			return nil, err
		}
		for _, m := range batch {
			if !seen[m.Topic] {
				seen[m.Topic] = true
				all = append(all, m)
			}
		}
	}
	return all, nil
}

// SetMetric writes a setting via POST /api/v1/metrics.
// topic is the MQTT-style path, e.g. "inverter_1/charge_current_limit".
// value is the new value as a string.
func (c *Client) SetMetric(topic, value string) error {
	body, _ := json.Marshal(map[string]any{"topic": topic, "value": value})
	resp, err := c.do(http.MethodPost, "/api/v1/metrics", bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}

	if resp.status != http.StatusOK {
		var errResp map[string]any
		if json.Unmarshal(resp.body, &errResp) == nil {
			if msg, ok := errResp["error"].(string); ok {
				return fmt.Errorf("API error %d: %s", resp.status, msg)
			}
		}
		return fmt.Errorf("API error %d: %s", resp.status, strings.TrimSpace(string(resp.body)))
	}
	return nil
}

func (c *Client) fetchMetrics(topic string) ([]Metric, error) {
	path := "/api/v1/metrics"
	if topic != "" {
		path += "?topic=" + url.QueryEscape(topic)
	}
	resp, err := c.do(http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}

	if resp.status == http.StatusNotFound {
		return nil, fmt.Errorf("HTTP 404: site may be running an outdated version (requires build 2026-03-24 or later)")
	}
	if resp.status != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.status, strings.TrimSpace(string(resp.body)))
	}

	var rows []struct {
		Topic  string `json:"topic"`
		Device string `json:"device"`
		Number *int   `json:"number"`
		Group  string `json:"group"`
		Name   string `json:"name"`
		Value  any    `json:"value"`
		Unit   string `json:"unit"`
	}
	if err := json.Unmarshal(resp.body, &rows); err != nil {
		return nil, fmt.Errorf("unexpected response: %w", err)
	}

	metrics := make([]Metric, len(rows))
	for i, r := range rows {
		num := 0
		if r.Number != nil {
			num = *r.Number
		}
		metrics[i] = Metric{
			Topic:  r.Topic,
			Device: r.Device,
			Number: num,
			Group:  r.Group,
			Name:   r.Name,
			Value:  r.Value,
			Unit:   r.Unit,
		}
	}
	return metrics, nil
}

type doResult struct {
	status int
	body   []byte
}

func (c *Client) do(method, path string, body io.Reader, contentType string) (doResult, error) {
	scheme := c.Scheme
	if scheme == "" {
		scheme = "http"
	}
	u := scheme + "://" + c.Host + path

	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return doResult{}, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Password != "" {
		req.SetBasicAuth("solar-assistant", c.Password)
	} else {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		if c.SiteID != 0 {
			req.Header.Set("site-id", fmt.Sprintf("%d", c.SiteID))
		}
		if c.SiteKey != "" {
			req.Header.Set("site-key", c.SiteKey)
		}
	}

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "> %s %s\n", method, u)
		for k, v := range req.Header {
			fmt.Fprintf(os.Stderr, "> %s: %s\n", k, strings.Join(v, ", "))
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return doResult{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "< %d %s\n", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return doResult{status: resp.StatusCode, body: respBody}, nil
}
