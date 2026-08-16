package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/copilot"
)

// Copilot participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("copilot", AdapterSpec("copilot", SpecOptions{Profile: copilot.Profile}))
}
