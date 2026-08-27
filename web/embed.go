package web

import "embed"

// FS is the embedded UI: the terminal SPA, the loopback-only settings page,
// and their shared assets.
//
//go:embed index.html settings.html favicon.svg css js vendor
var FS embed.FS
