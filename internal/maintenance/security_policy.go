package maintenance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	SecurityPolicyPlanSchema = 1
	SecurityPolicyMarker     = "# Managed by Nodex: unattended security updates"
	SecurityPolicyTTL        = 4 * time.Hour
)

var aptOriginValueRe = regexp.MustCompile(`^[A-Za-z0-9_${}.,:=+/-]+$`)
var aptSecurityOriginRe = regexp.MustCompile(`(?i)(^|[,=:_-])(security|esm)([,=:_-]|$)`)

// SecurityPolicyHost captures the read-only host state and exact managed-file
// replacement reviewed by the operator before unattended updates are enabled.
// It contains no credentials and only includes prior file content when that
// file already carries Nodex's ownership marker.
type SecurityPolicyHost struct {
	Name                         string   `json:"name" yaml:"name"`
	Address                      string   `json:"address" yaml:"address"`
	Role                         string   `json:"role" yaml:"role"`
	Environment                  string   `json:"environment,omitempty" yaml:"environment,omitempty"`
	MaintenanceGroup             string   `json:"maintenance_group,omitempty" yaml:"maintenance_group,omitempty"`
	Criticality                  string   `json:"criticality" yaml:"criticality"`
	SSHUser                      string   `json:"ssh_user" yaml:"ssh_user"`
	SSHPort                      int      `json:"ssh_port" yaml:"ssh_port"`
	KeyConfigured                bool     `json:"key_configured" yaml:"key_configured"`
	KnownHostsConfigured         bool     `json:"known_hosts_configured" yaml:"known_hosts_configured"`
	Distribution                 string   `json:"distribution" yaml:"distribution"`
	DistributionRelease          string   `json:"distribution_release" yaml:"distribution_release"`
	BeforeSecurityAllowedOrigins []string `json:"before_security_allowed_origins,omitempty" yaml:"before_security_allowed_origins,omitempty"`
	BeforeOriginsPatterns        []string `json:"before_origins_patterns,omitempty" yaml:"before_origins_patterns,omitempty"`
	SecurityAllowedOrigins       []string `json:"security_allowed_origins,omitempty" yaml:"security_allowed_origins,omitempty"`
	SecurityOriginsPatterns      []string `json:"security_origins_patterns,omitempty" yaml:"security_origins_patterns,omitempty"`
	BeforeNonSecurityOriginCount int      `json:"before_nonsecurity_origin_count" yaml:"before_nonsecurity_origin_count"`
	BeforePackageInstalled       bool     `json:"before_package_installed" yaml:"before_package_installed"`
	BeforePackageListsPeriodic   bool     `json:"before_package_lists_periodic" yaml:"before_package_lists_periodic"`
	BeforeUnattendedPeriodic     bool     `json:"before_unattended_periodic" yaml:"before_unattended_periodic"`
	BeforeAutomaticReboot        bool     `json:"before_automatic_reboot" yaml:"before_automatic_reboot"`
	BeforeTimerEnabled           string   `json:"before_timer_enabled" yaml:"before_timer_enabled"`
	BeforeTimerActive            string   `json:"before_timer_active" yaml:"before_timer_active"`
	BeforeConfigExists           bool     `json:"before_config_exists" yaml:"before_config_exists"`
	BeforeConfigChecksum         string   `json:"before_config_checksum,omitempty" yaml:"before_config_checksum,omitempty"`
	BeforeConfig                 string   `json:"before_config,omitempty" yaml:"before_config,omitempty"`
	AfterConfigChecksum          string   `json:"after_config_checksum,omitempty" yaml:"after_config_checksum,omitempty"`
	AfterConfig                  string   `json:"after_config" yaml:"after_config"`
	Blockers                     []string `json:"blockers,omitempty" yaml:"blockers,omitempty"`
}

type SecurityPolicyExclusion struct {
	Name   string `json:"name" yaml:"name"`
	Role   string `json:"role" yaml:"role"`
	Reason string `json:"reason" yaml:"reason"`
}

