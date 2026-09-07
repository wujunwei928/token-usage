package core

// Model alias resolution ported from rust/crates/ccusage-core/src/model_aliases.rs.
// TOKEN_USAGE_MODEL_ALIASES wins; CCUSAGE_MODEL_ALIASES is the legacy
// fallback (ADR 0008).

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
)

const (
	modelAliasesEnv       = "TOKEN_USAGE_MODEL_ALIASES"
	modelAliasesEnvLegacy = "CCUSAGE_MODEL_ALIASES"
)

var (
	aliasesMu    sync.RWMutex
	aliasesMap   map[string]string
	aliasesReady bool

	testAliasesMu sync.Mutex
)

// ResolveModelName resolves a model name through the model-aliases env
// (TOKEN_USAGE_MODEL_ALIASES, legacy CCUSAGE_MODEL_ALIASES), including -fast
// variants. 先归一再查别名:不同 agent 对同一模型的大小写和 agent 前缀
// ([omp] / [pi] 等)各不相同,统计与定价必须在同一个规范名上合并。
func ResolveModelName(model string) string {
	model = normalizeModelName(model)
	aliases := modelAliases()
	if alias, ok := aliases[model]; ok && alias != "" {
		return alias
	}
	if base, hasSuffix := strings.CutSuffix(model, "-fast"); hasSuffix {
		if alias, ok := aliases[base]; ok && alias != "" {
			return alias + "-fast"
		}
	}
	return model
}

// normalizeModelName canonicalizes an agent-reported model name: 去首尾空白、
// 剥离开头的 "[tag] " agent 前缀、统一小写。
func normalizeModelName(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "[") {
		if end := strings.Index(model, "]"); end >= 0 {
			model = strings.TrimSpace(model[end+1:])
		}
	}
	return model
}

func modelAliases() map[string]string {
	aliasesMu.RLock()
	if aliasesReady {
		aliases := aliasesMap
		aliasesMu.RUnlock()
		return aliases
	}
	aliasesMu.RUnlock()

	aliasesMu.Lock()
	defer aliasesMu.Unlock()
	if !aliasesReady {
		aliasesMap = loadModelAliasesFromEnv()
		aliasesReady = true
	}
	return aliasesMap
}

func loadModelAliasesFromEnv() map[string]string {
	raw := strings.TrimSpace(os.Getenv(modelAliasesEnv))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(modelAliasesEnvLegacy))
	}
	if raw == "" {
		return map[string]string{}
	}
	return parseModelAliases(raw)
}

func parseModelAliases(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			return parsed
		}
	}

	stripped := trimmed
	if inner, hasBraces := strings.CutPrefix(trimmed, "{"); hasBraces {
		if inner, hasClose := strings.CutSuffix(inner, "}"); hasClose {
			stripped = inner
		}
	}

	aliases := map[string]string{}
	for _, pair := range strings.FieldsFunc(stripped, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	}) {
		from, to, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from != "" && to != "" {
			aliases[from] = to
		}
	}
	return aliases
}

// SetModelAliasesForTests swaps the alias table and returns a restore func,
// mirroring the Rust set_model_aliases_for_tests guard.
func SetModelAliasesForTests(aliases map[string]string) (restore func()) {
	testAliasesMu.Lock()
	aliasesMu.Lock()
	previous, wasReady := aliasesMap, aliasesReady
	aliasesMap = aliases
	aliasesReady = true
	aliasesMu.Unlock()
	return func() {
		aliasesMu.Lock()
		aliasesMap = previous
		aliasesReady = wasReady
		aliasesMu.Unlock()
		testAliasesMu.Unlock()
	}
}
