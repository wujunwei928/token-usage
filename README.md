# ccusage-go

A Go implementation of [ccusage](https://github.com/ryoppippi/ccusage), tracking the Rust v20 line (`20.0.19`). It analyzes Claude Code and other AI agent usage from local logs and prints token/cost reports.

CLI surface, output bytes, and exit codes are behaviorally identical to the reference binary — user scripts and shell aliases can switch between the two seamlessly.

## Install

```sh
go install github.com/wujunwei/ccusage-go/cmd/ccusage@latest
```

Or build from source:

```sh
go build -o ccusage ./cmd/ccusage
```

Release binaries for linux/darwin/windows (amd64/arm64) are produced with GoReleaser (`.goreleaser.yml`).

## Usage

```sh
ccusage                    # all-agent daily report
ccusage claude daily       # Claude-only daily report
ccusage claude weekly --start-of-week monday
ccusage claude session --id <sessionId>
ccusage claude blocks      # 5-hour billing blocks
ccusage claude statusline  # Claude Code status bar hook (reads stdin)
ccusage codex daily        # Codex report
ccusage daily --sections daily,weekly,monthly,session --json
```

Run `ccusage --help` or any subcommand with `--help` for the full flag set.

## Parity verification

- `scripts/golden.sh [ref-binary] [case-prefix]` regenerates byte-level golden files from the reference binary.
- `go test ./...` diffs the Go binary against those goldens (no Rust toolchain needed).
- `scripts/compare.sh` runs a live differential against the installed `ccusage` on real data.

## Development

- Structure mirrors the reference Rust crates: `internal/core` (types/cost/pricing/aggregation), `internal/terminal` (table renderer), `internal/adapter/<agent>` (one package per agent), `internal/cli` (cobra command tree). See `docs/rewrite-plan.md` and `docs/adr/`.
- Pricing snapshots are embedded via `go:embed`; refresh with `scripts/update-pricing.sh`.
