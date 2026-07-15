package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
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

	// DefaultMaxUploadSize is the maximum upload file size (100 GiB).
	// This is a safety limit; Proxmox storage backends may have lower limits.
	DefaultMaxUploadSize int64 = 100 * 1024 * 1024 * 1024
)

// Client is a Proxmox API client.
type Client struct {
	endpoint     string
	endpointHost string // hostname only (no port), extracted from endpoint at construction
	baseURL      string
	client       *httpclient.Client
	token        string
	version      *VersionData
}

// New creates a new Proxmox API client.
func New(endpoint string, creds *domain.Credentials, opts ...httpclient.Option) (*Client, error) {
	normalized, err := NormalizeEndpoint(endpoint)
	if err != nil {
		return nil, err
	}

	// Parse the normalized endpoint to extract the hostname for endpoint validation.
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}

	c := httpclient.New(opts...)
	base := strings.TrimRight(normalized, "/") + DefaultAPIPath

	var token string
	if creds.TokenID != "" && creds.TokenSecret != "" {
		token = creds.TokenID + "=" + creds.TokenSecret
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

// ErrEndpointMismatch is returned when a mutating request targets a host
// that does not match the configured Proxmox endpoint.
var ErrEndpointMismatch = fmt.Errorf("request host does not match configured endpoint")

// validateEndpoint checks that the request URL's host matches the configured
// endpoint host, ignoring port. This prevents mutating requests from being
// sent to an unintended host due to redirect, misconfiguration, or SSRF.
//
// When endpointHost is empty (test-only clients constructed without New()),
// the check is skipped. Production clients created via New() always have
// endpointHost populated.
func (c *Client) validateEndpoint(reqURL *url.URL) error {
	if c.endpointHost == "" {
		return nil // No configured endpoint to validate against (test-only client).
	}
	if reqURL == nil {
		return fmt.Errorf("%w: request URL is nil", ErrEndpointMismatch)
	}
	reqHost := reqURL.Hostname()
	if reqHost == "" {
		return fmt.Errorf("%w: request URL has no host", ErrEndpointMismatch)
	}
	cfgHost := c.endpointHost
	if !strings.EqualFold(reqHost, cfgHost) {
		return fmt.Errorf("%w: request host %q does not match configured endpoint host %q",
			ErrEndpointMismatch, reqHost, cfgHost)
	}
	return nil
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
	path := "/nodes/" + url.PathEscape(node) + "/tasks/" + url.PathEscape(upid) + "/status"
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
		if app.HTTPStatusFromError(err) != http.StatusNotImplemented && !strings.Contains(err.Error(), "server error: 501") {
			return nil, err
		}
		var fallback EventListResponse
		if fallbackErr := c.get(ctx, "/cluster/log", &fallback); fallbackErr != nil {
			return nil, err
		}
		return fallback.Data, nil
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

// VMStart starts a VM and returns the task UPID.
func (c *Client) VMStart(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "start", nil)
}

// VMStop performs a force stop of a VM and returns the task UPID.
func (c *Client) VMStop(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "stop", nil)
}

// VMShutdown performs a graceful shutdown of a VM and returns the task UPID.
func (c *Client) VMShutdown(ctx context.Context, node string, vmid int, timeout int) (string, error) {
	body := url.Values{}
	if timeout > 0 {
		body.Set("timeout", strconv.Itoa(timeout))
	}
	return c.vmMutation(ctx, node, vmid, "shutdown", body)
}

// VMReset performs a hard reset of a VM and returns the task UPID.
func (c *Client) VMReset(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "reset", nil)
}

// VMReboot requests a reboot of a VM and returns the task UPID.
func (c *Client) VMReboot(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "reboot", nil)
}

// VMSuspend suspends a VM to disk and returns the task UPID.
func (c *Client) VMSuspend(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "suspend", nil)
}

// VMResume resumes a suspended VM and returns the task UPID.
func (c *Client) VMResume(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "resume", nil)
}

// VMPause freezes a running VM and returns the task UPID.
func (c *Client) VMPause(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "pause", nil)
}

// VMUnpause unfreezes a paused VM and returns the task UPID.
func (c *Client) VMUnpause(ctx context.Context, node string, vmid int) (string, error) {
	return c.vmMutation(ctx, node, vmid, "unpause", nil)
}

// CTStart starts a container and returns the task UPID.
func (c *Client) CTStart(ctx context.Context, node string, vmid int) (string, error) {
	return c.ctMutation(ctx, node, vmid, "start", nil)
}

// CTStop performs a force stop of a container and returns the task UPID.
func (c *Client) CTStop(ctx context.Context, node string, vmid int) (string, error) {
	return c.ctMutation(ctx, node, vmid, "stop", nil)
}

// CTShutdown performs a graceful shutdown of a container and returns the task UPID.
func (c *Client) CTShutdown(ctx context.Context, node string, vmid int, timeout int) (string, error) {
	body := url.Values{}
	if timeout > 0 {
		body.Set("timeout", strconv.Itoa(timeout))
	}
	return c.ctMutation(ctx, node, vmid, "shutdown", body)
}

// CTReboot requests a reboot of a container and returns the task UPID.
func (c *Client) CTReboot(ctx context.Context, node string, vmid int) (string, error) {
	return c.ctMutation(ctx, node, vmid, "reboot", nil)
}

// CTSuspend suspends a container and returns the task UPID.
func (c *Client) CTSuspend(ctx context.Context, node string, vmid int) (string, error) {
	return c.ctMutation(ctx, node, vmid, "suspend", nil)
}

// CTResume resumes a suspended container and returns the task UPID.
func (c *Client) CTResume(ctx context.Context, node string, vmid int) (string, error) {
	return c.ctMutation(ctx, node, vmid, "resume", nil)
}

// vmMutation executes a POST mutation on a QEMU VM status endpoint and returns the UPID.
func (c *Client) vmMutation(ctx context.Context, node string, vmid int, action string, body url.Values) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/status/" + action
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// ctMutation executes a POST mutation on an LXC container status endpoint and returns the UPID.
func (c *Client) ctMutation(ctx context.Context, node string, vmid int, action string, body url.Values) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/status/" + action
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// --- Phase 3: Config, Snapshot, Delete, Template mutations ---

