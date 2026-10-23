// Package migrations embeds the drizzle-kit SQL files (generated from
// frontend/src/lib/server/db/schema.ts) so the binaries can apply them at
// startup; see internal/migrate.
package migrations

import "embed"

//go:embed *.sql meta/_journal.json
var FS embed.FS
