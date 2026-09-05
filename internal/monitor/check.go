// Package monitor implements bounded, one-shot checks for explicitly
// configured endpoints. It has no discovery, daemon, or telemetry path.
package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

const (
	ReportSchemaVersion = 1
	DefaultConcurrency  = 8
	MaxConcurrency      = 32
	DefaultTimeout      = 10 * time.Second
	MaxResponseBytes    = 64 * 1024
)

type State string

const (
	Healthy     State = "healthy"
	Degraded    State = "degraded"
	Failed      State = "failed"
	Unknown     State = "unknown"
	Unsupported State = "unsupported"
	Blocked     State = "blocked"
)

type Result struct {
	Name          string `json:"name" yaml:"name"`
	Type          string `json:"type" yaml:"type"`
	Address       string `json:"address" yaml:"address"`
	State         State  `json:"state" yaml:"state"`
	Detail        string `json:"detail,omitempty" yaml:"detail,omitempty"`
	Latency       int64  `json:"latency_ms,omitempty" yaml:"latency_ms,omitempty"`
	CertExpiresAt int64  `json:"cert_expires_at,omitempty" yaml:"cert_expires_at,omitempty"`
}

type Report struct {
	Schema    int      `json:"schema" yaml:"schema"`
	CheckedAt int64    `json:"checked_at" yaml:"checked_at"`
	Overall   State    `json:"overall" yaml:"overall"`
	Results   []Result `json:"results" yaml:"results"`
}

// ProviderChecker supplies authenticated, provider-backed checks without
// coupling this package to a concrete provider implementation.
type ProviderChecker func(context.Context, string, config.MonitorTarget) (Result, bool)

// Check uses the safe defaults. Configuration validation must run before this
// function; invalid targets are represented as blocked rather than healthy.
func Check(ctx context.Context, targets map[string]config.MonitorTarget) Report {
	return CheckWithOptions(ctx, targets, DefaultConcurrency, 0)
}

// CheckWithOptions runs an explicitly configured set with bounded concurrency
// and one global cancellation scope. Results are always returned in name order.
func CheckWithOptions(parent context.Context, targets map[string]config.MonitorTarget, concurrency int, globalTimeout time.Duration) Report {
	return CheckWithProviderOptions(parent, targets, concurrency, globalTimeout, nil)
}

func CheckWithProviderOptions(parent context.Context, targets map[string]config.MonitorTarget, concurrency int, globalTimeout time.Duration, providerCheck ProviderChecker) Report {
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	ctx, cancel := parent, func() {}
	if globalTimeout > 0 {
		ctx, cancel = context.WithTimeout(parent, globalTimeout)
	}
	defer cancel()
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	results := make([]Result, len(names))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, name := range names {
		i, name := i, name
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = cancelledResult(name, targets[name])
				return
			}
			defer func() { <-sem }()
			results[i] = checkOne(ctx, name, targets[name], providerCheck)
		}()
	}
	wg.Wait()
	overall := Healthy
	for _, result := range results {
		overall = worse(overall, result.State)
	}
	if len(results) == 0 {
		overall = Unknown
	}
	return Report{Schema: ReportSchemaVersion, CheckedAt: time.Now().Unix(), Overall: overall, Results: results}
}

func cancelledResult(name string, target config.MonitorTarget) Result {
	return Result{Name: name, Type: target.Type, Address: SafeAddress(target.Address), State: Unknown, Detail: "check cancelled"}
}

var severity = map[State]int{Healthy: 0, Degraded: 1, Unsupported: 2, Unknown: 3, Failed: 4, Blocked: 5}

func worse(a, b State) State {
	if severity[b] > severity[a] {
		return b
	}
	return a
}

