package docs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type componentContext struct {
	Name               string
	Type               string
	FrontMatterSummary string
	Summary            string
	Description        string
	Categories         string
	Examples           []AnnotatedExample
	Fields             []FieldSpecCtx
	Footnotes          string
	CommonConfig       string
	AdvancedConfig     string
	Status             string
	Version            string
}

var componentTemplate = FieldsTemplate(false) + `---
title: {{.Name}}
type: {{.Type}}
status: {{.Status}}
{{if gt (len .FrontMatterSummary) 0 -}}
description: "{{.FrontMatterSummary}}"
{{end -}}
{{if gt (len .Categories) 0 -}}
categories: {{.Categories}}
{{end -}}
---

<!--
     THIS FILE IS AUTOGENERATED!

     To make changes please edit the corresponding source file under internal/impl/<provider>.
-->

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

{{if eq .Status "beta" -}}
:::caution BETA
This component is mostly stable but breaking changes could still be made outside of major version releases if a fundamental problem with the component is found.
:::
{{end -}}
{{if eq .Status "experimental" -}}
:::caution EXPERIMENTAL
This component is experimental and therefore subject to change or removal outside of major version releases.
:::
{{end -}}
{{if eq .Status "deprecated" -}}
:::warning DEPRECATED
This component is deprecated and will be removed in the next major version release. Please consider moving onto [alternative components](#alternatives).
:::
{{end -}}

{{if gt (len .Summary) 0 -}}
{{.Summary}}
{{end -}}{{if gt (len .Version) 0}}
Introduced in version {{.Version}}.
{{end}}
{{if eq .CommonConfig .AdvancedConfig -}}
` + "```yml" + `
# Config fields, showing default values
{{.CommonConfig -}}
` + "```" + `
{{else}}
<Tabs defaultValue="common" values={{"{"}}[
  { label: 'Common', value: 'common', },
  { label: 'Advanced', value: 'advanced', },
]{{"}"}}>

<TabItem value="common">

` + "```yml" + `
# Common config fields, showing default values
{{.CommonConfig -}}
` + "```" + `

</TabItem>
<TabItem value="advanced">

` + "```yml" + `
# All config fields, showing default values
{{.AdvancedConfig -}}
` + "```" + `

</TabItem>
</Tabs>
{{end -}}
{{if gt (len .Description) 0}}
{{.Description}}
{{end}}
{{if and (le (len .Fields) 4) (gt (len .Fields) 0) -}}
## Fields

{{template "field_docs" . -}}
{{end -}}

{{if gt (len .Examples) 0 -}}
## Examples

<Tabs defaultValue="{{ (index .Examples 0).Title }}" values={{"{"}}[
{{range $i, $example := .Examples -}}
  { label: '{{$example.Title}}', value: '{{$example.Title}}', },
{{end -}}
]{{"}"}}>

{{range $i, $example := .Examples -}}
<TabItem value="{{$example.Title}}">

{{if gt (len $example.Summary) 0 -}}
{{$example.Summary}}
{{end}}
{{if gt (len $example.Config) 0 -}}
` + "```yaml" + `{{$example.Config}}` + "```" + `
{{end}}
</TabItem>
{{end -}}
</Tabs>

{{end -}}

{{if gt (len .Fields) 4 -}}
## Fields

{{template "field_docs" . -}}
{{end -}}

{{if gt (len .Footnotes) 0 -}}
{{.Footnotes}}
{{end}}
`

func createOrderedConfig(t Type, rawExample any, filter FieldFilter) (*yaml.Node, error) {
	var newNode yaml.Node
	if err := newNode.Encode(rawExample); err != nil {
		return nil, err
	}

	sanitConf := NewSanitiseConfig()
	sanitConf.RemoveTypeField = true
	sanitConf.Filter = filter
	sanitConf.ForExample = true
	if err := SanitiseYAML(t, &newNode, sanitConf); err != nil {
		return nil, err
	}

	return &newNode, nil
}

func genExampleConfigs(t Type, nest bool, fullConfigExample any) (commonConfigStr, advConfigStr string, err error) {
	var advConfig, commonConfig any
	if advConfig, err = createOrderedConfig(t, fullConfigExample, func(f FieldSpec) bool {
		return !f.IsDeprecated
	}); err != nil {
		panic(err)
	}
	if commonConfig, err = createOrderedConfig(t, fullConfigExample, func(f FieldSpec) bool {
		return !f.IsAdvanced && !f.IsDeprecated
	}); err != nil {
		panic(err)
	}

	if nest {
		advConfig = map[string]any{string(t): advConfig}
		commonConfig = map[string]any{string(t): commonConfig}
	}

	advancedConfigBytes, err := marshalYAML(advConfig)
	if err != nil {
		panic(err)
	}
	commonConfigBytes, err := marshalYAML(commonConfig)
	if err != nil {
		panic(err)
	}

	return string(commonConfigBytes), string(advancedConfigBytes), nil
}

// AsMarkdown renders the spec of a component, along with a full configuration
// example, into a markdown document.
func (c *ComponentSpec) AsMarkdown(nest bool, fullConfigExample any) ([]byte, error) {
	if strings.Contains(c.Summary, "\n\n") {
		return nil, fmt.Errorf("%v component '%v' has a summary containing empty lines", c.Type, c.Name)
	}

	ctx := componentContext{
		Name:        c.Name,
		Type:        string(c.Type),
		Summary:     c.Summary,
		Description: c.Description,
		Examples:    c.Examples,
		Footnotes:   c.Footnotes,
		Status:      string(c.Status),
		Version:     c.Version,
	}
	if ctx.Status == "" {
		ctx.Status = string(StatusStable)
	}

	if len(c.Categories) > 0 {
		cats, _ := json.Marshal(c.Categories)
		ctx.Categories = string(cats)
	}

	var err error
	if ctx.CommonConfig, ctx.AdvancedConfig, err = genExampleConfigs(c.Type, nest, fullConfigExample); err != nil {
		return nil, err
	}

	if len(c.Description) > 0 && c.Description[0] == '\n' {
		ctx.Description = c.Description[1:]
	}
	if len(c.Footnotes) > 0 && c.Footnotes[0] == '\n' {
		ctx.Footnotes = c.Footnotes[1:]
	}

	flattenedFields := c.Config.FlattenChildrenForDocs()
	for _, v := range flattenedFields {
		if v.Spec.Kind == KindMap {
			v.Spec.Type = "object"
		} else if v.Spec.Kind == KindArray {
			v.Spec.Type = "array"
		} else if v.Spec.Kind == Kind2DArray {
			v.Spec.Type = "two-dimensional array"
		}
		v.Spec.Kind = KindScalar
		ctx.Fields = append(ctx.Fields, v)
	}

	var buf bytes.Buffer
	err = template.Must(template.New("component").Parse(componentTemplate)).Execute(&buf, ctx)

	return buf.Bytes(), err
}