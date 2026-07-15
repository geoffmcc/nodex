package client

import (
	"encoding/json"
	"strconv"
)

// APIResponse wraps a Proxmox API response.
type APIResponse struct {
	Data json.RawMessage `json:"data"`
}

// TaskResponse is the response from mutation endpoints that return a UPID.
// Proxmox returns {"data": "UPID:pve1:..."} for POST/PUT/DELETE operations.
type TaskResponse struct {
	Data string `json:"data"`
}

// NodeListResponse is the response from /nodes.
type NodeListResponse struct {
	Data []NodeItem `json:"data"`
}

// NodeItem represents a single node from the API.
type NodeItem struct {
	Status  string  `json:"status"`
	Maxmem  int64   `json:"maxmem"`
	CPU     float64 `json:"cpu"`
	Maxcpu  int     `json:"maxcpu"`
	Uptime  *int    `json:"uptime,omitempty"`
	Node    string  `json:"node"`
	Name    string  `json:"name"`
	ID      string  `json:"id"`
	Level   string  `json:"level"`
	Mem     int64   `json:"mem"`
	Disk    int64   `json:"disk"`
	Maxdisk int64   `json:"maxdisk"`
	Type    string  `json:"type"`
	IP      string  `json:"ip"`
}

// ClusterResourcesResponse is the response from /cluster/resources.
type ClusterResourcesResponse struct {
	Data []ClusterResource `json:"data"`
}

// ClusterResource represents a single resource from the cluster.
type ClusterResource struct {
	ID        string  `json:"id"`
	Type      string  `json:"type"`
	Status    string  `json:"status"`
	Name      string  `json:"name"`
	Node      string  `json:"node"`
	CPU       float64 `json:"cpu,omitempty"`
	MaxCPU    int     `json:"maxcpu,omitempty"`
	Mem       int64   `json:"mem,omitempty"`
	MaxMem    int64   `json:"maxmem,omitempty"`
	Disk      int64   `json:"disk,omitempty"`
	MaxDisk   int64   `json:"maxdisk,omitempty"`
	IP        string  `json:"ip,omitempty"`
	Template  int     `json:"template,omitempty"`
	VMID      int     `json:"vmid,omitempty"`
	Storage   string  `json:"storage,omitempty"`
	Content   string  `json:"content,omitempty"`
	MaxAge    int     `json:"maxage,omitempty"`
	Shared    int     `json:"shared,omitempty"`
	Heartbeat int     `json:"heartbeat,omitempty"`
	Tags      string  `json:"tags,omitempty"`
	StartTime int     `json:"uptime,omitempty"`
}

// VersionResponse is the response from /version.
type VersionResponse struct {
	Data VersionData `json:"data"`
}

// VersionData holds version information.
type VersionData struct {
	Release string `json:"release"`
	Version string `json:"version"`
	Repoid  string `json:"repoid"`
}

// NodeList is a convenience alias.
type NodeList = NodeListResponse

// ClusterResources is a convenience alias.
type ClusterResources = ClusterResourcesResponse

// NodeStatusResponse is the response from /nodes/{node}/status.
type NodeStatusResponse struct {
	Data NodeStatusData `json:"data"`
}

