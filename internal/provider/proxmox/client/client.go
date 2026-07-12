package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
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
func New(endpoint string, creds *domain.Credentials, opts ...httpclient.Option) *Client {
	c := httpclient.New(opts...)
	base := strings.TrimRight(endpoint, "/") + DefaultAPIPath

	var token string
	if creds.TokenID != "" && creds.TokenSecret != "" {
		token = creds.TokenID + "=" + creds.TokenSecret
	}

	return &Client{
		endpoint: endpoint,
		baseURL:  base,
		client:   c,
		token:    token,
	}
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
