package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestGetSDNSubnets(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"data":[
			{"subnet":"10.0.0.0-10.0.0.255","type":"vnet","vnet":"vnetA","zone":"simple","cidr":"24","gateway":"10.0.0.1"},
			{"subnet":"10.0.1.0-10.0.1.255","type":"vnet","vnet":"vnetB","zone":"simple","cidr":"24"}
		]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	subnets, err := c.GetSDNSubnets(context.Background())
	if err != nil {
		t.Fatalf("GetSDNSubnets: %v", err)
	}
	if gotPath != "/cluster/sdn/subnets" {
		t.Errorf("path = %q", gotPath)
	}
	if len(subnets) != 2 {
		t.Fatalf("len = %d", len(subnets))
	}
	first := subnets[0]
	if first.Subnet != "10.0.0.0-10.0.0.255" || first.VNet != "vnetA" || first.Zone != "simple" {
		t.Errorf("first = %+v", first)
	}
	if first.CIDR != "24" || first.Gateway != "10.0.0.1" {
		t.Errorf("first cidr/gateway = %q/%q", first.CIDR, first.Gateway)
	}
	if subnets[1].Gateway != "" {
		t.Errorf("absent gateway should be empty, got %q", subnets[1].Gateway)
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

func TestGetSDNSubnetsPropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetSDNSubnets(context.Background()); err == nil {
		t.Fatal("expected error from forbidden response")
	}
}
