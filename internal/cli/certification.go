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

func certificationArgs(args []string) (node, name, storage, ledger string, vmid int, err error) {
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
			return "", "", "", "", 0, fmt.Errorf("unknown certification argument %q", args[i])
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

func runCertification(ctx context.Context, cmdCtx *Context, args []string) error {
	if err := requireCertificationProfile(cmdCtx); err != nil {
		return err
	}
	node, name, storage, ledger, vmid, err := certificationArgs(args)
	if err != nil || node == "" || name == "" || storage == "" || vmid <= 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex --profile %s certification run --node <node> --vmid <id> --name nodex-cert-<name> --storage <storage> [--ledger <path>]", certification.RequiredProfile), app.ExitUsage)
	}
	prov, cleanup, err := connectProfile(ctx, cmdCtx, certification.RequiredProfile)
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := certification.Run(ctx, prov, certification.Request{Profile: certification.RequiredProfile, Node: node, VMID: vmid, Name: name, Storage: storage, ConfirmTarget: cmdCtx.Opts.ConfirmTarget, OptIn: cmdCtx.Opts.Yes}, ledger, now())
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
	prov, cleanup, err := connectProfile(ctx, cmdCtx, certification.RequiredProfile)
	if err != nil {
		return err
	}
	defer cleanup()
	res, err := certification.Cleanup(ctx, prov, ledger, cmdCtx.Opts.ConfirmTarget)
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
