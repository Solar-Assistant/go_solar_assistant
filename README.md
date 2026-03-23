# go_solar_assistant

Go client for SolarAssistant.

## Installation

```bash
go get github.com/Solar-Assistant/go_solar_assistant
```

## Cloud API

Interact with the SolarAssistant cloud API. All endpoints require an API key — generate one at [solar-assistant.io/user/edit#api](https://solar-assistant.io/user/edit#api).

```go
import (
    sa  "github.com/Solar-Assistant/go_solar_assistant"
    api "github.com/Solar-Assistant/go_solar_assistant/api/v1"
)

client := sa.NewClient("<api_key>")
```

### List sites

```go
sites, err := api.ListSites(client, nil)
```

Filter by inverter, battery, name, and more:

```go
sites, err := api.ListSites(client, map[string]any{
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

Returns a token for connecting to a site's WebSocket, along with the host and site key needed for a [cloud connection](#cloud-connection).

```go
resp, err := api.AuthorizeSite(client, siteID)
// resp.Host, resp.SiteKey, resp.Token
```

---

## Real-time metrics

Connect to a SolarAssistant unit and stream live metrics.

### Cloud connection

First obtain a token via [AuthorizeSite](#authorize-a-site), then connect using the fields from the response:

```go
sock, err := sa.Connect(sa.Options{
    Host:    resp.Host,
    Token:   resp.Token,
    SiteID:  resp.SiteID,
    SiteKey: resp.SiteKey,
})
if err != nil {
    log.Fatal(err)
}
defer sock.Close()

if err := sock.SubscribeMetrics(func(m sa.Metric) {
    fmt.Printf("%s/%s = %v %v\n", m.Device, m.Name, m.Value, m.Unit)
}); err != nil {
    log.Fatal(err)
}

sock.Listen() // blocks
```

Example metrics:

```
Totals/Battery voltage         = 53.1  V
Totals/Battery state of charge = 92    %
Totals/PV power                = 1240  W
Totals/Load power              = 860   W
Totals/Grid power              = -380  W
Inverters/Battery voltage      = 53.1  V
Inverters/PV power             = 1240  W
```

### Local fallback

Set `LocalIP` to transparently try the local network first and fall back to cloud if unreachable. The cloud JWT works for local connections too:

```go
sock, err := sa.Connect(sa.Options{
    Host:    resp.Host,
    Token:   resp.Token,
    SiteID:  resp.SiteID,
    SiteKey: resp.SiteKey,
    LocalIP: resp.LocalIP,
})
```

### Direct local connection (no cloud)

Connect directly using the unit's web password (set at `http://<your-unit>/configuration/system`), with no cloud account required:

```go
sock, err := sa.Connect(sa.Options{
    Host:     "192.168.1.100",
    Password: "<web-password>",
})
```

### Options

| Field | Type | Description |
|-------|------|-------------|
| `Host` | `string` | Hostname or host:port. Uses `wss://` unless it starts with `localhost` or `127.0.0.1`. |
| `Token` | `string` | JWT from `AuthorizeSite`. Required for cloud and local-fallback connections. |
| `Password` | `string` | Web password for direct local connections (set at `/configuration/system`). |
| `SiteID` | `int` | Required for cloud connections. Omit for direct local connections. |
| `SiteKey` | `string` | Required for cloud connections. Omit for direct local connections. |
| `LocalIP` | `string` | If set, tries local network first and falls back to `Host`. |

### Advanced: raw channel messages

For low-level access, use `Subscribe` directly. `"*"` is a wildcard for topic or event:

```go
sock.Subscribe("*", "*", func(msg sa.Message) {
    fmt.Println(msg.Topic, msg.Event, msg.Payload)
})
```
