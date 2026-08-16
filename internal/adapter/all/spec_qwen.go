package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/qwen"
)

// Qwen participates in the unified report via the adapter registry; its
// detection honors HasData so an empty window still reports Qwen detected.
func init() {
	RegisterSpec("qwen", AdapterSpec("qwen", SpecOptions{Profile: qwen.Profile}))
}
