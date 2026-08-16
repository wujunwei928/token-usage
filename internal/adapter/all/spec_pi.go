package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/pi"
)

// pi participates in the unified report via the adapter registry, with
// project-path metadata on session rows like the reference's
// load_pi_format_agent_rows.
func init() {
	RegisterSpec("pi", AdapterSpec("pi", SpecOptions{Profile: pi.Profile, IncludeProjectPath: true}))
}
