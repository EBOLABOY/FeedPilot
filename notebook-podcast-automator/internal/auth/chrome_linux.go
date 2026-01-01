//go:build linux

package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func detectChrome(debug bool) Browser {
	// Try standard Chrome first
	if path, err := exec.LookPath("google-chrome"); err == nil {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Google Chrome",
			Version: version,
		}
	}

	// Try Chromium as fallback
	if path, err := exec.LookPath("chromium"); err == nil {
		version := getChromeVersion(path)
		return Browser{
			Type:    BrowserChrome,
			Path:    path,
			Name:    "Chromium",
			Version: version,
		}
	}

	return Browser{Type: BrowserUnknown}
}

func getChromeVersion(path string) string {
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "google-chrome")
}

func getChromePath() string {
	for _, name := range []string{"google-chrome", "chrome", "chromium"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

// getBrowserPathForProfile returns the appropriate browser executable for a given browser type.
// On Linux, we try common binary names for each browser, then fall back to Chrome/Chromium.
func getBrowserPathForProfile(browserName string) string {
	switch browserName {
	case "Brave":
		for _, name := range []string{"brave-browser", "brave-browser-stable", "brave"} {
			if path, err := exec.LookPath(name); err == nil {
				return path
			}
		}
	case "Chrome Canary":
		// Linux doesn't have an official "Canary" channel in the same way as macOS/Windows.
		// Some distros package dev/unstable channels with these binary names.
		for _, name := range []string{"google-chrome-unstable", "google-chrome-dev", "google-chrome-beta"} {
			if path, err := exec.LookPath(name); err == nil {
				return path
			}
		}
	case "Edge":
		for _, name := range []string{"microsoft-edge", "microsoft-edge-stable", "microsoft-edge-beta", "microsoft-edge-dev"} {
			if path, err := exec.LookPath(name); err == nil {
				return path
			}
		}
	}

	return getChromePath()
}

func getCanaryProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "google-chrome-unstable")
}

func getBraveProfilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser")
}
