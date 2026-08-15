package report

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// DeviceFile is the persisted Device identity (Device ID + human label).
// Losing the file makes the machine a new device under the 3-device cap.
type DeviceFile struct {
	DeviceID string `json:"deviceId"`
	Label    string `json:"label"`
}

// DefaultDevicePath resolves the device file location:
// CCUSAGE_DEVICE_FILE wins, else <user config dir>/ccusage/device.json.
func DefaultDevicePath() (string, error) {
	if p := os.Getenv("CCUSAGE_DEVICE_FILE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ccusage", "device.json"), nil
}

// hostnameLabel is the default device label.
func hostnameLabel() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		return "device"
	}
	return host
}

// LoadDevice reads (creating on first run) the device identity.
func LoadDevice(path string) (*DeviceFile, error) {
	if raw, err := os.ReadFile(path); err == nil {
		var d DeviceFile
		if err := json.Unmarshal(raw, &d); err == nil && d.DeviceID != "" {
			if d.Label == "" {
				d.Label = hostnameLabel()
			}
			return &d, nil
		}
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	device := &DeviceFile{
		DeviceID: hex.EncodeToString(raw),
		Label:    hostnameLabel(),
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	content, _ := json.MarshalIndent(device, "", "  ")
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		return nil, err
	}
	return device, nil
}

// ErrNoServer / ErrNoToken mark configuration gaps the CLI turns into
// actionable advice.
var (
	ErrNoServer = errors.New("report server address is not configured")
	ErrNoToken  = errors.New("report token is not configured")
)
