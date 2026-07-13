package credentials

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/geoffmcc/nodex/internal/domain"
)

var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

var validBackends = map[string]bool{
	"keyring": true,
	"file":    true,
	"env":     true,
	"stdin":   true,
}

// ValidateName checks credential profile names used as backend keys or file names.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("credential name is empty")
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("malformed credential name %q", name)
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") || strings.Contains(name, ":") {
		return fmt.Errorf("malformed credential name %q", name)
	}
	if filepath.IsAbs(name) || (runtime.GOOS != "windows" && strings.HasPrefix(name, `\\`)) || looksLikeWindowsAbs(name) {
		return fmt.Errorf("credential name %q must not be an absolute path", name)
	}
	return nil
}

func looksLikeWindowsAbs(name string) bool {
	return len(name) >= 2 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) && name[1] == ':'
}

// ParseCredentialRefStrict parses and validates backend:name credential refs.
func ParseCredentialRefStrict(ref string) (backend, name string, err error) {
	if ref == "" {
		return "", "", fmt.Errorf("credential reference is empty")
	}
	if strings.Count(ref, ":") > 1 || strings.HasPrefix(ref, ":") || strings.HasSuffix(ref, ":") {
		return "", "", fmt.Errorf("malformed credential reference")
	}
	parts := strings.Split(ref, ":")
	if len(parts) == 1 {
		backend, name = "file", parts[0]
	} else {
		backend, name = parts[0], parts[1]
	}
	if !validBackends[backend] {
		return "", "", fmt.Errorf("unknown credential backend %q", backend)
	}
	if err := ValidateName(name); err != nil {
		return "", "", err
	}
	return backend, name, nil
}

// ValidateCredentials rejects missing or incomplete credential combinations.
func ValidateCredentials(profile string, creds *domain.Credentials) error {
	if creds == nil {
		return fmt.Errorf("profile %q credentials are missing", profile)
	}
	switch creds.Type {
	case "token", "":
		if creds.TokenID != "" || creds.TokenSecret != "" {
			if creds.TokenID == "" {
				return fmt.Errorf("profile %q token credentials missing token_id", profile)
			}
			if creds.TokenSecret == "" {
				return fmt.Errorf("profile %q token credentials missing token_secret", profile)
			}
			return nil
		}
		if creds.Token != "" {
			return nil
		}
		return fmt.Errorf("profile %q token credentials missing token_id/token_secret", profile)
	case "password":
		if creds.Username == "" {
			return fmt.Errorf("profile %q password credentials missing username", profile)
		}
		if creds.Password == "" {
			return fmt.Errorf("profile %q password credentials missing password", profile)
		}
		return nil
	default:
		return fmt.Errorf("profile %q has unsupported credential type %q", profile, creds.Type)
	}
}
