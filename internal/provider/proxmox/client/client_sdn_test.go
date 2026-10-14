package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestGetSDNSubnets(t *testing.T) {
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		gotPaths = append(gotPaths, r.URL.Path)
		switch r.URL.Path {
		case "/cluster/sdn/vnets":
			_, _ = w.Write([]byte(`{"data":[
				{"vnet":"vnetA","zone":"simple"},
				{"vnet":"vnetB","zone":"simple"}
			]}`))
		case "/cluster/sdn/vnets/vnetA/subnets":
			_, _ = w.Write([]byte(`{"data":[
				{"subnet":"vnetA-10.0.0.0-24","type":"subnet","vnet":"vnetA","zone":"simple","cidr":"10.0.0.0/24","gateway":"10.0.0.1"}
			]}`))
		case "/cluster/sdn/vnets/vnetB/subnets":
			_, _ = w.Write([]byte(`{"data":[
				{"subnet":"vnetB-10.0.1.0-24","type":"subnet","vnet":"vnetB","zone":"simple","cidr":"10.0.1.0/24"}
			]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	subnets, err := c.GetSDNSubnets(context.Background())
	if err != nil {
		t.Fatalf("GetSDNSubnets: %v", err)
	}
	wantPaths := []string{
		"/cluster/sdn/vnets",
		"/cluster/sdn/vnets/vnetA/subnets",
		"/cluster/sdn/vnets/vnetB/subnets",
	}
	if strings.Join(gotPaths, ",") != strings.Join(wantPaths, ",") {
		t.Errorf("paths = %v, want %v", gotPaths, wantPaths)
	}
	if len(subnets) != 2 {
		t.Fatalf("len = %d", len(subnets))
	}
	first := subnets[0]
	if first.Subnet != "vnetA-10.0.0.0-24" || first.VNet != "vnetA" || first.Zone != "simple" {
		t.Errorf("first = %+v", first)
	}
	if first.CIDR != "10.0.0.0/24" || first.Gateway != "10.0.0.1" {
		t.Errorf("first cidr/gateway = %q/%q", first.CIDR, first.Gateway)
	}
	if subnets[1].Gateway != "" {
		t.Errorf("absent gateway should be empty, got %q", subnets[1].Gateway)
	}
}

func TestGetSDNSubnetsNoVNets(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/cluster/sdn/vnets" {
			t.Fatalf("unexpected subnet request for empty topology: %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	subnets, err := c.GetSDNSubnets(context.Background())
	if err != nil {
		t.Fatalf("GetSDNSubnets: %v", err)
	}
	if len(subnets) != 0 {
		t.Fatalf("len = %d, want 0", len(subnets))
	}
	if len(paths) != 1 || paths[0] != "/cluster/sdn/vnets" {
		t.Errorf("paths = %v, want only the vnets listing", paths)
	}
}

func TestGetSDNSubnetsFillsMissingVNetZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster/sdn/vnets":
			_, _ = w.Write([]byte(`{"data":[{"vnet":"vnetA","zone":"simple"}]}`))
		case "/cluster/sdn/vnets/vnetA/subnets":
			_, _ = w.Write([]byte(`{"data":[{"subnet":"vnetA-10.0.0.0-24","cidr":"10.0.0.0/24"}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	subnets, err := c.GetSDNSubnets(context.Background())
	if err != nil {
		t.Fatalf("GetSDNSubnets: %v", err)
	}
	if len(subnets) != 1 {
		t.Fatalf("len = %d", len(subnets))
	}
	if subnets[0].VNet != "vnetA" || subnets[0].Zone != "simple" {
		t.Errorf("vnet/zone not filled from VNet: %+v", subnets[0])
	}
}

func TestGetSDNControllers(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":[
			{"controller":"evpn1","type":"evpn","status":"online","asn":65000},
			{"controller":"bgp1","type":"bgp"}
		]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	controllers, err := c.GetSDNControllers(context.Background())
	if err != nil {
		t.Fatalf("GetSDNControllers: %v", err)
	}
	if gotPath != "/cluster/sdn/controllers" {
		t.Errorf("path = %q", gotPath)
	}
	if len(controllers) != 2 {
		t.Fatalf("len = %d", len(controllers))
	}
	if controllers[0].Name != "evpn1" || controllers[0].Type != "evpn" || controllers[0].State != "online" || controllers[0].Asn != 65000 {
		t.Errorf("first = %+v", controllers[0])
	}
	if controllers[1].Asn != 0 {
		t.Errorf("absent asn should be 0, got %d", controllers[1].Asn)
	}
}

func TestGetSDNSubnetsPropagatesVNetsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetSDNSubnets(context.Background()); err == nil {
		t.Fatal("expected error from forbidden response")
	}
}

func TestGetSDNSubnetsPropagatesSubnetError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cluster/sdn/vnets" {
			_, _ = w.Write([]byte(`{"data":[{"vnet":"vnetA","zone":"simple"}]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetSDNSubnets(context.Background()); err == nil {
		t.Fatal("expected error from forbidden subnet response")
	}
}
