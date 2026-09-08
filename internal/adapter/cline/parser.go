package cline

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wujunwei928/token-usage/internal/core"
)

// clineMessagesFile is a lenient view of one <id>.messages.json.
type clineMessagesFile struct {
	SessionID string             `json:"sessionId"`
	Messages  []clineMessageItem `json:"messages"`
}

// clineMessageItem is one conversation message; only assistant rows carry
// metrics (the token four-tuple) and modelInfo.
type clineMessageItem struct {
	ID        string          `json:"id"`
	Role      string          `json:"role"`
	TS        int64           `json:"ts"`
	Metrics   *clineMetrics   `json:"metrics"`
	ModelInfo *clineModelInfo `json:"modelInfo"`
}

type clineMetrics struct {
	InputTokens      uint64    `json:"inputTokens"`
	OutputTokens     uint64    `json:"outputTokens"`
	CacheReadTokens  uint64    `json:"cacheReadTokens"`
	CacheWriteTokens uint64    `json:"cacheWriteTokens"`
	Cost             costValue `json:"cost"`
}

// costValue is metrics.cost: a non-negative finite number (JSON number or
// numeric string) counts as a provider-reported costUSD; negative, NaN/Inf,
// or unparsable values count as absent, mirroring the reference parser's
// extract_non_negative_finite_f64.
type costValue struct {
	value float64
	ok    bool
}

func (c *costValue) UnmarshalJSON(raw []byte) error {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return nil
	}
	v, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil
	}
	c.value, c.ok = v, true
	return nil
}

// ptr returns the provider-reported cost, or nil when absent.
func (c costValue) ptr() *float64 {
	if !c.ok {
		return nil
	}
	v := c.value
	return &v
}

type clineModelInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
}

// clineSessionFile is the subset of the sibling <id>.json manifest the
// adapter needs: it seeds the session id and the model/provider/workspace
// fallbacks for messages that carry no modelInfo of their own.
type clineSessionFile struct {
	SessionID     string `json:"session_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	CWD           string `json:"cwd"`
	WorkspaceRoot string `json:"workspace_root"`
}

// clineEntry is one intermediate usage record from one session directory.
type clineEntry struct {
	ID        string
	SessionID string
	CWD       string
	Model     string
	Provider  string
	Timestamp int64
	Usage     core.TokenUsageRaw
	CostUSD   *float64
}

// readFilePair parses one <task>/<task>.messages.json plus its sibling
// <task>.json manifest. Only assistant messages with metrics count — and a
// zero-token message still counts when it reports a cost.
func readFilePair(messagesPath string) []clineEntry {
	raw, err := os.ReadFile(messagesPath)
	if err != nil {
		return nil
	}
	var parsed clineMessagesFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	manifest := readManifest(messagesPath)
	fallbackTS := fileModifiedTimestamp(messagesPath)
	sessionID := firstNonEmpty(
		parsed.SessionID,
		manifest.SessionID,
		strings.TrimSuffix(filepath.Base(messagesPath), ".messages.json"),
	)
	// The manifest's workspace_root wins over its cwd; both normalize to
	// forward slashes so one project never splits into two.
	project := normalizeWorkspacePath(manifest.WorkspaceRoot)
	if project == "" {
		project = normalizeWorkspacePath(manifest.CWD)
	}
	// The manifest seeds the running model/provider that per-message
	// modelInfo then overrides.
	model := firstNonEmpty(manifest.Model, "unknown")
	provider := manifest.Provider
	var entries []clineEntry
	for _, msg := range parsed.Messages {
		if msg.Role != "assistant" {
			continue
		}
		if msg.ModelInfo != nil {
			if msg.ModelInfo.ID != "" {
				model = msg.ModelInfo.ID
			}
			if msg.ModelInfo.Provider != "" {
				provider = msg.ModelInfo.Provider
			}
		}
		if msg.Metrics == nil {
			continue
		}
		// Cline's inputTokens already contains the cache buckets, so input
		// excludes them (clamped at zero) while the cache classes stay
		// recorded separately — totals then match inputTokens+outputTokens.
		usage := core.TokenUsageRaw{
			InputTokens: saturatingSubU64(
				saturatingSubU64(msg.Metrics.InputTokens, msg.Metrics.CacheReadTokens),
				msg.Metrics.CacheWriteTokens,
			),
			OutputTokens:             msg.Metrics.OutputTokens,
			CacheCreationInputTokens: msg.Metrics.CacheWriteTokens,
			CacheReadInputTokens:     msg.Metrics.CacheReadTokens,
		}
		cost := msg.Metrics.Cost.ptr()
		if core.TotalUsageTokens(usage) == 0 && cost == nil {
			continue
		}
		timestamp := msg.TS
		if timestamp <= 0 {
			timestamp = fallbackTS
		}
		entries = append(entries, clineEntry{
			ID:        msg.ID,
			SessionID: sessionID,
			CWD:       project,
			Model:     model,
			Provider:  provider,
			Timestamp: timestamp,
			Usage:     usage,
			CostUSD:   cost,
		})
	}
	return entries
}

// readManifest loads the sibling <task>.json session manifest; an unreadable
// or malformed manifest degrades to the zero value.
func readManifest(messagesPath string) clineSessionFile {
	sessionPath := strings.TrimSuffix(messagesPath, ".messages.json") + ".json"
	raw, err := os.ReadFile(sessionPath)
	if err != nil {
		return clineSessionFile{}
	}
	var parsed clineSessionFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return clineSessionFile{}
	}
	return parsed
}

// normalizeWorkspacePath canonicalizes separators to forward slashes.
func normalizeWorkspacePath(path string) string {
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.FromSlash(strings.ReplaceAll(path, "\\", "/")))
}

func fileModifiedTimestamp(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().UnixMilli()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// saturatingSubU64 clamps at zero like the reference's saturating_sub.
func saturatingSubU64(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}
