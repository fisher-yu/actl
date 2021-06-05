package tpl

const ModelTpl = `
package {{.Models}}

{{$ilen := len .Imports}}
{{if gt $ilen 0}}
import (
	{{range .Imports}}"{{.}}"{{end}}
)
{{end}}

{{range .Tables}}
{{$name := Mapper .Name}}
type {{$name}} struct {
{{$table := .}}

{{range .ColumnsSeq}}{{$col := $table.GetColumn .}}	{{Mapper $col.Name}}	{{Type $col}} {{Tag $table $col}}
{{end}}
}
{{$tn := $table.Name}}
func (t *{{$name}}) TableName() string {
	return "{{$tn}}"
}

{{end}}
`
