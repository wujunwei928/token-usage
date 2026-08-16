package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/droid"
)

// Droid participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("droid", AdapterSpec("droid", SpecOptions{Profile: droid.Profile}))
}
