package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/geoffmcc/nodex/internal/app"
	"github.com/geoffmcc/nodex/internal/domain"
	"github.com/geoffmcc/nodex/internal/output"
)

// --- Synchronous config-update verification ---
//
// Some Proxmox config endpoints apply the change inline and return no task
// identifier. Demanding a task there produced a receipt that reported the
// outcome as unknown and recommended a reconcile step that could never
// succeed, even though the change had already been applied.
//
// For those endpoints the postcondition is read back and compared field by
// field. A changed digest alone is insufficient: the requested values are
// compared individually so a partial application is reported as a failure
// rather than as success.

// configFieldMismatch records a requested key whose readback value differs.
type configFieldMismatch struct {
	Key      string
	Request  string
	Observed string
	// Missing names the requested properties the readback did not satisfy. It is
	// set only for a property list, where the whole-value strings are otherwise
	// left to the reader to diff.
	Missing string
}

func (m configFieldMismatch) String() string {
	if m.Missing != "" {
		return fmt.Sprintf("%s: requested %q, observed %q (unsatisfied: %s)", m.Key, m.Request, m.Observed, m.Missing)
	}
	return fmt.Sprintf("%s: requested %q, observed %q", m.Key, m.Request, m.Observed)
}

// canonicalConfigValue renders a provider config value as a comparable string.
// Readback is typed (ints, bools, strings) while requests always arrive as
// strings from the command line, so both sides are normalized here. Numbers
// are compared numerically so "2" and 2 are the same setting.
func canonicalConfigValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case float32:
		return trimFloat(float64(t))
	case float64:
		return trimFloat(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func trimFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// configProperty is one element of a comma-separated provider value, such as a
// volume descriptor ("local:iso/x.iso,media=cdrom,size=1666190K").
//
// key is empty for the bare leading element that names the volume.
type configProperty struct {
	key   string
	value string
}

// parseConfigProperties splits a comma-separated provider value into its
// elements.
//
// A comma-less value that names a volume ("local:iso/x.iso",
// "local-lvm:vm-100-disk-0") counts as a single-element list, because that is
// what it is: the same descriptor before the provider appends properties to it.
// Any other comma-less value reports ok=false and compares as a plain string,
// which keeps ordinary settings such as a name or a core count out of this path
// even when they happen to contain punctuation.
func parseConfigProperties(value string) ([]configProperty, bool) {
	if !strings.Contains(value, ",") {
		if !strings.Contains(value, ":") || strings.Contains(value, "=") {
			return nil, false
		}
		return []configProperty{{value: value}}, true
	}
	parts := strings.Split(value, ",")
	properties := make([]configProperty, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		key, propertyValue, hasKey := strings.Cut(part, "=")
		if !hasKey {
			properties = append(properties, configProperty{value: part})
			continue
		}
		properties = append(properties, configProperty{key: strings.TrimSpace(key), value: strings.TrimSpace(propertyValue)})
	}
	return properties, true
}

// propertiesSatisfied reports whether every requested element appears in the
// observed list with the same key and value.
//
// Elements observed but not requested are ignored deliberately. A provider
// normalises what it stores — an ISO volume comes back carrying its byte size,
// a NIC comes back carrying a bridge tag — and that normalisation is not a
// change the caller made. Comparing the whole string literally instead reported
// a successful create as a verification failure, which invites a caller to retry
// a create that already succeeded and collide on the VMID.
//
// The guarantee that a partial application is reported as a failure is kept: every
// requested element must still be present with the value that was asked for.
func propertiesSatisfied(observed, requested []configProperty) bool {
	for _, want := range requested {
		found := false
		for _, got := range observed {
			if got.key == want.key && got.value == want.value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// missingProperties lists the requested elements the readback does not satisfy,
// so the mismatch names the property that is actually wrong instead of printing
// two whole descriptor strings and leaving the reader to diff them.
func missingProperties(observed, requested []configProperty) string {
	missing := make([]string, 0, len(requested))
	for _, want := range requested {
		found := false
		for _, got := range observed {
			if got.key == want.key && got.value == want.value {
				found = true
				break
			}
		}
		if found {
			continue
		}
		if want.key == "" {
			missing = append(missing, want.value)
			continue
		}
		missing = append(missing, want.key+"="+want.value)
	}
	return strings.Join(missing, ", ")
}

// cloudInitVolumeMatches accepts Proxmox's canonical readback for the
// storage:cloudinit shorthand. Proxmox expands it to a VM-specific volume and
// adds media=cdrom when the config is read back.
func cloudInitVolumeMatches(observed, requested string) bool {
	requestedStorage, requestedVolume, ok := strings.Cut(requested, ":")
	if !ok || requestedStorage == "" || requestedVolume != "cloudinit" {
		return false
	}
	observedProperties, ok := parseConfigProperties(observed)
	if !ok || len(observedProperties) < 2 {
		return false
	}
	volumeStorage, volumeName, ok := strings.Cut(observedProperties[0].value, ":")
	if !ok || volumeStorage != requestedStorage || !strings.HasPrefix(volumeName, "vm-") || !strings.HasSuffix(volumeName, "-cloudinit") {
		return false
	}
	vmid := strings.TrimSuffix(strings.TrimPrefix(volumeName, "vm-"), "-cloudinit")
	if _, err := strconv.Atoi(vmid); err != nil {
		return false
	}
	mediaIsCDROM := false
	for _, property := range observedProperties[1:] {
		if property.key == "media" && property.value == "cdrom" {
			mediaIsCDROM = true
		}
	}
	return mediaIsCDROM
}

// verifyConfigReadback compares each requested key against the provider's
// current config. It returns whether every requested field matched.
//
// A key absent from the readback is reported as a mismatch rather than
// skipped: the provider omitting a field is not evidence the field was set.
func verifyConfigReadback(observed map[string]interface{}, requested map[string]string) (bool, []configFieldMismatch) {
	mismatches := make([]configFieldMismatch, 0, len(requested))
	for key, want := range requested {
		raw, present := observed[key]
		got := ""
		if present {
			got = canonicalConfigValue(raw)
		}
		wantCanonical := strings.TrimSpace(want)
		// Compare numerically when both sides are numeric so that a request
		// of "2" matches a provider value of 2, and "08" matches 8.
		if n1, err1 := strconv.ParseFloat(wantCanonical, 64); err1 == nil {
			if n2, err2 := strconv.ParseFloat(got, 64); err2 == nil && n1 == n2 {
				continue
			}
		}
		if key == "net0" && present {
			requestedProperties, requestedIsList := parseConfigProperties(wantCanonical)
			observedProperties, observedIsList := parseConfigProperties(got)
			if requestedIsList && observedIsList && propertiesSatisfied(observedProperties, requestedProperties) {
				continue
			}
			if qemuNetworkInterfaceMatches(got, wantCanonical) {
				continue
			}
			mismatches = append(mismatches, configFieldMismatch{Key: key, Request: wantCanonical, Observed: got})
			continue
		}
		if present && cloudInitVolumeMatches(got, wantCanonical) {
			continue
		}
		// Compare a property list by the properties asked for, so a
		// provider-normalised element does not read as a change.
		if requestedProperties, isList := parseConfigProperties(wantCanonical); isList && present {
			if observedProperties, observedIsList := parseConfigProperties(got); observedIsList {
				if propertiesSatisfied(observedProperties, requestedProperties) {
					continue
				}
				mismatches = append(mismatches, configFieldMismatch{
					Key:      key,
					Request:  wantCanonical,
					Observed: got,
					Missing:  missingProperties(observedProperties, requestedProperties),
				})
				continue
			}
		}
		if !present || got != wantCanonical {
			if !present {
				got = ""
			}
			mismatches = append(mismatches, configFieldMismatch{Key: key, Request: wantCanonical, Observed: got})
		}
	}
	return len(mismatches) == 0, mismatches
}

// qemuNetworkInterfaceMatches allows Proxmox's readback to include the
// automatically assigned MAC address after a model name such as "virtio".
// Explicitly requested MACs and all remaining requested properties must
// still match exactly.
func qemuNetworkInterfaceMatches(observed, requested string) bool {
	observedParts := strings.Split(observed, ",")
	requestedParts := strings.Split(requested, ",")
	if len(observedParts) == 0 || len(requestedParts) == 0 {
		return false
	}
	observedModel, observedMAC, observedHasMAC := strings.Cut(strings.TrimSpace(observedParts[0]), "=")
	requestedModel, requestedMAC, requestedHasMAC := strings.Cut(strings.TrimSpace(requestedParts[0]), "=")
	if observedModel != requestedModel {
		return false
	}
	if requestedHasMAC && (!observedHasMAC || observedMAC != requestedMAC) {
		return false
	}
	if len(requestedParts) == 1 {
		return true
	}
	observedProperties, ok := parseNetworkProperties(strings.Join(observedParts[1:], ","))
	if !ok {
		return false
	}
	requestedProperties, ok := parseNetworkProperties(strings.Join(requestedParts[1:], ","))
	return ok && propertiesSatisfied(observedProperties, requestedProperties)
}

func parseNetworkProperties(value string) ([]configProperty, bool) {
	if value == "" {
		return nil, true
	}
	parts := strings.Split(value, ",")
	properties := make([]configProperty, 0, len(parts))
	for _, part := range parts {
		key, propertyValue, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(propertyValue) == "" {
			return nil, false
		}
		properties = append(properties, configProperty{key: strings.TrimSpace(key), value: strings.TrimSpace(propertyValue)})
	}
	return properties, true
}

type createdVolumeExpectation struct {
	key     string
	storage string
	sizeGiB *int
}

// createdConfigVerifier confirms that requested resource settings survived
// creation by reading the single-guest config endpoint after its task ends.
// Unknown or unreadable config is unverifiable; observed mismatches are failed.
func createdConfigVerifier(
	prov domain.Provider,
	resourceType, node string,
	vmid int,
	target string,
	requested map[string]string,
	volume *createdVolumeExpectation,
) postconditionVerifier {
	return func(ctx context.Context) postconditionOutcome {
		var observed map[string]interface{}
		switch resourceType {
		case "vm":
			inspector, ok := prov.(domain.VMInspector)
			if !ok {
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s config readback is unsupported", target)}
			}
			var err error
			observed, err = inspector.VMConfig(ctx, node, vmid)
			if err != nil {
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s config could not be read after creation: %v", target, err)}
			}
		case "container":
			inspector, ok := prov.(domain.ContainerInspector)
			if !ok {
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s config readback is unsupported", target)}
			}
			var err error
			observed, err = inspector.ContainerConfig(ctx, node, vmid)
			if err != nil {
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s config could not be read after creation: %v", target, err)}
			}
		default:
			return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("creation readback is unsupported for resource type %q", resourceType)}
		}

		_, mismatches := verifyConfigReadback(observed, requested)
		if volume != nil {
			actual, exists := observed[volume.key]
			actualValue := ""
			if exists {
				actualValue = canonicalConfigValue(actual)
			}
			if !exists || !createdVolumeMatches(actualValue, *volume) {
				wanted := "storage " + volume.storage
				if volume.sizeGiB != nil {
					wanted += fmt.Sprintf(" size %d GiB", *volume.sizeGiB)
				}
				mismatches = append(mismatches, configFieldMismatch{Key: volume.key, Request: wanted, Observed: actualValue})
			}
		}
		if len(mismatches) > 0 {
			return postconditionOutcome{Detail: fmt.Sprintf("%s config readback did not match: %s", target, joinMismatches(mismatches))}
		}
		return postconditionOutcome{Verified: true, Detail: fmt.Sprintf("%s requested creation settings matched provider readback", target)}
	}
}

func createdVolumeMatches(actual string, expected createdVolumeExpectation) bool {
	volumeID, parameters, hasParameters := strings.Cut(actual, ",")
	storageID, _, hasStorage := strings.Cut(volumeID, ":")
	if !hasStorage || storageID != expected.storage {
		return false
	}
	if expected.sizeGiB == nil {
		return true
	}
	if !hasParameters {
		return false
	}
	for _, parameter := range strings.Split(parameters, ",") {
		key, value, ok := strings.Cut(parameter, "=")
		if !ok || key != "size" {
			continue
		}
		value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(value, "GiB"), "GB"), "G"))
		size, err := strconv.Atoi(value)
		return err == nil && size == *expected.sizeGiB
	}
	return false
}