// NodeStatusData holds detailed node status information.
type NodeStatusData struct {
	CPU            float64   `json:"cpu"`
	MaxCPU         int       `json:"maxcpu"`
	Mem            int64     `json:"mem"`
	MaxMem         int64     `json:"maxmem"`
	Disk           int64     `json:"disk"`
	MaxDisk        int64     `json:"maxdisk"`
	Uptime         int       `json:"uptime"`
	Level          string    `json:"level"`
	SSLFingerprint string    `json:"ssl_fingerprint,omitempty"`
	ID             string    `json:"id"`
	Node           string    `json:"node"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	KVersion       string    `json:"kversion,omitempty"`
	PVEVersion     string    `json:"pveversion,omitempty"`
	LoadAvg        []float64 `json:"loadavg,omitempty"`
	Wait           float64   `json:"wait,omitempty"`
	Ksm            int       `json:"ksm,omitempty"`
	Numa           int       `json:"numa,omitempty"`
	IOMax          float64   `json:"io,omitempty"`
}

func (d *NodeStatusData) UnmarshalJSON(data []byte) error {
	type rawNodeStatusData struct {
		CPU            float64         `json:"cpu"`
		MaxCPU         json.RawMessage `json:"maxcpu,omitempty"`
		Mem            json.RawMessage `json:"mem,omitempty"`
		MaxMem         json.RawMessage `json:"maxmem,omitempty"`
		Disk           json.RawMessage `json:"disk,omitempty"`
		MaxDisk        json.RawMessage `json:"maxdisk,omitempty"`
		Uptime         int             `json:"uptime"`
		Level          string          `json:"level,omitempty"`
		SSLFingerprint string          `json:"ssl_fingerprint,omitempty"`
		ID             string          `json:"id,omitempty"`
		Node           string          `json:"node,omitempty"`
		Type           string          `json:"type,omitempty"`
		Status         string          `json:"status,omitempty"`
		KVersion       string          `json:"kversion,omitempty"`
		PVEVersion     string          `json:"pveversion,omitempty"`
		LoadAvg        json.RawMessage `json:"loadavg,omitempty"`
		Wait           float64         `json:"wait,omitempty"`
		Ksm            json.RawMessage `json:"ksm,omitempty"`
		Numa           int             `json:"numa,omitempty"`
		IOMax          float64         `json:"io,omitempty"`
		Memory         json.RawMessage `json:"memory,omitempty"`
		RootFS         json.RawMessage `json:"rootfs,omitempty"`
		CPUInfo        json.RawMessage `json:"cpuinfo,omitempty"`
		Idle           float64         `json:"idle,omitempty"`
	}
	var raw rawNodeStatusData
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	maxCPU := decodeInt(raw.MaxCPU)
	if maxCPU == 0 {
		type cpuInfo struct {
			Cpus int `json:"cpus"`
		}
		var ci cpuInfo
		if json.Unmarshal(raw.CPUInfo, &ci) == nil && ci.Cpus > 0 {
			maxCPU = ci.Cpus
		}
	}

	mem := decodeInt64(raw.Mem)
	maxMem := decodeInt64(raw.MaxMem)
	if mem == 0 && maxMem == 0 {
		type memInfo struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		}
		var mi memInfo
		if json.Unmarshal(raw.Memory, &mi) == nil {
			mem = mi.Used
			maxMem = mi.Total
		}
	}

	disk := decodeInt64(raw.Disk)
	maxDisk := decodeInt64(raw.MaxDisk)
	if disk == 0 && maxDisk == 0 {
		type diskInfo struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		}
		var di diskInfo
		if json.Unmarshal(raw.RootFS, &di) == nil {
			disk = di.Used
			maxDisk = di.Total
		}
	}

	*d = NodeStatusData{
		CPU:            raw.CPU,
		MaxCPU:         maxCPU,
		Mem:            mem,
		MaxMem:         maxMem,
		Disk:           disk,
		MaxDisk:        maxDisk,
		Uptime:         raw.Uptime,
		Level:          raw.Level,
		SSLFingerprint: raw.SSLFingerprint,
		ID:             raw.ID,
		Node:           raw.Node,
		Type:           raw.Type,
		Status:         raw.Status,
		KVersion:       raw.KVersion,
		PVEVersion:     raw.PVEVersion,
		LoadAvg:        decodeFloatSlice(raw.LoadAvg),
		Wait:           raw.Wait,
		Ksm:            decodeInt(raw.Ksm),
		Numa:           raw.Numa,
		IOMax:          raw.IOMax,
	}
	return nil
}

// NodeStatus is a convenience alias.
type NodeStatus = NodeStatusResponse

// Version is a convenience alias.
type Version = VersionResponse

// ClusterStatusResponse is the response from /cluster/status.
type ClusterStatusResponse struct {
	Data []ClusterStatusItem `json:"data"`
}

// ClusterStatusItem represents a single item from the cluster status API.
type ClusterStatusItem struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Level     string `json:"level,omitempty"`
	IP        string `json:"ip,omitempty"`
	Localmem  int64  `json:"localmem,omitempty"`
	Maxmem    int64  `json:"maxmem,omitempty"`
	Localdisk int64  `json:"localdisk,omitempty"`
	Maxdisk   int64  `json:"maxdisk,omitempty"`
	Quorate   int    `json:"quorate,omitempty"`
	Version   int    `json:"version,omitempty"`
	Commit    string `json:"commit,omitempty"`
}

func (i *ClusterStatusItem) UnmarshalJSON(data []byte) error {
	type rawClusterStatusItem struct {
		Type      string          `json:"type"`
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Status    string          `json:"status"`
		Level     string          `json:"level,omitempty"`
		IP        string          `json:"ip,omitempty"`
		Localmem  int64           `json:"localmem,omitempty"`
		Maxmem    int64           `json:"maxmem,omitempty"`
		Localdisk int64           `json:"localdisk,omitempty"`
		Maxdisk   int64           `json:"maxdisk,omitempty"`
		Quorate   json.RawMessage `json:"quorate,omitempty"`
		Version   json.RawMessage `json:"version,omitempty"`
		Commit    string          `json:"commit,omitempty"`
	}
	var raw rawClusterStatusItem
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*i = ClusterStatusItem{
		Type:      raw.Type,
		ID:        raw.ID,
		Name:      raw.Name,
		Status:    raw.Status,
		Level:     raw.Level,
		IP:        raw.IP,
		Localmem:  raw.Localmem,
		Maxmem:    raw.Maxmem,
		Localdisk: raw.Localdisk,
		Maxdisk:   raw.Maxdisk,
		Quorate:   decodeInt(raw.Quorate),
		Version:   decodeInt(raw.Version),
		Commit:    raw.Commit,
	}
	return nil
}

// ClusterStatus is a convenience alias.
type ClusterStatus = ClusterStatusResponse

// VMConfigResponse is the response from /nodes/{node}/qemu/{vmid}/config.
type VMConfigResponse struct {
	Data VMConfigData `json:"data"`
}

// VMConfigData holds VM configuration information.
type VMConfigData struct {
	VMID        int               `json:"vmid"`
	Name        string            `json:"name,omitempty"`
	CPU         int               `json:"cores,omitempty"`
	Memory      int               `json:"memory,omitempty"`
	Net0        string            `json:"net0,omitempty"`
	Scsi0       string            `json:"scsi0,omitempty"`
	Boot        string            `json:"boot,omitempty"`
	OnBoot      int               `json:"onboot,omitempty"`
	Agent       int               `json:"agent,omitempty"`
	SMBIOS1     string            `json:"smbios1,omitempty"`
	Numa        int               `json:"numa,omitempty"`
	OSType      string            `json:"ostype,omitempty"`
	Description string            `json:"description,omitempty"`
	Protection  int               `json:"protection,omitempty"`
	Tags        string            `json:"tags,omitempty"`
	VMGenID     string            `json:"vmgenid,omitempty"`
	Args        string            `json:"args,omitempty"`
	Bios        string            `json:"bios,omitempty"`
	IDE2        string            `json:"ide2,omitempty"`
	ScsiHW      string            `json:"scsihw,omitempty"`
	Unused0     string            `json:"unused0,omitempty"`
	Raw         map[string]string `json:"raw,omitempty"`
}

func (d *VMConfigData) UnmarshalJSON(data []byte) error {
	type rawVMConfigData struct {
		VMID        json.RawMessage   `json:"vmid"`
		Name        string            `json:"name,omitempty"`
		CPU         json.RawMessage   `json:"cores,omitempty"`
		Memory      json.RawMessage   `json:"memory,omitempty"`
		Net0        string            `json:"net0,omitempty"`
		Scsi0       string            `json:"scsi0,omitempty"`
		Boot        string            `json:"boot,omitempty"`
		OnBoot      json.RawMessage   `json:"onboot,omitempty"`
		Agent       json.RawMessage   `json:"agent,omitempty"`
		SMBIOS1     string            `json:"smbios1,omitempty"`
		Numa        json.RawMessage   `json:"numa,omitempty"`
		OSType      string            `json:"ostype,omitempty"`
		Description string            `json:"description,omitempty"`
		Protection  json.RawMessage   `json:"protection,omitempty"`
		Tags        string            `json:"tags,omitempty"`
		VMGenID     string            `json:"vmgenid,omitempty"`
		Args        string            `json:"args,omitempty"`
		Bios        string            `json:"bios,omitempty"`
		IDE2        string            `json:"ide2,omitempty"`
		ScsiHW      string            `json:"scsihw,omitempty"`
		Unused0     string            `json:"unused0,omitempty"`
		Raw         map[string]string `json:"raw,omitempty"`
	}
	var raw rawVMConfigData
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*d = VMConfigData{
		VMID:        decodeInt(raw.VMID),
		Name:        raw.Name,
		CPU:         decodeInt(raw.CPU),
		Memory:      decodeInt(raw.Memory),
		Net0:        raw.Net0,
		Scsi0:       raw.Scsi0,
		Boot:        raw.Boot,
		OnBoot:      decodeInt(raw.OnBoot),
		Agent:       decodeInt(raw.Agent),
		SMBIOS1:     raw.SMBIOS1,
		Numa:        decodeInt(raw.Numa),
		OSType:      raw.OSType,
		Description: raw.Description,
		Protection:  decodeInt(raw.Protection),
		Tags:        raw.Tags,
		VMGenID:     raw.VMGenID,
		Args:        raw.Args,
		Bios:        raw.Bios,
		IDE2:        raw.IDE2,
		ScsiHW:      raw.ScsiHW,
		Unused0:     raw.Unused0,
		Raw:         raw.Raw,
	}
	return nil
}

// VMConfig is a convenience alias.
type VMConfig = VMConfigResponse

// ContainerConfigResponse is the response from /nodes/{node}/lxc/{vmid}/config.
type ContainerConfigResponse struct {
	Data ContainerConfigData `json:"data"`
}

// ContainerConfigData holds container configuration information.
type ContainerConfigData struct {
	VMID         int               `json:"vmid"`
	Hostname     string            `json:"hostname,omitempty"`
	CPU          int               `json:"cores,omitempty"`
	Memory       int               `json:"memory,omitempty"`
	Swap         int               `json:"swap,omitempty"`
	Rootfs       string            `json:"rootfs,omitempty"`
	MP0          string            `json:"mp0,omitempty"`
	Net0         string            `json:"net0,omitempty"`
	OnBoot       int               `json:"onboot,omitempty"`
	OSType       string            `json:"ostype,omitempty"`
	Description  string            `json:"description,omitempty"`
	Protection   int               `json:"protection,omitempty"`
	Tags         string            `json:"tags,omitempty"`
	Unfiltered   int               `json:"unfiltered,omitempty"`
	Features     string            `json:"features,omitempty"`
	Architecture string            `json:"architecture,omitempty"`
	Nameserver   string            `json:"nameserver,omitempty"`
	SearchDomain string            `json:"searchdomain,omitempty"`
	Dev0         string            `json:"dev0,omitempty"`
	Fstab        string            `json:"fstab,omitempty"`
	Hookscript   string            `json:"hookscript,omitempty"`
	Raw          map[string]string `json:"raw,omitempty"`
}

// ContainerConfig is a convenience alias.
type ContainerConfig = ContainerConfigResponse

// StorageContentResponse is the response from /nodes/{node}/storage/{storage}/content.
type StorageContentResponse struct {
	Data []StorageContentItem `json:"data"`
}

// StorageContentItem represents a single content item in storage.
type StorageContentItem struct {
	Content string `json:"content"`
	Ctime   int    `json:"ctime,omitempty"`
	Format  string `json:"format,omitempty"`
	Volid   string `json:"volid,omitempty"`
	Size    int64  `json:"size,omitempty"`
	Subtype string `json:"subtype,omitempty"`
	VMID    int    `json:"vmid,omitempty"`
	Store   string `json:"store,omitempty"`
	Checked int    `json:"checked,omitempty"`
	Encrypt string `json:"encrypt,omitempty"`
	Source  string `json:"source,omitempty"`
}

// StorageContent is a convenience alias.
type StorageContent = StorageContentResponse

// TaskListResponse is the response from /nodes/{node}/tasks.
type TaskListResponse struct {
	Data []TaskListItem `json:"data"`
}

// TaskListItem represents a single task in the task list.
type TaskListItem struct {
	UPID       string `json:"upid"`
	Type       string `json:"type"`
	State      string `json:"state"` // running, stopped
	StartTime  int    `json:"starttime"`
	EndTime    int    `json:"endtime,omitempty"`
	Status     string `json:"status,omitempty"`     // OK on completion
	ExitStatus string `json:"exitstatus,omitempty"` // OK on task-status responses
	PID        int    `json:"pid,omitempty"`
	Worker     string `json:"worker,omitempty"`
}

// TaskDetailResponse is the response from /nodes/{node}/tasks/{upid}.
type TaskDetailResponse struct {
	Data TaskListItem `json:"data"`
}

// SnapshotListResponse is the response from /nodes/{node}/qemu/{vmid}/snapshot.
type SnapshotListResponse struct {
	Data []SnapshotListItem `json:"data"`
}

// SnapshotListItem represents a single snapshot.
type SnapshotListItem struct {
	Name   string `json:"name"`
	VMID   int    `json:"vmid,omitempty"`
	Ctime  int    `json:"ctime,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// EventListResponse is the response from /cluster/events.
type EventListResponse struct {
	Data []EventItem `json:"data"`
}

// EventItem represents a single cluster event.
type EventItem struct {
	Type    string `json:"type"`
	Time    int64  `json:"time"`
	Node    string `json:"node,omitempty"`
	ID      string `json:"id,omitempty"`
	Message string `json:"message,omitempty"`
	Msg     string `json:"msg,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

// SyslogResponse is the response from /nodes/{node}/syslog.
type SyslogResponse struct {
	Data []SyslogItem `json:"data"`
}

// SyslogItem represents a single syslog entry.
// Proxmox 9 /nodes/{node}/syslog returns entries with n (line number) and t (log text).
type SyslogItem struct {
	N int64  `json:"n"`
	T string `json:"t"`
}

// BackupStatusResponse is the response from /nodes/{node}/storage/{storage}/content for backup tasks.
type BackupStatusResponse struct {
	Data []BackupStatusItem `json:"data"`
}

// BackupStatusItem represents a backup task.
type BackupStatusItem struct {
	UPID      string `json:"upid"`
	Type      string `json:"type"`
	State     string `json:"state"`
	StartTime int    `json:"starttime"`
	EndTime   int    `json:"endtime,omitempty"`
	Status    string `json:"status,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Worker    string `json:"worker,omitempty"`
	Node      string `json:"node,omitempty"`
	Storage   string `json:"storage,omitempty"`
}

// FirewallRuleResponse is the response from /cluster/firewall/rules.
type FirewallRuleResponse struct {
	Data []FirewallRuleItem `json:"data"`
}

// FirewallRuleItem represents a single firewall rule.
type FirewallRuleItem struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	Enable   int    `json:"enable,omitempty"`
	Pos      int    `json:"pos,omitempty"`
	Proto    string `json:"proto,omitempty"`
	Dest     string `json:"dest,omitempty"`
	Dport    string `json:"dport,omitempty"`
	Source   string `json:"source,omitempty"`
	Sport    string `json:"sport,omitempty"`
	ICMPType string `json:"icmp_type,omitempty"`
	Log      string `json:"log,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

// HAResourceResponse is the response from /cluster/ha/resources.
type HAResourceResponse struct {
	Data []HAResourceItem `json:"data"`
}

// HAResourceItem represents a single HA resource.
type HAResourceItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	State    string `json:"state"`
	Node     string `json:"node,omitempty"`
	Group    string `json:"group,omitempty"`
	MaxRelay int    `json:"max_relocate,omitempty"`
}

// HAGroupResponse is the response from /cluster/ha/groups.
type HAGroupResponse struct {
	Data []HAGroupItem `json:"data"`
}

// HARuleResponse is the response from /cluster/ha/rules.
type HARuleResponse struct {
	Data []HARuleItem `json:"data"`
}

// HAGroupItem represents a single HA group.
type HAGroupItem struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Nodes      string `json:"nodes"`
	Comment    string `json:"comment,omitempty"`
	NoFailback int    `json:"nofailback,omitempty"`
}

// HARuleItem represents a single HA rule. Proxmox 9 migrated HA groups to rules.
type HARuleItem struct {
	Rule    string `json:"rule"`
	Type    string `json:"type"`
	Nodes   string `json:"nodes,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// NodeServicesResponse is the response from /nodes/{node}/services.
type NodeServicesResponse struct {
	Data []NodeServiceItem `json:"data"`
}

// NodeServiceItem represents a system service on a node.
type NodeServiceItem struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Active bool   `json:"active"`
}

