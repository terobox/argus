// Package web embeds the built frontend assets (web/dist) so the argus server
// serves the dashboard from a single binary. Build the frontend first with
// `pnpm build`; the dist directory is populated by Vite. The commit includes a
// placeholder (dist/.gitkeep) so the server compiles even before a frontend
// build has run.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist returns the built frontend assets as a filesystem rooted at "dist".
func Dist() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}
