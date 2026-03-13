package onboarding

import (
	"embed"
	"text/template"
)

//go:embed templates/*
var templatesFS embed.FS

func mustTemplate(name string, funcs template.FuncMap) *template.Template {
	return template.Must(template.New(name).Funcs(funcs).ParseFS(templatesFS, "templates/"+name))
}