// NodeNetworkResponse is the response from /nodes/{node}/network.
type NodeNetworkResponse struct {
	Data []NodeNetworkItem `json:"data"`
}

// NodeNetworkItem represents a network interface on a node.
type NodeNetworkItem struct {
	Name   string `json:"name"`
	Iface  string `json:"iface,omitempty"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Active int    `json:"active,omitempty"`
	Method string `json:"method,omitempty"`
	IP     string `json:"ip,omitempty"`
	CIDR   string `json:"cidr,omitempty"`
	MAC    string `json:"mac,omitempty"`
}

// NodeDNSResponse is the response from /nodes/{node}/dns.
type NodeDNSResponse struct {
	Data NodeDNSData `json:"data"`
}

// NodeDNSData holds DNS configuration for a node.
type NodeDNSData struct {
	DNS1         string `json:"dns1,omitempty"`
	DNS2         string `json:"dns2,omitempty"`
	SearchDomain string `json:"searchdomain,omitempty"`
}

// NodeTimeResponse is the response from /nodes/{node}/time.
type NodeTimeResponse struct {
	Data NodeTimeData `json:"data"`
}

// NodeTimeData holds time configuration for a node.
type NodeTimeData struct {
	TimeZone string `json:"timezone"`
	Epoch    int64  `json:"epoch"`
	Local    string `json:"localtime,omitempty"`
}

func (d *NodeTimeData) UnmarshalJSON(data []byte) error {
	type rawNodeTimeData struct {
		TimeZone string          `json:"timezone"`
		Epoch    int64           `json:"epoch"`
		Local    json.RawMessage `json:"localtime,omitempty"`
	}
	var raw rawNodeTimeData
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*d = NodeTimeData{TimeZone: raw.TimeZone, Epoch: raw.Epoch, Local: decodeString(raw.Local)}
	return nil
}

// NodeDisksResponse is the response from /nodes/{node}/disks/list.
type NodeDisksResponse struct {
	Data []NodeDiskItem `json:"data"`
}

// NodeDiskItem represents a physical disk on a node.
// Proxmox 9 returns devpath, size, vendor, model, etc.
type NodeDiskItem struct {
	DevPath string `json:"devpath,omitempty"`
	Size    int64  `json:"size"`
	Type    string `json:"type,omitempty"`
	Model   string `json:"model,omitempty"`
	Health  string `json:"health,omitempty"`
	Serial  string `json:"serial,omitempty"`
	Vendor  string `json:"vendor,omitempty"`
	WWN     string `json:"wwn,omitempty"`
}

// NodeCertificatesResponse is the response from /nodes/{node}/certificates.
type NodeCertificatesResponse struct {
	Data []NodeCertificateItem `json:"data"`
}

// NodeCertificateItem represents a TLS certificate category on a node.
// Proxmox 9 /nodes/{node}/certificates returns certificate categories with a name field.
// Individual certificate details (fingerprint, etc.) are available under .../certificates/{name}.
type NodeCertificateItem struct {
	Name string `json:"name"`
}

// NodeSubscriptionResponse is the response from /nodes/{node}/subscription.
type NodeSubscriptionResponse struct {
	Data NodeSubscriptionData `json:"data"`
}

// NodeSubscriptionData holds subscription status for a node.
type NodeSubscriptionData struct {
	Status  string `json:"status"`
	Key     string `json:"key,omitempty"`
	Expires string `json:"enddate,omitempty"`
}

// NodeUpdatesResponse is the response from /nodes/{node}/apt/update.
type NodeUpdatesResponse struct {
	Data []NodeUpdateItem `json:"data"`
}

// NodeUpdateItem represents an available update for a node.
type NodeUpdateItem struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

// FirewallAliasesResponse is the response from /cluster/firewall/aliases.
type FirewallAliasesResponse struct {
	Data []FirewallAliasItem `json:"data"`
}

// FirewallAliasItem represents a named address group.
type FirewallAliasItem struct {
	Name    string `json:"name"`
	CIDR    string `json:"cidr"`
	Comment string `json:"comment,omitempty"`
}

// FirewallIPSetsResponse is the response from /cluster/firewall/ipset.
type FirewallIPSetsResponse struct {
	Data []FirewallIPSetItem `json:"data"`
}

// FirewallIPSetItem represents an IP set.
type FirewallIPSetItem struct {
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
}

// FirewallIPSetEntriesResponse is the response from /cluster/firewall/ipset/{name}.
type FirewallIPSetEntriesResponse struct {
	Data []FirewallIPSetEntryItem `json:"data"`
}

// FirewallIPSetEntryItem represents a single entry in an IP set.
type FirewallIPSetEntryItem struct {
	CIDR    string `json:"cidr"`
	Comment string `json:"comment,omitempty"`
}

// FirewallSecurityGroupsResponse is the response from /cluster/firewall/groups.
type FirewallSecurityGroupsResponse struct {
	Data []FirewallSecurityGroupItem `json:"data"`
}

// FirewallSecurityGroupItem represents a firewall security group.
type FirewallSecurityGroupItem struct {
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
	Rules   []struct {
		Type     string `json:"type"`
		Action   string `json:"action"`
		Enable   int    `json:"enable,omitempty"`
		Pos      int    `json:"pos,omitempty"`
		Proto    string `json:"proto,omitempty"`
		Dest     string `json:"dest,omitempty"`
		Dport    string `json:"dport,omitempty"`
		Source   string `json:"source,omitempty"`
		Sport    string `json:"sport,omitempty"`
		ICMPType string `json:"icmp_type,omitempty"`
		Log      string `json:"log,omitempty"`
		Comment  string `json:"comment,omitempty"`
	} `json:"rules"`
}

// FirewallOptionsResponse is the response from /cluster/firewall/options.
type FirewallOptionsResponse struct {
	Data FirewallOptionsData `json:"data"`
}

// FirewallOptionsData holds cluster-level firewall options.
type FirewallOptionsData struct {
	Enable int `json:"enable"`
	Log    int `json:"log_in_drop"`
}

// NodeFirewallRulesResponse is the response from /nodes/{node}/firewall/rules.
type NodeFirewallRulesResponse struct {
	Data []FirewallRuleItem `json:"data"`
}

// VMFirewallRulesResponse is the response from /nodes/{node}/qemu/{vmid}/firewall/rules.
type VMFirewallRulesResponse struct {
	Data []FirewallRuleItem `json:"data"`
}

// HAStatusResponse is the response from /cluster/ha/status.
type HAStatusResponse struct {
	Data HAStatusData `json:"data"`
}

func (r *HAStatusResponse) UnmarshalJSON(data []byte) error {
	type rawHAStatusResponse struct {
		Data json.RawMessage `json:"data"`
	}
	var raw rawHAStatusResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var status HAStatusData
	if len(raw.Data) > 0 && string(raw.Data) != "null" {
		if raw.Data[0] == '[' {
			var items []HACurrentItem
			if err := json.Unmarshal(raw.Data, &items); err != nil {
				return err
			}
			status.Status = "ok"
			status.Quorum = len(items)
		} else if err := json.Unmarshal(raw.Data, &status); err != nil {
			return err
		}
	}
	r.Data = status
	return nil
}

// HAStatusData holds cluster HA status.
type HAStatusData struct {
	Quorum int    `json:"quorum"`
	Status string `json:"status"`
}

func decodeInt(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return 0
}

func decodeInt64(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func decodeString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}

func decodeFloatSlice(raw json.RawMessage) []float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var floats []float64
	if err := json.Unmarshal(raw, &floats); err == nil {
		return floats
	}
	var strings []string
	if err := json.Unmarshal(raw, &strings); err == nil {
		out := make([]float64, 0, len(strings))
		for _, s := range strings {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				continue
			}
			out = append(out, f)
		}
		return out
	}
	return nil
}

// HACurrentResponse is the response from /cluster/ha/current.
type HACurrentResponse struct {
	Data []HACurrentItem `json:"data"`
}

// HACurrentItem represents the current state of an HA resource.
type HACurrentItem struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	State  string `json:"state"`
	Node   string `json:"node,omitempty"`
	Status string `json:"status,omitempty"`
}

// SDNZonesResponse is the response from /cluster/sdn/zones.
type SDNZonesResponse struct {
	Data []SDNZoneItem `json:"data"`
}

// SDNZoneItem represents an SDN zone.
type SDNZoneItem struct {
	Name   string `json:"zone"`
	Type   string `json:"type"`
	Status string `json:"status,omitempty"`
	VNets  int    `json:"vnet-count,omitempty"`
}

// SDNVNetsResponse is the response from /cluster/sdn/vnets.
type SDNVNetsResponse struct {
	Data []SDNVNetItem `json:"data"`
}

// SDNVNetItem represents an SDN virtual network.
type SDNVNetItem struct {
	Name  string `json:"vnet"`
	Zone  string `json:"zone"`
	VLAN  int    `json:"vlan,omitempty"`
	Alias string `json:"alias,omitempty"`
}

// VMSnapshotConfigResponse is the response from /nodes/{node}/qemu/{vmid}/snapshot/{name}/config.
type VMSnapshotConfigResponse struct {
	Data map[string]interface{} `json:"data"`
}

// ContainerSnapshotConfigResponse is the response from /nodes/{node}/lxc/{vmid}/snapshot/{name}/config.
type ContainerSnapshotConfigResponse struct {
	Data map[string]interface{} `json:"data"`
}

// PoolsResponse is the response from /pools.
type PoolsResponse struct {
	Data []PoolItem `json:"data"`
}

// PoolItem represents a single resource pool.
type PoolItem struct {
	PoolID  string `json:"poolid"`
	Comment string `json:"comment,omitempty"`
	Members []struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Node   string `json:"node,omitempty"`
		VMID   int    `json:"vmid,omitempty"`
		PoolID string `json:"poolid"`
	} `json:"members,omitempty"`
}

// ClusterLogResponse is the response from /cluster/log.
type ClusterLogResponse struct {
	Data []ClusterLogItem `json:"data"`
}

// ClusterLogItem represents a single cluster log entry.
// Proxmox 9 /cluster/log returns task log entries with time, node, msg, etc.
type ClusterLogItem struct {
	Time    int64  `json:"time"`
	Tag     string `json:"tag,omitempty"`
	Node    string `json:"node,omitempty"`
	ID      string `json:"id,omitempty"`
	Message string `json:"msg,omitempty"`
	User    string `json:"user,omitempty"`
	Pri     int    `json:"pri,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

// BackupScheduleListResponse is the response from GET /cluster/backup.
type BackupScheduleListResponse struct {
	Data []BackupScheduleItem `json:"data"`
}

// BackupScheduleDetailResponse is the response from GET /cluster/backup/{id}.
type BackupScheduleDetailResponse struct {
	Data BackupScheduleItem `json:"data"`
}

// BackupScheduleItem represents a single backup job schedule.
type BackupScheduleItem struct {
	ID               string `json:"id"`
	Node             string `json:"node,omitempty"`
	Storage          string `json:"storage"`
	VMID             string `json:"vmid,omitempty"`
	All              int    `json:"all,omitempty"`
	Dow              string `json:"dow,omitempty"`
	Starttime        string `json:"starttime"`
	Mode             string `json:"mode"`
	Enabled          int    `json:"enabled,omitempty"`
	Compress         string `json:"compress,omitempty"`
	Comment          string `json:"comment,omitempty"`
	Bwlimit          int    `json:"bwlimit,omitempty"`
	Ionice           int    `json:"ionice,omitempty"`
	MailNotification string `json:"mailnotification,omitempty"`
	Mailto           string `json:"mailto,omitempty"`
	Maxfiles         int    `json:"maxfiles,omitempty"`
	PruneBackups     string `json:"prune-backups,omitempty"`
	Quiet            int    `json:"quiet,omitempty"`
	Remove           int    `json:"remove,omitempty"`
	Pool             string `json:"pool,omitempty"`
	Tmpdir           string `json:"tmpdir,omitempty"`
}

// BackupScheduleCreateRequest is the body for POST /cluster/backup.
type BackupScheduleCreateRequest struct {
	Node             string `json:"node,omitempty"`
	Storage          string `json:"storage"`
	VMID             string `json:"vmid,omitempty"`
	All              int    `json:"all,omitempty"`
	Dow              string `json:"dow,omitempty"`
	Starttime        string `json:"starttime"`
	Mode             string `json:"mode"`
	Enabled          int    `json:"enabled,omitempty"`
	Compress         string `json:"compress,omitempty"`
	Comment          string `json:"comment,omitempty"`
	Bwlimit          int    `json:"bwlimit,omitempty"`
	Ionice           int    `json:"ionice,omitempty"`
	MailNotification string `json:"mailnotification,omitempty"`
	Mailto           string `json:"mailto,omitempty"`
	Maxfiles         int    `json:"maxfiles,omitempty"`
	PruneBackups     string `json:"prune-backups,omitempty"`
	Quiet            int    `json:"quiet,omitempty"`
	Remove           int    `json:"remove,omitempty"`
	Pool             string `json:"pool,omitempty"`
	Tmpdir           string `json:"tmpdir,omitempty"`
}

// VMCloneRequest is the body for POST /nodes/{node}/qemu/{vmid}/clone.
type VMCloneRequest struct {
	NewID   int    `json:"newid"`
	Name    string `json:"name,omitempty"`
	Storage string `json:"storage,omitempty"`
}

// CTCloneRequest is the body for POST /nodes/{node}/lxc/{vmid}/clone.
type CTCloneRequest struct {
	NewID    int    `json:"newid"`
	Hostname string `json:"hostname,omitempty"`
	Storage  string `json:"storage,omitempty"`
}

// VMMigrateRequest is the body for POST /nodes/{node}/qemu/{vmid}/migrate.
type VMMigrateRequest struct {
	Target string `json:"target"`
	Online int    `json:"online,omitempty"`
}

// CTMigrateRequest is the body for POST /nodes/{node}/lxc/{vmid}/migrate.
type CTMigrateRequest struct {
	Target string `json:"target"`
}

// VMDiskResizeRequest is the body for PUT /nodes/{node}/qemu/{vmid}/resize.
type VMDiskResizeRequest struct {
	Disk string `json:"disk"`
	Size string `json:"size"`
}

// VMDiskMoveRequest is the body for POST /nodes/{node}/qemu/{vmid}/move_disk.
type VMDiskMoveRequest struct {
	Disk    string `json:"disk"`
	Storage string `json:"storage"`
}

// VzdumpCreateRequest is the body for POST /nodes/{node}/vzdump.
type VzdumpCreateRequest struct {
	VMID    string `json:"vmid"`
	Storage string `json:"storage"`
	Mode    string `json:"mode"`
}

// --- Access/Identity types ---

// AccessUsersResponse is the response from GET /access/users.
type AccessUsersResponse struct {
	Data []AccessUserItem `json:"data"`
}

// AccessUserItem represents a single user.
type AccessUserItem struct {
	UserID    string `json:"userid"`
	Comment   string `json:"comment,omitempty"`
	Email     string `json:"email,omitempty"`
	Enable    int    `json:"enable,omitempty"`
	Expire    int64  `json:"expire,omitempty"`
	FirstName string `json:"firstname,omitempty"`
	LastName  string `json:"lastname,omitempty"`
	Tokens    *int   `json:"tokens,omitempty"`
}

// AccessGroupsResponse is the response from GET /access/groups.
type AccessGroupsResponse struct {
	Data []AccessGroupItem `json:"data"`
}

// AccessGroupItem represents a single group.
type AccessGroupItem struct {
	GroupID string   `json:"groupid"`
	Comment string   `json:"comment,omitempty"`
	Members []string `json:"members,omitempty"`
}

// AccessRolesResponse is the response from GET /access/roles.
type AccessRolesResponse struct {
	Data []AccessRoleItem `json:"data"`
}

// AccessRoleItem represents a single role.
type AccessRoleItem struct {
	RoleID  string `json:"roleid"`
	Privs   string `json:"privs,omitempty"`
	Special int    `json:"special,omitempty"`
}

// AccessACLResponse is the response from GET /access/acl.
type AccessACLResponse struct {
	Data []AccessACLItem `json:"data"`
}

// AccessACLItem represents a single ACL entry.
type AccessACLItem struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	RoleID    string `json:"roleid"`
	Propagate int    `json:"propagate,omitempty"`
	UserID    string `json:"ugid,omitempty"`
	GroupID   string `json:"groupid,omitempty"`
	Realm     string `json:"realm,omitempty"`
}