// templateConfigVerifier confirms that a guest became a template by reading the
// authoritative config endpoint after the conversion and checking the template
// flag. The provider's container template endpoint applies the change
// synchronously and returns no task identifier, so completion must be modelled
// from the observed flag rather than from a task ID that never exists.
//
// A missing flag can be a stale observation. Taskless callers use bounded
// convergence checks before treating a readable mismatch as a failure.
func templateConfigVerifier(prov domain.Provider, resourceType, node string, vmid int) postconditionVerifier {
	return func(ctx context.Context) postconditionOutcome {
		var config map[string]interface{}
		var err error
		switch resourceType {
		case "vm":
			inspector, ok := prov.(domain.VMInspector)
			if !ok {
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s/%d VM config readback is unsupported", node, vmid)}
			}
			config, err = inspector.VMConfig(ctx, node, vmid)
		case "container":
			inspector, ok := prov.(domain.ContainerInspector)
			if !ok {
				return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s/%d container config readback is unsupported", node, vmid)}
			}
			config, err = inspector.ContainerConfig(ctx, node, vmid)
		default:
			return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("template verification is unsupported for resource type %q", resourceType)}
		}
		if err != nil {
			return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s/%d config could not be read after conversion: %v", node, vmid, err)}
		}
		value, exists := config["template"]
		if !exists {
			return postconditionOutcome{Detail: fmt.Sprintf("%s/%d config does not report the template flag after the conversion", node, vmid)}
		}
		canonical := canonicalConfigValue(value)
		if canonical == "1" || strings.EqualFold(canonical, "true") {
			return postconditionOutcome{Verified: true, Detail: fmt.Sprintf("%s/%d config reports template:1", node, vmid)}
		}
		return postconditionOutcome{Detail: fmt.Sprintf("%s/%d config reports template:%s after the conversion", node, vmid, canonical)}
	}
}

