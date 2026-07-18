package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// testCredentials returns fictional PBS API token credentials.
func testCredentials() *domain.Credentials {
	return &domain.Credentials{
		Type:        "token",
		TokenID:     "automation@pbs!nodex",
		TokenSecret: strings.Repeat("synthetic-pbs-secret-", 2),
	}
}

// newTestClient builds a Client pointed at an httptest server. The test
// server uses plain HTTP internally, so the client is assembled directly the
// same way other Nodex client tests do; New() is exercised separately.
func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	creds := testCredentials()
	return &Client{
		endpoint: srv.URL,
		baseURL:  srv.URL + DefaultAPIPath,
		client:   httpclient.New(),
		token:    creds.TokenID + ":" + creds.TokenSecret,
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"https://pbs.example.invalid:8007", "https://pbs.example.invalid:8007", false},
		{"https://pbs.example.invalid:8007/", "https://pbs.example.invalid:8007", false},
		{"http://pbs.example.invalid:8007", "", true},
		{"https://user:pass@pbs.example.invalid:8007", "", true},
		{"https://pbs.example.invalid:8007/api2/json", "", true},
		{"https://pbs.example.invalid:8007?x=1", "", true},
		{"https://pbs.example.invalid:8007#frag", "", true},
		{"://bad", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := NormalizeEndpoint(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("NormalizeEndpoint(%q) = %q, want error", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormalizeEndpoint(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeEndpoint(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewRejectsHTTP(t *testing.T) {
	if _, err := New("http://pbs.example.invalid:8007", testCredentials()); err == nil {
		t.Fatal("expected http endpoint to be rejected")
	}
}

func TestNewBuildsPBSToken(t *testing.T) {
	c, err := New("https://pbs.example.invalid:8007", testCredentials())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	creds := testCredentials()
	want := creds.TokenID + ":" + creds.TokenSecret
	if c.token != want {
		t.Errorf("token = %q, want PBS id:secret form %q", c.token, want)
	}
	if c.baseURL != "https://pbs.example.invalid:8007/api2/json" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.endpointHost != "pbs.example.invalid" {
		t.Errorf("endpointHost = %q", c.endpointHost)
	}
}

func TestAuthorizationHeaderUsesPBSScheme(t *testing.T) {
	var gotAuth string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":{"version":"4.0.1","release":"1","repoid":"fictional"}}`))
	}))

	if _, err := c.Version(context.Background()); err != nil {
		t.Fatalf("Version: %v", err)
	}
	creds := testCredentials()
	want := "PBSAPIToken=" + creds.TokenID + ":" + creds.TokenSecret
	if gotAuth != want {
		t.Errorf("Authorization = %q, want %q", gotAuth, want)
	}
	if !strings.HasPrefix(gotAuth, "PBSAPIToken=") {
		t.Errorf("Authorization must use the PBSAPIToken scheme, got %q", gotAuth)
	}
	if strings.Contains(gotAuth, "PVEAPIToken") {
		t.Error("PBS client must never send the PVE authorization scheme")
	}
}

func TestRequestPathsAndParams(t *testing.T) {
	var gotPath, gotQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	c := newTestClient(t, handler)
	ctx := context.Background()

	tests := []struct {
		name      string
		call      func() error
		wantPath  string
		wantQuery string
	}{
		{"datastores", func() error { _, err := c.Datastores(ctx); return err },
			"/api2/json/config/datastore", ""},
		{"snapshots with filter", func() error {
			_, err := c.Snapshots(ctx, "backups", domain.PBSSnapshotFilter{
				Namespace: "prod", BackupType: "vm", BackupID: "100",
			})
			return err
		}, "/api2/json/admin/datastore/backups/snapshots", "backup-id=100&backup-type=vm&ns=prod"},
		{"tasks with filter", func() error {
			_, err := c.Tasks(ctx, domain.PBSTaskFilter{Running: true, Errors: true, Limit: 5})
			return err
		}, "/api2/json/nodes/localhost/tasks", "errors=true&limit=5&running=true"},
		{"verify jobs", func() error { _, err := c.VerifyJobs(ctx); return err },
			"/api2/json/config/verify", ""},
		{"prune jobs", func() error { _, err := c.PruneJobs(ctx); return err },
			"/api2/json/config/prune", ""},
		{"sync jobs", func() error { _, err := c.SyncJobs(ctx); return err },
			"/api2/json/config/sync", ""},
		{"gc all", func() error { _, err := c.GCStatuses(ctx); return err },
			"/api2/json/admin/gc", ""},
		{"certificates", func() error { _, err := c.Certificates(ctx); return err },
			"/api2/json/nodes/localhost/certificates/info", ""},
		{"datastore usage", func() error { _, err := c.DatastoreUsages(ctx); return err },
			"/api2/json/status/datastore-usage", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotQuery = "", ""
			if err := tt.call(); err != nil {
				t.Fatalf("call: %v", err)
			}
			if gotPath != tt.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tt.wantPath)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("query = %q, want %q", gotQuery, tt.wantQuery)
			}
		})
	}
}

