package controller

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed config_templates/*
var configTemplates embed.FS

func renderTemplate(distribution, templateName string, templateData map[string]any) (string, error) {
	tpath := fmt.Sprintf("config_templates/%s/%s", distribution, templateName)

	t, err := template.ParseFS(configTemplates, tpath)
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
