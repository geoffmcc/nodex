package maintenance

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

var simulatedInstallRe = regexp.MustCompile(`^Inst\s+(\S+)(?:\s+\[[^]]*\])?\s+\((\S+)\s+(\S+)(?:\s+\[([^]]+)\])?.*\)$`)

type aptPolicyEvidence struct {
	Item        string   `json:"item"`
	StdoutLines []string `json:"stdout_lines"`
	RC          int      `json:"rc"`
	Failed      bool     `json:"failed"`
}

type aptIndexTarget struct {
	Site         string
	Release      string
	Codename     string
	Origin       string
	Label        string
	Component    string
	Architecture string
}

func parsePackageIntents(simulation []string, policies []aptPolicyEvidence, indexTargetLines []string) ([]PackageIntent, error) {
	type candidate struct {
		name, version, suite, architecture string
	}
	candidates := make(map[string]candidate)
	for _, line := range simulation {
		line = strings.TrimSpace(line)
		match := simulatedInstallRe.FindStringSubmatch(line)
		if match == nil {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "security") || strings.Contains(lower, "esm") {
				return nil, fmt.Errorf("unparseable security archive simulation entry")
			}
			continue
		}
		if !isSecurityArchive(match[3]) {
			continue
		}
		name := strings.SplitN(match[1], ":", 2)[0]
		architecture := match[4]
		if architecture == "" {
			if parts := strings.SplitN(match[1], ":", 2); len(parts) == 2 {
				architecture = parts[1]
			}
		}
		if name == "" || match[2] == "" || architecture == "" {
			return nil, fmt.Errorf("candidate is missing package name, version, or architecture")
		}
		key := packageIntentKey(name, architecture)
		if _, duplicate := candidates[key]; duplicate {
			return nil, fmt.Errorf("duplicate simulated security package %s:%s", name, architecture)
		}
		candidates[key] = candidate{name: name, version: match[2], suite: match[3], architecture: architecture}
	}
	if len(candidates) == 0 && len(policies) == 0 {
		return nil, nil
	}
	if len(candidates) != len(policies) {
		return nil, fmt.Errorf("APT candidate policy evidence does not cover the simulated security package set")
	}
	policyByName := make(map[string]aptPolicyEvidence, len(policies))
	for _, policy := range policies {
		if policy.Failed || policy.RC != 0 {
			return nil, fmt.Errorf("apt-cache policy failed")
		}
		itemParts := strings.SplitN(policy.Item, ":", 2)
		name := itemParts[0]
		key := ""
		if len(itemParts) == 2 {
			key = packageIntentKey(name, itemParts[1])
		} else {
			for candidateKey, candidate := range candidates {
				if candidate.name == name {
					if key != "" {
						return nil, fmt.Errorf("APT candidate policy omits architecture for multiarch package %s", name)
					}
					key = candidateKey
				}
			}
		}
		if _, exists := candidates[key]; !exists || policyByName[key].Item != "" {
			return nil, fmt.Errorf("APT candidate policy evidence is duplicate or unrequested")
		}
		policyByName[key] = policy
	}
	indexTargets, err := parseAPTIndexTargets(indexTargetLines)
	if err != nil {
		return nil, err
	}
	intents := make([]PackageIntent, 0, len(candidates))
	for key, candidate := range candidates {
		policy, ok := policyByName[key]
		if !ok {
			return nil, fmt.Errorf("APT candidate policy is missing for %s:%s", candidate.name, candidate.architecture)
		}
		version, sources, err := parseAPTPolicy(policy.StdoutLines, candidate.architecture, candidate.suite)
		if err != nil || version != candidate.version || len(sources) == 0 {
			return nil, fmt.Errorf("candidate version or source for %s:%s changed or could not be established", candidate.name, candidate.architecture)
		}
		origins, err := matchAPTOrigins(sources, indexTargets, candidate.architecture)
		if err != nil || len(origins) == 0 {
			return nil, fmt.Errorf("signed APT Release origin for %s:%s could not be established", candidate.name, candidate.architecture)
		}
		intents = append(intents, PackageIntent{Name: candidate.name, Version: candidate.version, Architecture: candidate.architecture, Origins: origins})
	}
	sortPackageIntents(intents)
	return intents, nil
}

func isSecurityArchive(archive string) bool {
	lower := strings.ToLower(archive)
	return strings.Contains(lower, "-security") || strings.Contains(lower, "esm")
}

func packageIntentKey(name, architecture string) string { return name + "\x00" + architecture }

