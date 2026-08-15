package core

import (
	"math"
	"sort"
)

// SummaryJSON renders a daily/weekly/monthly row for JSON output.
func SummaryJSON(row *UsageSummary) J {
	pairs := []any{
		"inputTokens", JUintV(row.InputTokens),
		"outputTokens", JUintV(row.OutputTokens),
		"cacheCreationTokens", JUintV(row.CacheCreationTokens),
		"cacheReadTokens", JUintV(row.CacheReadTokens),
		"totalTokens", JUintV(row.TotalTokens()),
		"totalCost", JFloatV(row.TotalCost),
		"modelsUsed", modelsUsedJ(row.ModelsUsed),
		"modelBreakdowns", modelBreakdownsJ(row.ModelBreakdowns),
	}
	if row.Date != nil {
		pairs = append(pairs, "date", JStrV(*row.Date))
	}
	if row.Month != nil {
		pairs = append(pairs, "month", JStrV(*row.Month))
	}
	if row.Week != nil {
		pairs = append(pairs, "week", JStrV(*row.Week))
	}
	if row.Project != nil {
		pairs = append(pairs, "project", JStrV(*row.Project))
	}
	if row.Credits != nil {
		pairs = append(pairs, "credits", JFloatV(*row.Credits))
	}
	return JObjV(pairs...)
}

// SessionSummaryJSON renders a session-grouped row for JSON output.
func SessionSummaryJSON(row *UsageSummary) J {
	pairs := []any{
		"sessionId", JOptStrV(row.SessionID),
		"inputTokens", JUintV(row.InputTokens),
		"outputTokens", JUintV(row.OutputTokens),
		"cacheCreationTokens", JUintV(row.CacheCreationTokens),
		"cacheReadTokens", JUintV(row.CacheReadTokens),
		"totalTokens", JUintV(row.TotalTokens()),
		"totalCost", JFloatV(row.TotalCost),
		"lastActivity", JOptStrV(row.LastActivity),
		"firstActivity", JOptStrV(row.FirstActivity),
		"modelsUsed", modelsUsedJ(row.ModelsUsed),
		"modelBreakdowns", modelBreakdownsJ(row.ModelBreakdowns),
		"projectPath", JOptStrV(row.ProjectPath),
	}
	if row.Credits != nil {
		pairs = append(pairs, "credits", JFloatV(*row.Credits))
	}
	return JObjV(pairs...)
}

// TotalsJSON sums rows into the totals object. The cost sum starts from
// negative zero, matching Rust's empty-float-sum behavior ("totalCost": -0.0).
func TotalsJSON(rows []UsageSummary) J {
	var input, output, cacheCreate, cacheRead, extra uint64
	for i := range rows {
		input += rows[i].InputTokens
		output += rows[i].OutputTokens
		cacheCreate += rows[i].CacheCreationTokens
		cacheRead += rows[i].CacheReadTokens
		extra += rows[i].ExtraTotalTokens
	}
	cost := math.Copysign(0, -1)
	for i := range rows {
		cost += rows[i].TotalCost
	}
	pairs := []any{
		"inputTokens", JUintV(input),
		"outputTokens", JUintV(output),
		"cacheCreationTokens", JUintV(cacheCreate),
		"cacheReadTokens", JUintV(cacheRead),
		"totalTokens", JUintV(input + output + cacheCreate + cacheRead + extra),
		"totalCost", JFloatV(cost),
	}
	var credits float64
	for i := range rows {
		if rows[i].Credits != nil {
			credits += *rows[i].Credits
		}
	}
	if credits > 0 {
		pairs = append(pairs, "credits", JFloatV(credits))
	}
	return JObjV(pairs...)
}

// GroupProjectOutput groups rows into a project -> rows JSON object.
func GroupProjectOutput(rows []UsageSummary) J {
	groups := map[string][]J{}
	for i := range rows {
		project := "unknown"
		if rows[i].Project != nil {
			project = *rows[i].Project
		}
		groups[project] = append(groups[project], SummaryJSON(&rows[i]))
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := []any{}
	for _, k := range keys {
		pairs = append(pairs, k, JArrV(groups[k]...))
	}
	return JObjV(pairs...)
}

// StripCostJ recursively removes cost fields for --no-cost.
func StripCostJ(v *J) {
	switch v.kind {
	case jObject:
		kept := v.keys[:0]
		for _, key := range v.keys {
			if key == "totalCost" || key == "costUSD" || key == "cost" {
				delete(v.vals, key)
				continue
			}
			child := v.vals[key]
			StripCostJ(&child)
			v.vals[key] = child
			kept = append(kept, key)
		}
		v.keys = kept
	case jArray:
		for i := range v.items {
			StripCostJ(&v.items[i])
		}
	}
}

func modelsUsedJ(models []string) J {
	items := make([]J, 0, len(models))
	for _, m := range models {
		items = append(items, JStrV(m))
	}
	return JArrV(items...)
}

func modelBreakdownsJ(breakdowns []ModelBreakdown) J {
	items := make([]J, 0, len(breakdowns))
	for i := range breakdowns {
		b := &breakdowns[i]
		items = append(items, JObjV(
			"modelName", JStrV(b.ModelName),
			"inputTokens", JUintV(b.InputTokens),
			"outputTokens", JUintV(b.OutputTokens),
			"cacheCreationTokens", JUintV(b.CacheCreationTokens),
			"cacheReadTokens", JUintV(b.CacheReadTokens),
			"cost", JFloatV(b.Cost),
		))
	}
	return JArrV(items...)
}
