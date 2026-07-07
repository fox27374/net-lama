package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

// Handler serves the embedded web UI.
func Handler() http.Handler {
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(content))
}