func parseAPTPolicy(lines []string, architecture, expectedSuite string) (string, []string, error) {
	candidate := ""
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Candidate:") {
			candidate = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "Candidate:"))
			break
		}
	}
	if candidate == "" || candidate == "(none)" {
		return "", nil, fmt.Errorf("candidate version is unavailable")
	}
	currentVersion := ""
	collect := false
	var origins []string
	for _, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && (fields[0] == "***" || isDebianVersion(fields[0])) && isNumeric(fields[len(fields)-1]) {
			version := fields[0]
			if version == "***" && len(fields) >= 3 {
				version = fields[1]
			}
			currentVersion = version
			collect = version == candidate
			continue
		}
		if !collect || len(fields) < 5 || fields[len(fields)-1] != "Packages" {
			continue
		}
		if _, err := fmt.Sscanf(fields[0], "%d", new(int)); err != nil {
			continue
		}
		if fields[1] == "/var/lib/dpkg/status" || fields[3] != architecture {
			continue
		}
		sourceURL, parseErr := url.Parse(fields[1])
		if parseErr != nil || sourceURL.User != nil || sourceURL.RawQuery != "" || sourceURL.Fragment != "" {
			return "", nil, fmt.Errorf("APT source contains embedded credentials or unsupported URL data")
		}
		origin := strings.Join(fields[1:4], " ")
		suite := expectedSuite
		if at := strings.LastIndexByte(suite, '/'); at >= 0 {
			suite = suite[at+1:]
		}
		if at := strings.IndexByte(suite, ':'); at >= 0 {
			suite = suite[at+1:]
		}
		if !strings.Contains(origin, ":") || suite == "" || !strings.Contains(strings.ToLower(origin), strings.ToLower(suite)) {
			continue
		}
		origins = append(origins, origin)
	}
	if currentVersion == "" {
		return "", nil, fmt.Errorf("candidate source table is missing")
	}
	return candidate, uniqueSorted(origins), nil
}

func parseAPTIndexTargets(lines []string) ([]aptIndexTarget, error) {
	var targets []aptIndexTarget
	for _, line := range lines {
		fields := strings.Split(line, "|")
		if len(fields) != 7 {
			return nil, fmt.Errorf("signed APT Release identity has an unexpected shape")
		}
		target := aptIndexTarget{
			Site: fields[0], Release: fields[1], Codename: fields[2], Origin: fields[3],
			Label: fields[4], Component: fields[5], Architecture: fields[6],
		}
		if target.Site == "" || target.Release == "" || target.Origin == "" || target.Component == "" || target.Architecture == "" {
			return nil, fmt.Errorf("signed APT Release identity is incomplete")
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func matchAPTOrigins(sources []string, targets []aptIndexTarget, architecture string) ([]APTOrigin, error) {
	var origins []APTOrigin
	for _, source := range sources {
		fields := strings.Fields(source)
		if len(fields) != 3 {
			return nil, fmt.Errorf("APT candidate source is malformed")
		}
		suiteComponent := strings.SplitN(fields[1], "/", 2)
		if len(suiteComponent) != 2 {
			return nil, fmt.Errorf("APT candidate source has no archive component")
		}
		matched := false
		for _, target := range targets {
			if target.Site != fields[0] || target.Release != suiteComponent[0] || target.Component != suiteComponent[1] || target.Architecture != architecture {
				continue
			}
			siteURL, parseErr := url.Parse(target.Site)
			if parseErr != nil || siteURL.User != nil || siteURL.RawQuery != "" || siteURL.Fragment != "" {
				return nil, fmt.Errorf("APT site contains embedded credentials or unsupported URL data")
			}
			matched = true
			origins = append(origins, APTOrigin{
				Source: source, Site: target.Site, Release: target.Release, Codename: target.Codename,
				Origin: target.Origin, Label: target.Label, Component: target.Component, Architecture: target.Architecture,
			})
		}
		if !matched {
			return nil, fmt.Errorf("candidate source is missing signed Release identity")
		}
	}
	seen := make(map[string]bool, len(origins))
	unique := origins[:0]
	for _, origin := range origins {
		if !seen[origin.Record()] {
			seen[origin.Record()] = true
			unique = append(unique, origin)
		}
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i].Record() < unique[j].Record() })
	return unique, nil
}

func isNumeric(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isDebianVersion(value string) bool {
	return value != "" && (value[0] >= '0' && value[0] <= '9' || strings.ContainsAny(value, ":~.+-"))
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
