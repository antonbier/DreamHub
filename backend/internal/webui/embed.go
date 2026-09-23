// Package webui embeds the built frontend (frontend/, `npm run build`,
// which outputs straight into dist/ here — see frontend/vite.config.ts) so
// the Go binary can serve it without any files on disk (ARCHITECTURE.md:
// DreamHub ships as a single static binary). A minimal placeholder
// dist/index.html is tracked in git so the backend builds and runs before
// the frontend has ever been built; see backend/README.md.
package webui

import "embed"

//go:embed dist
var Dist embed.FS
