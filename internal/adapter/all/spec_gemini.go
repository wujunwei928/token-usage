package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/gemini"
)

// Gemini participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("gemini", AdapterSpec("gemini", SpecOptions{Profile: gemini.Profile}))
}