// AccessDomainsResponse is the response from GET /access/domains.
type AccessDomainsResponse struct {
	Data []AccessDomainItem `json:"data"`
}

// AccessDomainItem represents a single authentication realm.
type AccessDomainItem struct {
	Realm   string `json:"realm"`
	Type    string `json:"type"`
	Comment string `json:"comment,omitempty"`
	Default int    `json:"default,omitempty"`
	TFA     string `json:"tfa,omitempty"`
}

// AccessTokensResponse is the response from GET /access/users/{user}/token.
type AccessTokensResponse struct {
	Data []AccessTokenItem `json:"data"`
}

// AccessTokenItem represents a single API token (metadata only, no secret).
type AccessTokenItem struct {
	TokenID  string `json:"tokenid"`
	Comment  string `json:"comment,omitempty"`
	Expire   int64  `json:"expire,omitempty"`
	Privsep  int    `json:"privsep,omitempty"`
	Created  int64  `json:"created,omitempty"`
	UserID   string `json:"userid,omitempty"`
	Disabled int    `json:"disabled,omitempty"`
}

// --- Firewall mutation request types ---

// FirewallRuleCreateRequest is the body for POST firewall rule creation.
type FirewallRuleCreateRequest struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	Enable   int    `json:"enable,omitempty"`
	Pos      int    `json:"pos,omitempty"`
	Proto    string `json:"proto,omitempty"`
	Dest     string `json:"dest,omitempty"`
	Dport    string `json:"dport,omitempty"`
	Source   string `json:"source,omitempty"`
	Sport    string `json:"sport,omitempty"`
	ICMPType string `json:"icmp_type,omitempty"`
	Log      string `json:"log,omitempty"`
	Comment  string `json:"comment,omitempty"`
	IFace    string `json:"iface,omitempty"`
	Macro    string `json:"macro,omitempty"`
}

