package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/hermes"
)

// Hermes participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("hermes", AdapterSpec("hermes", SpecOptions{Profile: hermes.Profile}))
}
