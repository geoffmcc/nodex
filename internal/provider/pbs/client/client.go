// Package client implements a typed HTTPS client for the Proxmox Backup
// Server API (/api2/json on port 8007 by convention). It shares Nodex's
// hardened transport (HTTPS only, TLS 1.2+, additive CA trust, bounded
// retries for GETs, response body limits) and authenticates with the PBS
// API-token scheme:
//
//	Authorization: PBSAPIToken=user@realm!tokenname:secret
//
// Note the ':' separating token name and secret — PBS differs from PVE's
// 'PVEAPIToken=user@realm!tokenid=secret' here.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

const (
	// DefaultAPIPath is the PBS API base path.
	DefaultAPIPath = "/api2/json"

	// localNode is the node name PBS accepts for the local host in REST paths.
	localNode = "localhost"
)

var successCodes = map[int]bool{
	http.StatusOK:       true,
	http.StatusCreated:  true,
	http.StatusAccepted: true,
}

// Client is a Proxmox Backup Server API client.
type Client struct {
	endpoint     string
	endpointHost string
	baseURL      string
	client       *httpclient.Client
	token        string
	version      *VersionData
}

// New creates a new PBS API client.
func New(endpoint string, creds *domain.Credentials, opts ...httpclient.Option) (*Client, error) {
	normalized, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	c := httpclient.New(opts...)
	base := strings.TrimRight(normalized, "/") + DefaultAPIPath

	var token string
	if creds.TokenID != "" && creds.TokenSecret != "" {
		// PBS separates token id and secret with ':'.
		token = creds.TokenID + ":" + creds.TokenSecret
	}

	return &Client{
		endpoint:     normalized,
		endpointHost: parsed.Hostname(),
		baseURL:      base,
		client:       c,
		token:        token,
	}, nil
}

// NormalizeEndpoint validates and canonicalizes the configured endpoint.
// The rules match Nodex's endpoint policy: HTTPS, host only, no userinfo,
// no path, no query or fragment.
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

// ValidateUPID checks that a task identifier looks like a PBS UPID and is
// safe to embed in a request path. PBS UPIDs are treated as opaque beyond
// the prefix; their internal format differs from PVE UPIDs.
func ValidateUPID(upid string) error {
	if !strings.HasPrefix(upid, "UPID:") {
		return fmt.Errorf("invalid UPID: must start with \"UPID:\"")
	}
	if len(upid) < 36 || len(upid) > 256 {
		return fmt.Errorf("invalid UPID: unexpected length")
	}
	if strings.ContainsAny(upid, " \t\r\n/\\?#%") {
		return fmt.Errorf("invalid UPID: contains unsafe characters")
	}
	return nil
}

// Version returns the PBS server version (GET /version).
func (c *Client) Version(ctx context.Context) (*VersionData, error) {
	var resp VersionResponse
	if err := c.get(ctx, "/version", &resp); err != nil {
		return nil, err
	}
	c.version = &resp.Data
	return &resp.Data, nil
}

// VersionData returns the stored version data, if any.
func (c *Client) VersionData() *VersionData {
	return c.version
}

