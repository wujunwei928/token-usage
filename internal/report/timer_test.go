package report

import (
	"strings"
	"testing"
)

func TestInstallIntoIdempotent(t *testing.T) {
	entry := reportEntry("/usr/local/bin/ccusage", "/tmp/ccusage-report.log")
	if !strings.Contains(entry, cronMarker) || !strings.Contains(entry, "0 * * * *") {
		t.Fatalf("entry malformed: %q", entry)
	}

	// Empty crontab gets the entry appended with a newline.
	next, changed := installInto("", entry)
	if !changed || !strings.HasSuffix(next, "\n") {
		t.Fatalf("install on empty: changed=%v content=%q", changed, next)
	}

	// Second install is a no-op.
	again, changed := installInto(next, entry)
	if changed || again != next {
		t.Fatal("second install mutated the crontab")
	}

	// Content without trailing newline still separates cleanly.
	next2, _ := installInto("0 9 * * * backup", entry)
	if !strings.Contains(next2, "backup\n"+cronMarker) {
		t.Fatalf("missing-newline handling broken: %q", next2)
	}
}

func TestUninstallInto(t *testing.T) {
	entry := reportEntry("/usr/bin/ccusage", "/tmp/ccusage-report.log")
	withEntry, _ := installInto("0 9 * * * backup\n", entry)

	restored, removed := uninstallInto(withEntry)
	if !removed {
		t.Fatal("uninstall reported nothing removed")
	}
	if strings.Contains(restored, cronMarker) || strings.Contains(restored, "ccusage report") {
		t.Fatalf("marker survived uninstall: %q", restored)
	}
	if !strings.Contains(restored, "backup") {
		t.Fatal("uninstall ate an unrelated entry")
	}

	if _, removed := uninstallInto("0 9 * * * backup\n"); removed {
		t.Fatal("uninstall claimed removal on a crontab without the marker")
	}
}