func checkOne(parent context.Context, name string, target config.MonitorTarget, providerCheck ProviderChecker) Result {
	r := Result{Name: name, Type: target.Type, Address: SafeAddress(target.Address), State: Unknown}
	timeout := DefaultTimeout
	if target.Timeout > 0 {
		timeout = time.Duration(target.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	start := time.Now()
	var err error
	if providerCheck != nil {
		if checked, handled := providerCheck(ctx, name, target); handled {
			checked.Name, checked.Type = name, target.Type
			checked.Address = SafeAddress(target.Address)
			if _, known := severity[checked.State]; !known {
				checked.State = Unknown
				if checked.Detail == "" {
					checked.Detail = "provider returned an invalid state"
				}
			}
			checked.Latency = time.Since(start).Milliseconds()
			return checked
		}
	}
	switch target.Type {
	case "http", "https", "pve-api", "pbs-api", "pve-tasks", "pbs-tasks", "application":
		var status int
		status, err = httpCheck(ctx, target.Address, target.CAFile)
		if err == nil && (target.ExpectedStatus > 0 && status != target.ExpectedStatus || target.ExpectedStatus == 0 && (status < 200 || status >= 400)) {
			err = fmt.Errorf("HTTP status %d", status)
		}
	case "tcp":
		err = tcpCheck(ctx, target.Address, false, "")
	case "tls":
		r.CertExpiresAt, err = tlsCheck(ctx, target.Address, target.CAFile)
		if err == nil && r.CertExpiresAt <= time.Now().Unix() {
			err = fmt.Errorf("TLS certificate is expired")
		} else if err == nil && target.ExpiresIn > 0 && r.CertExpiresAt-time.Now().Unix() < int64(target.ExpiresIn)*86400 {
			r.State, r.Detail = Degraded, "TLS certificate expires within warning threshold"
		}
	case "dns":
		err = dnsCheck(ctx, target.Address, target.Resolver)
	case "datastore", "backup-age", "backup-verification", "backup-coverage", "service":
		// These checks require provider/Ansible evidence and are not inferred
		// from a URL. The standalone monitor cannot fabricate that evidence.
		r.State, r.Detail = Unsupported, "requires configured provider or Ansible health integration"
	default:
		r.State, r.Detail = Unsupported, "unsupported monitoring check type"
	}
	r.Latency = time.Since(start).Milliseconds()
	if r.State == Unsupported {
		return r
	}
	if err != nil {
		if ctx.Err() != nil {
			r.State, r.Detail = Unknown, "check timed out or was cancelled"
		} else {
			r.State, r.Detail = Failed, redact.String(output.SanitizeTerminal(err.Error()))
		}
		return r
	}
	if r.State == Unknown {
		r.State = Healthy
	}
	if r.Detail == "" {
		r.Detail = "check passed"
	}
	return r
}

func SafeAddress(address string) string {
	u, err := url.Parse(address)
	if err != nil || u.User != nil {
		return "<redacted>"
	}
	if u.RawQuery != "" {
		u.RawQuery = "<redacted>"
	}
	if u.Fragment != "" {
		u.Fragment = "<redacted>"
	}
	return u.String()
}

func httpCheck(ctx context.Context, address, caFile string) (int, error) {
	u, err := url.Parse(address)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return 0, fmt.Errorf("invalid credential-free URL")
	}
	opts := []httpclient.Option{httpclient.WithMaxRetries(0), httpclient.WithMaxBodySize(MaxResponseBytes)}
	if caFile != "" {
		option, err := httpclient.WithCACert(caFile)
		if err != nil {
			return 0, fmt.Errorf("custom CA is invalid")
		}
		opts = append(opts, option)
	}
	client := httpclient.New(opts...)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, fmt.Errorf("build request")
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, MaxResponseBytes+1)); err != nil {
		return 0, fmt.Errorf("read HTTP response")
	}
	return resp.StatusCode, nil
}

func tcpCheck(ctx context.Context, address string, tlsMode bool, caFile string) error {
	d := net.Dialer{}
	if !tlsMode {
		conn, err := d.DialContext(ctx, "tcp", address)
		if err != nil {
			return fmt.Errorf("TCP connection failed")
		}
		return conn.Close()
	}
	_, err := tlsCheck(ctx, address, caFile)
	return err
}

func tlsCheck(ctx context.Context, address, caFile string) (int64, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName(address)}
	if caFile != "" {
		option, err := httpclient.WithCACert(caFile)
		if err != nil {
			return 0, fmt.Errorf("custom CA is invalid")
		}
		c := httpclient.New(option)
		tr, ok := c.Transport().(*http.Transport)
		if ok && tr.TLSClientConfig != nil {
			config.RootCAs = tr.TLSClientConfig.RootCAs
		}
	}
	conn, err := (&tls.Dialer{NetDialer: &net.Dialer{}, Config: config}).DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, fmt.Errorf("TLS connection failed")
	}
	defer func() { _ = conn.Close() }()
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return 0, fmt.Errorf("TLS connection did not negotiate TLS")
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return 0, fmt.Errorf("TLS peer certificate missing")
	}
	return state.PeerCertificates[0].NotAfter.Unix(), nil
}

func dnsCheck(ctx context.Context, address, resolver string) error {
	r := net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		if network != "udp" && network != "tcp" {
			return nil, fmt.Errorf("unsupported DNS transport %q", network)
		}
		return (&net.Dialer{}).DialContext(ctx, network, resolver)
	}}
	if _, err := r.LookupHost(ctx, strings.TrimSpace(address)); err != nil {
		return fmt.Errorf("DNS lookup failed")
	}
	return nil
}
func serverName(address string) string {
	if host, _, err := net.SplitHostPort(address); err == nil {
		return host
	}
	return address
}
