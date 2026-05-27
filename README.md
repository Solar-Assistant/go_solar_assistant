# go_solar_assistant

Go client for SolarAssistant.

## Installation

```bash
go get github.com/Solar-Assistant/go_solar_assistant
```

## Cloud API

Interact with the SolarAssistant cloud API. All endpoints require an API key — generate one at [solar-assistant.io/user/edit#api](https://solar-assistant.io/user/edit#api).

```go
import "github.com/Solar-Assistant/go_solar_assistant/cloud"

client := cloud.NewClient("<api_key>")
```

### List sites

```go
sites, err := client.ListSites(nil)
```

Filter by inverter, battery, name, and more:

```go
sites, err := client.ListSites(map[string]any{
    "inverter": "srne",
    "limit":    50,
    "offset":   20,
})
```

Common filters:

| Key | Example |
|-----|---------|
| `name` | `"name": "my-site"` |
| `inverter` | `"inverter": "srne"` |
| `battery` | `"battery": "daly"` |
| `inverter_params_output_power` | `"inverter_params_output_power": "5000"` |
| `last_seen_after` | `"last_seen_after": "2026-01-01"` |
| `build_date_after` | `"build_date_after": "2026-02-26"` |
| `limit` | `"limit": 50` |
| `offset` | `"offset": 20` |

### Authorize a site

Returns a short-lived token and connection details for a site. The token works for both cloud and local connections.

```go
resp, err := client.AuthorizeSite(siteID)
// resp.Host, resp.SiteID, resp.SiteKey, resp.Token, resp.LocalIP
```

---

## Device — REST

Read and write metrics directly on a SolarAssistant unit via REST.

```go
import "github.com/Solar-Assistant/go_solar_assistant/device"
```

### Local connection

```go
c := device.NewClient("192.168.1.100")
c.Password = "<web-password>" // set at http://<your-unit>/configuration/system
```

### Cloud-proxied connection

First obtain connection details via [AuthorizeSite](#authorize-a-site):

```go
c := device.NewClient(resp.Host)
c.Scheme  = "https"
c.Token   = resp.Token
c.SiteID  = resp.SiteID
c.SiteKey = resp.SiteKey
```

### Read metrics

```go
// All metrics
metrics, err := c.GetMetrics()

// Filtered by topic glob
metrics, err := c.GetMetrics("battery_1/*", "total/pv_power")
```

### Write a metric

```go
err := c.SetMetric("inverter_1/charge_current_limit", "20")
```

---

## Device — WebSocket

Stream live metrics via WebSocket.

```go
import "github.com/Solar-Assistant/go_solar_assistant/device"
```

### Cloud connection

First obtain connection details via [AuthorizeSite](#authorize-a-site):

```go
sock, err := device.Connect(device.Options{
    Host:    resp.Host,
    Token:   resp.Token,
    SiteID:  resp.SiteID,
    SiteKey: resp.SiteKey,
    LocalIP: resp.LocalIP, // if set, tries local network first and falls back to cloud
})
if err != nil {
    log.Fatal(err)
}
defer sock.Close()

if err := sock.SubscribeMetrics(func(m device.Metric) {
    fmt.Printf("%s/%s = %v %v\n", m.Device, m.Name, m.Value, m.Unit)
}); err != nil {
    log.Fatal(err)
}

sock.Listen() // blocks
```

### Direct local connection (no cloud)

Connect using the unit's web password, no cloud account required:

```go
sock, err := device.Connect(device.Options{
    Host:     "192.168.1.100",
    Password: "<web-password>",
})
```

### Topic filters

Subscribe to specific topics with optional server-side throttling:

```go
sock.SubscribeMetrics(fn,
    device.TopicFilter{Topic: "battery_1/*"},
    device.TopicFilter{Topic: "total/pv_power", MaxFrequencyS: 10},
)
```

If no filters are passed the server applies a default set of common metrics (`total/*`, battery voltages and SOC, inverter PV/load/grid power, etc.).

### Options

| Field | Type | Description |
|-------|------|-------------|
| `Host` | `string` | Hostname or host:port of the cloud proxy. |
| `Token` | `string` | JWT from `AuthorizeSite`. Required for cloud and local-fallback connections. |
| `Password` | `string` | Web password for direct local connections. |
| `SiteID` | `int` | Required for cloud connections. |
| `SiteKey` | `string` | Required for cloud connections. |
| `LocalIP` | `string` | If set, tries local network first and falls back to `Host`. |
| `Verbose` | `bool` | Log all WebSocket frames to stderr. |

### Advanced: raw channel messages

```go
sock.Subscribe("*", "*", func(msg device.Message) {
    fmt.Println(msg.Topic, msg.Event, msg.Payload)
})
```
