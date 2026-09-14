package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/certification"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/output"
	"github.com/geoffmcc/nodex/internal/transport/httpclient"
)

func certificationLedgerPath(args []string) (string, []string, error) {
	path, rest := "", make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--ledger" {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", nil, fmt.Errorf("--ledger requires a path")
			}
			path, i = args[i+1], i+1
		} else {
			rest = append(rest, args[i])
		}
	}
	if path == "" {
		dir, err := config.Dir()
		if err != nil {
			return "", nil, err
		}
		path = filepath.Join(dir, "certification-ledger.json")
	}
	return path, rest, nil
}

func certificationArgs(args []string) (environment, suite, node, name, storage, ledger string, vmid int, err error) {
	ledger, args, err = certificationLedgerPath(args)
	if err != nil {
		return
	}
	for i := 0; i < len(args); i++ {
		value := func() (string, error) {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", fmt.Errorf("%s requires a value", args[i])
			}
			i++
			return args[i], nil
		}
		switch args[i] {
		case "--environment":
			environment, err = value()
		case "--suite":
			suite, err = value()
		case "--node":
			node, err = value()
		case "--name":
			name, err = value()
		case "--storage":
			storage, err = value()
		case "--vmid":
			var s string
			s, err = value()
			if err == nil {
				vmid, err = strconv.Atoi(s)
			}
		default:
			return "", "", "", "", "", "", 0, fmt.Errorf("unknown certification argument %q", args[i])
		}
		if err != nil {
			return
		}
	}
	return
}

func requireCertificationProfile(cmdCtx *Context) error {
	if cmdCtx.Opts.Profile != certification.RequiredProfile {
		return app.NewExitError(fmt.Errorf("certification requires explicit --profile %s", certification.RequiredProfile), app.ExitUsage)
	}
	return nil
}

func certificationAuthorization(cfg *config.Config, environment string) (*certification.Authorization, string, error) {
	env, ok := cfg.Certifications[environment]
	if !ok {
		return nil, "", fmt.Errorf("certification environment %q is not configured", environment)
	}
	profile, ok := cfg.Profiles[env.Profile]
	if !ok {
		return nil, "", fmt.Errorf("certification profile %q is not configured", env.Profile)
	}
	return &certification.Authorization{Environment: environment, Profile: env.Profile, Endpoint: env.Endpoint, Provider: env.Provider, ExpectedFingerprint: env.ExpectedFingerprint, TrustedCAIdentity: env.TrustedCAIdentity, Nodes: env.Nodes, Storage: env.Storage, VMIDMin: env.VMIDMin, VMIDMax: env.VMIDMax, Suites: env.Suites, AllowMutations: env.AllowMutations, MaxResources: env.MaxResources, ExpiresAt: env.ExpiresAt}, profile.CAFile, nil
}

func runCertification(ctx context.Context, cmdCtx *Context, args []string) error {
	if err := requireCertificationProfile(cmdCtx); err != nil {
		return err
	}
	environment, suite, node, name, storage, ledger, vmid, err := certificationArgs(args)
	if err != nil || environment == "" || suite == "" || node == "" || name == "" || storage == "" || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex --profile %s certification run --environment <name> --suite <readonly|disposable-mutations> --node <node> --vmid <id> --name nodex-cert-<name> --storage <storage> [--ledger <path>]", certification.RequiredProfile), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	auth, caFile, err := certificationAuthorization(cfg, environment)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	prov, cleanup, err := connectProfileWithOptions(ctx, cmdCtx, certification.RequiredProfile, httpclient.WithLeafCertificateFingerprint(auth.ExpectedFingerprint))
	if err != nil {
		return err
	}
	defer cleanup()
	request := certification.Request{Profile: certification.RequiredProfile, Environment: environment, Suite: suite, Endpoint: auth.Endpoint, CAFile: caFile, Node: node, VMID: vmid, Name: name, Storage: storage, ConfirmTarget: cmdCtx.Opts.ConfirmTarget, OptIn: cmdCtx.Opts.Yes, Authorization: auth}
	res, err := certification.Run(ctx, prov, request, ledger, now())
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	return writeCertification(cmdCtx, res)
}

func runCertificationCleanup(ctx context.Context, cmdCtx *Context, args []string) error {
	if err := requireCertificationProfile(cmdCtx); err != nil {
		return err
	}
	ledger, rest, err := certificationLedgerPath(args)
	if err != nil || len(rest) != 0 || !cmdCtx.Opts.Yes {
		return app.NewExitError(fmt.Errorf("usage: nodex --profile %s --yes --confirm-target <ledger-entry-id> certification cleanup [--ledger <path>]", certification.RequiredProfile), app.ExitUsage)
	}
	cfg, err := config.Read()
	if err != nil {
		return err
	}
	loaded, err := certification.Load(ledger)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	var target *certification.Entry
	for i := range loaded.Entries {
		if loaded.Entries[i].ID == cmdCtx.Opts.ConfirmTarget {
			target = &loaded.Entries[i]
			break
		}
	}
	if target == nil || target.Environment == "" {
		return app.NewExitError(fmt.Errorf("ledger entry is not bound to a configured certification environment"), app.ExitConflict)
	}
	auth, caFile, err := certificationAuthorization(cfg, target.Environment)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	if auth.Profile != certification.RequiredProfile {
		return app.NewExitError(fmt.Errorf("certification environment must use explicit profile %s", certification.RequiredProfile), app.ExitValidationError)
	}
	prov, cleanup, err := connectProfileWithOptions(ctx, cmdCtx, certification.RequiredProfile, httpclient.WithLeafCertificateFingerprint(auth.ExpectedFingerprint))
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := certification.Cleanup(ctx, prov, ledger, cmdCtx.Opts.ConfirmTarget, auth, caFile)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	return writeCertification(cmdCtx, res)
}

func runCertificationReport(_ context.Context, cmdCtx *Context, args []string) error {
	ledger, rest, err := certificationLedgerPath(args)
	if err != nil || len(rest) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex certification report [--ledger <path>]"), app.ExitUsage)
	}
	l, err := certification.Load(ledger)
	if err != nil {
		return app.NewExitError(err, app.ExitValidationError)
	}
	return writeCertification(cmdCtx, l)
}

func writeCertification(cmdCtx *Context, value any) error {
	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		return output.WriteJSON(cmdCtx.Writer, value)
	case output.FormatYAML:
		return output.WriteYAML(cmdCtx.Writer, value)
	}
	fmt.Fprintf(cmdCtx.Writer, "%v\n", value)
	return nil
}

var now = func() time.Time { return time.Now() }
