package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/cline"
)

// Cline participates in the unified report via the adapter registry.
func init() {
	RegisterSpec("cline", AdapterSpec("cline", SpecOptions{Profile: cline.Profile, IncludeProjectPath: true}))
}