// FirewallAliasCreateRequest is the body for POST /cluster/firewall/aliases.
type FirewallAliasCreateRequest struct {
	Name    string `json:"name"`
	CIDR    string `json:"cidr"`
	Comment string `json:"comment,omitempty"`
}

// FirewallIPSetCreateRequest is the body for POST /cluster/firewall/ipset.
type FirewallIPSetCreateRequest struct {
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
}

// FirewallIPSetEntryRequest is the body for POST /cluster/firewall/ipset/{name}.
type FirewallIPSetEntryRequest struct {
	CIDR    string `json:"cidr"`
	Comment string `json:"comment,omitempty"`
}

// FirewallGroupCreateRequest is the body for POST /cluster/firewall/groups.
type FirewallGroupCreateRequest struct {
	Name    string `json:"name"`
	Comment string `json:"comment,omitempty"`
}

// FirewallOptionsUpdateRequest is the body for PUT /cluster/firewall/options.
type FirewallOptionsUpdateRequest struct {
	Enable       int    `json:"enable,omitempty"`
	PolicyIn     string `json:"policy_in,omitempty"`
	PolicyOut    string `json:"policy_out,omitempty"`
	LogInDrop    int    `json:"log_in_drop,omitempty"`
	LogRateLimit string `json:"log_ratelimit,omitempty"`
	NFConntrack  int    `json:"nf_conntrack_max,omitempty"`
	Digest       string `json:"digest,omitempty"`
}

