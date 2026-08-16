package all

import (
	"github.com/wujunwei928/token-usage/internal/adapter/omp"
)

// omp participates in the unified report via the adapter registry, with the
// pi-format session rows' project-path metadata.
func init() {
	RegisterSpec("omp", AdapterSpec("omp", SpecOptions{Profile: omp.Profile, IncludeProjectPath: true}))
}
