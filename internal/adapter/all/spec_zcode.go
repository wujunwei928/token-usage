package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/zcode"
)

// zcode participates in the unified report via the adapter registry, at the
// roster position appended after the reference roster: an adapter beyond
// upstream ccusage (ADR 0006). Detection honors HasData.
func init() {
	RegisterSpec("zcode", AdapterSpec("zcode", SpecOptions{Profile: zcode.Profile}))
}
