#!/usr/bin/env bash
# Cross-platform compile gate for token-usage.
#
# Default: `go build` + `go vet` every shipped OS/arch pair — results are
# discarded (cache-only), serving purely as a compile check. With --emit,
# the token-usage binary is additionally written to bin/<os>-<arch>/.
#
# Usage: scripts/build-all.sh [--emit] [--test]

set -euo pipefail
cd "$(dirname "$0")/.."

usage() {
	cat <<'EOF'
Usage: scripts/build-all.sh [options]

Compile-check token-usage for every shipped OS/arch pair
(linux/darwin/windows x amd64/arm64) via `go build` + `go vet`.
By default results are discarded — this is a check, not a release.

Options:
  --emit      Also write release binaries to bin/<os>-<arch>/token-usage[.exe].
              (/bin/ is gitignored.)
  --test      Additionally run the native unit test suite (go test ./...).
  -h, --help  Show this help and exit.

Exit status: 0 if all requested steps passed, 1 otherwise.

Examples:
  scripts/build-all.sh                 # compile check only, no output files
  scripts/build-all.sh --emit          # check + emit 6 binaries into bin/
  scripts/build-all.sh --emit --test   # check + emit + native tests
EOF
}

emit_binaries=0
run_tests=0
for arg in "$@"; do
	case $arg in
	--emit) emit_binaries=1 ;;
	--test) run_tests=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "error: unknown option: $arg" >&2
		usage >&2
		exit 2
		;;
	esac
done

targets=(
	"linux amd64"
	"linux arm64"
	"darwin amd64"
	"darwin arm64"
	"windows amd64"
	"windows arm64"
)

fail=0
for target in "${targets[@]}"; do
	read -r goos goarch <<<"$target"
	echo "==> ${goos}/${goarch}"
	# Full-repo gate first: multi-package go build only checks, writes nothing.
	if ! GOOS=$goos GOARCH=$goarch go build ./...; then
		fail=1
		continue
	fi
	# vet type-checks _test.go files too, guarding cross-platform test builds.
	if ! GOOS=$goos GOARCH=$goarch go vet ./...; then
		fail=1
		continue
	fi
	if ((emit_binaries)); then
		out="bin/${goos}-${goarch}/token-usage"
		[[ $goos == windows ]] && out+=".exe"
		if ! GOOS=$goos GOARCH=$goarch go build -o "$out" ./cmd/token-usage; then
			fail=1
		fi
	fi
done

if ((run_tests)); then
	echo "==> native unit tests"
	go test ./... || fail=1
fi

if ((fail)); then
	echo "FAIL: some target failed to build" >&2
	exit 1
fi

summary="OK: all targets compiled (check only)"
if ((emit_binaries)); then
	summary="OK: all targets compiled, binaries in bin/<os>-<arch>/"
fi
echo "$summary"