func TestStoreNameEscapedInPath(t *testing.T) {
	var gotEscaped string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEscaped = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{"total":1,"used":1,"avail":0}}`))
	}))
	if _, err := c.DatastoreStatus(context.Background(), "odd store/../x"); err != nil {
		t.Fatalf("DatastoreStatus: %v", err)
	}
	if strings.Contains(gotEscaped, "/../") {
		t.Errorf("store name was not escaped: %q", gotEscaped)
	}
}

func TestValidateUPID(t *testing.T) {
	valid := "UPID:pbs:00001234:00005678:00000001:65f00000:garbage_collection:backups:automation@pbs!nodex:"
	tests := []struct {
		name  string
		upid  string
		valid bool
	}{
		{"valid PBS upid", valid, true},
		{"missing prefix", "NOTUPID:pbs:00001234:", false},
		{"too short", "UPID:x:", false},
		{"embedded slash", "UPID:pbs:00001234:00005678:00000001:65f00000:reader:x/../y:root@pam:", false},
		{"embedded space", "UPID:pbs:00001234:00005678:00000001:65f00000:reader:a b:root@pam:", false},
		{"embedded newline", "UPID:pbs:00001234:00005678:00000001:65f00000:reader:a\nb:root@pam:", false},
		{"embedded percent", "UPID:pbs:00001234:00005678:00000001:65f00000:reader:a%2Fb:root@pam:", false},
		{"too long", "UPID:" + strings.Repeat("x", 300), false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUPID(tt.upid)
			if tt.valid && err != nil {
				t.Errorf("expected valid, got: %v", err)
			}
			if !tt.valid && err == nil {
				t.Error("expected invalid, got nil")
			}
		})
	}
}

func TestTaskStatusRejectsInvalidUPIDWithoutRequest(t *testing.T) {
	requested := false
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	if _, err := c.TaskStatus(context.Background(), "not-a-upid"); err == nil {
		t.Fatal("expected invalid UPID error")
	}
	if _, err := c.TaskLog(context.Background(), "not-a-upid"); err == nil {
		t.Fatal("expected invalid UPID error")
	}
	if requested {
		t.Error("invalid UPID must be rejected before any request is sent")
	}
}

func TestErrorResponseRedactedAndSanitized(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("auth failed for PBSAPIToken=root@pbs!x:leaked-secret-value \x1b]0;owned\x07"))
	}))
	_, err := c.Datastores(context.Background())
	if err == nil {
		t.Fatal("expected error for 401")
	}
	msg := err.Error()
	if strings.Contains(msg, "leaked-secret-value") {
		t.Errorf("error message leaked token secret: %q", msg)
	}
	if strings.Contains(msg, "\x1b") || strings.Contains(msg, "owned") {
		t.Errorf("error message not terminal-sanitized: %q", msg)
	}
	if !strings.Contains(msg, "401") {
		t.Errorf("error message should include status code: %q", msg)
	}
}

func TestDecodeRejectsTrailingData(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}{"sneaky":true}`))
	}))
	if _, err := c.Datastores(context.Background()); err == nil {
		t.Fatal("expected trailing-data rejection")
	}
}

func TestContextCancellation(t *testing.T) {
	block := make(chan struct{})
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	t.Cleanup(func() { close(block) })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Datastores(ctx); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestEmptyListsDecodeToEmpty(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	ctx := context.Background()
	stores, err := c.Datastores(ctx)
	if err != nil {
		t.Fatalf("Datastores: %v", err)
	}
	if len(stores) != 0 {
		t.Errorf("expected empty list, got %d items", len(stores))
	}
	tasks, err := c.Tasks(ctx, domain.PBSTaskFilter{})
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected empty task list, got %d items", len(tasks))
	}
}
