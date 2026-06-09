package controller

import (
	"bytes"
	"embed"
	"fmt"
	"path/filepath"
	"text/template"
)

//go:embed config_templates/*
var configTemplates embed.FS

var tFuncs = template.FuncMap{
	"idx1": func(i int) int { return i + 1 },
}

func renderTemplate(prefix, templateName string, templateData map[string]any) (string, error) {
	tpath := filepath.Join("config_templates", prefix, templateName)

	t, err := template.New(templateName).Funcs(tFuncs).ParseFS(configTemplates, tpath)
	if err != nil {
		return "", fmt.Errorf("failed to parse config template %s: %w", tpath, err)
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, templateData)
	if err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", tpath, err)
	}

	return buf.String(), nil
}
