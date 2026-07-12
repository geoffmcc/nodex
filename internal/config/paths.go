package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	dirName        = "nodex"
	configFile     = "config.yaml"
	credentialFile = "credentials"
)

// Dir returns the platform-appropriate configuration directory.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "linux":
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, dirName), nil
		}
		return filepath.Join(home, ".config", dirName), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Nodex"), nil
	case "windows":
		appData := os.Getenv("AppData")
		if appData == "" {
			return filepath.Join(home, "AppData", "Roaming", "Nodex"), nil
		}
		return filepath.Join(appData, "Nodex"), nil
	default:
		return filepath.Join(home, ".config", dirName), nil
	}
}

// ConfigPath returns the full path to config.yaml.
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFile), nil
}

// CredentialPath returns the full path to the credentials file.
func CredentialPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, credentialFile), nil
}

// EnsureDir creates the config directory if it does not exist.
func EnsureDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
