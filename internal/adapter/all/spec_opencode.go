// OpenCode's participation in the unified (all) report, registered by name.
// The captured-shared-args quirk (JSON forced on, mirroring the pre-refactor
// spec) lives in the adapter's factory closure.

package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/opencode"
)

func init() {
	RegisterSpec("opencode", AdapterSpec("opencode", SpecOptions{Profile: opencode.Profile}))
}
