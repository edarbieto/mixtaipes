package httpapi

import (
	"io/fs"
	"net/http"
	"strings"
)

// shareShells maps share-link prefixes to the static page that renders them client-side.
var shareShells = map[string]string{
	"/m/":    "m/index.html",
	"/es/m/": "es/m/index.html",
}

// staticHandler serves the Astro build. Share links (/m/{slug}) have no pre-rendered
// page per slug, so they get the share shell, which fetches the playlist itself.
func staticHandler(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shell, ok := shareShell(r.URL.Path); ok {
			http.ServeFileFS(w, r, files, shell)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func shareShell(path string) (string, bool) {
	for prefix, shell := range shareShells {
		slug, ok := strings.CutPrefix(path, prefix)
		if ok && slug != "" && !strings.Contains(slug, "/") {
			return shell, true
		}
	}
	return "", false
}
