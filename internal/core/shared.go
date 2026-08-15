package core

import (
	"os"
	"strconv"
)

// CostMode selects where costs come from.
type CostMode int

// Cost modes.
const (
	ModeAuto CostMode = iota
	ModeCalculate
	ModeDisplay
)

// ParseCostMode maps the CLI string onto a CostMode; ok is false for unknown.
func ParseCostMode(value string) (CostMode, bool) {
	switch value {
	case "auto":
		return ModeAuto, true
	case "calculate":
		return ModeCalculate, true
	case "display":
		return ModeDisplay, true
	}
	return ModeAuto, false
}

// SortOrder orders report rows by their date key.
type SortOrder int

// Sort orders.
const (
	OrderAsc SortOrder = iota
	OrderDesc
)

// ParseSortOrder maps the CLI string onto a SortOrder; ok is false for unknown.
func ParseSortOrder(value string) (SortOrder, bool) {
	switch value {
	case "asc":
		return OrderAsc, true
	case "desc":
		return OrderDesc, true
	}
	return OrderAsc, false
}

// WeekDay numbers weekdays with Sunday = 0, matching the reference.
type WeekDay int

// Weekday constants.
const (
	Sunday WeekDay = iota
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

// ParseWeekDay maps the CLI string onto a WeekDay; ok is false for unknown.
func ParseWeekDay(value string) (WeekDay, bool) {
	days := map[string]WeekDay{
		"sunday": Sunday, "monday": Monday, "tuesday": Tuesday, "wednesday": Wednesday,
		"thursday": Thursday, "friday": Friday, "saturday": Saturday,
	}
	d, ok := days[value]
	return d, ok
}

// NamedPiStore names an additional pi-format session store from the
// ccusage.json pi.stores section (ticket 10; consumed by all-agent reports).
type NamedPiStore struct {
	Name string
	Path string
}

// SharedArgs mirrors the reference SharedArgs: flags shared by report commands.
type SharedArgs struct {
	Since            *string
	Until            *string
	Last             *uint32
	JSON             bool
	Mode             CostMode
	Debug            bool
	DebugSamples     int
	Order            SortOrder
	Breakdown        bool
	Offline          bool
	NoOffline        bool
	Color            bool
	NoColor          bool
	Timezone         *string
	JQ               *string
	Config           *string
	Compact          bool
	SingleThread     bool
	NoCost           bool
	// PricingOverrides carries the merged ccusage.json pricingOverrides into
	// the pricing map load (ticket 10).
	PricingOverrides map[string]PricingOverride
	// PIStores carries validated ccusage.json pi.stores entries (ticket 10).
	PIStores []NamedPiStore
}

// OfflineEffective resolves the --offline/--no-offline combo. The reference
// binary ignores a CCUSAGE_OFFLINE env var (dropped after the TS era), so this
// port does too.
func (s *SharedArgs) OfflineEffective() bool {
	if s.NoOffline {
		return false
	}
	return s.Offline
}

// LogLevel returns LOG_LEVEL when set to a valid u8, else nil.
func LogLevel() *int {
	raw, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		return nil
	}
	level, err := strconv.ParseUint(raw, 10, 8)
	if err != nil {
		return nil
	}
	v := int(level)
	return &v
}
