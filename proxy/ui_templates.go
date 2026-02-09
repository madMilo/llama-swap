package proxy

import (
	"fmt"
	"html/template"

	"github.com/eknkc/amber"
)

type UITemplates struct {
	compiled map[string]*template.Template
}

func loadUITemplates() (*UITemplates, error) {
	templatePaths := map[string]string{
		"pages/models":     "ui/templates/pages/models.amber",
		"pages/running":    "ui/templates/pages/running.amber",
		"pages/logs":       "ui/templates/pages/logs.amber",
		"partials/models":  "ui/templates/partials/models.amber",
		"partials/running": "ui/templates/partials/running.amber",
		"partials/logs":    "ui/templates/partials/logs.amber",
	}

	opts := amber.Options{
		PrettyPrint:       true,
		LineNumbers:       false,
		VirtualFilesystem: uiVirtualFS(),
	}

	compiled := make(map[string]*template.Template, len(templatePaths))
	for name, path := range templatePaths {
		data, err := readUITemplate(path)
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", path, err)
		}
		tmpl, err := amber.CompileData(data, path, opts)
		if err != nil {
			return nil, fmt.Errorf("compile template %s: %w", path, err)
		}
		compiled[name] = tmpl
	}

	return &UITemplates{compiled: compiled}, nil
}

func (t *UITemplates) Template(name string) *template.Template {
	if t == nil {
		return nil
	}
	return t.compiled[name]
}
