package cloud

import (
	"encoding/json"
	"fmt"
)

const sitesEndpoint = "/api/v1/sites"
const sitesAuthorizeEndpoint = "/api/v1/sites/%d/authorize"

type SiteOwner struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
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
	Arch           string         `json:"arch"`
	Board          string         `json:"board"`
	Beta           bool           `json:"beta"`
	BuildDate      string         `json:"build_date"`
	LastSeenAt     string         `json:"last_seen_at"`
	LocalIP        string         `json:"local_ip"`
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

// ListSites queries sites using the provided filter params (key:value pairs).
// Supported keys: name, inverter, battery, inverter_params_output_power,
// last_seen_after, build_date_after, limit, offset.
func (c *Client) ListSites(params map[string]any) ([]Site, error) {
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

// AuthorizeSite returns a short-lived token and connection details for a site.
// The token can be used for both cloud and direct local WebSocket connections.
func (c *Client) AuthorizeSite(siteID int) (*AuthorizeResponse, error) {
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
