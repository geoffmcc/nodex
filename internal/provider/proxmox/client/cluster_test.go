package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestCreateClusterContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cluster/config" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.FormValue("clustername") != "lab" || r.FormValue("link0") != "10.0.0.10" {
			t.Fatalf("form = %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"data":"UPID:pve1:clustercreate:"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	upid, err := c.CreateCluster(context.Background(), ClusterInitRequest{ClusterName: "lab", Link0: "10.0.0.10"})
	if err != nil || upid != "UPID:pve1:clustercreate:" {
		t.Fatalf("CreateCluster = %q, %v", upid, err)
	}
}

func TestJoinClusterRefusesBeforeRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	_, err := c.JoinCluster(context.Background(), ClusterJoinRequest{Hostname: "10.0.0.20", Fingerprint: strings.Repeat("AA:", 31) + "AA"})
	if err == nil || !strings.Contains(err.Error(), "does not accept or transport passwords") {
		t.Fatalf("JoinCluster error = %v", err)
	}
	if called {
		t.Fatal("join refusal made a network request")
	}
}
