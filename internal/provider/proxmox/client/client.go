package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

const (
	// DefaultAPIPath is the Proxmox API base path.
	DefaultAPIPath = "/api2/json"
)

// Client is a Proxmox API client.
type Client struct {
	endpoint string
	baseURL  string
	client   *httpclient.Client
	token    string
}

// New creates a new Proxmox API client.
func New(endpoint string, creds *domain.Credentials, opts ...httpclient.Option) (*Client, error) {
	normalized, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	c := httpclient.New(opts...)
	base := strings.TrimRight(normalized, "/") + DefaultAPIPath

	var token string
	if creds.TokenID != "" && creds.TokenSecret != "" {
		token = creds.TokenID + "=" + creds.TokenSecret
	}

	return &Client{
		endpoint: normalized,
		baseURL:  base,
		client:   c,
		token:    token,
	}, nil
}

// NormalizeEndpoint validates and canonicalizes the configured endpoint.
func NormalizeEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("malformed endpoint URL")
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("endpoint must use https scheme")
	}
	if u.Host == "" || u.User != nil {
		return "", fmt.Errorf("endpoint must include a host and must not include user info")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("endpoint must not include query string or fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("endpoint must not include a path")
	}
	u.Path, u.RawPath, u.RawQuery, u.Fragment = "", "", "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

// Version returns the Proxmox version.
func (c *Client) Version(ctx context.Context) (*VersionData, error) {
	var resp VersionResponse
	if err := c.get(ctx, "/version", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Nodes returns all nodes from the cluster.
func (c *Client) Nodes(ctx context.Context) ([]NodeItem, error) {
	var resp NodeListResponse
	if err := c.get(ctx, "/nodes", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// ClusterResources returns all cluster resources.
func (c *Client) ClusterResources(ctx context.Context) ([]ClusterResource, error) {
	var resp ClusterResourcesResponse
	if err := c.get(ctx, "/cluster/resources", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	return nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.token)
	}

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, truncated := readLimited(resp.Body, c.client.MaxErrorBodySize())
		msg := redact.String(output.SanitizeTerminal(string(body)))
		if truncated {
			msg += "... [truncated]"
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, msg)
	}

	body, truncated := readLimited(resp.Body, c.client.MaxBodySize())
	if truncated {
		return fmt.Errorf("response body exceeds %d bytes", c.client.MaxBodySize())
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if tok, err := dec.Token(); err != io.EOF || tok != nil {
		return fmt.Errorf("decode response: trailing data")
	}
	return nil
}

func readLimited(r io.Reader, limit int64) ([]byte, bool) {
	body, _ := io.ReadAll(io.LimitReader(r, limit+1))
	if int64(len(body)) > limit {
		return body[:limit], true
	}
	return body, false
}
