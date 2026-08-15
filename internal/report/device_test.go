package report

import (
	"os"
	"testing"
)

// TOKEN_USAGE_DEVICE_FILE wins; CCUSAGE_DEVICE_FILE remains the legacy
// fallback (ADR 0008).
func TestDefaultDevicePathEnvDualRead(t *testing.T) {
	t.Run("both set, TOKEN_USAGE wins", func(t *testing.T) {
		t.Setenv("TOKEN_USAGE_DEVICE_FILE", "/new/device.json")
		t.Setenv("CCUSAGE_DEVICE_FILE", "/legacy/device.json")
		path, err := DefaultDevicePath()
		if err != nil || path != "/new/device.json" {
			t.Fatalf("got path=%q err=%v, want /new/device.json", path, err)
		}
	})

	t.Run("legacy fallback", func(t *testing.T) {
		os.Unsetenv("TOKEN_USAGE_DEVICE_FILE")
		t.Setenv("CCUSAGE_DEVICE_FILE", "/legacy/device.json")
		path, err := DefaultDevicePath()
		if err != nil || path != "/legacy/device.json" {
			t.Fatalf("got path=%q err=%v, want /legacy/device.json", path, err)
		}
	})
}
