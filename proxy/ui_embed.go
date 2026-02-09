package proxy

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/static ui/templates
var uiFS embed.FS

func GetUIStaticFS() (http.FileSystem, error) {
	subFS, err := fs.Sub(uiFS, "ui/static")
	if err != nil {
		return nil, err
	}
	return http.FS(subFS), nil
}

func readUITemplate(path string) ([]byte, error) {
	return uiFS.ReadFile(path)
}

func readUIStatic(path string) ([]byte, error) {
	return uiFS.ReadFile("ui/static/" + path)
}

func uiVirtualFS() http.FileSystem {
	return http.FS(uiFS)
}
