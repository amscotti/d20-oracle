// Package web holds the frontend assets embedded into the d20-oracle binary.
package web

import "embed"

// FS contains the page templates and static assets (CSS and JS).
//
//go:embed templates static
var FS embed.FS
