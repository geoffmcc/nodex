package httpclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"time"
)

const (
	// DefaultMaxBodySize is the maximum response body size (50 MiB).
	DefaultMaxBodySize int64 = 50 * 1024 * 1024

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
	httpClient  *http.Client
	maxBodySize int64
	maxRetries  int
	baseDelay   time.Duration
	maxDelay    time.Duration
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
	ca, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("parse CA cert")
	}
	return func(c *Client) {
		t, ok := c.httpClient.Transport.(*http.Transport)
		if !ok {
			t = &http.Transport{}
			c.httpClient.Transport = t
		}
		t.TLSClientConfig.RootCAs = pool
	}, nil
}

// WithInsecureTLS disables TLS certificate verification.
func WithInsecureTLS() Option {
	return func(c *Client) {
		t, ok := c.httpClient.Transport.(*http.Transport)
		if !ok {
			t = &http.Transport{}
			c.httpClient.Transport = t
		}
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
}

// WithMaxBodySize sets the maximum response body size.
func WithMaxBodySize(n int64) Option {
	return func(c *Client) {
		c.maxBodySize = n
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
		},
		maxBodySize: DefaultMaxBodySize,
		maxRetries:  DefaultMaxRetries,
		baseDelay:   DefaultBaseDelay,
		maxDelay:    DefaultMaxDelay,
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

		resp, err := c.httpClient.Do(req.WithContext(ctx))
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
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
	delay += time.Duration(rand.Int64N(int64(2*jitter+1))) - time.Duration(jitter)
	return delay
}

// MaxBodySize returns the configured maximum body size.
func (c *Client) MaxBodySize() int64 {
	return c.maxBodySize
}
