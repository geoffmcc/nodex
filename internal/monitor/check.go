// Package monitor implements bounded, one-shot checks for explicitly
// configured endpoints. It has no discovery, daemon, or telemetry path.
package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/geoffmcc/nodex/internal/config"
)

type State string

const (
	Healthy State = "healthy"
	Failed  State = "failed"
	Unknown State = "unknown"
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
	Results []Result `json:"results" yaml:"results"`
}

// Check runs all configured targets with bounded concurrency and per-target
// cancellation. Target addresses are configuration data and are echoed only
// after config validation has rejected credentials and malformed schemes.
func Check(ctx context.Context, targets map[string]config.MonitorTarget) Report {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	results := make([]Result, len(names))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, name := range names {
		i, name := i, name
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[i] = Result{Name: name, Type: targets[name].Type, Address: targets[name].Address, State: Unknown, Detail: "check cancelled"}
				return
			}
			defer func() { <-sem }()
			results[i] = checkOne(ctx, name, targets[name])
		}()
	}
	wg.Wait()
	return Report{Results: results}
}

func checkOne(parent context.Context, name string, target config.MonitorTarget) Result {
	r := Result{Name: name, Type: target.Type, Address: target.Address, State: Unknown}
	timeout := 10 * time.Second
	if target.Timeout > 0 {
		timeout = time.Duration(target.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	start := time.Now()
	var err error
	switch target.Type {
	case "http", "https":
		var status int
		status, err = httpCheck(ctx, target.Address)
		if err == nil && (status < 200 || status >= 400) {
			err = fmt.Errorf("HTTP status %d", status)
		}
	case "tcp", "tls":
		var expires int64
		expires, err = tcpCheck(ctx, target.Address, target.Type == "tls")
		if err == nil && target.Type == "tls" {
			r.CertExpiresAt = expires
			if expires <= time.Now().Unix() {
				err = fmt.Errorf("TLS certificate is expired")
			} else if target.ExpiresIn > 0 && expires-time.Now().Unix() < int64(target.ExpiresIn)*86400 {
				r.Detail = "TLS certificate expires soon"
			}
		}
	case "dns":
		err = dnsCheck(ctx, target.Address, target.Resolver)
	default:
		err = fmt.Errorf("unsupported check type %q", target.Type)
	}
	r.Latency = time.Since(start).Milliseconds()
	if err != nil {
		if ctx.Err() != nil {
			r.Detail = "check timed out or was cancelled"
		} else {
			r.State = Failed
			r.Detail = err.Error()
		}
		return r
	}
	r.State = Healthy
	if r.Detail == "" {
		r.Detail = "check passed"
	}
	return r
}

func httpCheck(ctx context.Context, address string) (int, error) {
	u, err := url.Parse(address)
	if err != nil || u.User != nil {
		return 0, fmt.Errorf("invalid credential-free URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	client := &http.Client{Timeout: 0, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return fmt.Errorf("redirects are not allowed")
	}}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP request failed")
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func tcpCheck(ctx context.Context, address string, tlsMode bool) (int64, error) {
	d := net.Dialer{}
	if tlsMode {
		conn, err := (&tls.Dialer{NetDialer: &d, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName(address)}}).DialContext(ctx, "tcp", address)
		if err != nil {
			return 0, fmt.Errorf("TLS connection failed")
		}
		tlsConn, ok := conn.(*tls.Conn)
		if !ok || len(tlsConn.ConnectionState().PeerCertificates) == 0 {
			_ = conn.Close()
			return 0, fmt.Errorf("TLS peer certificate missing")
		}
		expires := tlsConn.ConnectionState().PeerCertificates[0].NotAfter.Unix()
		return expires, conn.Close()
	}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, fmt.Errorf("TCP connection failed")
	}
	return 0, conn.Close()
}

func dnsCheck(ctx context.Context, address, resolver string) error {
	r := net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", resolver)
	}}
	_, err := r.LookupHost(ctx, strings.TrimSpace(address))
	if err != nil {
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