// convergeTemplateReadback retries reads only, never the conversion. Both the
// attempt count and each wait are bounded, and caller cancellation is honored.
func convergeTemplateReadback(verify postconditionVerifier, initialDelay time.Duration, attempts int) postconditionVerifier {
	return func(ctx context.Context) postconditionOutcome {
		delay := initialDelay
		var result postconditionOutcome
		for attempt := 0; attempt < attempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return postconditionOutcome{Unverifiable: true, Detail: "template readback interrupted: " + err.Error()}
			}
			result = verify(ctx)
			if result.Verified || result.Unverifiable || attempt == attempts-1 {
				return result
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return postconditionOutcome{Unverifiable: true, Detail: "template readback interrupted: " + ctx.Err().Error()}
			case <-timer.C:
			}
			delay = min(delay*2, 2*time.Second)
		}
		return result
	}
}

// runSynchronousMutation models a mutation endpoint that applies inline and
// returns no task identifier.
//
// Some provider endpoints have no task to poll even though the command name
// looks asynchronous. Treating them as task-based produced an OUTCOME_UNKNOWN
// receipt and a reconcile recommendation that could never resolve. Reporting
// them as synchronous, then confirming the requested postcondition by readback,
// describes what the endpoint actually guarantees.
func runSynchronousMutation(ctx context.Context, cmdCtx *Context, prov domain.Provider, operation, target, safetyTier string, verify postconditionVerifier) error {
	profileName, _ := resolveProfileName(cmdCtx)
	result := output.NewOperationResult(operation, prov.Name(), profileName)
	result.Target = target
	result.Safety = safetyTier
	result.Submitted = true
	result.Success = true
	result.Synchronous = true
	if verify != nil {
		applyPostcondition(&result, verify(ctx))
	}
	if err := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result); err != nil {
		return err
	}
	if result.Verification == "failed" {
		return app.MarkEmitted(app.NewExitError(
			fmt.Errorf("%s was applied but the observed result does not satisfy the request", operation),
			app.ExitPartialFailure,
		))
	}
	return nil
}

