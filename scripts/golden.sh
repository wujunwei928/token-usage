#!/usr/bin/env bash
# Regenerate golden files from the reference ccusage binary.
#
# Usage: scripts/golden.sh [path-to-reference-ccusage] [case-name-filter]
#
# The env contract below MUST stay in sync with internal/golden/golden_test.go:
#   NO_COLOR=1 TZ=UTC LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=dumb COLUMNS=100
#   HOME=<repo>/testdata/fixtures/home
#   PATH=/usr/local/bin:/usr/bin:/bin
#   CLAUDE_CONFIG_DIR and XDG_CONFIG_HOME unset unless the case sets them.
# A case's "env" object overrides base values (later duplicate keys collapse to
# the case value). Its "env_remove" list names env vars removed AFTER overrides
# are applied, so a case can set FORCE_COLOR=1 while removing NO_COLOR. Args
# and env values expand $FIXTURE (the fixtures root) and $REPO (the repository
# root); ticket 10 added arg expansion for --config paths.
set -euo pipefail
REPO=$(cd "$(dirname "$0")/.." && pwd)
CCUSAGE_BIN=${1:-ccusage}
CASE_FILTER=${2:-}
if ! [[ "$CCUSAGE_BIN" = /* ]]; then
  CCUSAGE_BIN=$(command -v "$CCUSAGE_BIN")
fi
export CCUSAGE_BIN

python3 - "$REPO" "$CASE_FILTER" <<'EOF'
import json, os, subprocess, sys

repo = sys.argv[1]
case_filter = sys.argv[2] if len(sys.argv) > 2 else ""
bin_path = os.environ["CCUSAGE_BIN"]
cases_dir = os.path.join(repo, "testdata", "golden", "cases")
out_dir = os.path.join(repo, "testdata", "golden", "out")
fixture = os.path.join(repo, "testdata", "fixtures")
os.makedirs(out_dir, exist_ok=True)

def base_env():
    env = {
        "NO_COLOR": "1",
        "TZ": "UTC",
        "LANG": "C.UTF-8",
        "LC_ALL": "C.UTF-8",
        "TERM": "dumb",
        "COLUMNS": "100",
        "HOME": os.path.join(fixture, "home"),
        "PATH": "/usr/local/bin:/usr/bin:/bin",
    }
    return env

for case_file in sorted(os.listdir(cases_dir)):
    if not case_file.endswith(".json"):
        continue
    name = case_file[:-5]
    if case_filter and not name.startswith(case_filter):
        continue
    with open(os.path.join(cases_dir, case_file)) as f:
        case = json.load(f)
    env = base_env()
    for k, v in (case.get("env") or {}).items():
        env[k] = v.replace("$FIXTURE", fixture).replace("$REPO", repo)
    for k in case.get("env_remove") or []:
        env.pop(k, None)
    stdin_data = case.get("stdin", "").encode()
    args = [a.replace("$FIXTURE", fixture).replace("$REPO", repo) for a in case["args"]]
    p = subprocess.run([bin_path] + args, env=env, cwd=repo,
                       input=stdin_data, capture_output=True)
    with open(os.path.join(out_dir, name + ".out"), "wb") as f:
        f.write(p.stdout)
    with open(os.path.join(out_dir, name + ".err"), "wb") as f:
        f.write(p.stderr)
    with open(os.path.join(out_dir, name + ".code"), "w") as f:
        f.write(str(p.returncode))
    print(f"{name}: exit={p.returncode} out={len(p.stdout)}B err={len(p.stderr)}B")
EOF
