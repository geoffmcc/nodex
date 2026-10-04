package cli

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/geoffmcc/nodex/internal/redact"
)

const maxGuestConfigPreviewValueRunes = 120

var guestConfigPreviewSafeFields = map[string]struct{}{
	"agent": {}, "balloon": {}, "boot": {}, "bootdisk": {}, "cores": {},
	"cpu": {}, "cpuunits": {}, "hostname": {}, "hotplug": {}, "machine": {},
	"memory": {}, "name": {}, "onboot": {}, "ostype": {}, "protection": {},
	"scsihw": {}, "sockets": {}, "swap": {}, "tags": {}, "unprivileged": {},
}

// guestConfigUpdateConfirmationMessage adds a deterministic preview of the
// requested fields. Values are shown only for a narrow allowlist of fields
// whose contents are non-sensitive by meaning; other values are redacted so
// provider/version-specific configuration remains usable without leaking
// credentials through a confirmation prompt.
func guestConfigUpdateConfirmationMessage(message string, params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var preview strings.Builder
	preview.WriteString(message)
	preview.WriteString("\nRequested changes (sensitive or unrecognized values are redacted):")
	for _, key := range keys {
		preview.WriteString("\n  ")
		preview.WriteString(escapeGuestConfigPreviewText(key))
		preview.WriteByte('=')

		value := "[REDACTED]"
		if _, safe := guestConfigPreviewSafeFields[strings.ToLower(key)]; safe && !isSensitiveGuestConfigKey(key) {
			value = truncateGuestConfigPreviewValue(redact.String(params[key]))
			value = escapeGuestConfigPreviewText(value)
		}
		preview.WriteString(value)
	}
	return preview.String()
}

func isSensitiveGuestConfigKey(key string) bool {
	key = strings.ToLower(key)
	for _, fragment := range []string{"password", "passwd", "pwd", "secret", "token", "credential", "key", "auth"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func truncateGuestConfigPreviewValue(value string) string {
	if utf8.RuneCountInString(value) <= maxGuestConfigPreviewValueRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxGuestConfigPreviewValueRunes]) + "..."
}

// Quote control characters and separators that could forge additional preview
// lines while keeping ordinary key/value text readable.
func escapeGuestConfigPreviewText(value string) string {
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}
