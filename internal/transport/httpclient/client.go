package httpclient

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/logging"
	"github.com/geoffmcc/nodex/internal/redact"
)

const (
	// DefaultMaxBodySize is the maximum response body size (50 MiB).
	DefaultMaxBodySize int64 = 50 * 1024 * 1024

	// DefaultMaxErrorBodySize is the maximum non-success response body size.
	DefaultMaxErrorBodySize int64 = 256 * 1024

	// RetryErrorBodySize is the maximum 5xx response body included in retry errors.
	RetryErrorBodySize int64 = 1024

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

// RetryPolicy controls which HTTP methods are eligible for automatic retry.
type RetryPolicy int

const (
	// RetryIdempotent retries only safe idempotent methods (GET, HEAD).
	// This is the default for the retrying Do path.
	RetryIdempotent RetryPolicy = iota

	// RetryNone disables all automatic retries. Every error is returned
	// immediately. Use for non-idempotent mutations (POST).
	RetryNone

	// RetrySafe retries idempotent operations including PUT and DELETE
	// in addition to GET and HEAD. Use when the server guarantees
	// idempotency for those methods.
	RetrySafe
)

// isRetryableMethod reports whether the given HTTP method is eligible for
// retry under the specified policy.
func isRetryableMethod(method string, policy RetryPolicy) bool {
	switch policy {
	case RetryNone:
		return false
	case RetrySafe:
		return method == http.MethodGet || method == http.MethodHead ||
			method == http.MethodPut || method == http.MethodDelete
	default: // RetryIdempotent
		return method == http.MethodGet || method == http.MethodHead
	}
}

var (
	// ErrHTTPSDowngrade is returned when a redirect would downgrade from
	// HTTPS to HTTP, which could expose credentials and data to
	// interception.
	ErrHTTPSDowngrade = errors.New("redirect blocked: HTTPS to HTTP downgrade")

	// ErrCrossOriginRedirect is returned when a redirect would follow to
	// a different host than the original request, which could leak
	// authorization credentials to an unintended destination.
	ErrCrossOriginRedirect = errors.New("redirect blocked: cross-origin redirect to different host")
)

// Client is a minimal HTTP client with TLS, timeout, retry, and jitter.
type Client struct {
	httpClient       *http.Client
	maxBodySize      int64
	maxErrorBodySize int64
	maxRetries       int
	baseDelay        time.Duration
	maxDelay         time.Duration
	retryPolicy      RetryPolicy
	debugLogger      *logging.Logger
}

// WithDebugLogger attaches a logger used to emit opt-in request diagnostics.
// A nil logger leaves diagnostics disabled, which is the default.
func WithDebugLogger(l *logging.Logger) Option {
	return func(c *Client) {
		c.debugLogger = l
	}
}

// DebugLogger returns the attached diagnostics logger, or nil when
// diagnostics are disabled.
func (c *Client) DebugLogger() *logging.Logger {
	if c == nil {
		return nil
	}
	return c.debugLogger
}

// DebugEnabled reports whether debug diagnostics are active.
func (c *Client) DebugEnabled() bool {
	if c == nil || c.debugLogger == nil {
		return false
	}
	return c.debugLogger.Level() == logging.LevelDebug
}

// Transport returns a clone of the configured HTTP transport for protocols
// that must share the client's TLS trust policy, such as WebSocket consoles.
func (c *Client) Transport() http.RoundTripper {
	if c == nil || c.httpClient == nil || c.httpClient.Transport == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			return transport.Clone()
		}
		return http.DefaultTransport
	}
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		return transport.Clone()
	}
	return c.httpClient.Transport
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

// WithLeafCertificateFingerprint pins the SHA-256 fingerprint of the leaf
// certificate for every connection made by the client. Normal CA and hostname
// verification still runs first; the pin closes the gap between an identity
// preflight and later provider requests when a hostname resolves differently.
func WithLeafCertificateFingerprint(expected string) Option {
	expected = strings.ToLower(strings.TrimSpace(expected))
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
		prior := cfg.VerifyConnection
		cfg.VerifyConnection = func(state tls.ConnectionState) error {
			if prior != nil {
				if err := prior(state); err != nil {
					return err
				}
			}
			if len(state.PeerCertificates) == 0 {
				return fmt.Errorf("TLS peer certificate missing")
			}
			sum := sha256.Sum256(state.PeerCertificates[0].Raw)
			if !strings.EqualFold(hex.EncodeToString(sum[:]), expected) {
				return fmt.Errorf("TLS leaf certificate fingerprint does not match authorization")
			}
			return nil
		}
		t.TLSClientConfig = cfg
	}
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

// WithRetryPolicy sets the retry policy that controls which HTTP methods
// are eligible for automatic retry in the Do method.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(c *Client) {
		c.retryPolicy = policy
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
		maxBodySize:      DefaultMaxBodySize,
		maxErrorBodySize: DefaultMaxErrorBodySize,
		maxRetries:       DefaultMaxRetries,
		baseDelay:        DefaultBaseDelay,
		maxDelay:         DefaultMaxDelay,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.httpClient.CheckRedirect = c.checkRedirect
	return c
}

