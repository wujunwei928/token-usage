// Package golden runs the built token-usage binary against every golden case
// and byte-compares stdout/stderr/exit code with files generated from the
// reference binary by scripts/golden.sh.
//
// Env contract (MUST stay in sync with scripts/golden.sh):
//
//	NO_COLOR=1 TZ=UTC LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=dumb COLUMNS=100
//	HOME=<repo>/testdata/fixtures/home
//	PATH=/usr/local/bin:/usr/bin:/bin
//	CLAUDE_CONFIG_DIR, TOKEN_USAGE_CONFIG_DIR, and XDG_CONFIG_HOME unset
//	unless the case sets them.
//
// A case's "env" object overrides base values. Its "env_remove" list names env
// vars removed AFTER overrides are applied, so a case can set FORCE_COLOR=1
// while removing NO_COLOR. Args and env values expand $FIXTURE (the fixtures
// root) and $REPO (the repository root); ticket 10 added arg expansion for
// --config paths.
package golden

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "token-usage-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)
	binPath = filepath.Join(tmp, "token-usage")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/token-usage")
	build.Dir = repoRoot()
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Dir(filepath.Dir(dir))
}

type testCase struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
	// EnvRemove, when present, lists env vars removed after Env overrides are
	// applied (so a case can set FORCE_COLOR=1 and remove NO_COLOR).
	EnvRemove []string `json:"env_remove"`
	// Stdin, when present, is a literal string fed on stdin.
	Stdin string `json:"stdin"`
}

// buildEnv resolves the run environment per the package env contract.
func buildEnv(tc testCase, fixture, root string) []string {
	env := map[string]string{
		"NO_COLOR": "1",
		"TZ":       "UTC",
		"LANG":     "C.UTF-8",
		"LC_ALL":   "C.UTF-8",
		"TERM":     "dumb",
		"COLUMNS":  "100",
		"HOME":     filepath.Join(fixture, "home"),
		"PATH":     "/usr/local/bin:/usr/bin:/bin",
	}
	for k, v := range tc.Env {
		env[k] = strings.NewReplacer("$FIXTURE", fixture, "$REPO", root).Replace(v)
	}
	for _, k := range tc.EnvRemove {
		delete(env, k)
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func TestGolden(t *testing.T) {
	root := repoRoot()
	casesDir := filepath.Join(root, "testdata", "golden", "cases")
	outDir := filepath.Join(root, "testdata", "golden", "out")
	fixture := filepath.Join(root, "testdata", "fixtures")

	entries, err := os.ReadDir(casesDir)
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		ran++
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(casesDir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			var tc testCase
			if err := json.Unmarshal(raw, &tc); err != nil {
				t.Fatal(err)
			}

			env := buildEnv(tc, fixture, root)

			args := make([]string, len(tc.Args))
			for i, arg := range tc.Args {
				args[i] = strings.NewReplacer("$FIXTURE", fixture, "$REPO", root).Replace(arg)
			}
			cmd := exec.Command(binPath, args...)
			cmd.Dir = root
			cmd.Env = env
			if tc.Stdin != "" {
				cmd.Stdin = strings.NewReader(tc.Stdin)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			runErr := cmd.Run()
			exitCode := 0
			if runErr != nil {
				if ee, ok := runErr.(*exec.ExitError); ok {
					exitCode = ee.ExitCode()
				} else {
					t.Fatalf("spawn: %v", runErr)
				}
			}

			checkBytes := func(kind string, got []byte) {
				want, err := os.ReadFile(filepath.Join(outDir, name+"."+kind))
				if err != nil {
					t.Fatalf("missing golden %s: %v (run scripts/golden.sh)", kind, err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("%s mismatch:\n--- want ---\n%q\n--- got ---\n%q", kind, want, got)
				}
			}
			checkBytes("out", stdout.Bytes())
			checkBytes("err", stderr.Bytes())
			wantCode, err := os.ReadFile(filepath.Join(outDir, name+".code"))
			if err != nil {
				t.Fatalf("missing golden code: %v", err)
			}
			if got := strconv.Itoa(exitCode); strings.TrimSpace(string(wantCode)) != got {
				t.Errorf("exit code mismatch: want %s got %d", strings.TrimSpace(string(wantCode)), exitCode)
			}
		})
	}
	if ran == 0 {
		t.Fatal("no golden cases found")
	}
}
