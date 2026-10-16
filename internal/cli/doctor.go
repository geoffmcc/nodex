package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
	"github.com/geoffmcc/nodex/internal/credentials"
	"github.com/geoffmcc/nodex/internal/output"
)

// checkResult holds the result of a single doctor check.
type checkResult struct {
	Name    string `json:"name" yaml:"name"`
	Status  string `json:"status" yaml:"status"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

func runDoctor(ctx context.Context, cmdCtx *Context, args []string) error {
	if len(args) != 0 {
		return app.NewExitError(fmt.Errorf("usage: nodex doctor"), app.ExitUsage)
	}
	var results []checkResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Run local checks.
	wg.Add(1)
	go func() {
		defer wg.Done()
		r := checkConfig()
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	}()

	// Run profile connectivity checks.
	cfg, err := config.Read()
	if err == nil {
		for _, name := range doctorProfileNames(cfg, cmdCtx.Opts.Profile) {
			p, ok := cfg.Profiles[name]
			if !ok {
				mu.Lock()
				results = append(results, checkResult{Name: "profile " + name, Status: "fail", Message: "selected profile not found"})
				mu.Unlock()
				continue
			}
			name, p := name, p
			wg.Add(1)
			go func() {
				defer wg.Done()
				var rows []checkResult
				if r, ok := checkCredentialFile(name, p, ""); ok {
					rows = append(rows, r)
				}
				if r, ok := checkSSHKey(name, p); ok {
					rows = append(rows, r)
				}
				if r, ok := checkKnownHosts(name, p); ok {
					rows = append(rows, r)
				}
				rows = append(rows, checkProfile(ctx, cmdCtx, name, p))
				mu.Lock()
				results = append(results, rows...)
				mu.Unlock()
			}()
		}
	}

	wg.Wait()

	// Sort results by name for consistent output.
	sortResults(results)

	// Count statuses.
	pass, fail, warn := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "warn":
			warn++
		}
	}

	switch cmdCtx.Opts.Output {
	case output.FormatJSON:
		type doctorReport struct {
			Pass    int           `json:"pass" yaml:"pass"`
			Fail    int           `json:"fail" yaml:"fail"`
			Warn    int           `json:"warn" yaml:"warn"`
			Results []checkResult `json:"results" yaml:"results"`
		}
		if err := output.WriteJSON(cmdCtx.Writer, doctorReport{
			Pass:    pass,
			Fail:    fail,
			Warn:    warn,
			Results: results,
		}); err != nil {
			return err
		}
		return doctorExitError(fail)

	case output.FormatYAML:
		type doctorReport struct {
			Pass    int           `json:"pass" yaml:"pass"`
			Fail    int           `json:"fail" yaml:"fail"`
			Warn    int           `json:"warn" yaml:"warn"`
			Results []checkResult `json:"results" yaml:"results"`
		}
		if err := output.WriteYAML(cmdCtx.Writer, doctorReport{
			Pass:    pass,
			Fail:    fail,
			Warn:    warn,
			Results: results,
		}); err != nil {
			return err
		}
		return doctorExitError(fail)

	default:
		headers := []string{"CHECK", "STATUS", "MESSAGE"}
		rows := make([][]string, 0, len(results))
		for _, r := range results {
			status := r.Status
			switch status {
			case "pass":
				status = "OK"
			case "fail":
				status = "FAIL"
			case "warn":
				status = "WARN"
			}
			rows = append(rows, []string{r.Name, status, r.Message})
		}
		if err := output.WriteTable(cmdCtx.Writer, headers, rows); err != nil {
			return err
		}
		fmt.Fprintf(cmdCtx.Writer, "\n%d passed, %d failed, %d warnings\n", pass, fail, warn)
		return doctorExitError(fail)
	}
}

func doctorProfileNames(cfg *config.Config, selected string) []string {
	if selected != "" {
		return []string{selected}
	}
	return config.ProfileNames(cfg)
}

func doctorExitError(fail int) error {
	if fail > 0 {
		return fmt.Errorf("doctor found %d issue(s)", fail)
	}
	return nil
}

func checkConfig() checkResult {
	cfg, err := config.Read()
	if err != nil {
		return checkResult{Name: "config", Status: "fail", Message: err.Error()}
	}
	if cfg == nil {
		return checkResult{Name: "config", Status: "fail", Message: "config is nil"}
	}
	return checkResult{Name: "config", Status: "pass", Message: fmt.Sprintf("schema v%d", cfg.Version)}
}

// checkCredentialFile reports on the availability and permissions of the
// file-backend credential for a profile. Other credential backends are outside
// this filesystem check. credDir may be empty to use the default directory.
//
// Unsafe file-backed credentials fail closed because the resolver refuses to
// use them. The message supplies remediation rather than treating the file as
// usable after merely warning.
func checkCredentialFile(profileName string, p config.Profile, credDir string) (checkResult, bool) {
	ref := p.CredentialRef
	if ref == "" {
		// Resolve() checks the profile-named file after trying environment
		// credentials, so include it when it exists as a possible fallback.
		ref = profileName
	}
	backend, name, err := credentials.ParseCredentialRefStrict(ref)
	if err != nil || backend != "file" {
		return checkResult{}, false
	}
	path, err := credentials.CredentialFilePath(credDir, name)
	if err != nil {
		return checkResult{
			Name:    fmt.Sprintf("credentials/%s", profileName),
			Status:  "fail",
			Message: fmt.Sprintf("invalid credential reference: %v", err),
		}, true
	}
	if err := credentials.CheckSecretFilePermissions(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) && p.CredentialRef == "" {
			return checkResult{}, false
		}
		message := err.Error()
		if errors.Is(err, fs.ErrNotExist) {
			message = fmt.Sprintf("credential file %s is missing", path)
		}
		return checkResult{Name: fmt.Sprintf("credentials/%s", profileName), Status: "fail", Message: message}, true
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) && p.CredentialRef == "" {
			return checkResult{}, false
		}
		message := fmt.Sprintf("cannot inspect %s: %v", path, err)
		if errors.Is(err, fs.ErrNotExist) {
			message = fmt.Sprintf("credential file %s is missing", path)
		}
		return checkResult{
			Name:    fmt.Sprintf("credentials/%s", profileName),
			Status:  "fail",
			Message: message,
		}, true
	}
	if info.IsDir() {
		return checkResult{
			Name:    fmt.Sprintf("credentials/%s", profileName),
			Status:  "fail",
			Message: fmt.Sprintf("credential path %s is a directory", path),
		}, true
	}
	message := fmt.Sprintf("permissions %04o; no group/other access", info.Mode().Perm())
	if runtime.GOOS == "windows" {
		message = "present; ACL is restricted to the current user and trusted administrators"
	}
	return checkResult{
		Name:    fmt.Sprintf("credentials/%s", profileName),
		Status:  "pass",
		Message: message,
	}, true
}

// checkSSHKey reports on the presence, readability, and permissions of a
// profile's SSH key without reading or parsing key material. A profile with no
// key configured returns ok=false so it produces no row.
func checkSSHKey(profileName string, p config.Profile) (checkResult, bool) {
	if p.SSHKeyFile == "" {
		return checkResult{}, false
	}
	resultName := fmt.Sprintf("profile/%s/ssh-key", profileName)
	if err := credentials.CheckSecretFilePermissions(p.SSHKeyFile); err != nil {
		message := err.Error()
		if errors.Is(err, fs.ErrNotExist) {
			message = fmt.Sprintf("SSH key %s is missing", p.SSHKeyFile)
		}
		return checkResult{Name: resultName, Status: "fail", Message: message}, true
	}
	info, err := os.Stat(p.SSHKeyFile)
	if err != nil {
		message := fmt.Sprintf("cannot inspect %s: %v", p.SSHKeyFile, err)
		if errors.Is(err, fs.ErrNotExist) {
			message = fmt.Sprintf("SSH key %s is missing", p.SSHKeyFile)
		}
		return checkResult{
			Name:    resultName,
			Status:  "fail",
			Message: message,
		}, true
	}
	if info.IsDir() {
		return checkResult{Name: resultName, Status: "fail", Message: fmt.Sprintf("%s is a directory", p.SSHKeyFile)}, true
	}
	message := fmt.Sprintf("present; permissions %04o, no group/other access", info.Mode().Perm())
	if runtime.GOOS == "windows" {
		message = "present; ACL is restricted to the current user and trusted administrators"
	}
	return checkResult{Name: resultName, Status: "pass", Message: message}, true
}

// checkKnownHosts reports whether the OpenSSH trust database used by SFTP is
// present and readable. It deliberately does not attempt a network connection
// or claim that this profile's host has a matching entry; the SSH handshake
// performs that cryptographic check.
func checkKnownHosts(profileName string, p config.Profile) (checkResult, bool) {
	if p.SSHKeyFile == "" {
		return checkResult{}, false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return checkResult{Name: fmt.Sprintf("profile/%s/ssh-known-hosts", profileName), Status: "fail", Message: fmt.Sprintf("resolve home directory: %v", err)}, true
	}
	return checkKnownHostsFile(profileName, filepath.Join(home, ".ssh", "known_hosts"))
}

func checkKnownHostsFile(profileName, path string) (checkResult, bool) {
	name := fmt.Sprintf("profile/%s/ssh-known-hosts", profileName)
	info, err := os.Stat(path)
	if err != nil {
		message := fmt.Sprintf("cannot inspect %s: %v", path, err)
		if errors.Is(err, fs.ErrNotExist) {
			message = fmt.Sprintf("SSH known_hosts file %s is missing", path)
		}
		return checkResult{Name: name, Status: "fail", Message: message}, true
	}
	if info.IsDir() {
		return checkResult{Name: name, Status: "fail", Message: fmt.Sprintf("%s is a directory", path)}, true
	}
	f, err := os.Open(path) // #nosec G304 -- this path is derived from the current user's home directory and opened only to confirm readability.
	if err != nil {
		return checkResult{Name: name, Status: "fail", Message: fmt.Sprintf("cannot read SSH known_hosts file %s: %v", path, err)}, true
	}
	if err := f.Close(); err != nil {
		return checkResult{Name: name, Status: "fail", Message: fmt.Sprintf("cannot close SSH known_hosts file %s: %v", path, err)}, true
	}
	return checkResult{Name: name, Status: "pass", Message: "present and readable; host key is checked during SFTP handshake"}, true
}

func checkProfile(ctx context.Context, cmdCtx *Context, name string, p config.Profile) checkResult {
	if p.Endpoint == "" {
		return checkResult{
			Name:    fmt.Sprintf("profile/%s", name),
			Status:  "warn",
			Message: "no endpoint configured",
		}
	}

	prov, cleanup, err := connectProfile(ctx, cmdCtx, name)
	if err != nil {
		return checkResult{
			Name:    fmt.Sprintf("profile/%s", name),
			Status:  "fail",
			Message: err.Error(),
		}
	}
	defer cleanup()

	// Use the provider's Health method as a connectivity check.
	if err := prov.Health(ctx); err != nil {
		return checkResult{
			Name:    fmt.Sprintf("profile/%s", name),
			Status:  "fail",
			Message: err.Error(),
		}
	}

	return checkResult{
		Name:    fmt.Sprintf("profile/%s", name),
		Status:  "pass",
		Message: p.Endpoint,
	}
}

// sortResults orders results by name.
//
//nolint:gosec // G602 is a false positive: the inner loop starts at i+1 and both loops are bounded by len(results), so every index is in range.
func sortResults(results []checkResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Name > results[j].Name {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
