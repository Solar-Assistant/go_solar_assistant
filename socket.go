// WebSocket client for SolarAssistant.
//
// Basic usage:
//
//	sock, err := sa_socket.Connect(sa_socket.Options{
//	    Host:  "192.168.1.100",
//	    Token: "<token>",
//	})
//	sock.SubscribeMetrics(func(m sa_socket.Metric) {
//	    fmt.Printf("%s/%s = %v %v\n", m.Device, m.Name, m.Value, m.Unit)
//	})
//	sock.Listen()  // blocks
package solar_assistant

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const heartbeatInterval = 30 * time.Second

// Options configures a WebSocket connection to SolarAssistant.
type Options struct {
	// Host is the hostname (and optional port) to connect to.
	// e.g. "192.168.1.100" or "eu1.solar-assistant.io"
	Host string

	// LocalIP is the local IP address of the device. If set, Connect will
	// try the local address first and fall back to Host if unreachable.
	LocalIP string

	// Token is the JWT used to authenticate cloud connections.
	// Obtained via the AuthorizeSite API endpoint.
	Token string

	// Password is the web password for direct local connections.
	// Set at http://<your-unit>/configuration/system.
	// Use this instead of Token when connecting without a cloud account.
	Password string

	// SiteID and SiteKey are required for cloud connections.
	// Leave zero/empty for direct device connections.
	SiteID  int
	SiteKey string

	// Verbose logs all WebSocket frames sent and received to stderr.
	Verbose bool
}

// Metric is a single metric value received from the device.
type Metric struct {
	Topic  string
	Device string
	Number int
	Group  string
	Name   string
	Value  any
	Unit   string
}

// TopicFilter specifies a topic glob pattern and optional throttling for metric subscriptions.
type TopicFilter struct {
	Topic        string `json:"topic"`
	MaxFrequencyS int   `json:"max_frequency_s,omitempty"`
}

// MetricHandler is called for each incoming metric.
type MetricHandler func(m Metric)

// Message is a raw channel message for advanced use.
type Message struct {
	JoinRef string
	Ref     string
	Topic   string
	Event   string
	Payload map[string]any
}

// MessageHandler is called for every matching incoming message.
type MessageHandler func(msg Message)

type subscription struct {
	topic, event string
	fn           MessageHandler
}

// Socket is a connected SolarAssistant socket.
type Socket struct {
	conn          *websocket.Conn
	ref           atomic.Int64
	mu            sync.Mutex
	subs          []subscription
	ConnectedHost string // the host actually used for this connection
	verbose       bool
}

const localDialTimeout = 500 * time.Millisecond
const dialTimeout = 5 * time.Second

// Connect dials the SolarAssistant WebSocket and returns a ready Socket.
// If LocalIP is set, tries WS on the local address first (units do not
// expose port 443), then falls back to the cloud proxy host.
func Connect(opts Options) (*Socket, error) {
	if opts.LocalIP != "" {
		// If Password is set, use ?password= auth. Otherwise use the cloud JWT via ?token=.
		credential, isPassword := opts.Token, false
		if opts.Password != "" {
			credential, isPassword = opts.Password, true
		}
		sock, err := dial("ws", opts.LocalIP, credential, isPassword, 0, "", localDialTimeout, nil, opts.Verbose)
		if err == nil {
			sock.ConnectedHost = opts.LocalIP
			return sock, nil
		}
		if opts.Host == "" {
			return nil, fmt.Errorf("could not connect to %s: %w", opts.LocalIP, err)
		}
	}
	// Fall back to cloud proxy
	sock, err := dial("wss", opts.Host, opts.Token, false, opts.SiteID, opts.SiteKey, dialTimeout, nil, opts.Verbose)
	if err != nil {
		return nil, err
	}
	sock.ConnectedHost = opts.Host
	return sock, nil
}

// dial connects to the WebSocket endpoint.
// For local connections (local=true) the credential is sent as ?password=
// For cloud connections (local=false) it is sent as ?token= with site headers.
func dial(scheme, host, token string, local bool, siteID int, siteKey string, timeout time.Duration, tlsCfg *tls.Config, verbose bool) (*Socket, error) {
	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/api/websocket",
	}
	q := u.Query()
	q.Set("vsn", "2.0.0")
	if local {
		q.Set("password", token)
	} else {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()

	h := http.Header{}
	if !local {
		if siteID != 0 {
			h.Set("site-id", fmt.Sprintf("%d", siteID))
		}
		if siteKey != "" {
			h.Set("site-key", siteKey)
		}
	}

	dialer := &websocket.Dialer{
		HandshakeTimeout: timeout,
		TLSClientConfig:  tlsCfg,
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "> WS %s\n", u.String())
		for k := range h {
			fmt.Fprintf(os.Stderr, "> %s: %s\n", k, h.Get(k))
		}
	}

	conn, resp, err := dialer.Dial(u.String(), h)
	if err != nil {
		if resp != nil {
			switch resp.StatusCode {
			case 401, 403:
				return nil, fmt.Errorf("authentication failed (HTTP %d) — check your token", resp.StatusCode)
			case 404:
				return nil, fmt.Errorf("WebSocket endpoint not found (HTTP 404) — site may be running an outdated version (requires build 2026-03-24 or later)")
			case 502, 503, 504:
				return nil, fmt.Errorf("site is offline or unreachable (HTTP %d)", resp.StatusCode)
			default:
				return nil, fmt.Errorf("connection rejected (HTTP %d)", resp.StatusCode)
			}
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("connection timed out — is the device reachable?")
		}
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, fmt.Errorf("proxy unreachable (%s) — site may be offline or decommissioned", dnsErr.Name)
		}
		return nil, fmt.Errorf("websocket dial: %w", err)
	}
	return &Socket{conn: conn, verbose: verbose}, nil
}