// readbackContainerConfig reads the current container config for comparison.
func readbackContainerConfig(ctx context.Context, prov domain.Provider, node string, vmid int) (map[string]interface{}, error) {
	ci, ok := prov.(domain.ContainerInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: container configuration readback not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return ci.ContainerConfig(ctx, node, vmid)
}

// readbackVMConfig reads the current VM config for comparison.
func readbackVMConfig(ctx context.Context, prov domain.Provider, node string, vmid int) (map[string]interface{}, error) {
	vi, ok := prov.(domain.VMInspector)
	if !ok {
		return nil, app.NewExitError(
			fmt.Errorf("%w: VM configuration readback not supported by provider %q", app.ErrUnsupportedCap, prov.Name()),
			app.ExitUnsupportedCap,
		)
	}
	return vi.VMConfig(ctx, node, vmid)
}

// guestConfigUpdateVerifier checks requested VM/CT fields after an asynchronous
// config task completes. The prior read is used only to report whether the
// requested values differed before submission.
func guestConfigUpdateVerifier(prov domain.Provider, resourceType, node string, vmid int, requested map[string]string, before map[string]interface{}, beforeErr error) postconditionVerifier {
	return func(ctx context.Context) postconditionOutcome {
		var observed map[string]interface{}
		var err error
		if resourceType == "vm" {
			observed, err = readbackVMConfig(ctx, prov, node, vmid)
		} else {
			observed, err = readbackContainerConfig(ctx, prov, node, vmid)
		}
		if err != nil {
			return postconditionOutcome{Unverifiable: true, Detail: fmt.Sprintf("%s configuration could not be read after the update: %v", resourceType, err)}
		}
		matched, mismatches := verifyConfigReadback(observed, requested)
		outcome := postconditionOutcome{Verified: matched}
		if !matched {
			outcome.Detail = fmt.Sprintf("%d of %d requested %s configuration fields did not read back as requested: %s", len(mismatches), len(requested), resourceType, joinMismatches(mismatches))
		}
		if beforeErr == nil {
			priorMatched, _ := verifyConfigReadback(before, requested)
			changed := !priorMatched
			outcome.Changed = &changed
		}
		return outcome
	}
}

// runSynchronousConfigUpdate models a config endpoint that applies inline and
// returns no task identifier.
//
// The change is confirmed by reading the requested fields back rather than by
// polling a task. This reports the endpoint contract accurately: the request
// completed, and the postcondition was observed. It does not claim the change
// is already effective in a running guest, which is a separate question the
// plan treats as distinct from the saved setting.
func runSynchronousConfigUpdate(
	ctx context.Context,
	cmdCtx *Context,
	prov domain.Provider,
	operation, target, node string,
	vmid int,
	requested map[string]string,
	before map[string]interface{},
	beforeErr error,
) error {
	profileName, _ := resolveProfileName(cmdCtx)
	result := output.NewOperationResult(operation, prov.Name(), profileName)
	result.Target = target
	result.Safety = "reversible"
	result.Submitted = true
	result.Success = true
	// No task was returned because none exists for this endpoint. Recording
	// that explicitly is what lets a receipt report a confirmed completion
	// instead of an unobserved asynchronous outcome.
	result.Synchronous = true

	observed, err := readbackContainerConfig(ctx, prov, node, vmid)
	if err != nil {
		// The change was applied; only the postcondition is unobservable.
		// Report that honestly rather than claiming either success or failure,
		// and use the same exit semantics as a completed asynchronous task
		// whose postcondition could not be checked: success with `unsupported`
		// verification and a warning, not an error.
		result.Status = "applied-unverified"
		result.Verification = "unsupported"
		result.Warnings = append(result.Warnings,
			"configuration was applied by a synchronous provider endpoint, but readback verification is unavailable: "+err.Error())
		if writeErr := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result); writeErr != nil {
			return writeErr
		}
		return nil
	}

	matched, mismatches := verifyConfigReadback(observed, requested)

	// "Changed" is only reportable when the prior state was actually observed.
	// Guessing either way would misrepresent an already-correct value as an
	// applied change, or an applied change as a no-op.
	if beforeErr == nil && len(requested) > 0 {
		priorMatched, _ := verifyConfigReadback(before, requested)
		changed := !priorMatched
		result.Changed = &changed
		if priorMatched {
			result.Warnings = append(result.Warnings,
				"the requested values were already set; no change was necessary")
		}
	}

	if !matched {
		result.Status = "verification-failed"
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("%d of %d requested fields did not read back as requested", len(mismatches), len(requested)))
		for _, m := range mismatches {
			result.Warnings = append(result.Warnings, "  "+m.String())
		}
		if writeErr := output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result); writeErr != nil {
			return writeErr
		}
		return app.MarkEmitted(app.NewExitError(
			fmt.Errorf("%s applied but readback did not match the request: %s", target, joinMismatches(mismatches)),
			app.ExitPartialFailure,
		))
	}

	result.Status = "verified"
	return output.WriteResult(cmdCtx.Writer, cmdCtx.Opts.Output, result)
}

func joinMismatches(mismatches []configFieldMismatch) string {
	parts := make([]string, 0, len(mismatches))
	for _, m := range mismatches {
		parts = append(parts, m.String())
	}
	return strings.Join(parts, "; ")
}
