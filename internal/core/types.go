// Package core is the Go port of the ccusage-core crate: entry types, cost
// modes, aggregation, and report output.
package core

// TokenUsageRaw is the raw usage object from a JSONL entry.
type TokenUsageRaw struct {
	InputTokens              uint64            `json:"input_tokens"`
	OutputTokens             uint64            `json:"output_tokens"`
	CacheCreationInputTokens uint64            `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     uint64            `json:"cache_read_input_tokens"`
	Speed                    *string           `json:"speed"`
	CacheCreation            *CacheCreationRaw `json:"cache_creation"`
}

// CacheCreationRaw carries the split 5m/1h cache write counters.
type CacheCreationRaw struct {
	Ephemeral5mInputTokens uint64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens uint64 `json:"ephemeral_1h_input_tokens"`
}

// CacheCreationTokenCount prefers the split counters when present.
func (u TokenUsageRaw) CacheCreationTokenCount() uint64 {
	if u.CacheCreation != nil {
		return u.CacheCreation.Ephemeral5mInputTokens + u.CacheCreation.Ephemeral1hInputTokens
	}
	return u.CacheCreationInputTokens
}

// TotalUsageTokens sums the four token classes.
func TotalUsageTokens(u TokenUsageRaw) uint64 {
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokenCount() + u.CacheReadInputTokens
}

// Speed values recognized in raw usage objects.
const (
	SpeedStandard = "standard"
	SpeedFast     = "fast"
)

// UsageMessage is the message envelope of a JSONL entry.
type UsageMessage struct {
	Usage  TokenUsageRaw `json:"usage"`
	Model  *string       `json:"model"`
	ID     *string       `json:"id"`
}

// UsageEntry is one parsed JSONL line (camelCase field names).
type UsageEntry struct {
	SessionID         *string       `json:"sessionId"`
	Timestamp         string        `json:"timestamp"`
	Version           *string       `json:"version"`
	Message           UsageMessage  `json:"message"`
	CostUSD           *float64      `json:"costUSD"`
	RequestID         *string       `json:"requestId"`
	IsAPIErrorMessage *bool         `json:"isApiErrorMessage"`
	IsSidechain       *bool         `json:"isSidechain"`
}

// TokenCounts accumulates usage across entries.
type TokenCounts struct {
	InputTokens          uint64
	OutputTokens         uint64
	CacheCreationTokens  uint64
	CacheReadTokens      uint64
	ExtraTotalTokens     uint64
}

// AddUsage folds a raw usage object into the accumulator.
func (c *TokenCounts) AddUsage(u TokenUsageRaw) {
	c.InputTokens += u.InputTokens
	c.OutputTokens += u.OutputTokens
	c.CacheCreationTokens += u.CacheCreationTokenCount()
	c.CacheReadTokens += u.CacheReadInputTokens
}

// Total sums all token classes including extras.
func (c TokenCounts) Total() uint64 {
	return c.InputTokens + c.OutputTokens + c.CacheCreationTokens + c.CacheReadTokens + c.ExtraTotalTokens
}

// ModelBreakdown is the per-model split of a summary row.
type ModelBreakdown struct {
	ModelName            string  `json:"modelName"`
	InputTokens          uint64  `json:"inputTokens"`
	OutputTokens         uint64  `json:"outputTokens"`
	CacheCreationTokens  uint64  `json:"cacheCreationTokens"`
	CacheReadTokens      uint64  `json:"cacheReadTokens"`
	ExtraTotalTokens     uint64  `json:"-"`
	Cost                 float64 `json:"costUSD"`
	MissingPricing       bool    `json:"-"`
}

// LoadedEntry is a validated entry enriched with load-time context.
type LoadedEntry struct {
	Data                UsageEntry
	Timestamp           int64 // Unix milliseconds
	Date                string
	Project             string
	SessionID           string
	ProjectPath         string
	Cost                float64
	ExtraTotalTokens    uint64
	Credits             *float64
	MessageCount        *uint64
	Model               *string
	UsageLimitResetTime *int64
	MissingPricingModel *string
}

// UsageSummary is one aggregated report row.
type UsageSummary struct {
	Date                *string  `json:"date,omitempty"`
	Month               *string  `json:"month,omitempty"`
	Week                *string  `json:"week,omitempty"`
	SessionID           *string  `json:"sessionId,omitempty"`
	ProjectPath         *string  `json:"projectPath,omitempty"`
	LastActivity        *string  `json:"lastActivity,omitempty"`
	FirstActivity       *string  `json:"firstActivity,omitempty"`
	InputTokens         uint64   `json:"inputTokens"`
	OutputTokens        uint64   `json:"outputTokens"`
	CacheCreationTokens uint64   `json:"cacheCreationTokens"`
	CacheReadTokens     uint64   `json:"cacheReadTokens"`
	ExtraTotalTokens    uint64   `json:"-"`
	TotalCost           float64  `json:"totalCost"`
	Credits             *float64 `json:"credits,omitempty"`
	MessageCount        *uint64  `json:"messageCount,omitempty"`
	ModelsUsed          []string `json:"modelsUsed"`
	ModelBreakdowns     []ModelBreakdown `json:"modelBreakdowns"`
	Project             *string  `json:"project,omitempty"`
	Versions            []string `json:"versions,omitempty"`
}

// TotalTokens sums the four token classes plus extras.
func (s UsageSummary) TotalTokens() uint64 {
	return s.InputTokens + s.OutputTokens + s.CacheCreationTokens + s.CacheReadTokens + s.ExtraTotalTokens
}
