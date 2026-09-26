package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// PVE returns `tokens` as an array of token objects and `groups` as a CSV string
// when full=1 is requested. This locks in that shape so a future type change
// cannot silently break decoding.
func TestGetUsersRequestsFullAndDecodesGroupAndTokenDetail(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":[
			{"userid":"root@pam","enable":1,"groups":"admins,ops",
			 "tokens":[{"tokenid":"t1","expire":0,"privsep":1},{"tokenid":"t2","comment":"ci"}]},
			{"userid":"svc@pve","enable":0}
		]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	users, err := c.GetUsers(context.Background())
	if err != nil {
		t.Fatalf("GetUsers: %v", err)
	}
	if gotPath != "/access/users" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "full=1" {
		t.Errorf("query = %q, want full=1", gotQuery)
	}
	if len(users) != 2 {
		t.Fatalf("len = %d", len(users))
	}

	root := users[0]
	if root.Groups != "admins,ops" {
		t.Errorf("groups = %q", root.Groups)
	}
	if len(root.Tokens) != 2 {
		t.Fatalf("len(tokens) = %d, want 2", len(root.Tokens))
	}
	if root.Tokens[0].TokenID != "t1" || root.Tokens[0].PrivSep != 1 {
		t.Errorf("token[0] = %+v", root.Tokens[0])
	}
	if root.Tokens[1].TokenID != "t2" || root.Tokens[1].Comment != "ci" {
		t.Errorf("token[1] = %+v", root.Tokens[1])
	}

	// A user with no tokens must stay nil, not an empty slice, so callers can
	// tell "reported as zero" from "not reported".
	if users[1].Tokens != nil {
		t.Errorf("absent tokens = %+v, want nil", users[1].Tokens)
	}
	if users[1].Groups != "" {
		t.Errorf("absent groups = %q", users[1].Groups)
	}
}

// Older PVE releases do not know the full parameter; that must degrade to the
// plain index instead of failing the command.
func TestGetUsersFallsBackWhenFullIsRejected(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		if r.URL.RawQuery != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":{"full":"unknown parameter"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"userid":"root@pam","enable":1}]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	users, err := c.GetUsers(context.Background())
	if err != nil {
		t.Fatalf("GetUsers: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("requests = %v, want 2 (full=1 then fallback)", paths)
	}
	if paths[0] != "/access/users?full=1" || paths[1] != "/access/users" {
		t.Errorf("requests = %v", paths)
	}
	if len(users) != 1 || users[0].UserID != "root@pam" {
		t.Errorf("users = %+v", users)
	}
}

// An unrelated failure must not be masked by a second request.
func TestGetUsersDoesNotFallbackOnServerError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetUsers(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}
