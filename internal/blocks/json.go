package blocks

import (
	"math"

	"github.com/wujunwei928/token-usage/internal/core"
)

// BlockJSON renders one block for --json output. Burn rate and projection
// appear only on active blocks; tokenLimitStatus only when a limit resolves.
func BlockJSON(block *Block, tokenLimit *string, maxTokens uint64) core.J {
	pairs := []any{
		"id", core.JStrV(block.ID),
		"startTime", core.JStrV(core.FormatRFC3339Millis(block.StartTime)),
		"endTime", core.JStrV(core.FormatRFC3339Millis(block.EndTime)),
		"actualEndTime", optTimestampJ(block.ActualEndTime),
		"isActive", core.JBoolV(block.IsActive),
		"isGap", core.JBoolV(block.IsGap),
		"entries", core.JUintV(uint64(len(block.Entries))),
		"tokenCounts", core.JObjV(
			"inputTokens", core.JUintV(block.TokenCounts.InputTokens),
			"outputTokens", core.JUintV(block.TokenCounts.OutputTokens),
			"cacheCreationInputTokens", core.JUintV(block.TokenCounts.CacheCreationTokens),
			"cacheReadInputTokens", core.JUintV(block.TokenCounts.CacheReadTokens),
		),
		"totalTokens", core.JUintV(block.TokenCounts.Total()),
		"costUSD", jsonFloatJ(block.CostUSD),
		"models", modelsJ(block.Models),
		"burnRate", burnRateJ(block),
		"projection", projectionJ(block),
	}
	if projection := ProjectBlockUsage(block); projection != nil {
		if limit := ParseTokenLimit(tokenLimit, maxTokens); limit != nil {
			percent := float64(projection.TotalTokens) / float64(*limit) * 100.0
			status := "ok"
			if projection.TotalTokens > *limit {
				status = "exceeds"
			} else if float64(projection.TotalTokens) > float64(*limit)*WarningThreshold {
				status = "warning"
			}
			pairs = append(pairs, "tokenLimitStatus", core.JObjV(
				"limit", core.JUintV(*limit),
				"projectedUsage", core.JUintV(projection.TotalTokens),
				"percentUsed", core.JFloatV(percent),
				"status", core.JStrV(status),
			))
		}
	}
	if block.UsageLimitResetTime != nil {
		pairs = append(pairs, "usageLimitResetTime", core.JStrV(core.FormatRFC3339Millis(*block.UsageLimitResetTime)))
	}
	return core.JObjV(pairs...)
}

func optTimestampJ(ts *int64) core.J {
	if ts == nil {
		return core.JNullV
	}
	return core.JStrV(core.FormatRFC3339Millis(*ts))
}

func burnRateJ(block *Block) core.J {
	// Burn rate only appears on active blocks.
	if !block.IsActive {
		return core.JNullV
	}
	rate := CalculateBurnRate(block)
	if rate == nil {
		return core.JNullV
	}
	return core.JObjV(
		"tokensPerMinute", core.JFloatV(rate.TokensPerMinute),
		"tokensPerMinuteForIndicator", core.JFloatV(rate.TokensPerMinuteForIndicator),
		"costPerHour", core.JFloatV(rate.CostPerHour),
	)
}

func projectionJ(block *Block) core.J {
	projection := ProjectBlockUsage(block)
	if projection == nil {
		return core.JNullV
	}
	return core.JObjV(
		"totalTokens", core.JUintV(projection.TotalTokens),
		"totalCost", core.JFloatV(projection.TotalCost),
		"remainingMinutes", core.JUintV(projection.RemainingMinutes),
	)
}

func modelsJ(models []string) core.J {
	items := make([]core.J, 0, len(models))
	for _, m := range models {
		items = append(items, core.JStrV(m))
	}
	return core.JArrV(items...)
}

// jsonFloatJ mirrors the reference json_float: finite whole floats in the i64
// range render as integers, everything else as a float.
func jsonFloatJ(v float64) core.J {
	if !math.IsInf(v, 0) && !math.IsNaN(v) && v == math.Trunc(v) &&
		v >= -9223372036854775808.0 && v <= 9223372036854775807.0 {
		return core.JIntV(int64(v))
	}
	return core.JFloatV(v)
}