// NodeStatus returns the PBS host status (GET /nodes/localhost/status).
func (c *Client) NodeStatus(ctx context.Context) (*NodeStatusData, error) {
	var resp NodeStatusResponse
	if err := c.get(ctx, "/nodes/"+localNode+"/status", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Datastores lists datastore configurations (GET /config/datastore).
func (c *Client) Datastores(ctx context.Context) ([]DatastoreConfig, error) {
	var resp DatastoreListResponse
	if err := c.get(ctx, "/config/datastore", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Datastore returns one datastore configuration (GET /config/datastore/{name}).
func (c *Client) Datastore(ctx context.Context, name string) (*DatastoreConfig, error) {
	var resp DatastoreResponse
	if err := c.get(ctx, "/config/datastore/"+url.PathEscape(name), &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DatastoreStatus returns datastore usage (GET /admin/datastore/{store}/status).
func (c *Client) DatastoreStatus(ctx context.Context, store string) (*DatastoreStatusData, error) {
	var resp DatastoreStatusResponse
	if err := c.get(ctx, "/admin/datastore/"+url.PathEscape(store)+"/status", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// DatastoreUsages returns usage summaries for all datastores
// (GET /status/datastore-usage).
func (c *Client) DatastoreUsages(ctx context.Context) ([]DatastoreUsageItem, error) {
	var resp DatastoreUsageResponse
	if err := c.get(ctx, "/status/datastore-usage", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Snapshots lists backup snapshots in a datastore
// (GET /admin/datastore/{store}/snapshots).
func (c *Client) Snapshots(ctx context.Context, store string, filter domain.PBSSnapshotFilter) ([]SnapshotItem, error) {
	q := url.Values{}
	if filter.Namespace != "" {
		q.Set("ns", filter.Namespace)
	}
	if filter.BackupType != "" {
		q.Set("backup-type", filter.BackupType)
	}
	if filter.BackupID != "" {
		q.Set("backup-id", filter.BackupID)
	}
	path := "/admin/datastore/" + url.PathEscape(store) + "/snapshots"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var resp SnapshotListResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Tasks lists tasks (GET /nodes/localhost/tasks).
func (c *Client) Tasks(ctx context.Context, filter domain.PBSTaskFilter) ([]TaskItem, error) {
	q := url.Values{}
	if filter.Running {
		q.Set("running", "true")
	}
	if filter.Errors {
		q.Set("errors", "true")
	}
	if filter.Limit > 0 {
		q.Set("limit", strconv.FormatInt(filter.Limit, 10))
	}
	if filter.Store != "" {
		q.Set("store", filter.Store)
	}
	if filter.TypeFilter != "" {
		q.Set("typefilter", filter.TypeFilter)
	}
	if filter.Since > 0 {
		q.Set("since", strconv.FormatInt(filter.Since, 10))
	}
	if filter.Until > 0 {
		q.Set("until", strconv.FormatInt(filter.Until, 10))
	}
	path := "/nodes/" + localNode + "/tasks"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var resp TaskListResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// TaskStatus returns detailed task state
// (GET /nodes/localhost/tasks/{upid}/status).
func (c *Client) TaskStatus(ctx context.Context, upid string) (*TaskStatusData, error) {
	if err := ValidateUPID(upid); err != nil {
		return nil, err
	}
	var resp TaskStatusResponse
	if err := c.get(ctx, "/nodes/"+localNode+"/tasks/"+url.PathEscape(upid)+"/status", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// TaskLog returns the task log (GET /nodes/localhost/tasks/{upid}/log).
func (c *Client) TaskLog(ctx context.Context, upid string) ([]TaskLogLine, error) {
	if err := ValidateUPID(upid); err != nil {
		return nil, err
	}
	var resp TaskLogResponse
	if err := c.get(ctx, "/nodes/"+localNode+"/tasks/"+url.PathEscape(upid)+"/log", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// VerifyJobs lists verification jobs (GET /config/verify).
func (c *Client) VerifyJobs(ctx context.Context) ([]VerifyJobConfig, error) {
	var resp VerifyJobListResponse
	if err := c.get(ctx, "/config/verify", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// PruneJobs lists prune jobs (GET /config/prune).
func (c *Client) PruneJobs(ctx context.Context) ([]PruneJobConfig, error) {
	var resp PruneJobListResponse
	if err := c.get(ctx, "/config/prune", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// SyncJobs lists sync jobs (GET /config/sync).
func (c *Client) SyncJobs(ctx context.Context) ([]SyncJobConfig, error) {
	var resp SyncJobListResponse
	if err := c.get(ctx, "/config/sync", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GCStatuses returns garbage-collection status for all datastores
// (GET /admin/gc).
func (c *Client) GCStatuses(ctx context.Context) ([]GCStatusData, error) {
	var resp GCListResponse
	if err := c.get(ctx, "/admin/gc", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GCStatus returns garbage-collection status for one datastore
// (GET /admin/datastore/{store}/gc).
func (c *Client) GCStatus(ctx context.Context, store string) (*GCStatusData, error) {
	var resp GCStatusResponse
	if err := c.get(ctx, "/admin/datastore/"+url.PathEscape(store)+"/gc", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Subscription returns subscription state (GET /nodes/localhost/subscription).
func (c *Client) Subscription(ctx context.Context) (*SubscriptionData, error) {
	var resp SubscriptionResponse
	if err := c.get(ctx, "/nodes/"+localNode+"/subscription", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// Certificates returns certificate information
// (GET /nodes/localhost/certificates/info).
func (c *Client) Certificates(ctx context.Context) ([]CertificateInfo, error) {
	var resp CertificateListResponse
	if err := c.get(ctx, "/nodes/"+localNode+"/certificates/info", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	return nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	u := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "PBSAPIToken="+c.token)
	}

	resp, err := c.client.Do(ctx, req)
	if err != nil {
		return &app.ProviderError{StatusCode: 0, Detail: fmt.Sprintf("execute request: %v", err), Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	return c.decodeResponse(resp, result)
}

// decodeResponse reads, validates, and decodes a PBS API response.
func (c *Client) decodeResponse(resp *http.Response, result any) error {
	if !successCodes[resp.StatusCode] {
		body, truncated := readLimited(resp.Body, c.client.MaxErrorBodySize())
		msg := redact.String(output.SanitizeTerminal(string(body)))
		if truncated {
			msg += "... [truncated]"
		}
		return &app.ProviderError{
			StatusCode: resp.StatusCode,
			Detail:     fmt.Sprintf("API error %d: %s", resp.StatusCode, msg),
		}
	}

	body, truncated := readLimited(resp.Body, c.client.MaxBodySize())
	if truncated {
		return fmt.Errorf("response body exceeds %d bytes", c.client.MaxBodySize())
	}
	if result == nil {
		return nil
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
