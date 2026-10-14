package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func TestGetNodeCertificates(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"data":[
			{
				"filename":"/etc/pve/local/pve-ssl.pem",
				"fingerprint":"AA:BB:CC",
				"issuer":"CN=Proxmox Virtual Environment CA,O=Proxmox Server Solutions GmbH",
				"notafter":2051222400,
				"notbefore":1735689600,
				"pem":"-----BEGIN CERTIFICATE-----REDACTED-----END CERTIFICATE-----",
				"public-key-bits":2048,
				"public-key-type":"rsa",
				"san":["proxmox.example.com"],
				"subject":"CN=proxmox.example.com"
			}
		]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	certs, err := c.GetNodeCertificates(context.Background(), "proxmox")
	if err != nil {
		t.Fatalf("GetNodeCertificates: %v", err)
	}
	if gotPath != "/nodes/proxmox/certificates/info" {
		t.Errorf("path = %q", gotPath)
	}
	if len(certs) != 1 {
		t.Fatalf("len = %d", len(certs))
	}
	first := certs[0]
	if first.Subject != "CN=proxmox.example.com" {
		t.Errorf("subject = %q", first.Subject)
	}
	if first.Issuer == "" || first.Fingerprint != "AA:BB:CC" {
		t.Errorf("issuer/fingerprint = %q/%q", first.Issuer, first.Fingerprint)
	}
	if first.NotBefore != 1735689600 || first.NotAfter != 2051222400 {
		t.Errorf("validity window = %d..%d", first.NotBefore, first.NotAfter)
	}
	if first.PublicKeyType != "rsa" || first.PublicKeyBits != 2048 {
		t.Errorf("public key = %s/%d", first.PublicKeyType, first.PublicKeyBits)
	}
	if len(first.SAN) != 1 || first.SAN[0] != "proxmox.example.com" {
		t.Errorf("san = %v", first.SAN)
	}
}

func TestGetNodeCertificatesEmptyIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetNodeCertificates(context.Background(), "proxmox"); err == nil {
		t.Fatal("expected error for empty certificate response")
	}
}

func TestGetNodeCertificatesPropagatesError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetNodeCertificates(context.Background(), "proxmox"); err == nil {
		t.Fatal("expected error from forbidden response")
	}
}

func TestGetNodeCertificatesRejectsDirectoryEntries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"name":"info"}]}`))
	}))
	defer server.Close()
	c := &Client{baseURL: server.URL, client: httpclient.New()}
	if _, err := c.GetNodeCertificates(context.Background(), "proxmox"); err == nil {
		t.Fatal("directory entries must not be reported as certificate metadata")
	}
}