// SecurityPolicyPlan is a digest-bound plan for installing Nodex's dedicated
// unattended-security APT drop-in. Original administrator-owned APT files are
// never overwritten; only the Nodex-owned drop-in is backed up or replaced.
type SecurityPolicyPlan struct {
	Schema               int                       `json:"schema" yaml:"schema"`
	PlanID               string                    `json:"plan_id" yaml:"plan_id"`
	CreatedAt            int64                     `json:"created_at" yaml:"created_at"`
	ExpiresAt            int64                     `json:"expires_at" yaml:"expires_at"`
	Operation            string                    `json:"operation" yaml:"operation"`
	RebootPolicy         string                    `json:"reboot_policy" yaml:"reboot_policy"`
	SafetyClassification string                    `json:"safety_classification" yaml:"safety_classification"`
	Hosts                []SecurityPolicyHost      `json:"hosts" yaml:"hosts"`
	ExcludedHosts        []SecurityPolicyExclusion `json:"excluded_hosts,omitempty" yaml:"excluded_hosts,omitempty"`
	Warnings             []string                  `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Blockers             []string                  `json:"blockers,omitempty" yaml:"blockers,omitempty"`
	Digest               string                    `json:"digest" yaml:"digest"`
}

func (p SecurityPolicyPlan) digest() (string, error) {
	p.Digest = ""
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("canonicalize security policy plan: %w", err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (p *SecurityPolicyPlan) Finalize() error {
	if p == nil {
		return fmt.Errorf("security policy plan is nil")
	}
	if p.Schema == 0 {
		p.Schema = SecurityPolicyPlanSchema
	}
	if p.Operation == "" {
		p.Operation = "configure"
	}
	if p.RebootPolicy == "" {
		p.RebootPolicy = RebootPolicyNever
	}
	if p.SafetyClassification == "" {
		p.SafetyClassification = "disruptive"
	}
	sort.Slice(p.Hosts, func(i, j int) bool { return p.Hosts[i].Name < p.Hosts[j].Name })
	sort.Slice(p.ExcludedHosts, func(i, j int) bool { return p.ExcludedHosts[i].Name < p.ExcludedHosts[j].Name })
	sort.Strings(p.Warnings)
	sort.Strings(p.Blockers)
	for i := range p.Hosts {
		allowed, err := normalizeSecurityOrigins(p.Hosts[i].SecurityAllowedOrigins)
		if err != nil {
			return err
		}
		beforeAllowed, err := normalizeSecurityOrigins(p.Hosts[i].BeforeSecurityAllowedOrigins)
		if err != nil {
			return err
		}
		patterns, err := normalizeSecurityOrigins(p.Hosts[i].SecurityOriginsPatterns)
		if err != nil {
			return err
		}
		beforePatterns, err := normalizeSecurityOrigins(p.Hosts[i].BeforeOriginsPatterns)
		if err != nil {
			return err
		}
		p.Hosts[i].SecurityAllowedOrigins = allowed
		p.Hosts[i].SecurityOriginsPatterns = patterns
		p.Hosts[i].BeforeSecurityAllowedOrigins = beforeAllowed
		p.Hosts[i].BeforeOriginsPatterns = beforePatterns
		sort.Strings(p.Hosts[i].Blockers)
	}
	if err := p.validate(false, time.Time{}); err != nil {
		return err
	}
	d, err := p.digest()
	if err != nil {
		return err
	}
	p.Digest = d
	return nil
}

func (p SecurityPolicyPlan) Verify(now time.Time) error {
	return p.validate(true, now)
}

func (p SecurityPolicyPlan) VerifyForRestore() error {
	return p.validate(true, time.Time{})
}

func (p SecurityPolicyPlan) validate(checkDigest bool, now time.Time) error {
	if p.Schema != SecurityPolicyPlanSchema {
		return fmt.Errorf("unsupported security policy plan schema %d (expected %d)", p.Schema, SecurityPolicyPlanSchema)
	}
	if !validPlanIdentifier(p.PlanID) || p.CreatedAt <= 0 || p.ExpiresAt <= p.CreatedAt {
		return fmt.Errorf("invalid security policy plan identity or timestamps")
	}
	if p.Operation != "configure" || p.RebootPolicy != RebootPolicyNever || p.SafetyClassification != "disruptive" {
		return fmt.Errorf("invalid security policy plan safety settings")
	}
	if len(p.Hosts) == 0 {
		return fmt.Errorf("security policy plan has no eligible hosts")
	}
	if !now.IsZero() && p.ExpiresAt <= now.Unix() {
		return fmt.Errorf("security policy plan expired at %s", time.Unix(p.ExpiresAt, 0).UTC().Format(time.RFC3339))
	}
	seen := map[string]bool{}
	for _, h := range p.Hosts {
		if !validPlanIdentifier(h.Name) || h.Address == "" || h.SSHUser == "" || seen[h.Name] {
			return fmt.Errorf("invalid or duplicate security policy host %q", h.Name)
		}
		seen[h.Name] = true
		if h.SSHPort < 0 || h.SSHPort > 65535 {
			return fmt.Errorf("host %q has invalid SSH port", h.Name)
		}
		if h.BeforeNonSecurityOriginCount < 0 {
			return fmt.Errorf("host %q has a negative non-security origin count", h.Name)
		}
		if len(h.BeforeConfig) > 64*1024 || len(h.AfterConfig) > 64*1024 {
			return fmt.Errorf("host %q security policy config exceeds the size limit", h.Name)
		}
		hostBlocked := len(h.Blockers) > 0 || len(p.Blockers) > 0
		if h.BeforeConfigExists && h.BeforeConfig != "" {
			if ValidateSecurityPolicyConfig(h.BeforeConfig) != nil || hashPolicyConfig(h.BeforeConfig) != h.BeforeConfigChecksum {
				return fmt.Errorf("host %q prior policy file is not Nodex-managed or its checksum is invalid", h.Name)
			}
		} else if !hostBlocked && h.BeforeConfigExists {
			return fmt.Errorf("host %q prior policy content is missing", h.Name)
		}
		if !h.BeforeConfigExists && h.BeforeConfig != "" {
			return fmt.Errorf("host %q has prior policy content but reports no file", h.Name)
		}
		if !h.BeforeConfigExists && h.BeforeConfigChecksum != "" {
			return fmt.Errorf("host %q has a checksum for an absent prior policy file", h.Name)
		}
		if h.AfterConfig != "" {
			if ValidateSecurityPolicyConfig(h.AfterConfig) != nil || hashPolicyConfig(h.AfterConfig) != h.AfterConfigChecksum {
				return fmt.Errorf("host %q target policy config or checksum is invalid", h.Name)
			}
			expected, err := BuildSecurityPolicyConfig(h.SecurityAllowedOrigins, h.SecurityOriginsPatterns)
			if err != nil || expected != h.AfterConfig {
				return fmt.Errorf("host %q target config does not match its reviewed security origins", h.Name)
			}
		} else if !hostBlocked {
			return fmt.Errorf("host %q has no target policy config or blocker", h.Name)
		}
		origins := append(append([]string(nil), h.SecurityAllowedOrigins...), h.SecurityOriginsPatterns...)
		origins = append(origins, h.BeforeSecurityAllowedOrigins...)
		origins = append(origins, h.BeforeOriginsPatterns...)
		for _, origin := range origins {
			if !aptOriginValueRe.MatchString(origin) || !isSecurityOrigin(origin) {
				return fmt.Errorf("host %q has an invalid or non-security APT origin", h.Name)
			}
		}
	}
	if checkDigest {
		if len(p.Digest) != sha256.Size*2 {
			return fmt.Errorf("security policy plan has no valid digest")
		}
		want, err := p.digest()
		if err != nil {
			return err
		}
		if want != p.Digest {
			return fmt.Errorf("security policy plan digest mismatch: the plan was modified after creation")
		}
	}
	return nil
}

// BuildSecurityPolicyConfig creates a narrow APT drop-in. #clear limits the
// effective unattended-upgrade origins to security entries while leaving all
// other administrator-owned APT configuration untouched.
func BuildSecurityPolicyConfig(allowedOrigins, originsPatterns []string) (string, error) {
	var err error
	allowedOrigins, err = normalizeSecurityOrigins(allowedOrigins)
	if err != nil {
		return "", err
	}
	originsPatterns, err = normalizeSecurityOrigins(originsPatterns)
	if err != nil {
		return "", err
	}
	if len(allowedOrigins)+len(originsPatterns) == 0 {
		return "", fmt.Errorf("no effective APT security origins were found")
	}
	var b strings.Builder
	b.WriteString(SecurityPolicyMarker + ".\n")
	b.WriteString("# Nodex owns only this drop-in; administrator APT files remain unchanged.\n")
	b.WriteString("APT::Periodic::Update-Package-Lists \"1\";\n")
	b.WriteString("APT::Periodic::Unattended-Upgrade \"1\";\n")
	b.WriteString("Unattended-Upgrade::Automatic-Reboot \"false\";\n")
	b.WriteString("#clear Unattended-Upgrade::Allowed-Origins;\n")
	b.WriteString("Unattended-Upgrade::Allowed-Origins {\n")
	for _, origin := range allowedOrigins {
		fmt.Fprintf(&b, "  \"%s\";\n", origin)
	}
	b.WriteString("};\n")
	b.WriteString("#clear Unattended-Upgrade::Origins-Pattern;\n")
	b.WriteString("Unattended-Upgrade::Origins-Pattern {\n")
	for _, origin := range originsPatterns {
		fmt.Fprintf(&b, "  \"%s\";\n", origin)
	}
	b.WriteString("};\n")
	return b.String(), nil
}

// ValidateSecurityPolicyConfig accepts only the exact directive shape Nodex
// generates, so a file with a marker plus hand-edited directives is never
// treated as safe to overwrite or restore automatically.
func ValidateSecurityPolicyConfig(content string) error {
	lines := strings.Split(content, "\n")
	allowed, err := parseSecurityOriginBlock(lines, "Unattended-Upgrade::Allowed-Origins {")
	if err != nil {
		return err
	}
	patterns, err := parseSecurityOriginBlock(lines, "Unattended-Upgrade::Origins-Pattern {")
	if err != nil {
		return err
	}
	want, err := BuildSecurityPolicyConfig(allowed, patterns)
	if err != nil {
		return err
	}
	if content != want {
		return fmt.Errorf("managed security policy content does not match Nodex's generated format")
	}
	return nil
}

func parseSecurityOriginBlock(lines []string, start string) ([]string, error) {
	var result []string
	inBlock := false
	found := false
	for _, line := range lines {
		if line == start {
			if found {
				return nil, fmt.Errorf("duplicate APT origin block")
			}
			found, inBlock = true, true
			continue
		}
		if !inBlock {
			continue
		}
		if line == "};" {
			inBlock = false
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, `"`) || !strings.HasSuffix(trimmed, `";`) {
			return nil, fmt.Errorf("invalid APT origin list entry")
		}
		value, err := strconv.Unquote(strings.TrimSuffix(trimmed, ";"))
		if err != nil || !aptOriginValueRe.MatchString(value) || !isSecurityOrigin(value) {
			return nil, fmt.Errorf("invalid security APT origin")
		}
		result = append(result, value)
	}
	if inBlock || !found {
		return nil, fmt.Errorf("missing or incomplete APT origin block")
	}
	return result, nil
}

func hashPolicyConfig(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func SecurityPolicyConfigChecksum(content string) string { return hashPolicyConfig(content) }

func IsSecurityAPTOrigin(origin string) bool {
	return aptOriginValueRe.MatchString(origin) && isSecurityOrigin(origin)
}

func normalizeSecurityOrigins(origins []string) ([]string, error) {
	set := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin == "" || !aptOriginValueRe.MatchString(origin) || !isSecurityOrigin(origin) {
			return nil, fmt.Errorf("invalid or non-security APT origin")
		}
		set[origin] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for origin := range set {
		result = append(result, origin)
	}
	sort.Strings(result)
	return result, nil
}

func isSecurityOrigin(origin string) bool {
	return aptSecurityOriginRe.MatchString(origin)
}

func LoadSecurityPolicyPlanFile(path string, now time.Time) (SecurityPolicyPlan, error) {
	return loadSecurityPolicyPlanFile(path, now, false)
}

func LoadSecurityPolicyPlanForRestore(path string) (SecurityPolicyPlan, error) {
	return loadSecurityPolicyPlanFile(path, time.Time{}, true)
}

func loadSecurityPolicyPlanFile(path string, now time.Time, restoring bool) (SecurityPolicyPlan, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return SecurityPolicyPlan{}, fmt.Errorf("inspect security policy plan: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return SecurityPolicyPlan{}, fmt.Errorf("security policy plan path is not a regular file")
	}
	f, err := os.Open(path) // #nosec G304 -- the operator explicitly selects the local plan file.
	if err != nil {
		return SecurityPolicyPlan{}, fmt.Errorf("open security policy plan: %w", err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 8<<20))
	if err != nil {
		return SecurityPolicyPlan{}, fmt.Errorf("read security policy plan: %w", err)
	}
	var p SecurityPolicyPlan
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		dec := yaml.NewDecoder(strings.NewReader(string(b)))
		dec.KnownFields(true)
		if err := dec.Decode(&p); err != nil {
			return SecurityPolicyPlan{}, fmt.Errorf("decode security policy plan YAML: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				return SecurityPolicyPlan{}, fmt.Errorf("security policy plan contains multiple YAML documents")
			}
			return SecurityPolicyPlan{}, fmt.Errorf("read security policy plan YAML: %w", err)
		}
	default:
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return SecurityPolicyPlan{}, fmt.Errorf("decode security policy plan JSON: %w", err)
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			if err == nil {
				return SecurityPolicyPlan{}, fmt.Errorf("security policy plan contains multiple JSON values")
			}
			return SecurityPolicyPlan{}, fmt.Errorf("read security policy plan JSON: %w", err)
		}
	}
	if restoring {
		err = p.VerifyForRestore()
	} else {
		err = p.Verify(now)
	}
	if err != nil {
		return SecurityPolicyPlan{}, err
	}
	return p, nil
}

func SecurityPolicyReceiptPath(dir, planID, operation string) string {
	if !validPlanIdentifier(planID) || (operation != "apply" && operation != "restore") {
		return ""
	}
	return filepath.Join(dir, planID+"-"+operation+".json")
}
