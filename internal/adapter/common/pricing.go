package common

import (
	"github.com/wujunwei928/token-usage/internal/core"
)

// LoadPricingRawOffline loads the pricing table with the raw --offline flag
// (no --no-offline fold) and the LOG_LEVEL spinner gate — the semantics the
// amp/copilot/gemini/kimi/openclaw factories carried. Pricing stays a
// per-agent choice made at the factory call site (ADR 0009); candidate 3
// owns unifying the semantics themselves.
func LoadPricingRawOffline(shared *core.SharedArgs) *core.PricingMap {
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.Offline, refreshLog, shared.PricingOverrides)
}

// LoadPricingDisplayGated skips the pricing table entirely in display mode
// and folds --no-offline into the offline flag — the semantics the
// goose/kilo/pi factories carried.
func LoadPricingDisplayGated(shared *core.SharedArgs) *core.PricingMap {
	if shared.Mode == core.ModeDisplay {
		return nil
	}
	refreshLog := true
	if level := core.LogLevel(); level != nil && *level == 0 {
		refreshLog = false
	}
	return core.LoadWithOverrides(shared.OfflineEffective(), refreshLog, shared.PricingOverrides)
}
