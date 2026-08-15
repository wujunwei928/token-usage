package server

import (
	"embed"
)

// templateFS holds the SSR page templates; staticFS holds the embedded
// frontend assets (ECharts, stylesheet). Both ship inside the single server
// binary.
//
//go:embed web/templates/*.html
var templateFS embed.FS

//go:embed web/static
var staticFS embed.FS
