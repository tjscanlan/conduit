// Package ui embeds the server-rendered templates and static assets into the binary.
// Imported by cmd/ui to serve files without a separate asset directory at runtime.
package ui

import "embed"

//go:embed static templates
var Assets embed.FS
