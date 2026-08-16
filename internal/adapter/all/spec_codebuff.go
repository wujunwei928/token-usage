package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/codebuff"
)

// Codebuff participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("codebuff", AdapterSpec("codebuff", SpecOptions{Profile: codebuff.Profile}))
}
