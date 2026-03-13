package outpost

import "embed"

// WebUI contains the built frontend assets.
// If the directory doesn't exist at build time, the binary still compiles
// but the embedded FS will be empty.
//
//go:embed all:web-ui/dist
var WebUI embed.FS
