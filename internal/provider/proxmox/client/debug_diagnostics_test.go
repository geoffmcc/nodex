package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/logging"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// decodeTarget mirrors a response shape that survives HTTP but breaks a strict
// decode, which is the case --debug exists to explain.
type decodeTarget struct {
	Data []struct {
		Count int `json:"ctime"`
	} `json:"data"`
}

// newDiagnosticsServer serves one string-valued ctime payload, the exact shape
// F-06 recorded, and reports the paths it was asked for.
func newDiagnosticsServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = append(*seen, r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"ctime":"not-a-number"}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newDiagnosticsClient(t *testing.T, srv *httptest.Server, opts ...httpclient.Option) *Client {
	t.Helper()
	caPath := writeServerCA(t, srv.Certificate())
	caOpt, err := httpclient.WithCACert(caPath)
	if err != nil {
		t.Fatalf("WithCACert: %v", err)
	}
	return &Client{
		endpoint: srv.URL,
		baseURL:  srv.URL + DefaultAPIPath,
		client:   httpclient.New(append([]httpclient.Option{caOpt}, opts...)...),
	}
}

// callDecodeTarget issues the request that fails to decode.
func callDecodeTarget(t *testing.T, c *Client) error {
	t.Helper()
	var out decodeTarget
	err := c.get(context.Background(), "/nodes/node1/storage/pbs/content", &out)
	if err == nil {
		t.Fatal("expected a decode failure")
	}
	return err
}

func TestDebugDiagnosticsExplainDecodeFailure(t *testing.T) {
	var buf bytes.Buffer
	c := newDiagnosticsClient(t, newDiagnosticsServer(t, nil),
		httpclient.WithDebugLogger(logging.New(&buf, logging.LevelDebug, true)))

	err := callDecodeTarget(t, c)

	logged := buf.String()
	if !strings.Contains(logged, "decode failed") {
		t.Fatalf("expected a decode failure diagnostic, got %q", logged)
	}
	// The operator needs the request that produced it, not just the failure.
	if !strings.Contains(logged, "GET /api2/json/nodes/node1/storage/pbs/content -> 200") {
		t.Errorf("diagnostic does not identify the request; got %q", logged)
	}
	// The operator needs the payload that failed to decode.
	if !strings.Contains(logged, "not-a-number") {
		t.Errorf("diagnostic does not show the offending payload; got %q", logged)
	}
	if !strings.Contains(logged, "[DEBUG]") {
		t.Errorf("diagnostic is not marked as debug output; got %q", logged)
	}
	// The envelope error must stay unchanged: diagnostics go to the log stream,
	// never into the error an agent parses.
	if strings.Contains(err.Error(), "not-a-number") {
		t.Errorf("payload leaked into the error detail: %v", err)
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error detail changed unexpectedly: %v", err)
	}
}

func TestDebugDiagnosticsDisabledByDefault(t *testing.T) {
	var buf bytes.Buffer
	// A logger attached but not at debug level must stay silent, mirroring the
	// flag-gated behaviour of the CLI.
	for _, level := range []logging.Level{logging.LevelInfo, logging.LevelWarn, logging.LevelError} {
		c := newDiagnosticsClient(t, newDiagnosticsServer(t, nil),
			httpclient.WithDebugLogger(logging.New(&buf, level, true)))
		if err := callDecodeTarget(t, c); err == nil {
			t.Fatalf("expected a decode failure at level %v", level)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("diagnostics written below debug level: %q", buf.String())
	}

	// No logger at all, the default construction, must also stay silent.
	plain := newDiagnosticsClient(t, newDiagnosticsServer(t, nil))
	if err := callDecodeTarget(t, plain); err == nil {
		t.Fatal("expected a decode failure without a logger")
	}
	if buf.Len() != 0 {
		t.Fatalf("diagnostics written without a logger: %q", buf.String())
	}
	if plain.client.DebugEnabled() {
		t.Error("DebugEnabled = true without an attached logger")
	}
}

func TestDebugBodyPreviewIsBounded(t *testing.T) {
	oversize := bytes.Repeat([]byte("x"), debugBodyPreviewLimit*3)
	got := debugBodyPreview(oversize)

	if !strings.HasSuffix(got, "[768 bytes total]") {
		t.Errorf("preview did not report the true body size: %q", got)
	}
	// One byte over the cap must already truncate, so the preview cannot grow
	// with the payload it is describing.
	if strings.Count(got, "x") != debugBodyPreviewLimit {
		t.Errorf("preview echoed %d bytes, want %d",
			strings.Count(got, "x"), debugBodyPreviewLimit)
	}

	// A small body is rendered quoted-and-escaped so that newlines and
	// control characters cannot forge extra log lines.
	small := []byte(`{"data":[]}`)
	if got := debugBodyPreview(small); !strings.Contains(got, `{\"data\":[]}`) {
		t.Errorf("small body preview = %q, want the escaped body", got)
	}
	if got := debugBodyPreview([]byte("line1\nline2")); strings.Contains(got, "\n") {
		t.Errorf("preview contains a raw newline and could forge a log line: %q", got)
	}
}

func TestDebugRequestLineOmitsQueryAndHeaders(t *testing.T) {
	const token = "nodex-token-that-must-not-be-logged"
	var buf bytes.Buffer
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv.Close()

	c := newDiagnosticsClient(t, srv, httpclient.WithDebugLogger(logging.New(&buf, logging.LevelDebug, true)))
	// The query carries a secret, so it must not survive into the log stream.
	var out decodeTarget
	if err := c.get(context.Background(), "/nodes?token="+token, &out); err != nil {
		t.Fatalf("request: %v", err)
	}

	logged := buf.String()
	if strings.Contains(logged, token) {
		t.Fatalf("query string leaked into diagnostics: %q", logged)
	}
	if !strings.Contains(logged, "GET /api2/json/nodes -> 200") {
		t.Errorf("diagnostic missing the path: %q", logged)
	}
}
