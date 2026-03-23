// Package v1 implements the SolarAssistant cloud API v1 endpoints.
//
// All list/index endpoints accept a q parameter — a space-separated list of
// key:value filter pairs — and standard limit/offset pagination params.
// See [solar_assistant.Client.Get] for how these are sent.
package v1

import (
	"encoding/json"
	"fmt"

	sa "solar_assistant"
)

// Endpoint: GET /api/v1/sites
//
// Example filters:
//
//	name:my-site
//	inverter:srne
//	battery:daly
//	inverter_params_output_power:5000
//	last_seen_after:2026-01-01
//	build_date_after:2026-02-26
const sitesEndpoint = "/api/v1/sites"

// Endpoint: POST /api/v1/sites/:id/authorize
//
// Returns a short-lived token for connecting to a site's WebSocket.
// The token and SiteKey are used when connecting via the cloud.
const sitesAuthorizeEndpoint = "/api/v1/sites/%d/authorize"

type SiteOwner struct {
	Email string `json:"email"`
}

type Site struct {
	ID             int            `json:"id"`
	Name           string         `json:"name"`
	Inverter       string         `json:"inverter"`
	InverterCount  int            `json:"inverter_count"`
	InverterParams map[string]any `json:"inverter_params"`
	Battery        string         `json:"battery"`
	BatteryCount   int            `json:"battery_count"`
	BatteryParams  map[string]any `json:"battery_params"`
	Proxy          string         `json:"proxy"`
	WebPort        any            `json:"web_port"`
	SSHPort        any            `json:"ssh_port"`
	Arch           string         `json:"arch"`
	BuildDate      string         `json:"build_date"`
	LastSeenAt     string         `json:"last_seen_at"`
	Owner          SiteOwner      `json:"owner"`
}

type AuthorizeResponse struct {
	Host     string `json:"host"`
	SiteID   int    `json:"site_id"`
	SiteName string `json:"site_name"`
	SiteKey  string `json:"site_key"`
	Token    string `json:"token"`
	LocalIP  string `json:"local_ip"`
}

// ListSites queries sites using the provided filter args (key:value pairs).
func ListSites(c *sa.Client, params map[string]any) ([]Site, error) {
	body, err := c.Get(sitesEndpoint, params)
	if err != nil {
		return nil, err
	}
	var sites []Site
	if err := json.Unmarshal(body, &sites); err != nil {
		return nil, fmt.Errorf("unexpected response: %w", err)
	}
	return sites, nil
}

// AuthorizeSite returns a token for connecting to a site's WebSocket.
func AuthorizeSite(c *sa.Client, siteID int) (*AuthorizeResponse, error) {
	body, err := c.Post(fmt.Sprintf(sitesAuthorizeEndpoint, siteID))
	if err != nil {
		return nil, err
	}
	var resp AuthorizeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unexpected response: %w", err)
	}
	return &resp, nil
}