// VMConfigUpdate updates a VM configuration and returns the task UPID.
func (c *Client) VMConfigUpdate(ctx context.Context, node string, vmid int, params url.Values) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/config"
	if err := c.post(ctx, path, params, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTConfigUpdate updates a container configuration and returns the task UPID.
func (c *Client) CTConfigUpdate(ctx context.Context, node string, vmid int, params url.Values) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/config"
	if err := c.put(ctx, path, params, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMDelete deletes a VM and returns the task UPID.
// The purge=1 parameter ensures all HA, firewall, and backup configurations
// referencing this VMID are also removed.
func (c *Client) VMDelete(ctx context.Context, node string, vmid int) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "?purge=1"
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTDelete deletes a container and returns the task UPID.
func (c *Client) CTDelete(ctx context.Context, node string, vmid int) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid)
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMSnapshotCreate creates a VM snapshot and returns the task UPID.
func (c *Client) VMSnapshotCreate(ctx context.Context, node string, vmid int, name, description string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if name == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/snapshot"
	body := url.Values{}
	body.Set("snapname", name)
	if description != "" {
		body.Set("description", description)
	}
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMSnapshotDelete deletes a VM snapshot and returns the task UPID.
func (c *Client) VMSnapshotDelete(ctx context.Context, node string, vmid int, name string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if name == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/snapshot/" + url.PathEscape(name)
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMSnapshotRollback rolls back a VM to a snapshot and returns the task UPID.
func (c *Client) VMSnapshotRollback(ctx context.Context, node string, vmid int, name string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if name == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/snapshot/" + url.PathEscape(name) + "/rollback"
	if err := c.post(ctx, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTSnapshotCreate creates a container snapshot and returns the task UPID.
func (c *Client) CTSnapshotCreate(ctx context.Context, node string, vmid int, name, description string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if name == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/snapshot"
	body := url.Values{}
	body.Set("snapname", name)
	if description != "" {
		body.Set("description", description)
	}
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTSnapshotDelete deletes a container snapshot and returns the task UPID.
func (c *Client) CTSnapshotDelete(ctx context.Context, node string, vmid int, name string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if name == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/snapshot/" + url.PathEscape(name)
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTSnapshotRollback rolls back a container to a snapshot and returns the task UPID.
func (c *Client) CTSnapshotRollback(ctx context.Context, node string, vmid int, name string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if name == "" {
		return "", fmt.Errorf("snapshot name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/snapshot/" + url.PathEscape(name) + "/rollback"
	if err := c.post(ctx, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMCloudInit regenerates the cloud-init configuration for a VM and returns the task UPID.
func (c *Client) VMCloudInit(ctx context.Context, node string, vmid int) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/cloudinit"
	if err := c.put(ctx, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMTemplate converts a VM to a template and returns the task UPID.
func (c *Client) VMTemplate(ctx context.Context, node string, vmid int) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/template"
	if err := c.post(ctx, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTTemplate converts a container to a template and returns the task UPID.
func (c *Client) CTTemplate(ctx context.Context, node string, vmid int) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/template"
	if err := c.post(ctx, path, nil, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// --- Phase 4: Backup, Storage, Migration, Clone, Disk mutations ---

// CreateBackup creates a manual backup task via POST /nodes/{node}/vzdump.
func (c *Client) CreateBackup(ctx context.Context, node string, vmid int, storage, mode string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if storage == "" {
		return "", fmt.Errorf("storage name is required")
	}
	if mode == "" {
		mode = "snapshot"
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/vzdump"
	body := url.Values{}
	body.Set("vmid", strconv.Itoa(vmid))
	body.Set("storage", storage)
	body.Set("mode", mode)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// RestoreVM creates a new VM from a backup archive via POST /nodes/{node}/qemu.
func (c *Client) RestoreVM(ctx context.Context, node string, vmid int, archive, storage string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("new VMID is required")
	}
	if archive == "" {
		return "", fmt.Errorf("archive volume ID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu"
	body := url.Values{}
	body.Set("vmid", strconv.Itoa(vmid))
	body.Set("archive", archive)
	if storage != "" {
		body.Set("storage", storage)
	}
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// GetBackupSchedules returns all backup job schedules from GET /cluster/backup.
func (c *Client) GetBackupSchedules(ctx context.Context) ([]BackupScheduleItem, error) {
	var resp BackupScheduleListResponse
	if err := c.get(ctx, "/cluster/backup", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetBackupSchedule returns a single backup job schedule from GET /cluster/backup/{id}.
func (c *Client) GetBackupSchedule(ctx context.Context, id string) (*BackupScheduleItem, error) {
	if id == "" {
		return nil, fmt.Errorf("backup schedule ID is required")
	}
	var resp BackupScheduleDetailResponse
	path := "/cluster/backup/" + url.PathEscape(id)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CreateBackupSchedule creates a backup job schedule via POST /cluster/backup.
func (c *Client) CreateBackupSchedule(ctx context.Context, schedule BackupScheduleCreateRequest) (string, error) {
	if schedule.Storage == "" {
		return "", fmt.Errorf("storage name is required")
	}
	if schedule.Mode == "" {
		return "", fmt.Errorf("backup mode is required")
	}
	if schedule.Starttime == "" {
		return "", fmt.Errorf("start time is required")
	}
	var resp TaskResponse
	body := url.Values{}
	body.Set("storage", schedule.Storage)
	body.Set("mode", schedule.Mode)
	body.Set("starttime", schedule.Starttime)
	if schedule.Node != "" {
		body.Set("node", schedule.Node)
	}
	if schedule.VMID != "" {
		body.Set("vmid", schedule.VMID)
	}
	if schedule.Dow != "" {
		body.Set("dow", schedule.Dow)
	}
	if schedule.Compress != "" {
		body.Set("compress", schedule.Compress)
	}
	if schedule.Comment != "" {
		body.Set("comment", schedule.Comment)
	}
	if schedule.MailNotification != "" {
		body.Set("mailnotification", schedule.MailNotification)
	}
	if schedule.Mailto != "" {
		body.Set("mailto", schedule.Mailto)
	}
	if schedule.PruneBackups != "" {
		body.Set("prune-backups", schedule.PruneBackups)
	}
	if schedule.Pool != "" {
		body.Set("pool", schedule.Pool)
	}
	if schedule.Tmpdir != "" {
		body.Set("tmpdir", schedule.Tmpdir)
	}
	if schedule.Bwlimit > 0 {
		body.Set("bwlimit", strconv.Itoa(schedule.Bwlimit))
	}
	if schedule.Ionice > 0 {
		body.Set("ionice", strconv.Itoa(schedule.Ionice))
	}
	if schedule.Maxfiles > 0 {
		body.Set("maxfiles", strconv.Itoa(schedule.Maxfiles))
	}
	if schedule.All != 0 {
		body.Set("all", strconv.Itoa(schedule.All))
	}
	if schedule.Enabled != 0 {
		body.Set("enabled", strconv.Itoa(schedule.Enabled))
	}
	if schedule.Quiet != 0 {
		body.Set("quiet", strconv.Itoa(schedule.Quiet))
	}
	if schedule.Remove != 0 {
		body.Set("remove", strconv.Itoa(schedule.Remove))
	}
	if err := c.post(ctx, "/cluster/backup", body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// UpdateBackupSchedule updates a backup job schedule via PUT /cluster/backup/{id}.
func (c *Client) UpdateBackupSchedule(ctx context.Context, id string, schedule BackupScheduleCreateRequest) error {
	if id == "" {
		return fmt.Errorf("backup schedule ID is required")
	}
	path := "/cluster/backup/" + url.PathEscape(id)
	body := url.Values{}
	if schedule.Storage != "" {
		body.Set("storage", schedule.Storage)
	}
	if schedule.Mode != "" {
		body.Set("mode", schedule.Mode)
	}
	if schedule.Starttime != "" {
		body.Set("starttime", schedule.Starttime)
	}
	if schedule.Node != "" {
		body.Set("node", schedule.Node)
	}
	if schedule.VMID != "" {
		body.Set("vmid", schedule.VMID)
	}
	if schedule.Dow != "" {
		body.Set("dow", schedule.Dow)
	}
	if schedule.Compress != "" {
		body.Set("compress", schedule.Compress)
	}
	if schedule.Comment != "" {
		body.Set("comment", schedule.Comment)
	}
	if schedule.MailNotification != "" {
		body.Set("mailnotification", schedule.MailNotification)
	}
	if schedule.Mailto != "" {
		body.Set("mailto", schedule.Mailto)
	}
	if schedule.PruneBackups != "" {
		body.Set("prune-backups", schedule.PruneBackups)
	}
	if schedule.Pool != "" {
		body.Set("pool", schedule.Pool)
	}
	if schedule.Tmpdir != "" {
		body.Set("tmpdir", schedule.Tmpdir)
	}
	if schedule.Bwlimit > 0 {
		body.Set("bwlimit", strconv.Itoa(schedule.Bwlimit))
	}
	if schedule.Ionice > 0 {
		body.Set("ionice", strconv.Itoa(schedule.Ionice))
	}
	if schedule.Maxfiles > 0 {
		body.Set("maxfiles", strconv.Itoa(schedule.Maxfiles))
	}
	if schedule.All != 0 {
		body.Set("all", strconv.Itoa(schedule.All))
	}
	if schedule.Enabled != 0 {
		body.Set("enabled", strconv.Itoa(schedule.Enabled))
	}
	if schedule.Quiet != 0 {
		body.Set("quiet", strconv.Itoa(schedule.Quiet))
	}
	if schedule.Remove != 0 {
		body.Set("remove", strconv.Itoa(schedule.Remove))
	}
	var resp TaskResponse
	if err := c.put(ctx, path, body, &resp); err != nil {
		return err
	}
	return nil
}

// DeleteBackupSchedule deletes a backup job schedule via DELETE /cluster/backup/{id}.
func (c *Client) DeleteBackupSchedule(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("backup schedule ID is required")
	}
	path := "/cluster/backup/" + url.PathEscape(id)
	return c.del(ctx, path, nil)
}

// UploadContent uploads a file to storage via POST /nodes/{node}/storage/{storage}/upload.
// Proxmox rejects chunked uploads, so the multipart body is streamed with a known Content-Length.
// Only regular files are accepted; symlinks, directories, and special files are rejected.
func (c *Client) UploadContent(ctx context.Context, node, storage, localPath string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if storage == "" {
		return "", fmt.Errorf("storage name is required")
	}
	if localPath == "" {
		return "", fmt.Errorf("local file path is required")
	}

	info, err := os.Lstat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat local file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file (symlinks, directories, and special files are not supported)", localPath)
	}
	if info.Size() > DefaultMaxUploadSize {
		return "", fmt.Errorf("%s size %d exceeds maximum upload size %d", localPath, info.Size(), DefaultMaxUploadSize)
	}

	filename := filepath.Base(localPath)
	contentType := inferUploadContentType(filename)
	if contentType == "" {
		return "", fmt.Errorf("unsupported upload content type for %s (supported extensions: .iso, .qcow2, .raw, .vmdk, .tar, .tar.gz, .tgz, .tar.xz, .tar.zst)", filename)
	}
	file, err := os.Open(localPath) // #nosec G304 -- localPath validated by os.Lstat + IsRegular above.
	if err != nil {
		return "", fmt.Errorf("open local file: %w", err)
	}
	defer func() { _ = file.Close() }()

	var prefix bytes.Buffer
	writer := multipart.NewWriter(&prefix)
	if err := writer.WriteField("content", contentType); err != nil {
		return "", fmt.Errorf("write content field: %w", err)
	}
	if _, err := writer.CreateFormFile("filename", filename); err != nil {
		return "", fmt.Errorf("create multipart form: %w", err)
	}
	boundary := writer.Boundary()
	var suffix bytes.Buffer
	fmt.Fprintf(&suffix, "\r\n--%s--\r\n", boundary)
	body := io.MultiReader(&prefix, file, &suffix)
	contentLength := int64(prefix.Len()) + info.Size() + int64(suffix.Len())

	u := c.baseURL + "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, body)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	// CR-001: Block mutating requests targeting a host other than the configured endpoint.
	if err := c.validateEndpoint(req.URL); err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = contentLength
	if c.token != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.token)
	}

	resp, err := c.client.DoMutation(ctx, req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var taskResp TaskResponse
	if err := c.decodeResponse(resp, &taskResp); err != nil {
		return "", err
	}
	return taskResp.Data, nil
}

func inferUploadContentType(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".iso"):
		return "iso"
	case strings.HasSuffix(lower, ".qcow2"), strings.HasSuffix(lower, ".raw"), strings.HasSuffix(lower, ".vmdk"):
		return "import"
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"), strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".tar.zst"):
		return "vztmpl"
	default:
		return ""
	}
}

// DownloadContent returns the raw bytes of a storage volume via GET /nodes/{node}/storage/{storage}/download.
func (c *Client) DownloadContent(ctx context.Context, node, storage, volumeID string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if storage == "" {
		return "", fmt.Errorf("storage name is required")
	}
	if volumeID == "" {
		return "", fmt.Errorf("volume ID is required")
	}
	// Return the download URL so the caller can stream the content directly.
	// Proxmox returns raw content, not JSON, for this endpoint.
	downloadURL := c.baseURL + "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/download/" + url.PathEscape(volumeID)
	return downloadURL, nil
}

// DownloadContentBody downloads storage content and writes the raw body to the provided writer.
func (c *Client) DownloadContentBody(ctx context.Context, node, storage, volumeID string, w io.Writer) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if storage == "" {
		return fmt.Errorf("storage name is required")
	}
	if volumeID == "" {
		return fmt.Errorf("volume ID is required")
	}

	u := c.baseURL + "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/download/" + url.PathEscape(volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, truncated := readLimited(resp.Body, c.client.MaxErrorBodySize())
		msg := redact.String(output.SanitizeTerminal(string(body)))
		if truncated {
			msg += "... [truncated]"
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, msg)
	}

	limited := io.LimitReader(resp.Body, c.client.MaxBodySize()+1)
	n, err := io.Copy(w, limited)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > c.client.MaxBodySize() {
		return fmt.Errorf("download exceeds %d bytes", c.client.MaxBodySize())
	}
	return nil
}

// DeleteContent deletes a storage volume via DELETE /nodes/{node}/storage/{storage}/content/{volume}.
func (c *Client) DeleteContent(ctx context.Context, node, storage, volumeID string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if storage == "" {
		return "", fmt.Errorf("storage name is required")
	}
	if volumeID == "" {
		return "", fmt.Errorf("volume ID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/storage/" + url.PathEscape(storage) + "/content/" + url.PathEscape(volumeID)
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMMigrate migrates a VM to a target node via POST /nodes/{node}/qemu/{vmid}/migrate.
func (c *Client) VMMigrate(ctx context.Context, node string, vmid int, target string, online bool) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if target == "" {
		return "", fmt.Errorf("target node is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/migrate"
	body := url.Values{}
	body.Set("target", target)
	if online {
		body.Set("online", "1")
	}
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTMigrate migrates a container to a target node via POST /nodes/{node}/lxc/{vmid}/migrate.
func (c *Client) CTMigrate(ctx context.Context, node string, vmid int, target string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if target == "" {
		return "", fmt.Errorf("target node is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/migrate"
	body := url.Values{}
	body.Set("target", target)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMClone clones a VM via POST /nodes/{node}/qemu/{vmid}/clone.
func (c *Client) VMClone(ctx context.Context, node string, vmid, newVmid int, name, storage string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("source VMID is required")
	}
	if newVmid <= 0 {
		return "", fmt.Errorf("new VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/clone"
	body := url.Values{}
	body.Set("newid", strconv.Itoa(newVmid))
	if name != "" {
		body.Set("name", name)
	}
	if storage != "" {
		body.Set("storage", storage)
	}
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CTClone clones a container via POST /nodes/{node}/lxc/{vmid}/clone.
func (c *Client) CTClone(ctx context.Context, node string, vmid, newVmid int, hostname, storage string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("source VMID is required")
	}
	if newVmid <= 0 {
		return "", fmt.Errorf("new VMID is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/clone"
	body := url.Values{}
	body.Set("newid", strconv.Itoa(newVmid))
	if hostname != "" {
		body.Set("hostname", hostname)
	}
	if storage != "" {
		body.Set("storage", storage)
	}
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMDiskResize resizes a VM disk via PUT /nodes/{node}/qemu/{vmid}/resize.
func (c *Client) VMDiskResize(ctx context.Context, node string, vmid int, disk, size string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if disk == "" {
		return "", fmt.Errorf("disk identifier is required")
	}
	if size == "" {
		return "", fmt.Errorf("size is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/resize"
	body := url.Values{}
	body.Set("disk", disk)
	body.Set("size", size)
	if err := c.put(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// VMDiskMove moves a VM disk to a different storage via POST /nodes/{node}/qemu/{vmid}/move_disk.
func (c *Client) VMDiskMove(ctx context.Context, node string, vmid int, disk, storage string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return "", fmt.Errorf("VMID is required")
	}
	if disk == "" {
		return "", fmt.Errorf("disk identifier is required")
	}
	if storage == "" {
		return "", fmt.Errorf("target storage is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/move_disk"
	body := url.Values{}
	body.Set("disk", disk)
	body.Set("storage", storage)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// GetPools returns all resource pools.
func (c *Client) GetPools(ctx context.Context) ([]PoolItem, error) {
	var resp PoolsResponse
	if err := c.get(ctx, "/pools", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetClusterLog returns cluster-wide log entries.
func (c *Client) GetClusterLog(ctx context.Context) ([]ClusterLogItem, error) {
	var resp ClusterLogResponse
	if err := c.get(ctx, "/cluster/log", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// --- Phase 5: Network mutations ---

// ApplyNodeNetwork applies network configuration on a node via PUT /nodes/{node}/network.
// The Proxmox API returns {"data": null} on success (no task UPID).
func (c *Client) ApplyNodeNetwork(ctx context.Context, node string, config map[string]interface{}) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/network"
	body := url.Values{}
	for key, val := range config {
		body.Set(key, fmt.Sprintf("%v", val))
	}
	return c.put(ctx, path, body, nil)
}

// RevertNodeNetwork reverts pending network changes on a node via POST /nodes/{node}/network.
// The Proxmox API returns {"data": null} on success (no task UPID).
func (c *Client) RevertNodeNetwork(ctx context.Context, node string) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/network"
	return c.post(ctx, path, nil, nil)
}

// --- Phase 5: Firewall mutations ---

// CreateFirewallRule creates a firewall rule at the cluster level via POST /cluster/firewall/rules.
func (c *Client) CreateFirewallRule(ctx context.Context, rule FirewallRuleCreateRequest) (*FirewallRuleItem, error) {
	if rule.Type == "" {
		return nil, fmt.Errorf("rule type is required (in, out, or group)")
	}
	if rule.Action == "" {
		return nil, fmt.Errorf("rule action is required (accept, deny, reject)")
	}
	var resp FirewallRuleCreateResponse
	body := firewallRuleToValues(rule)
	if err := c.post(ctx, "/cluster/firewall/rules", body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateFirewallRule updates a firewall rule at the cluster level via PUT /cluster/firewall/rules/{pos}.
func (c *Client) UpdateFirewallRule(ctx context.Context, pos int, rule FirewallRuleCreateRequest) error {
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/cluster/firewall/rules/" + strconv.Itoa(pos)
	body := firewallRuleToValues(rule)
	return c.put(ctx, path, body, nil)
}

// DeleteFirewallRule deletes a firewall rule at the cluster level via DELETE /cluster/firewall/rules/{pos}.
func (c *Client) DeleteFirewallRule(ctx context.Context, pos int) error {
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/cluster/firewall/rules/" + strconv.Itoa(pos)
	return c.del(ctx, path, nil)
}

// CreateNodeFirewallRule creates a firewall rule on a node via POST /nodes/{node}/firewall/rules.
func (c *Client) CreateNodeFirewallRule(ctx context.Context, node string, rule FirewallRuleCreateRequest) (*FirewallRuleItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if rule.Type == "" {
		return nil, fmt.Errorf("rule type is required (in, out, or group)")
	}
	if rule.Action == "" {
		return nil, fmt.Errorf("rule action is required (accept, deny, reject)")
	}
	var resp FirewallRuleCreateResponse
	path := "/nodes/" + url.PathEscape(node) + "/firewall/rules"
	body := firewallRuleToValues(rule)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateNodeFirewallRule updates a firewall rule on a node via PUT /nodes/{node}/firewall/rules/{pos}.
func (c *Client) UpdateNodeFirewallRule(ctx context.Context, node string, pos int, rule FirewallRuleCreateRequest) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/firewall/rules/" + strconv.Itoa(pos)
	body := firewallRuleToValues(rule)
	return c.put(ctx, path, body, nil)
}

// DeleteNodeFirewallRule deletes a firewall rule on a node via DELETE /nodes/{node}/firewall/rules/{pos}.
func (c *Client) DeleteNodeFirewallRule(ctx context.Context, node string, pos int) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/firewall/rules/" + strconv.Itoa(pos)
	return c.del(ctx, path, nil)
}

// CreateVMFirewallRule creates a firewall rule on a VM via POST /nodes/{node}/qemu/{vmid}/firewall/rules.
func (c *Client) CreateVMFirewallRule(ctx context.Context, node string, vmid int, rule FirewallRuleCreateRequest) (*FirewallRuleItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	if rule.Type == "" {
		return nil, fmt.Errorf("rule type is required (in, out, or group)")
	}
	if rule.Action == "" {
		return nil, fmt.Errorf("rule action is required (accept, deny, reject)")
	}
	var resp FirewallRuleCreateResponse
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/firewall/rules"
	body := firewallRuleToValues(rule)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateVMFirewallRule updates a firewall rule on a VM via PUT /nodes/{node}/qemu/{vmid}/firewall/rules/{pos}.
func (c *Client) UpdateVMFirewallRule(ctx context.Context, node string, vmid int, pos int, rule FirewallRuleCreateRequest) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return fmt.Errorf("VMID is required")
	}
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/firewall/rules/" + strconv.Itoa(pos)
	body := firewallRuleToValues(rule)
	return c.put(ctx, path, body, nil)
}

// DeleteVMFirewallRule deletes a firewall rule on a VM via DELETE /nodes/{node}/qemu/{vmid}/firewall/rules/{pos}.
func (c *Client) DeleteVMFirewallRule(ctx context.Context, node string, vmid int, pos int) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return fmt.Errorf("VMID is required")
	}
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/qemu/" + strconv.Itoa(vmid) + "/firewall/rules/" + strconv.Itoa(pos)
	return c.del(ctx, path, nil)
}

// CreateCTFirewallRule creates a firewall rule on a container via POST /nodes/{node}/lxc/{vmid}/firewall/rules.
func (c *Client) CreateCTFirewallRule(ctx context.Context, node string, vmid int, rule FirewallRuleCreateRequest) (*FirewallRuleItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return nil, fmt.Errorf("VMID is required")
	}
	if rule.Type == "" {
		return nil, fmt.Errorf("rule type is required (in, out, or group)")
	}
	if rule.Action == "" {
		return nil, fmt.Errorf("rule action is required (accept, deny, reject)")
	}
	var resp FirewallRuleCreateResponse
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/firewall/rules"
	body := firewallRuleToValues(rule)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// UpdateCTFirewallRule updates a firewall rule on a container via PUT /nodes/{node}/lxc/{vmid}/firewall/rules/{pos}.
func (c *Client) UpdateCTFirewallRule(ctx context.Context, node string, vmid int, pos int, rule FirewallRuleCreateRequest) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return fmt.Errorf("VMID is required")
	}
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/firewall/rules/" + strconv.Itoa(pos)
	body := firewallRuleToValues(rule)
	return c.put(ctx, path, body, nil)
}

// DeleteCTFirewallRule deletes a firewall rule on a container via DELETE /nodes/{node}/lxc/{vmid}/firewall/rules/{pos}.
func (c *Client) DeleteCTFirewallRule(ctx context.Context, node string, vmid int, pos int) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	if vmid <= 0 {
		return fmt.Errorf("VMID is required")
	}
	if pos < 0 {
		return fmt.Errorf("rule position is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/lxc/" + strconv.Itoa(vmid) + "/firewall/rules/" + strconv.Itoa(pos)
	return c.del(ctx, path, nil)
}

// CreateFirewallAlias creates a firewall alias via POST /cluster/firewall/aliases.
func (c *Client) CreateFirewallAlias(ctx context.Context, name, cidr, comment string) error {
	if name == "" {
		return fmt.Errorf("alias name is required")
	}
	if cidr == "" {
		return fmt.Errorf("CIDR is required")
	}
	body := url.Values{}
	body.Set("name", name)
	body.Set("cidr", cidr)
	if comment != "" {
		body.Set("comment", comment)
	}
	return c.post(ctx, "/cluster/firewall/aliases", body, nil)
}

// DeleteFirewallAlias deletes a firewall alias via DELETE /cluster/firewall/aliases/{name}.
func (c *Client) DeleteFirewallAlias(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("alias name is required")
	}
	path := "/cluster/firewall/aliases/" + url.PathEscape(name)
	return c.del(ctx, path, nil)
}

// CreateFirewallIPSet creates a firewall IP set via POST /cluster/firewall/ipset.
func (c *Client) CreateFirewallIPSet(ctx context.Context, name, comment string) error {
	if name == "" {
		return fmt.Errorf("IP set name is required")
	}
	body := url.Values{}
	body.Set("name", name)
	if comment != "" {
		body.Set("comment", comment)
	}
	return c.post(ctx, "/cluster/firewall/ipset", body, nil)
}

// AddFirewallIPSetEntry adds an entry to an IP set via POST /cluster/firewall/ipset/{name}.
func (c *Client) AddFirewallIPSetEntry(ctx context.Context, name, cidr, comment string) error {
	if name == "" {
		return fmt.Errorf("IP set name is required")
	}
	if cidr == "" {
		return fmt.Errorf("CIDR is required")
	}
	body := url.Values{}
	body.Set("cidr", cidr)
	if comment != "" {
		body.Set("comment", comment)
	}
	path := "/cluster/firewall/ipset/" + url.PathEscape(name)
	return c.post(ctx, path, body, nil)
}

// RemoveFirewallIPSetEntry removes an entry from an IP set via DELETE /cluster/firewall/ipset/{name}/{cidr}.
func (c *Client) RemoveFirewallIPSetEntry(ctx context.Context, name, cidr string) error {
	if name == "" {
		return fmt.Errorf("IP set name is required")
	}
	if cidr == "" {
		return fmt.Errorf("CIDR is required")
	}
	path := "/cluster/firewall/ipset/" + url.PathEscape(name) + "/" + url.PathEscape(cidr)
	return c.del(ctx, path, nil)
}

// DeleteFirewallIPSet deletes a firewall IP set via DELETE /cluster/firewall/ipset/{name}.
func (c *Client) DeleteFirewallIPSet(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("IP set name is required")
	}
	path := "/cluster/firewall/ipset/" + url.PathEscape(name)
	return c.del(ctx, path, nil)
}

// CreateFirewallGroup creates a firewall security group via POST /cluster/firewall/groups.
func (c *Client) CreateFirewallGroup(ctx context.Context, name, comment string) error {
	if name == "" {
		return fmt.Errorf("group name is required")
	}
	body := url.Values{}
	body.Set("group", name)
	if comment != "" {
		body.Set("comment", comment)
	}
	return c.post(ctx, "/cluster/firewall/groups", body, nil)
}

// DeleteFirewallGroup deletes a firewall security group via DELETE /cluster/firewall/groups/{name}.
func (c *Client) DeleteFirewallGroup(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("group name is required")
	}
	path := "/cluster/firewall/groups/" + url.PathEscape(name)
	return c.del(ctx, path, nil)
}

// UpdateFirewallOptions updates firewall options via PUT /cluster/firewall/options.
func (c *Client) UpdateFirewallOptions(ctx context.Context, opts FirewallOptionsUpdateRequest) error {
	body := url.Values{}
	if opts.Enable != 0 {
		body.Set("enable", strconv.Itoa(opts.Enable))
	}
	if opts.PolicyIn != "" {
		body.Set("policy_in", opts.PolicyIn)
	}
	if opts.PolicyOut != "" {
		body.Set("policy_out", opts.PolicyOut)
	}
	if opts.LogInDrop != 0 {
		body.Set("log_in_drop", strconv.Itoa(opts.LogInDrop))
	}
	if opts.LogRateLimit != "" {
		body.Set("log_ratelimit", opts.LogRateLimit)
	}
	if opts.NFConntrack != 0 {
		body.Set("nf_conntrack_max", strconv.Itoa(opts.NFConntrack))
	}
	if opts.Digest != "" {
		body.Set("digest", opts.Digest)
	}
	return c.put(ctx, "/cluster/firewall/options", body, nil)
}

// --- Phase 5: Identity ---

// GetUsers returns all users from GET /access/users.
func (c *Client) GetUsers(ctx context.Context) ([]AccessUserItem, error) {
	var resp AccessUsersResponse
	if err := c.get(ctx, "/access/users", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetGroups returns all groups from GET /access/groups.
func (c *Client) GetGroups(ctx context.Context) ([]AccessGroupItem, error) {
	var resp AccessGroupsResponse
	if err := c.get(ctx, "/access/groups", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetRoles returns all roles from GET /access/roles.
func (c *Client) GetRoles(ctx context.Context) ([]AccessRoleItem, error) {
	var resp AccessRolesResponse
	if err := c.get(ctx, "/access/roles", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetACL returns ACL entries from GET /access/acl.
func (c *Client) GetACL(ctx context.Context) ([]AccessACLItem, error) {
	var resp AccessACLResponse
	if err := c.get(ctx, "/access/acl", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetDomains returns authentication domains from GET /access/domains.
func (c *Client) GetDomains(ctx context.Context) ([]AccessDomainItem, error) {
	var resp AccessDomainsResponse
	if err := c.get(ctx, "/access/domains", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetTokens returns API tokens for a user from GET /access/users/{user}/token.
func (c *Client) GetTokens(ctx context.Context, user string) ([]AccessTokenItem, error) {
	if user == "" {
		return nil, fmt.Errorf("user ID is required")
	}
	var resp AccessTokensResponse
	path := "/access/users/" + url.PathEscape(user) + "/token"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// CreateUser creates a new user via POST /access/users.
func (c *Client) CreateUser(ctx context.Context, userid, password, email, firstname, lastname, comment string) error {
	if userid == "" {
		return fmt.Errorf("user ID is required")
	}
	body := url.Values{}
	body.Set("userid", userid)
	if password != "" {
		body.Set("password", password)
	}
	if email != "" {
		body.Set("email", email)
	}
	if firstname != "" {
		body.Set("firstname", firstname)
	}
	if lastname != "" {
		body.Set("lastname", lastname)
	}
	if comment != "" {
		body.Set("comment", comment)
	}
	return c.post(ctx, "/access/users", body, nil)
}

// DeleteUser deletes a user via DELETE /access/users/{userid}.
func (c *Client) DeleteUser(ctx context.Context, userid string) error {
	if userid == "" {
		return fmt.Errorf("user ID is required")
	}
	path := "/access/users/" + url.PathEscape(userid)
	return c.del(ctx, path, nil)
}

// AddACL adds an ACL entry via PUT /access/acl.
func (c *Client) AddACL(ctx context.Context, path, role, user, group string, propagate int) error {
	if path == "" {
		return fmt.Errorf("ACL path is required")
	}
	if role == "" {
		return fmt.Errorf("role ID is required")
	}
	body := url.Values{}
	body.Set("path", path)
	body.Set("roles", role)
	if user != "" {
		body.Set("users", user)
	}
	if group != "" {
		body.Set("groups", group)
	}
	if propagate != 0 {
		body.Set("propagate", strconv.Itoa(propagate))
	}
	return c.put(ctx, "/access/acl", body, nil)
}

// --- Phase 5: Firewall helper ---

// FirewallRuleCreateResponse is the response from POST firewall rule creation.
type FirewallRuleCreateResponse struct {
	Data FirewallRuleItem `json:"data"`
}

// firewallRuleToValues converts a FirewallRuleCreateRequest to url.Values.
func firewallRuleToValues(rule FirewallRuleCreateRequest) url.Values {
	v := url.Values{}
	v.Set("type", rule.Type)
	v.Set("action", rule.Action)
	if rule.Enable != 0 {
		v.Set("enable", strconv.Itoa(rule.Enable))
	}
	if rule.Pos > 0 {
		v.Set("pos", strconv.Itoa(rule.Pos))
	}
	if rule.Proto != "" {
		v.Set("proto", rule.Proto)
	}
	if rule.Dest != "" {
		v.Set("dest", rule.Dest)
	}
	if rule.Dport != "" {
		v.Set("dport", rule.Dport)
	}
	if rule.Source != "" {
		v.Set("source", rule.Source)
	}
	if rule.Sport != "" {
		v.Set("sport", rule.Sport)
	}
	if rule.ICMPType != "" {
		v.Set("icmp_type", rule.ICMPType)
	}
	if rule.Log != "" {
		v.Set("log", rule.Log)
	}
	if rule.Comment != "" {
		v.Set("comment", rule.Comment)
	}
	if rule.IFace != "" {
		v.Set("iface", rule.IFace)
	}
	if rule.Macro != "" {
		v.Set("macro", rule.Macro)
	}
	return v
}

// --- Phase 6: Ceph, SDN, Replication ---

// GetCephStatus returns Ceph cluster health status.
func (c *Client) GetCephStatus(ctx context.Context, node string) (map[string]interface{}, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp CephStatusResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/status"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetCephOSDs returns the Ceph OSD tree.
func (c *Client) GetCephOSDs(ctx context.Context, node string) (*CephOSDListResponse, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp CephOSDListResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/osd"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetCephMONs returns the Ceph monitor list.
func (c *Client) GetCephMONs(ctx context.Context, node string) ([]CephMONItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp CephMONListResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/mon"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetCephPools returns the Ceph pool list.
func (c *Client) GetCephPools(ctx context.Context, node string) ([]CephPoolItem, error) {
	if node == "" {
		return nil, fmt.Errorf("node name is required")
	}
	var resp CephPoolListResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/pool"
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// CreateOSD creates a new Ceph OSD and returns the task UPID.
func (c *Client) CreateOSD(ctx context.Context, node, dev string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if dev == "" {
		return "", fmt.Errorf("device name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/osd"
	body := url.Values{}
	body.Set("dev", dev)
	if err := c.post(ctx, path, body, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// OSDOut marks an OSD as out.
func (c *Client) OSDOut(ctx context.Context, node string, osdid int) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/ceph/osd/" + strconv.Itoa(osdid) + "/out"
	return c.post(ctx, path, nil, nil)
}

// OSDIn marks an OSD as in.
func (c *Client) OSDIn(ctx context.Context, node string, osdid int) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/ceph/osd/" + strconv.Itoa(osdid) + "/in"
	return c.post(ctx, path, nil, nil)
}

// DestroyOSD destroys a Ceph OSD and returns the task UPID.
func (c *Client) DestroyOSD(ctx context.Context, node string, osdid int) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/osd/" + strconv.Itoa(osdid)
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// CreatePool creates a new Ceph pool and returns the task UPID.
func (c *Client) CreatePool(ctx context.Context, node string, params url.Values) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/pool"
	if err := c.post(ctx, path, params, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// DestroyPool destroys a Ceph pool and returns the task UPID.
func (c *Client) DestroyPool(ctx context.Context, node, name string) (string, error) {
	if node == "" {
		return "", fmt.Errorf("node name is required")
	}
	if name == "" {
		return "", fmt.Errorf("pool name is required")
	}
	var resp TaskResponse
	path := "/nodes/" + url.PathEscape(node) + "/ceph/pool/" + url.PathEscape(name)
	if err := c.del(ctx, path, &resp); err != nil {
		return "", err
	}
	return resp.Data, nil
}

// --- SDN mutations ---

// CreateSDNZone creates an SDN zone.
func (c *Client) CreateSDNZone(ctx context.Context, zoneType, zone string) error {
	body := url.Values{}
	body.Set("type", zoneType)
	body.Set("zone", zone)
	return c.post(ctx, "/cluster/sdn/zones", body, nil)
}

// DeleteSDNZone deletes an SDN zone.
func (c *Client) DeleteSDNZone(ctx context.Context, zone string) error {
	path := "/cluster/sdn/zones/" + url.PathEscape(zone)
	return c.del(ctx, path, nil)
}

// CreateSDNVNet creates an SDN virtual network.
func (c *Client) CreateSDNVNet(ctx context.Context, vnet, zone string) error {
	body := url.Values{}
	body.Set("vnet", vnet)
	body.Set("zone", zone)
	return c.post(ctx, "/cluster/sdn/vnets", body, nil)
}

// DeleteSDNVNet deletes an SDN virtual network.
func (c *Client) DeleteSDNVNet(ctx context.Context, vnet string) error {
	path := "/cluster/sdn/vnets/" + url.PathEscape(vnet)
	return c.del(ctx, path, nil)
}

// CreateSDNSubnet creates an SDN subnet.
func (c *Client) CreateSDNSubnet(ctx context.Context, vnet, cidr, gateway string) error {
	body := url.Values{}
	body.Set("subnet", cidr)
	body.Set("type", "subnet")
	if gateway != "" {
		body.Set("gateway", gateway)
	}
	path := "/cluster/sdn/vnets/" + url.PathEscape(vnet) + "/subnets"
	return c.post(ctx, path, body, nil)
}

// DeleteSDNSubnet deletes an SDN subnet.
func (c *Client) DeleteSDNSubnet(ctx context.Context, vnet, subnet string) error {
	path := "/cluster/sdn/vnets/" + url.PathEscape(vnet) + "/subnets/" + url.PathEscape(subnet)
	return c.del(ctx, path, nil)
}

// CreateSDNController creates an SDN controller.
func (c *Client) CreateSDNController(ctx context.Context, ctrl string) error {
	body := url.Values{}
	body.Set("controller", ctrl)
	return c.post(ctx, "/cluster/sdn/controllers", body, nil)
}

// DeleteSDNController deletes an SDN controller.
func (c *Client) DeleteSDNController(ctx context.Context, ctrl string) error {
	path := "/cluster/sdn/controllers/" + url.PathEscape(ctrl)
	return c.del(ctx, path, nil)
}

// --- Replication ---

// GetReplicationJobs returns all replication jobs.
func (c *Client) GetReplicationJobs(ctx context.Context) ([]ReplicationJobItem, error) {
	var resp ReplicationListResponse
	if err := c.get(ctx, "/cluster/replication", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetReplicationJob returns a single replication job.
func (c *Client) GetReplicationJob(ctx context.Context, id string) (*ReplicationJobItem, error) {
	var resp ReplicationGetResponse
	path := "/cluster/replication/" + url.PathEscape(id)
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp.Data, nil
}

// CreateReplication creates a new replication job.
func (c *Client) CreateReplication(ctx context.Context, req ReplicationCreateRequest) error {
	body := url.Values{}
	body.Set("id", req.ID)
	body.Set("guest", strconv.Itoa(req.Guest))
	body.Set("type", req.Type)
	body.Set("target", req.Target)
	if req.Schedule != "" {
		body.Set("schedule", req.Schedule)
	}
	if req.Comment != "" {
		body.Set("comment", req.Comment)
	}
	if req.Rate > 0 {
		body.Set("rate", strconv.FormatInt(req.Rate, 10))
	}
	if req.Source != "" {
		body.Set("source", req.Source)
	}
	return c.post(ctx, "/cluster/replication", body, nil)
}

// UpdateReplication updates a replication job.
func (c *Client) UpdateReplication(ctx context.Context, id string, req ReplicationUpdateRequest) error {
	body := url.Values{}
	if req.Target != "" {
		body.Set("target", req.Target)
	}
	if req.Schedule != "" {
		body.Set("schedule", req.Schedule)
	}
	if req.Comment != "" {
		body.Set("comment", req.Comment)
	}
	if req.Rate > 0 {
		body.Set("rate", strconv.FormatInt(req.Rate, 10))
	}
	if req.Source != "" {
		body.Set("source", req.Source)
	}
	if req.Delete != "" {
		body.Set("delete", req.Delete)
	}
	path := "/cluster/replication/" + url.PathEscape(id)
	return c.put(ctx, path, body, nil)
}

// DeleteReplication deletes a replication job.
func (c *Client) DeleteReplication(ctx context.Context, id string) error {
	path := "/cluster/replication/" + url.PathEscape(id)
	return c.del(ctx, path, nil)
}

// ScheduleReplication schedules a replication job to run now.
func (c *Client) ScheduleReplication(ctx context.Context, node, id string) error {
	if node == "" {
		return fmt.Errorf("node name is required")
	}
	path := "/nodes/" + url.PathEscape(node) + "/replication/" + url.PathEscape(id) + "/schedule_now"
	return c.post(ctx, path, nil, nil)
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
		return &app.ProviderError{StatusCode: 0, Detail: fmt.Sprintf("execute request: %v", err), Err: err}
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
	// CR-001: Block mutating requests targeting a host other than the configured endpoint.
	if err := c.validateEndpoint(req.URL); err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "PVEAPIToken="+c.token)
	}

	resp, err := c.client.DoMutation(ctx, req)
	if err != nil {
		return &app.ProviderError{StatusCode: 0, Detail: fmt.Sprintf("execute request: %v", err), Err: err}
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
		return newProviderError(resp.StatusCode, fmt.Sprintf("API error %d: %s", resp.StatusCode, msg))
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

// newProviderError creates an app.ProviderError from an HTTP status code and detail.
func newProviderError(statusCode int, detail string) *app.ProviderError {
	return &app.ProviderError{
		StatusCode: statusCode,
		Detail:     detail,
	}
}

func readLimited(r io.Reader, limit int64) ([]byte, bool) {
	body, _ := io.ReadAll(io.LimitReader(r, limit+1))
	if int64(len(body)) > limit {
		return body[:limit], true
	}
	return body, false
}
