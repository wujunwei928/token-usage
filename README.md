# ccusage-go

[English](README.md) | [简体中文](README.zh-CN.md)

A Go implementation of [ccusage](https://github.com/ryoppippi/ccusage), tracking the Rust v20 line (`20.0.19`). It analyzes Claude Code and other AI agent usage from local logs and prints token/cost reports.

CLI surface, output bytes, and exit codes are behaviorally identical to the reference binary — user scripts and shell aliases can switch between the two seamlessly.

On top of the CLI it ships an optional **Token Leaderboard**: clients report aggregated local usage to a single-binary server that renders a community leaderboard and personal dashboards. See [Token Leaderboard](#token-leaderboard) below.

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

## Token Leaderboard

An end-to-end usage leaderboard built on the same adapters: the client aggregates each day's usage into hourly `(hour × tool × model)` token cells and uploads a Report Snapshot; the server stores them in SQLite and renders the web pages. Raw log entries never leave the machine (see `docs/adr/0001-aggregate-only-reporting.md`).

```sh
# One-command demo: builds both binaries, seeds 5 users × 30 days, serves on :8787
scripts/demo.sh [port]

# Server (single binary + SQLite, embedded web assets)
go build -o lbserver ./cmd/server
./lbserver add-user -db leaderboard.db --name alice --city 北京
./lbserver serve -db leaderboard.db -addr 0.0.0.0:8787

# Client: report today (re-running replaces the day, latest-wins)
ccusage report --server http://<host>:8787 --token <token>
ccusage report --since 2026-02-15   # one-shot backfill, one request
ccusage report --install-timer      # hourly crontab entry
ccusage report --dry-run            # print the snapshot without sending
```

Server address and token resolve from flags, `CCUSAGE_REPORT_SERVER`/`CCUSAGE_REPORT_TOKEN`, or `ccusage.json` (`reportServer`/`reportToken`), in that precedence.

Web pages: `/` leaderboard (tool/model/city/range/cache-inclusion filters), `/me` personal dashboard (stat cards, hourly×tool timeline, 30-day trend, breakdowns, devices), `/pricing` (453-model rate card with official/estimated sources), `/about` (data rules), plus register/login/settings with one-time User Token minting. Guard rails: 3 devices per user, 2MB request cap, 60 reports/hour per token, anomaly flag past 1B tokens/day.

Deployment, pricing overrides, and ops notes: [`server/README.md`](server/README.md). Domain glossary: [`server/CONTEXT.md`](server/CONTEXT.md).

## Parity verification

- `scripts/golden.sh [ref-binary] [case-prefix]` regenerates byte-level golden files from the reference binary.
- `go test ./...` diffs the Go binary against those goldens (no Rust toolchain needed).
- `scripts/compare.sh` runs a live differential against the installed `ccusage` on real data.

## Development

- Structure mirrors the reference Rust crates: `internal/core` (types/cost/pricing/aggregation), `internal/terminal` (table renderer), `internal/adapter/<agent>` (one package per agent), `internal/cli` (cobra command tree). The leaderboard adds `internal/report` (snapshot builder/client) and `cmd/server` + `internal/server` (ingest, store, pricing, SSR web). See `docs/rewrite-plan.md` and `docs/adr/`.
- `internal/e2e` runs the full-stack seam: real server binary + real `ccusage report` over fixture agent logs.
- Pricing snapshots are embedded via `go:embed`; refresh with `scripts/update-pricing.sh`.