// --- Network mutation request type ---

// NodeNetworkApplyRequest holds the network configuration to apply.
// The Proxmox API accepts the full network config as the body.
type NodeNetworkApplyRequest struct {
	Interfaces []NodeNetworkApplyItem `json:"interfaces"`
}

// NodeNetworkApplyItem represents a single network interface configuration.
type NodeNetworkApplyItem struct {
	Name            string `json:"name"`
	Type            string `json:"type"`
	Autostart       int    `json:"autostart,omitempty"`
	Method          string `json:"method,omitempty"`
	Method6         string `json:"method6,omitempty"`
	Address         string `json:"address,omitempty"`
	Netmask         string `json:"netmask,omitempty"`
	Gateway         string `json:"gateway,omitempty"`
	BridgePorts     string `json:"bridge_ports,omitempty"`
	BridgeVLANAware int    `json:"bridge_vlan_aware,omitempty"`
	Comments        string `json:"comments,omitempty"`
	MTU             int    `json:"mtu,omitempty"`
}

// --- Ceph types ---

// CephStatusResponse is the response from /nodes/{node}/ceph/status.
type CephStatusResponse struct {
	Data map[string]interface{} `json:"data"`
}

// CephOSDListResponse is the response from /nodes/{node}/ceph/osd.
type CephOSDListResponse struct {
	Data struct {
		Root struct {
			Children []CephOSDTreeNode `json:"children"`
		} `json:"root"`
	} `json:"data"`
}

