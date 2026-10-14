package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// A modern job is expressed with the systemd-calendar `schedule` field and a
// blank starttime. The client must send that field and must not fabricate a
// starttime, otherwise PVE rejects the request as a malformed clock time.
func TestCreateBackupScheduleSendsCalendarSchedule(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/backup" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody, _ = url.ParseQuery(string(raw))
		_, _ = w.Write([]byte(`{"data":"UPID:proxmox:00001234:00001234:00000000:cluster-backup:root@pam:"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	upid, err := c.CreateBackupSchedule(context.Background(), BackupScheduleCreateRequest{
		Storage:  "pbs",
		Mode:     "snapshot",
		Schedule: "mon..fri 03:30",
	})
	if err != nil {
		t.Fatalf("CreateBackupSchedule: %v", err)
	}
	if upid == "" {
		t.Fatal("expected a UPID")
	}
	if gotBody.Get("schedule") != "mon..fri 03:30" {
		t.Errorf("schedule = %q, want %q", gotBody.Get("schedule"), "mon..fri 03:30")
	}
	if _, present := gotBody["starttime"]; present {
		t.Errorf("starttime must be omitted for a calendar schedule, got %q", gotBody.Get("starttime"))
	}
}

// A legacy job keeps the deprecated starttime/dow pair and must not gain a
// schedule field.
func TestCreateBackupScheduleSendsLegacyStarttime(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotBody, _ = url.ParseQuery(string(raw))
		_, _ = w.Write([]byte(`{"data":"UPID:proxmox:00001234:00001234:00000000:cluster-backup:root@pam:"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.CreateBackupSchedule(context.Background(), BackupScheduleCreateRequest{
		Storage:   "pbs",
		Mode:      "snapshot",
		Starttime: "03:30",
		Dow:       "mon,fri",
	}); err != nil {
		t.Fatalf("CreateBackupSchedule: %v", err)
	}
	if gotBody.Get("starttime") != "03:30" || gotBody.Get("dow") != "mon,fri" {
		t.Errorf("starttime/dow = %q/%q, want 03:30/mon,fri", gotBody.Get("starttime"), gotBody.Get("dow"))
	}
	if _, present := gotBody["schedule"]; present {
		t.Errorf("schedule must be omitted for a legacy job, got %q", gotBody.Get("schedule"))
	}
}

// A request without either timing form is rejected locally before any round
// trip, matching the CLI's own validation.
func TestCreateBackupScheduleRequiresTiming(t *testing.T) {
	c := &Client{baseURL: "https://unused.invalid", client: httpclient.New()}
	if _, err := c.CreateBackupSchedule(context.Background(), BackupScheduleCreateRequest{
		Storage: "pbs",
		Mode:    "snapshot",
	}); err == nil {
		t.Fatal("expected an error when neither schedule nor starttime is supplied")
	}
}

// Update must forward a calendar schedule so converting a legacy job to a
// modern schedule actually reaches PVE.
func TestUpdateBackupScheduleSendsCalendarSchedule(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody, _ = url.ParseQuery(string(raw))
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if err := c.UpdateBackupSchedule(context.Background(), "backup-abc", BackupScheduleCreateRequest{
		Schedule: "sun 04:00",
	}); err != nil {
		t.Fatalf("UpdateBackupSchedule: %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/cluster/backup/backup-abc" {
		t.Fatalf("request = %s %s, want PUT /cluster/backup/backup-abc", gotMethod, gotPath)
	}
	if gotBody.Get("schedule") != "sun 04:00" {
		t.Errorf("schedule = %q, want %q", gotBody.Get("schedule"), "sun 04:00")
	}
	if _, present := gotBody["starttime"]; present {
		t.Errorf("starttime must be omitted, got %q", gotBody.Get("starttime"))
	}
}

// The read path must surface both the modern schedule and the computed
// next-run timestamp so the CLI can render modern and legacy jobs.
func TestGetBackupSchedulesMapsScheduleAndNextRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster/backup" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[
			{"id":"backup-modern","storage":"pbs","mode":"snapshot","schedule":"mon..fri 03:30","next-run":1760500000,"enabled":1},
			{"id":"backup-legacy","storage":"local","mode":"suspend","starttime":"01:15","dow":"sun"}
		]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	items, err := c.GetBackupSchedules(context.Background())
	if err != nil {
		t.Fatalf("GetBackupSchedules: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	modern := items[0]
	if modern.Schedule != "mon..fri 03:30" || modern.NextRun != 1760500000 || modern.Starttime != "" {
		t.Errorf("modern = %+v, want schedule/next-run and blank starttime", modern)
	}
	legacy := items[1]
	if legacy.Schedule != "" || legacy.NextRun != 0 || legacy.Starttime != "01:15" || legacy.Dow != "sun" {
		t.Errorf("legacy = %+v, want starttime/dow and blank schedule", legacy)
	}
}
