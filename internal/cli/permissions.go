package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/provider"
	_ "github.com/geoffmcc/nodex/internal/provider/pbs"
	_ "github.com/geoffmcc/nodex/internal/provider/proxmox"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

const (
	permissionConfirmed   = "confirmed"
	permissionMissing     = "missing"
	permissionUnsupported = "unsupported"
	permissionUnknown     = "unknown"
)

type permissionCheck struct {
	Name   string `json:"name" yaml:"name"`
	Status string `json:"status" yaml:"status"`
	Detail string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

func runProfileDiagnosePermissions(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 1 {
		return app.NewExitError(fmt.Errorf("usage: nodex profile diagnose-permissions <profile>"), app.ExitUsage)
	}
	name := args[0]
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	p, ok := cfg.Profiles[name]
	if !ok {
		return app.NewExitError(fmt.Errorf("%w: profile %q not found", app.ErrProfileNotFound, name), app.ExitConfig)
	}
	checks := diagnoseProfile(ctx, cmdCtx, name, p)
	if err := writePermissionChecks(cmdCtx, name, checks); err != nil {
		return err
	}
	for _, check := range checks {
		if check.Status != permissionConfirmed {
			return app.NewExitError(fmt.Errorf("permission diagnosis incomplete: %s is %s", check.Name, check.Status), app.ExitPartialFailure)
		}
	}
	return nil
}

func diagnoseProfile(ctx context.Context, cmdCtx *Context, name string, p config.Profile) []permissionCheck {
	checks := make([]permissionCheck, 0, 8)
	add := func(n, s, d string) { checks = append(checks, permissionCheck{Name: n, Status: s, Detail: d}) }

	providerName := config.NormalizeProvider(p.Provider)
	prov, err := provider.Get(providerName)
	if err != nil {
		add("provider", permissionUnsupported, "provider is not available in this build")
		return checks
	}
	add("provider", permissionConfirmed, "provider is available")
	if p.Endpoint == "" {
		add("endpoint", permissionMissing, "endpoint is not configured")
		return checks
	}
	if err := config.ValidateEndpoint(p.Endpoint); err != nil {
		add("endpoint", permissionMissing, "endpoint does not satisfy HTTPS policy")
		return checks
	}
	add("endpoint", permissionConfirmed, "HTTPS endpoint is configured")

	if p.CredentialRef == "" {
		add("credential", permissionMissing, "no credential reference is configured")
		return checks
	}
	backend, refName, err := credentials.ParseCredentialRefStrict(p.CredentialRef)
	if err != nil {
		add("credential", permissionMissing, "credential reference is invalid")
		return checks
	}
	resolver := credentials.NewResolver("")
	backendStore, ok := resolver.GetBackend(backend)
	if !ok || backend == "stdin" {
		add("credential", permissionUnknown, "credential backend cannot be checked without reading a secret")
		return checks
	}
	creds, err := backendStore.Get(ctx, refName)
	if err != nil {
		add("credential", permissionMissing, "credential reference is unavailable")
		return checks
	}
	if err := credentials.ValidateCredentials(name, creds); err != nil {
		add("credential", permissionMissing, "credential reference contains incomplete credentials")
		return checks
	}
	add("credential", permissionConfirmed, "credential reference resolves")

	opts := []httpclient.Option{httpclient.WithTimeout(cmdCtx.Opts.Timeout)}
	if p.CAFile != "" {
		caOpt, caErr := httpclient.WithCACert(p.CAFile)
		if caErr != nil {
			add("ca_file", permissionMissing, "CA file is unavailable or invalid")
			return checks
		}
		opts = append(opts, caOpt)
		add("ca_file", permissionConfirmed, "custom CA file is valid")
	}
	if configurable, ok := prov.(interface {
		ConnectWithOptions(string, *domain.Credentials, ...httpclient.Option) error
	}); ok {
		err = configurable.ConnectWithOptions(p.Endpoint, creds, opts...)
	} else {
		err = prov.Connect(ctx, p.Endpoint, creds)
	}
	if err != nil {
		add("connectivity", classifyPermissionError(err), "provider connection failed")
		return checks
	}
	defer prov.Close()
	if err := prov.Health(ctx); err != nil {
		add("connectivity", classifyPermissionError(err), "provider health check failed")
		return checks
	}
	add("connectivity", permissionConfirmed, "provider responded to a read-only health check")
	if api, ok := prov.(interface{ APIVersion() string }); ok && api.APIVersion() != "" {
		add("version", permissionConfirmed, "provider API version reported")
	} else {
		add("version", permissionUnknown, "provider API version is not exposed")
	}
	if len(prov.Capabilities()) > 0 {
		add("capabilities", permissionConfirmed, "provider capabilities reported")
	} else {
		add("capabilities", permissionUnknown, "provider reported no capabilities")
	}
	if access, ok := prov.(domain.AccessProvider); ok {
		if _, err := access.Roles(ctx); err != nil {
			add("access.roles", classifyPermissionError(err), "role listing was not permitted")
		} else {
			add("access.roles", permissionConfirmed, "role listing permitted")
		}
		if _, err := access.ACL(ctx); err != nil {
			add("access.acl", classifyPermissionError(err), "ACL listing was not permitted")
		} else {
			add("access.acl", permissionConfirmed, "ACL listing permitted")
		}
	} else {
		add("access.roles", permissionUnsupported, "provider does not expose access diagnostics")
		add("access.acl", permissionUnsupported, "provider does not expose access diagnostics")
	}
	return checks
}

func classifyPermissionError(err error) string {
	if app.IsAuthError(err) || app.IsAuthorizationError(err) || strings.Contains(strings.ToLower(err.Error()), "permission denied") {
		return permissionMissing
	}
	return permissionUnknown
}

func writePermissionChecks(cmdCtx *Context, name string, checks []permissionCheck) error {
	type report struct {
		Profile string            `json:"profile" yaml:"profile"`
		Checks  []permissionCheck `json:"checks" yaml:"checks"`
	}
	r := report{Profile: name, Checks: checks}
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, r)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, r)
	default:
		rows := make([][]string, 0, len(checks))
		for _, c := range checks {
			rows = append(rows, []string{c.Name, c.Status, c.Detail})
		}
		return output.WriteTable(cmdCtx.Writer, []string{"CHECK", "STATUS", "DETAIL"}, rows)
	}
}
