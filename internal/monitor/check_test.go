package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geoffmcc/nodex/internal/config"
)

func TestCheckHTTPIsDeterministicAndSorted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	report := Check(context.Background(), map[string]config.MonitorTarget{
		"zulu":  {Type: "http", Address: server.URL},
		"alpha": {Type: "http", Address: server.URL},
	})
	if len(report.Results) != 2 || report.Results[0].Name != "alpha" || report.Results[1].Name != "zulu" {
		t.Fatalf("results are not sorted: %#v", report.Results)
	}
	for _, result := range report.Results {
		if result.State != Healthy {
			t.Errorf("%s state = %s, want healthy", result.Name, result.State)
		}
	}
}

func TestCheckRejectsRedirects(t *testing.T) {
	server := httptest.NewServer(http.RedirectHandler("/next", http.StatusFound))
	defer server.Close()
	report := Check(context.Background(), map[string]config.MonitorTarget{"redirect": {Type: "http", Address: server.URL}})
	if report.Results[0].State != Failed || report.Results[0].Detail == "" {
		t.Fatalf("redirect result = %#v", report.Results[0])
	}
}

func TestCheckCancellationIsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := Check(ctx, map[string]config.MonitorTarget{"cancelled": {Type: "tcp", Address: "127.0.0.1:1"}})
	if report.Results[0].State != Unknown {
		t.Fatalf("state = %s, want unknown", report.Results[0].State)
	}
}

func TestCheckWithProviderUsesInjectedResult(t *testing.T) {
	calls := 0
	report := CheckWithProviderOptions(context.Background(), map[string]config.MonitorTarget{
		"pve": {Type: "pve-api", Address: "https://not-used.invalid", Environment: "lab"},
	}, 1, 0, func(_ context.Context, name string, target config.MonitorTarget) (Result, bool) {
		calls++
		return Result{Name: name, Type: target.Type, State: Healthy, Detail: "provider health passed"}, true
	})
	if calls != 1 || report.Overall != Healthy || report.Results[0].Detail != "provider health passed" {
		t.Fatalf("provider result = %#v, calls=%d", report, calls)
	}
}

func TestSafeAddressDoesNotExposeURLQuery(t *testing.T) {
	if got := SafeAddress("https://example.test/check?token=secret#fragment"); got != "https://example.test/check?<redacted>#%3Credacted%3E" {
		t.Fatalf("safe address = %q", got)
	}
}
