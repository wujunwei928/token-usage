# token-usage

[English](README.md) | [简体中文](README.zh-CN.md)

A Go port of [ccusage](https://github.com/ryoppippi/ccusage) (Rust v20 line), rebranded as the independent tool `token-usage`: it analyzes Claude Code and other AI agent usage from local logs and prints token/cost reports.

CLI surface, semantics, and exit codes follow the reference; branded strings (command name, version line, help hints) intentionally differ — a deliberate rebrand, not a drop-in alias ([ADR 0008](docs/adr/0008-rebrand-to-token-usage.md)). The bundled `zcode` adapter is a superset beyond upstream ([ADR 0006](docs/adr/0006-adapters-beyond-upstream.md)): it joins the all-agent reports, which can therefore show more rows than the reference. Config discovery uses its own `token-usage` namespace instead of upstream's Claude config dirs ([ADR 0007](docs/adr/0007-token-usage-config-namespace.md)).

On top of the CLI it ships an optional **Token Leaderboard**: clients report aggregated local usage to a single-binary server that renders a community leaderboard and personal dashboards. See [Token Leaderboard](#token-leaderboard) below.

## Install

```sh
go install github.com/wujunwei928/token-usage/cmd/token-usage@latest
```

Or build from source:

```sh
go build -o token-usage ./cmd/token-usage
```

Release binaries for linux/darwin/windows (amd64/arm64) are produced with GoReleaser (`.goreleaser.yml`).

## Usage

```sh
token-usage                # all-agent daily report
token-usage claude daily   # Claude-only daily report
token-usage claude weekly --start-of-week monday
token-usage claude session --id <sessionId>
token-usage claude blocks      # 5-hour billing blocks
token-usage claude statusline  # Claude Code status bar hook (reads stdin)
token-usage codex daily        # Codex report
token-usage zcode daily        # ZCode report (reads the ~/.zcode/cli analytics db)
token-usage daily --sections daily,weekly,monthly,session --json
```

Run `token-usage --help` or any subcommand with `--help` for the full flag set.

### ZCode

The `zcode` adapter goes beyond upstream ccusage ([ADR 0005](docs/adr/0005-zcode-adapter-sqlite-source.md), [ADR 0006](docs/adr/0006-adapters-beyond-upstream.md)). Instead of scanning JSONL it reads ZCode's local analytics database (`~/.zcode/cli/db/db.sqlite`, opened read-only; `ZCODE_DATA_DIR` overrides the location) and counts every model API call attempt — retries, failed calls, and auxiliary calls such as session titles included — with subagent sessions attributed to their parent session. Sessions predating the CLI's `model_usage` table fall back to per-message tokens from the same database. GLM models are usually absent from the pricing tables: costs show `$0.00` with a missing-pricing warning until you add prices via `pricingOverrides` in the token-usage config.

### Configuration

The `ccusage.json`-style config lives in token-usage's own `token-usage` namespace — upstream ccusage's config files (the Claude config dirs) are deliberately not read, so the two tools never share config files ([ADR 0007](docs/adr/0007-token-usage-config-namespace.md)). Discovery order: `./.token-usage/config.json`, then `~/.config/token-usage/config.json` and `~/.token-usage/config.json`; `TOKEN_USAGE_CONFIG_DIR` overrides the global lookup and `--config` points at any file. The file format (`defaults`/`commands` sections, `pricingOverrides`, …) is unchanged from upstream.

## Token Leaderboard

An end-to-end usage leaderboard built on the same adapters: the client aggregates each day's usage into hourly `(hour × tool × model)` token cells and uploads a Report Snapshot; the server stores them in SQLite and renders the web pages. Raw log entries never leave the machine (see `docs/adr/0001-aggregate-only-reporting.md`).

```sh
# One-command demo: builds both binaries, seeds 5 users × 30 days, serves on :8787
scripts/demo.sh [port]

# Server (single binary + SQLite, embedded web assets)
go build -o token-usage-server ./cmd/server
./token-usage-server add-user -db leaderboard.db --name alice --city 北京
./token-usage-server serve -db leaderboard.db --addr 0.0.0.0:8787

# Client: report today (re-running replaces the day, latest-wins)
token-usage report --server http://<host>:8787 --token <token>
token-usage report --since 2026-02-15   # one-shot backfill, one request
token-usage report --install-timer      # hourly crontab entry
token-usage report --dry-run            # print the snapshot without sending
```

Server address and token resolve from flags, `TOKEN_USAGE_REPORT_SERVER`/`TOKEN_USAGE_REPORT_TOKEN` (legacy `CCUSAGE_REPORT_*` still honored), or the token-usage config (`reportServer`/`reportToken`), in that precedence.

Web pages: `/` leaderboard (tool/model/city/range/cache-inclusion filters), `/me` personal dashboard (stat cards, hourly×tool timeline, 30-day trend, breakdowns, devices), `/pricing` (453-model rate card with official/estimated sources), `/about` (data rules), plus register/login/settings with one-time User Token minting. Guard rails: 3 devices per user, 2MB request cap, 60 reports/hour per token, anomaly flag past 1B tokens/day.

Deployment, pricing overrides, and ops notes: [`server/README.md`](server/README.md). Domain glossary: [`server/CONTEXT.md`](server/CONTEXT.md).

## Parity verification

- `scripts/golden.sh [ref-binary] [case-prefix]` regenerates byte-level golden files from the reference binary (post-rebrand, branded strings need the ccusage→token-usage adjustments re-applied — [ADR 0008](docs/adr/0008-rebrand-to-token-usage.md); the checked-in goldens are the spec).
- `go test ./...` diffs the Go binary against those goldens (no Rust toolchain needed).
- `scripts/compare.sh` runs a live differential against the installed `ccusage` on real data.

## Development

- Structure mirrors the reference Rust crates: `internal/core` (types/cost/pricing/aggregation), `internal/terminal` (table renderer), `internal/adapter/<agent>` (one package per agent), `internal/cli` (cobra command tree). The leaderboard adds `internal/report` (snapshot builder/client) and `cmd/server` + `internal/server` (ingest, store, pricing, SSR web). See `docs/rewrite-plan.md` and `docs/adr/`.
- `internal/e2e` runs the full-stack seam: real server binary + real `token-usage report` over fixture agent logs.
- Pricing snapshots are embedded via `go:embed`; refresh with `scripts/update-pricing.sh`.
