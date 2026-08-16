package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/openclaw"
)

// OpenClaw participates in the unified report via the adapter registry; the
// unified report passes no custom path.
func init() {
	RegisterSpec("openclaw", AdapterSpec("openclaw", SpecOptions{Profile: openclaw.Profile}))
}
