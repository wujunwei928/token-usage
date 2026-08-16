package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/amp"
)

// Amp participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("amp", AdapterSpec("amp", SpecOptions{Profile: amp.Profile}))
}