// CephOSDTreeNode represents a node in the OSD tree.
type CephOSDTreeNode struct {
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Type        string            `json:"type,omitempty"`
	Status      string            `json:"status,omitempty"`
	In          int               `json:"in,omitempty"`
	Host        string            `json:"host,omitempty"`
	DeviceClass string            `json:"device_class,omitempty"`
	TotalSpace  int64             `json:"total_space,omitempty"`
	BytesUsed   int64             `json:"bytes_used,omitempty"`
	PercentUsed float64           `json:"percent_used,omitempty"`
	Leaf        int               `json:"leaf,omitempty"`
	Children    []CephOSDTreeNode `json:"children,omitempty"`
}

func (n *CephOSDTreeNode) UnmarshalJSON(data []byte) error {
	type rawCephOSDTreeNode struct {
		ID          json.RawMessage   `json:"id"`
		Name        string            `json:"name"`
		Type        string            `json:"type,omitempty"`
		Status      string            `json:"status,omitempty"`
		In          json.RawMessage   `json:"in,omitempty"`
		Host        string            `json:"host,omitempty"`
		DeviceClass string            `json:"device_class,omitempty"`
		TotalSpace  int64             `json:"total_space,omitempty"`
		BytesUsed   int64             `json:"bytes_used,omitempty"`
		PercentUsed float64           `json:"percent_used,omitempty"`
		Leaf        json.RawMessage   `json:"leaf,omitempty"`
		Children    []CephOSDTreeNode `json:"children,omitempty"`
	}
	var raw rawCephOSDTreeNode
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*n = CephOSDTreeNode{
		ID:          decodeInt(raw.ID),
		Name:        raw.Name,
		Type:        raw.Type,
		Status:      raw.Status,
		In:          decodeInt(raw.In),
		Host:        raw.Host,
		DeviceClass: raw.DeviceClass,
		TotalSpace:  raw.TotalSpace,
		BytesUsed:   raw.BytesUsed,
		PercentUsed: raw.PercentUsed,
		Leaf:        decodeInt(raw.Leaf),
		Children:    raw.Children,
	}
	return nil
}

