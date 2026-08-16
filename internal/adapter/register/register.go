// Package register pulls in every entries-capable Agent Adapter so their
// self-registrations fire. Any consumer of the adapter registry that does
// not already import the adapter packages transitively (the all-report specs
// do) imports this package instead. codex is absent by design: its Groups
// pipeline never registers a lossy entries adapter (ADR 0009).
package register

import (
	_ "github.com/wujunwei928/token-usage/internal/adapter/amp"
	_ "github.com/wujunwei928/token-usage/internal/adapter/claude"
	_ "github.com/wujunwei928/token-usage/internal/adapter/codebuff"
	_ "github.com/wujunwei928/token-usage/internal/adapter/copilot"
	_ "github.com/wujunwei928/token-usage/internal/adapter/droid"
	_ "github.com/wujunwei928/token-usage/internal/adapter/gemini"
	_ "github.com/wujunwei928/token-usage/internal/adapter/goose"
	_ "github.com/wujunwei928/token-usage/internal/adapter/hermes"
	_ "github.com/wujunwei928/token-usage/internal/adapter/kilo"
	_ "github.com/wujunwei928/token-usage/internal/adapter/kimi"
	_ "github.com/wujunwei928/token-usage/internal/adapter/omp"
	_ "github.com/wujunwei928/token-usage/internal/adapter/openclaw"
	_ "github.com/wujunwei928/token-usage/internal/adapter/opencode"
	_ "github.com/wujunwei928/token-usage/internal/adapter/pi"
	_ "github.com/wujunwei928/token-usage/internal/adapter/qwen"
	_ "github.com/wujunwei928/token-usage/internal/adapter/zcode"
)
