// HTTP client for the SolarAssistant cloud API.
package solar_assistant

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const DefaultBaseURL = "https://solar-assistant.io"

// Client performs authenticated requests against the SolarAssistant cloud API.
type Client struct {
	BaseURL string
	Verbose bool
	apiKey  string
	http    *http.Client
}

// NewClient returns a Client using the given cloud API key.
func NewClient(apiKey string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// paginationKeys are sent as top-level query params rather than inside ?q=.
var paginationKeys = map[string]bool{"limit": true, "offset": true}

// Get calls GET <path> on the API.
//
// params is a map of filters for index/list endpoints. The keys "limit" and
// "offset" are sent as top-level query params (integer values). All other
// keys are joined as "key:value" pairs and sent as the ?q= parameter, which
// is the standard filter mechanism across all v1 list endpoints.
// Pass nil or an empty map to fetch all records.
func (c *Client) Get(path string, params map[string]any) ([]byte, error) {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	var filters []string
	for k, v := range params {
		if paginationKeys[k] {
			q.Set(k, fmt.Sprintf("%v", v))
		} else {
			filters = append(filters, fmt.Sprintf("%s:%v", k, v))
		}
	}
	if len(filters) > 0 {
		q.Set("q", strings.Join(filters, " "))
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}

	return c.do(http.MethodGet, u.String(), nil)
}

// Post calls POST <path> on the API with no request body.
func (c *Client) Post(path string) ([]byte, error) {
	return c.do(http.MethodPost, c.BaseURL+path, nil)
}

func (c *Client) do(method, rawURL string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "> %s %s\n", method, rawURL)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if c.Verbose {
		fmt.Fprintf(os.Stderr, "< %d %s\n", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return respBody, nil
}
