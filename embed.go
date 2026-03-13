package outpost

import "embed"

// WebUI contains the built frontend assets.
// If the directory doesn't exist at build time, the binary still compiles
// but the embedded FS will be empty.
//
//go:embed all:web-ui/dist
var WebUI embed.FS

// Migrations contains the SQL migration files.
//
//go:embed migrations/*.sql
var Migrations embed.FS

// OpenAPISpec contains the OpenAPI 3.0 specification file.
//
//go:embed docs/openapi.yaml
var OpenAPISpec []byte