type metricDef struct {
	Device string
	Number int
	Group  string
	Name   string
	Unit   string
}

// SubscribeMetrics registers a handler called for each metric on every push
// and joins the metrics channel. Definitions arrive once; data arrives continuously.
// Pass TopicFilter values to subscribe to specific topics server-side.
//
// If no filters are passed, the server applies its default subscription:
//
//	total/*
//	battery_*/voltage
//	battery_*/state_of_charge
//	battery_*/power
//	battery_*/temperature
//	inverter_*/pv_power
//	inverter_*/load_power
//	inverter_*/grid_power
//	inverter_*/device_mode
//	inverter_*/temperature
//
// Only metrics in groups Info, Status, and Settings are sent.
func (s *Socket) SubscribeMetrics(fn MetricHandler, filters ...TopicFilter) error {
	defs := map[string]metricDef{}
	var defsMu sync.Mutex

	s.Subscribe("metrics", "definition", func(msg Message) {
		items, _ := msg.Payload["definitions"].([]any)
		defsMu.Lock()
		defer defsMu.Unlock()
		for _, item := range items {
			mm, _ := item.(map[string]any)
			topic := strVal(mm["topic"])
			defs[topic] = metricDef{
				Device: strVal(mm["device"]),
				Number: intVal(mm["number"]),
				Group:  strVal(mm["group"]),
				Name:   strVal(mm["name"]),
				Unit:   strVal(mm["unit"]),
			}
		}
	})

	s.Subscribe("metrics", "data", func(msg Message) {
		items, _ := msg.Payload["metrics"].([]any)
		defsMu.Lock()
		defer defsMu.Unlock()
		for _, item := range items {
			mm, _ := item.(map[string]any)
			topic := strVal(mm["topic"])
			def := defs[topic]
			fn(Metric{
				Topic:  topic,
				Device: def.Device,
				Number: def.Number,
				Group:  def.Group,
				Name:   def.Name,
				Value:  mm["value"],
				Unit:   def.Unit,
			})
		}
	})

	payload := map[string]any{}
	if len(filters) > 0 {
		payload["topics"] = filters
	}
	return s.joinWithPayload("metrics", payload)
}

// Join sends a join request for the given channel topic.
func (s *Socket) Join(topic string) error {
	return s.joinWithPayload(topic, map[string]any{})
}

// JoinWithPayload sends a join request with a custom payload.
func (s *Socket) JoinWithPayload(topic string, payload map[string]any) error {
	return s.joinWithPayload(topic, payload)
}

func (s *Socket) joinWithPayload(topic string, payload map[string]any) error {
	joinRef := s.nextRef()
	return s.send(joinRef, s.nextRef(), topic, "phx_join", payload)
}

// Subscribe registers a handler for a specific topic and event.
// Use "*" as a wildcard for either field.
// For metrics use SubscribeMetrics instead.
func (s *Socket) Subscribe(topic, event string, fn MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, subscription{topic, event, fn})
}

// Listen reads messages in a loop, dispatching to subscribers.
// Also starts the heartbeat. Blocks until the connection closes.
func (s *Socket) Listen() error {
	go s.heartbeat()
	for {
		_, raw, err := s.conn.ReadMessage()
		if err != nil {
			return err
		}
		if s.verbose {
			fmt.Fprintf(os.Stderr, "< recv %s\n", raw)
		}
		msg, err := decode(raw)
		if err != nil {
			continue
		}
		s.dispatch(msg)
	}
}

// Close closes the WebSocket connection.
func (s *Socket) Close() error {
	return s.conn.Close()
}

func (s *Socket) heartbeat() {
	for {
		time.Sleep(heartbeatInterval)
		if err := s.send("", s.nextRef(), "phoenix", "heartbeat", map[string]any{}); err != nil {
			return
		}
	}
}

func (s *Socket) dispatch(msg Message) {
	s.mu.Lock()
	subs := s.subs
	s.mu.Unlock()
	for _, sub := range subs {
		if (sub.topic == "*" || sub.topic == msg.Topic) &&
			(sub.event == "*" || sub.event == msg.Event) {
			sub.fn(msg)
		}
	}
}

func (s *Socket) send(joinRef, ref, topic, event string, payload any) error {
	data, err := json.Marshal([]any{joinRef, ref, topic, event, payload})
	if err != nil {
		return err
	}
	if s.verbose {
		fmt.Fprintf(os.Stderr, "> send %s\n", data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

func (s *Socket) nextRef() string {
	return fmt.Sprintf("%d", s.ref.Add(1))
}

func decode(raw []byte) (Message, error) {
	var parts [5]any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return Message{}, err
	}
	payload, _ := parts[4].(map[string]any)
	return Message{
		JoinRef: strVal(parts[0]),
		Ref:     strVal(parts[1]),
		Topic:   strVal(parts[2]),
		Event:   strVal(parts[3]),
		Payload: payload,
	}, nil
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func intVal(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
