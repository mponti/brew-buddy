package web

import (
	"embed"
)

// Embed the 'templates' directory.
// The path is relative to this file (internal/web/web.go).
//
//go:embed templates
var Assets embed.FS

// GetTemplatesFS returns the embedded filesystem.
// (Exporting the variable directly is also fine, but a getter is sometimes preferred).
func GetTemplatesFS() embed.FS {
	return Assets
}
