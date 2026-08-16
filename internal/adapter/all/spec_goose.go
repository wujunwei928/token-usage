package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/goose"
)

// Goose participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("goose", AdapterSpec("goose", SpecOptions{Profile: goose.Profile}))
}
