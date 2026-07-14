package httpclient

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultMaxBodySize is the maximum response body size (50 MiB).
	DefaultMaxBodySize int64 = 50 * 1024 * 1024

	// DefaultMaxErrorBodySize is the maximum non-success response body size.
	DefaultMaxErrorBodySize int64 = 256 * 1024

	// DefaultTimeout is the default request timeout.
	DefaultTimeout = 30 * time.Second

	// DefaultMaxRetries is the default maximum retry attempts.
	DefaultMaxRetries = 2

	// DefaultBaseDelay is the base delay for retry backoff.
	DefaultBaseDelay = 200 * time.Millisecond

	// DefaultMaxDelay is the maximum delay for retry backoff.
	DefaultMaxDelay = 500 * time.Millisecond

	// JitterFraction is the ±25% jitter range.
	JitterFraction = 0.25
)

// Client is a minimal HTTP client with TLS, timeout, retry, and jitter.
type Client struct {
	httpClient       *http.Client
	maxBodySize      int64
	maxErrorBodySize int64
	maxRetries       int
	baseDelay        time.Duration
	maxDelay         time.Duration
}

// Option configures the Client.
type Option func(*Client)

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// WithCACert sets a custom CA certificate for TLS.
// Returns (Option, error) because it may fail to read/parse the cert.
func WithCACert(path string) (Option, error) {
	ca, err := os.ReadFile(path) // #nosec G304 -- ca_file is an explicit user-configured trust anchor path.
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("parse CA cert")
	}
	return func(c *Client) {
		t, ok := c.httpClient.Transport.(*http.Transport)
		if !ok {
			t = &http.Transport{}
			c.httpClient.Transport = t
		}
		var cfg *tls.Config
		if t.TLSClientConfig != nil {
			cfg = t.TLSClientConfig.Clone()
		}
		if cfg == nil {
			cfg = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		cfg.RootCAs = pool
		t.TLSClientConfig = cfg
	}, nil
}

// WithMaxBodySize sets the maximum response body size.
func WithMaxBodySize(n int64) Option {
	return func(c *Client) {
		c.maxBodySize = n
	}
}

// WithMaxErrorBodySize sets the maximum error response body size.
func WithMaxErrorBodySize(n int64) Option {
	return func(c *Client) {
		c.maxErrorBodySize = n
	}
}

// WithMaxRetries sets the maximum retry attempts.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		c.maxRetries = n
	}
}

// WithRetryDelays sets the base and max retry delays.
func WithRetryDelays(base, max time.Duration) Option {
	return func(c *Client) {
		c.baseDelay = base
		c.maxDelay = max
	}
}

// New creates a new Client with the given options.
func New(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
				},
			},
			// Prevent credential forwarding on redirect: reject redirects
			// that change host or downgrade from HTTPS to HTTP.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if len(via) > 0 {
					orig := via[len(via)-1]
					// Reject HTTPS → HTTP downgrade (prevents token leakage).
					if orig.URL.Scheme == "https" && req.URL.Scheme == "http" {
						return fmt.Errorf("redirect from https to http not allowed")
					}
					// Reject cross-origin redirects (prevents token forwarding to a different host).
					if req.URL.Host != orig.URL.Host {
						return fmt.Errorf("redirect to different host %q not allowed", req.URL.Host)
					}
				}
				return nil
			},
		},
		maxBodySize:      DefaultMaxBodySize,
		maxErrorBodySize: DefaultMaxErrorBodySize,
		maxRetries:       DefaultMaxRetries,
		baseDelay:        DefaultBaseDelay,
		maxDelay:         DefaultMaxDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Do executes an HTTP request with retry and jitter.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			delay := c.jitteredDelay(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.httpClient.Do(req.WithContext(ctx)) // #nosec G704 -- callers validate configured endpoints before constructing requests.
		if err != nil {
			if strings.Contains(err.Error(), "certificate") || strings.Contains(err.Error(), "tls:") {
				return nil, err
			}
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// jitteredDelay calculates a jittered delay for the given attempt.
func (c *Client) jitteredDelay(attempt int) time.Duration {
	delay := c.baseDelay * time.Duration(1<<(attempt-1))
	if delay > c.maxDelay {
		delay = c.maxDelay
	}
	jitter := float64(delay) * JitterFraction
	span := int64(2*jitter + 1)
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(span))
	if err != nil {
		return delay
	}
	delay += time.Duration(n.Int64()) - time.Duration(jitter)
	return delay
}

// DoMutation executes a single HTTP request without retry.
// State-changing operations (POST, PUT, DELETE) must use this method
// to prevent unsafe automatic retries.
func (c *Client) DoMutation(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.httpClient.Do(req.WithContext(ctx)) // #nosec G704 -- callers validate configured endpoints before constructing requests.
}

// MaxBodySize returns the configured maximum body size.
func (c *Client) MaxBodySize() int64 {
	return c.maxBodySize
}

// MaxErrorBodySize returns the configured maximum error body size.
func (c *Client) MaxErrorBodySize() int64 {
	return c.maxErrorBodySize
}
