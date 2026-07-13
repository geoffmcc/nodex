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

	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/redact"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

// Success status codes for API operations.
var successCodes = map[int]bool{
	http.StatusOK:       true, // 200
	http.StatusCreated:  true, // 201
	http.StatusAccepted: true, // 202
}

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
	version  *VersionData
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
	c.version = &resp.Data
	return &resp.Data, nil
}

// VersionData returns the stored version data, if any.
func (c *Client) VersionData() *VersionData {
	return c.version
}

// Release returns the release string from the stored version data.
func (c *Client) Release() string {
	if c.version == nil {
		return ""
	}
	return c.version.Release
}

// VersionAtLeast checks if the stored version is at least the specified major.minor.
func (c *Client) VersionAtLeast(major, minor int) bool {
	if c.version == nil {
		return false
	}
	v := c.version.Version
	if v == "" {
		return false
	}
	// Parse version string like "8.1.4" or "9.2.1"
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false
	}
	maj, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	min, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	if maj > major {
		return true
	}
	if maj == major {
		return min >= minor
	}
	return false
}

// GetNodeStatus returns detailed status for a specific node.
func (c *Client) GetNodeStatus(ctx context.Context, node string) (*NodeStatusData, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeStatusResponse
	path := "/nodes/" + url.PathEscape(node) + "/status"
	if err := c.get(ctx, path, &resp); err != nil {
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

// GetClusterStatus returns the cluster status including quorum and node info.
func (c *Client) GetClusterStatus(ctx context.Context) ([]ClusterStatusItem, error) {
	var resp ClusterStatusResponse
	if err := c.get(ctx, "/cluster/status", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetVMConfig returns configuration for a specific VM.
func (c *Client) GetVMConfig(ctx context.Context, node string, vmid int) (*VMConfigData, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	var resp VMConfigResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/config"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetContainerConfig returns configuration for a specific container.
func (c *Client) GetContainerConfig(ctx context.Context, node string, vmid int) (*ContainerConfigData, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	var resp ContainerConfigResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/config"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetStorageContent returns the content of a specific storage.
func (c *Client) GetStorageContent(ctx context.Context, node, storage string) ([]StorageContentItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if storage == "" {
		return nil, fmt.Errorf("storage name is required")
	}
	var resp StorageContentResponse
	path := "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/content"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetTasks returns all tasks for a specific node.
func (c *Client) GetTasks(ctx context.Context, node string) ([]TaskListItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp TaskListResponse
	path := "/nodes/" + url.PathEscape(node) + "/tasks"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetTask returns details for a specific task by UPID.
func (c *Client) GetTask(ctx context.Context, node, upid string) (*TaskListItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if upid == "" {
		return nil, fmt.Errorf("task UPID is required")
	}
	var resp TaskDetailResponse
	path := "/nodes/" + url.PathEscape(node) + "/tasks/" + url.PathEscape(upid)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetVMSnapshots returns snapshots for a VM.
func (c *Client) GetVMSnapshots(ctx context.Context, node string, vmid int) ([]SnapshotListItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	var resp SnapshotListResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/snapshot"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetContainerSnapshots returns snapshots for a container.
func (c *Client) GetContainerSnapshots(ctx context.Context, node string, vmid int) ([]SnapshotListItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	var resp SnapshotListResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/snapshot"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetEvents returns cluster events.
func (c *Client) GetEvents(ctx context.Context) ([]EventItem, error) {
	var resp EventListResponse
	if err := c.get(ctx, "/cluster/events", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetSyslog returns syslog entries for a specific node.
func (c *Client) GetSyslog(ctx context.Context, node string) ([]SyslogItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp SyslogResponse
	path := "/nodes/" + url.PathEscape(node) + "/syslog"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetBackupStatus returns backup tasks for a specific node.
func (c *Client) GetBackupStatus(ctx context.Context, node string) ([]BackupStatusItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp BackupStatusResponse
	path := "/nodes/" + url.PathEscape(node) + "/tasks"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	// Filter for backup tasks (vzdump)
	result := make([]BackupStatusItem, 0)
	for _, item := range resp.Data {
		if item.Type == "vzdump" {
			item.Node = node
			result = append(result, item)
		}
	}
	return result, nil
}

// GetFirewallRules returns cluster firewall rules.
func (c *Client) GetFirewallRules(ctx context.Context) ([]FirewallRuleItem, error) {
	var resp FirewallRuleResponse
	if err := c.get(ctx, "/cluster/firewall/rules", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetHAResources returns HA resources.
func (c *Client) GetHAResources(ctx context.Context) ([]HAResourceItem, error) {
	var resp HAResourceResponse
	if err := c.get(ctx, "/cluster/ha/resources", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetHAGroups returns HA groups.
func (c *Client) GetHAGroups(ctx context.Context) ([]HAGroupItem, error) {
	var resp HAGroupResponse
	if err := c.get(ctx, "/cluster/ha/groups", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetNodeServices returns services running on a specific node.
func (c *Client) GetNodeServices(ctx context.Context, node string) ([]NodeServiceItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeServicesResponse
	path := "/nodes/" + url.PathEscape(node) + "/services"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetNodeNetwork returns network interfaces on a specific node.
func (c *Client) GetNodeNetwork(ctx context.Context, node string) ([]NodeNetworkItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeNetworkResponse
	path := "/nodes/" + url.PathEscape(node) + "/network"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetNodeDNS returns DNS configuration for a specific node.
func (c *Client) GetNodeDNS(ctx context.Context, node string) (*NodeDNSData, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeDNSResponse
	path := "/nodes/" + url.PathEscape(node) + "/dns"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetNodeTime returns time configuration for a specific node.
func (c *Client) GetNodeTime(ctx context.Context, node string) (*NodeTimeData, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeTimeResponse
	path := "/nodes/" + url.PathEscape(node) + "/time"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetNodeDisks returns disk inventory for a specific node.
func (c *Client) GetNodeDisks(ctx context.Context, node string) ([]NodeDiskItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeDisksResponse
	path := "/nodes/" + url.PathEscape(node) + "/disks/list"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetNodeCertificates returns TLS certificates for a specific node.
func (c *Client) GetNodeCertificates(ctx context.Context, node string) ([]NodeCertificateItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeCertificatesResponse
	path := "/nodes/" + url.PathEscape(node) + "/certificates"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetNodeSubscription returns subscription status for a specific node.
func (c *Client) GetNodeSubscription(ctx context.Context, node string) (*NodeSubscriptionData, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeSubscriptionResponse
	path := "/nodes/" + url.PathEscape(node) + "/subscription"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetNodeUpdates returns available updates for a specific node.
func (c *Client) GetNodeUpdates(ctx context.Context, node string) ([]NodeUpdateItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeUpdatesResponse
	path := "/nodes/" + url.PathEscape(node) + "/updates"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetFirewallAliases returns cluster firewall aliases.
func (c *Client) GetFirewallAliases(ctx context.Context) ([]FirewallAliasItem, error) {
	var resp FirewallAliasesResponse
	if err := c.get(ctx, "/cluster/firewall/aliases", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetFirewallIPSets returns cluster firewall IP sets.
func (c *Client) GetFirewallIPSets(ctx context.Context) ([]FirewallIPSetItem, error) {
	var resp FirewallIPSetsResponse
	if err := c.get(ctx, "/cluster/firewall/ipset", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetFirewallIPSetEntries returns entries for a specific IP set.
func (c *Client) GetFirewallIPSetEntries(ctx context.Context, name string) ([]FirewallIPSetEntryItem, error) {
	if name == "" {
		return nil, fmt.Errorf("IP set name is required")
	}
	var resp FirewallIPSetEntriesResponse
	path := "/cluster/firewall/ipset/" + url.PathEscape(name)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetFirewallSecurityGroups returns cluster firewall security groups.
func (c *Client) GetFirewallSecurityGroups(ctx context.Context) ([]FirewallSecurityGroupItem, error) {
	var resp FirewallSecurityGroupsResponse
	if err := c.get(ctx, "/cluster/firewall/groups", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetFirewallOptions returns cluster firewall options.
func (c *Client) GetFirewallOptions(ctx context.Context) (*FirewallOptionsData, error) {
	var resp FirewallOptionsResponse
	if err := c.get(ctx, "/cluster/firewall/options", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetNodeFirewallRules returns firewall rules for a specific node.
func (c *Client) GetNodeFirewallRules(ctx context.Context, node string) ([]FirewallRuleItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp NodeFirewallRulesResponse
	path := "/nodes/" + url.PathEscape(node) + "/firewall/rules"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetVMFirewallRules returns firewall rules for a specific VM.
func (c *Client) GetVMFirewallRules(ctx context.Context, node string, vmid int) ([]FirewallRuleItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	var resp VMFirewallRulesResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/firewall/rules"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetHAStatus returns cluster HA status.
func (c *Client) GetHAStatus(ctx context.Context) (*HAStatusData, error) {
	var resp HAStatusResponse
	if err := c.get(ctx, "/cluster/ha/status", &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// GetHACurrent returns current HA resource states.
func (c *Client) GetHACurrent(ctx context.Context) ([]HACurrentItem, error) {
	var resp HACurrentResponse
	if err := c.get(ctx, "/cluster/ha/current", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetSDNZones returns SDN zones.
func (c *Client) GetSDNZones(ctx context.Context) ([]SDNZoneItem, error) {
	var resp SDNZonesResponse
	if err := c.get(ctx, "/cluster/sdn/zones", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetSDNVNets returns SDN virtual networks.
func (c *Client) GetSDNVNets(ctx context.Context) ([]SDNVNetItem, error) {
	var resp SDNVNetsResponse
	if err := c.get(ctx, "/cluster/sdn/vnets", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetVMSnapshotConfig returns configuration for a specific VM snapshot.
func (c *Client) GetVMSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	if name == "" {
		return nil, fmt.Errorf("snapshot name is required")
	}
	var resp VMSnapshotConfigResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/snapshot/" + url.PathEscape(name) + "/config"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetContainerSnapshotConfig returns configuration for a specific container snapshot.
func (c *Client) GetContainerSnapshotConfig(ctx context.Context, node string, vmid int, name string) (map[string]interface{}, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	if name == "" {
		return nil, fmt.Errorf("snapshot name is required")
	}
	var resp ContainerSnapshotConfigResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/snapshot/" + url.PathEscape(name) + "/config"
	if err := c.get(ctx, path, &resp); err != nil {
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

	return c.decodeResponse(resp, result)
}

// post executes a POST request to the Proxmox API with form-encoded body.
// POST requests are never retried to prevent duplicate state changes.
func (c *Client) post(ctx context.Context, path string, body url.Values, result any) error {
	return c.sendMutation(ctx, http.MethodPost, path, body, result)
}

// put executes a PUT request to the Proxmox API with form-encoded body.
// PUT requests are never retried to prevent duplicate state changes.
func (c *Client) put(ctx context.Context, path string, body url.Values, result any) error {
	return c.sendMutation(ctx, http.MethodPut, path, body, result)
}

// del executes a DELETE request to the Proxmox API.
// DELETE requests are never retried to prevent duplicate state changes.
func (c *Client) del(ctx context.Context, path string, result any) error {
	return c.sendMutation(ctx, http.MethodDelete, path, nil, result)
}

// sendMutation builds and executes a mutation request (POST, PUT, DELETE).
// Mutations use DoMutation to prevent automatic retries.
func (c *Client) sendMutation(ctx context.Context, method, path string, body url.Values, result any) error {
	u := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(body.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.token)
	}

	resp, err := c.client.DoMutation(ctx, req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return c.decodeResponse(resp, result)
}

// decodeResponse reads, validates, and decodes a Proxmox API response.
func (c *Client) decodeResponse(resp *http.Response, result any) error {
	if !successCodes[resp.StatusCode] {
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
