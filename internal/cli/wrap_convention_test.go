// Package cli_test contains cross-layer invariant tests.
package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// wrapLiteralRe captures the operation name in a `fmt.Errorf("<op>: %w", err)`
// wrap. Only literal, non-interpolated operation names are considered: a wrap
// built with a format variable names its operation at runtime and cannot be
// compared by source text.
var wrapLiteralRe = regexp.MustCompile(`fmt\.Errorf\(\s*"([^"%\\]*(?:\\.[^"%\\]*)*):\s*%w"`)

// collectWrapLiterals maps each operation name to the files that wrap with it.
func collectWrapLiterals(t *testing.T, dir string) map[string]map[string]bool {
	t.Helper()
	// Collect first, then read. Reading inside the Walk callback is a
	// symlink TOCTOU pattern (gosec G122) and is needless here: the file list
	// is small and the walk has already pinned down every path.
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}

	out := make(map[string]map[string]bool)
	for _, path := range files {
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		for _, m := range wrapLiteralRe.FindAllStringSubmatch(string(src), -1) {
			op := m[1]
			if out[op] == nil {
				out[op] = make(map[string]bool)
			}
			out[op][path] = true
		}
	}
	return out
}

// benignCollisions are operation names that appear in both layers yet cannot
// produce a doubled prefix, because the CLI site calls a provider method that
// wraps something else. Every entry records why it is safe, and
// TestNoStaleWrapExemptions fails if an entry stops colliding, so the list
// cannot silently rot.
var benignCollisions = map[string]string{
	"get cluster status": "cli/resource.go runClusterStatus calls Provider.Cluster(), which " +
		"wraps \"list nodes\"; the CLI literal names what the user asked for " +
		"(nodex cluster status), not the API call, so nothing is doubled",
}

// TestNoStaleWrapExemptions keeps benignCollisions honest: an exemption whose
// collision has disappeared is dead weight and must be removed.
func TestNoStaleWrapExemptions(t *testing.T) {
	cli := collectWrapLiterals(t, filepath.Join("..", "..", "internal", "cli"))
	prov := collectWrapLiterals(t, filepath.Join("..", "..", "internal", "provider", "proxmox"))
	for op := range benignCollisions {
		if len(cli[op]) == 0 || len(prov[op]) == 0 {
			t.Errorf("exemption %q no longer collides between layers; delete it from benignCollisions", op)
		}
	}
}

// TestNoDuplicatedWrapLiteralsAcrossLayers guards the convention that exactly
// one layer names an operation: the provider says which API call failed, the
// CLI says what the user asked for, and the two never use the same words.
//
// When both layers used the same literal the error was visibly doubled —
// `get vm config: get vm config: provider error: ...` — and the user saw the
// operation name twice while learning nothing extra. A string-set intersection
// catches every such case at once, present and future.
//
// This is a source-level check on purpose: the duplication is a property of
// two independently compiling packages, so no runtime test can see it.
func TestNoDuplicatedWrapLiteralsAcrossLayers(t *testing.T) {
	cli := collectWrapLiterals(t, filepath.Join("..", "..", "internal", "cli"))
	prov := collectWrapLiterals(t, filepath.Join("..", "..", "internal", "provider", "proxmox"))

	var collisions []string
	for op, provFiles := range prov {
		cliFiles := cli[op]
		if len(cliFiles) == 0 || len(provFiles) == 0 {
			continue
		}
		if _, exempt := benignCollisions[op]; exempt {
			continue
		}
		cl := make([]string, 0, len(cliFiles))
		for f := range cliFiles {
			cl = append(cl, filepath.Base(f))
		}
		pf := make([]string, 0, len(provFiles))
		for f := range provFiles {
			pf = append(pf, filepath.Base(f))
		}
		collisions = append(collisions, op+" (cli: "+strings.Join(cl, ",")+" | proxmox: "+strings.Join(pf, ",")+")")
	}

	if len(collisions) > 0 {
		t.Errorf("%d operation(s) are named by both the CLI and the provider layer, "+
			"so the error prefix is emitted twice: %s",
			len(collisions), strings.Join(collisions, "; "))
	}
}

// TestProviderNamesItsOperations is the positive half of the convention: the
// provider layer is the one that keeps naming operations, so the CLI can drop
// its duplicate without the error becoming context-free. Without this, a
// future "fix" could strip both layers and satisfy the test above.
func TestProviderNamesItsOperations(t *testing.T) {
	prov := collectWrapLiterals(t, filepath.Join("..", "..", "internal", "provider", "proxmox"))
	if len(prov) == 0 {
		t.Fatal("no provider operation names found; the provider layer must keep naming the API call that failed")
	}
}

// TestNoDoubledPrefixInWrapLiterals catches the degenerate form directly: a
// literal that already repeats its own operation, e.g. "get vm config: get vm
// config: %w" left behind by a partial edit.
func TestNoDoubledPrefixInWrapLiterals(t *testing.T) {
	for _, dir := range []string{
		filepath.Join("..", "..", "internal", "cli"),
		filepath.Join("..", "..", "internal", "provider", "proxmox"),
	} {
		ops := collectWrapLiterals(t, dir)
		for op := range ops {
			half := len(op) / 2
			if half >= 4 && op[:half] == op[half:] {
				t.Errorf("literal %q repeats itself; the prefix would be emitted twice", op)
			}
		}
	}
}