// CephMONListResponse is the response from /nodes/{node}/ceph/mon.
type CephMONListResponse struct {
	Data []CephMONItem `json:"data"`
}

// CephMONItem represents a Ceph monitor entry.
type CephMONItem struct {
	Name             string `json:"name"`
	Host             string `json:"host,omitempty"`
	Quorum           int    `json:"quorum,omitempty"`
	State            string `json:"state,omitempty"`
	Rank             int    `json:"rank,omitempty"`
	CephVersionShort string `json:"ceph_version_short,omitempty"`
}

// CephPoolListResponse is the response from /nodes/{node}/ceph/pool.
type CephPoolListResponse struct {
	Data []CephPoolItem `json:"data"`
}

// CephPoolItem represents a Ceph pool entry.
type CephPoolItem struct {
	Pool            int     `json:"pool"`
	PoolName        string  `json:"pool_name"`
	Size            int     `json:"size"`
	MinSize         int     `json:"min_size"`
	PGNum           int     `json:"pg_num"`
	CrushRule       int     `json:"crush_rule"`
	CrushRuleName   string  `json:"crush_rule_name,omitempty"`
	Type            string  `json:"type"`
	PGNumFinal      int     `json:"pg_num_final,omitempty"`
	PercentUsed     float64 `json:"percent_used,omitempty"`
	BytesUsed       int64   `json:"bytes_used,omitempty"`
	PGAutoscaleMode string  `json:"pg_autoscale_mode,omitempty"`
}

// --- SDN types ---

// SDNCreateZoneRequest is the body for POST /cluster/sdn/zones.
type SDNCreateZoneRequest struct {
	Type string `json:"type"`
	Zone string `json:"zone"`
}

// SDNCreateVNetRequest is the body for POST /cluster/sdn/vnets.
type SDNCreateVNetRequest struct {
	VNet string `json:"vnet"`
	Zone string `json:"zone"`
}

// SDNCreateSubnetRequest is the body for POST /cluster/sdn/vnets/{vnet}/subnets.
type SDNCreateSubnetRequest struct {
	Subnet  string `json:"subnet"`
	Type    string `json:"type"`
	Gateway string `json:"gateway,omitempty"`
}

// SDNCreateControllerRequest is the body for POST /cluster/sdn/controllers.
type SDNCreateControllerRequest struct {
	Controller string `json:"controller"`
}

// SDNZonesMutationResponse is the response from SDN zone mutations.
type SDNZonesMutationResponse struct {
	Data interface{} `json:"data"`
}

// --- Replication types ---

// ReplicationListResponse is the response from /cluster/replication.
type ReplicationListResponse struct {
	Data []ReplicationJobItem `json:"data"`
}

// ReplicationGetResponse is the response from /cluster/replication/{id}.
type ReplicationGetResponse struct {
	Data ReplicationJobItem `json:"data"`
}

// ReplicationJobItem represents a replication job.
type ReplicationJobItem struct {
	ID        string `json:"id"`
	Guest     int    `json:"guest"`
	Type      string `json:"type"`
	Source    string `json:"source,omitempty"`
	Target    string `json:"target"`
	Schedule  string `json:"schedule,omitempty"`
	Comment   string `json:"comment,omitempty"`
	Disable   int    `json:"disable"`
	Rate      int64  `json:"rate,omitempty"`
	JobNum    int    `json:"jobnum,omitempty"`
	LastSync  int64  `json:"last_sync,omitempty"`
	FailCount int    `json:"fail_count,omitempty"`
}

// ReplicationCreateRequest is the body for POST /cluster/replication.
type ReplicationCreateRequest struct {
	ID       string `json:"id"`
	Guest    int    `json:"guest"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	Schedule string `json:"schedule,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Rate     int64  `json:"rate,omitempty"`
	Source   string `json:"source,omitempty"`
}

// ReplicationUpdateRequest is the body for PUT /cluster/replication/{id}.
type ReplicationUpdateRequest struct {
	Target   string `json:"target,omitempty"`
	Schedule string `json:"schedule,omitempty"`
	Comment  string `json:"comment,omitempty"`
	Rate     int64  `json:"rate,omitempty"`
	Disable  int    `json:"disable,omitempty"`
	Source   string `json:"source,omitempty"`
	Delete   string `json:"delete,omitempty"`
}
