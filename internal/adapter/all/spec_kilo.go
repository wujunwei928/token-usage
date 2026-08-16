package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/kilo"
)

// Kilo participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("kilo", AdapterSpec("kilo", SpecOptions{Profile: kilo.Profile}))
}
