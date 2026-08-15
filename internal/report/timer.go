package report

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The hourly report timer is a marked crontab entry, so install/uninstall
// stay idempotent even when the user edits their crontab by hand. The
// crontab-content logic lives in pure functions (unit-tested); only the thin
// read/write shell touches the real crontab.
const cronMarker = "# ccusage-report-timer (auto-managed)"

func readCrontab() (string, error) {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		// exit 1 from crontab -l means "no crontab" — treat as empty.
		if _, ok := err.(*exec.ExitError); ok {
			return "", nil
		}
		return "", err
	}
	return string(out), nil
}

func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ErrNoCrontab surfaces when the platform has no crontab.
var ErrNoCrontab = errors.New("crontab not available")

// TimerInstalled reports whether the marked entry exists.
func TimerInstalled() (bool, error) {
	current, err := readCrontab()
	if err != nil {
		return false, err
	}
	return strings.Contains(current, cronMarker), nil
}

// reportEntry builds the marked two-line crontab block for self.
func reportEntry(self, logPath string) string {
	return fmt.Sprintf("%s\n0 * * * * %s report --quiet >>%s 2>&1\n", cronMarker, self, logPath)
}

// installInto returns the new crontab content with the marked entry
// appended; ok=false when it is already present (idempotent no-op).
func installInto(current, entry string) (string, bool) {
	if strings.Contains(current, cronMarker) {
		return current, false
	}
	var buf bytes.Buffer
	if current != "" && !strings.HasSuffix(current, "\n") {
		buf.WriteString("\n")
	}
	buf.WriteString(entry)
	return current + buf.String(), true
}

// uninstallInto strips the marked two-line block; ok=false when absent.
func uninstallInto(current string) (string, bool) {
	if !strings.Contains(current, cronMarker) {
		return current, false
	}
	lines := strings.Split(current, "\n")
	kept := make([]string, 0, len(lines))
	skipNext := false
	for _, line := range lines {
		if line == cronMarker {
			skipNext = true
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), true
}

// InstallTimer appends the hourly entry once; a second call is a no-op.
func InstallTimer() (installed bool, err error) {
	current, err := readCrontab()
	if err != nil {
		return false, fmt.Errorf("cannot read crontab: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	next, changed := installInto(current, reportEntry(self, "/tmp/ccusage-report.log"))
	if !changed {
		return false, nil
	}
	if err := writeCrontab(next); err != nil {
		return false, fmt.Errorf("cannot write crontab: %w", err)
	}
	return true, nil
}

// UninstallTimer removes the marked entry if present.
func UninstallTimer() (removed bool, err error) {
	current, err := readCrontab()
	if err != nil {
		return false, fmt.Errorf("cannot read crontab: %w", err)
	}
	next, changed := uninstallInto(current)
	if !changed {
		return false, nil
	}
	if err := writeCrontab(next); err != nil {
		return false, fmt.Errorf("cannot write crontab: %w", err)
	}
	return true, nil
}
