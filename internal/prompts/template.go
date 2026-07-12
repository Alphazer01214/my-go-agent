package prompts

import (
	"bytes"
	"text/template"
)

type Template struct {
	name string
	tmpl *template.Template
}

func NewTemplate(name string, text string) (*Template, error) {
	tmpl, err := template.New(name).Parse(text)
	if err != nil {
		return nil, err
	}
	return &Template{
		name: name,
		tmpl: tmpl,
	}, nil
}

func (t *Template) Name() string {
	return t.name
}

// Render 渲染模板，模板内容的 {{.xxx}} 需要和 data 的 field 一致
func (t *Template) Render(data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
