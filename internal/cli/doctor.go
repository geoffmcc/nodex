package cli

import (
	"context"
	"fmt"
	"sync"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/config"
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
		for name, p := range cfg.Profiles {
			name, p := name, p
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := checkProfile(ctx, cmdCtx, name, p)
				mu.Lock()
				results = append(results, r)
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

func sortResults(results []checkResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Name > results[j].Name {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