// checkRedirect is the CheckRedirect function for the underlying http.Client.
// It enforces two safety invariants:
//   - HTTPS-to-HTTP downgrades are forbidden (prevents credential/data leakage).
//   - Cross-origin redirects to a different host are forbidden (prevents
//     authorization forwarding to an unintended destination).
func (c *Client) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}

	original := via[0]

	// Block HTTPS → HTTP downgrade.
	if original.URL.Scheme == "https" && req.URL.Scheme == "http" {
		return fmt.Errorf("%w: %s -> %s", ErrHTTPSDowngrade, original.URL.Redacted(), req.URL.Redacted())
	}

	// Block redirect to a different host.
	if !strings.EqualFold(original.URL.Host, req.URL.Host) {
		return fmt.Errorf("%w: %s -> %s", ErrCrossOriginRedirect, original.URL.Host, req.URL.Host)
	}

	return nil
}

// Do executes an HTTP request with retry and jitter.
// Retry eligibility is controlled by the client's RetryPolicy: by default
// only GET and HEAD are retried; POST never retries; use WithRetryPolicy
// to override.
func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	retryable := isRetryableMethod(req.Method, c.retryPolicy)

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if !retryable {
				return nil, fmt.Errorf("non-retryable method %s: %w", req.Method, lastErr)
			}
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
			// 501 and every other permanent 5xx are terminal by
			// classification: the response is final, so it is handed back
			// unretried for the caller to decode. Replacing it with a retry
			// artifact is what buried the real PVE message.
			if !isRetryableStatus(resp.StatusCode) && resp.StatusCode != http.StatusInternalServerError {
				return resp, nil
			}

			// 502/503/504 always retry; a 500 retries only when its body does
			// not name a permanent condition. The body is read exactly once
			// and either restored onto the response or turned into the
			// recorded error.
			raw, truncated, err := readBody(resp.Body, RetryErrorBodySize)
			_ = resp.Body.Close()
			if err != nil {
				lastErr = fmt.Errorf("server error: %d: read body: %w", resp.StatusCode, err)
				continue
			}
			if resp.StatusCode == http.StatusInternalServerError && !truncated && isDeterministicBody(string(raw)) {
				restoreBody(resp, raw)
				return resp, nil
			}
			lastErr = serverError(resp.StatusCode, raw, truncated)
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isRetryableStatus reports whether a 5xx status is worth retrying.
//
// Only 502, 503 and 504 describe a condition that can plausibly clear by
// itself. Everything else in the 5xx range is either permanent by definition
// (501 Not Implemented, per RFC 9110) or specific to a single request, and
// retrying it only spends the backoff budget to reach a conclusion that
// cannot change.
func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// deterministicBodies are response bodies that describe a permanent condition.
// A 500 carrying one of these will fail identically on every subsequent
// attempt, so the retry budget buys nothing but latency.
var deterministicBodies = []string{
	"binary not installed",
	"does not exist",
	"not found",
	"no such file",
	"permission check failed",
	"parameter verification failed",
	"not running",
	"is running",
}

// isDeterministicBody reports whether a 5xx body names a permanent condition.
func isDeterministicBody(body string) bool {
	lower := strings.ToLower(body)
	for _, marker := range deterministicBodies {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// serverError builds the error recorded for a retried response from its
// already-read body. The decoded PVE `message` is preferred over the raw
// document, because it is the actionable part:
// `{"message":"binary not installed: /usr/bin/ceph-mon"}` tells the operator
// what to do, where the raw JSON does not.
func serverError(status int, raw []byte, truncated bool) error {
	body := sanitizeErrorBody(raw)
	if body == "" {
		return fmt.Errorf("server error: %d", status)
	}
	if message := decodeMessage(body); message != "" {
		body = message
	}
	if truncated {
		body += "... [truncated]"
	}
	return fmt.Errorf("server error: %d: %s", status, body)
}

// decodeMessage extracts the `message` field from a PVE error body, returning
// "" when the body is not that shape. Proxmox reports API errors as
// {"message": "...", "data": null}; the message is prose, so it needs no
// further unwrapping.
func decodeMessage(body string) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Message)
}

// readBody reads up to limit bytes, reporting whether more were available.
func readBody(r io.Reader, limit int64) ([]byte, bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return body[:limit], true, nil
	}
	return body, false, nil
}

// restoreBody puts a previously read body back on resp so that the caller's
// decoder sees it as an unread response.
func restoreBody(resp *http.Response, body []byte) {
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
}

// sanitizeErrorBody strips control characters and redacts a captured error
// body, returning "" for an empty result.
func sanitizeErrorBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	cleaned := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, string(raw)))
	return redact.String(cleaned)
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
