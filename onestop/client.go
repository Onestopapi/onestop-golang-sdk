// Package onestop is the OneStop Antibot API client.
//
// Create a Client with New, derive a Session per browser identity with
// Client.Session, then call the provider methods on that Session.
package onestop

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

// DefaultEndpoint is used when Config.Endpoint is empty.
const DefaultEndpoint = "https://onestopantibot.com/generate"

const (
	ipEndpoint     = "https://ip.onestopantibot.com/"
	defaultTimeout = 60 * time.Second
)

// Config configures the API connection. Browser identity belongs in Session.
type Config struct {
	APIKey     string
	Identifier string // Sent as the User-Agent on API requests.
	SubKey     string // Optional, sent as x-sub-key.
	Endpoint   string // Defaults to DefaultEndpoint.
	// Compression is zstd (default), gzip or identity.
	Compression string
	// Timeout is the whole-request deadline for every call. Zero or negative
	// means 60 seconds.
	Timeout time.Duration
	// HTTPClient is the API transport. The default ignores proxy environment
	// variables.
	HTTPClient *http.Client
	// IPHTTPClient is used by GetIP, usually your target proxy. Defaults to
	// HTTPClient.
	IPHTTPClient *http.Client
	// Defaults are merged into every Session created by this client.
	Defaults Session
	// DebugProxy sends every API call through this proxy URL, such as
	// http://127.0.0.1:8888, on the SDK's own client (HTTPClient is not used)
	// and accepts its certificate. GetIP is unaffected.
	DebugProxy string
}

// Client sends API requests. Reuse one Client across concurrent calls.
type Client struct {
	ctx         context.Context
	apiKey      string
	identifier  string
	subKey      string
	endpoint    string
	compression string
	encoder     *zstd.Encoder
	timeout     time.Duration
	http        *http.Client
	ipHTTP      *http.Client
	defaults    Session
}

// New requires an API key and identifier.
func New(config Config) (*Client, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Identifier) == "" {
		return nil, errors.New("onestop: APIKey and Identifier are required")
	}
	compression := config.Compression
	if compression == "" {
		compression = "zstd"
	}
	if compression != "zstd" && compression != "gzip" && compression != "identity" {
		return nil, errors.New("onestop: compression must be zstd, gzip or identity")
	}
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Transport: &http.Transport{}}
	}
	ipHTTP := config.IPHTTPClient
	if ipHTTP == nil {
		ipHTTP = httpClient
	}
	if config.DebugProxy != "" {
		httpClient = &http.Client{Transport: &http.Transport{
			Proxy:             func(*http.Request) (*url.URL, error) { return url.Parse(config.DebugProxy) },
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			ForceAttemptHTTP2: true,
		}}
	}
	c := &Client{
		ctx: context.Background(), apiKey: config.APIKey, identifier: config.Identifier, subKey: config.SubKey,
		endpoint: endpoint, compression: compression, timeout: timeout, http: httpClient,
		ipHTTP: ipHTTP, defaults: config.Defaults,
	}
	if compression == "zstd" {
		c.encoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	}
	return c, nil
}

// WithContext returns a copy of the client bound to ctx for cancellation and
// deadlines. Create sessions from the returned client.
func (c *Client) WithContext(ctx context.Context) *Client {
	bound := *c
	bound.ctx = ctx
	return &bound
}

// Session returns the client defaults merged with overrides; non-empty
// override fields win.
func (c *Client) Session(overrides Session) Session {
	return Session{
		UserAgent:      nonEmpty(overrides.UserAgent, c.defaults.UserAgent),
		AcceptLanguage: nonEmpty(overrides.AcceptLanguage, c.defaults.AcceptLanguage),
		IP:             nonEmpty(overrides.IP, c.defaults.IP),
		Site:           nonEmpty(overrides.Site, c.defaults.Site),
		Referer:        nonEmpty(overrides.Referer, c.defaults.Referer),
		client:         c,
	}
}

// GetIP returns the public IP seen by ip.onestopantibot.com, using
// Config.IPHTTPClient.
func (c *Client) GetIP() (string, error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ipEndpoint, nil)
	c.setAuthHeaders(req)
	res, err := c.ipHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("onestop: request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", &APIError{StatusCode: res.StatusCode, Message: "IP lookup failed"}
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("onestop: read response: %w", err)
	}
	return strings.TrimSpace(string(body)), nil
}

func (c *Client) setAuthHeaders(req *http.Request) {
	req.Header.Set("x-key", c.apiKey)
	if c.subKey != "" {
		req.Header.Set("x-sub-key", c.subKey)
	}
	req.Header.Set("User-Agent", c.identifier)
}

func nonEmpty(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}

// Ptr returns a pointer to value, for optional fields where an explicit zero
// must be sent (e.g. ImageWidth: onestop.Ptr(0)).
func Ptr[T any](value T) *T { return &value }
