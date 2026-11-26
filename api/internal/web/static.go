package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"fmt"
)

//go:embed all:dist
var assets embed.FS

func Handler() (http.Handler, error) {
	files, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded frontend assets: %w", err)
	}
	fileServer := http.FileServer(http.FS(files))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		if file, err := files.Open(name); err == nil {
			_ = file.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(files, "index.html"); err != nil {
			http.Error(w, "frontend assets are not built into this binary", http.StatusServiceUnavailable)
			return
		}
		r.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, r)
	}), nil
}
