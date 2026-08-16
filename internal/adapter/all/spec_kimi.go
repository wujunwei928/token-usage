package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/kimi"
)

// Kimi participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("kimi", AdapterSpec("kimi", SpecOptions{Profile: kimi.Profile}))
}
