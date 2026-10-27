package gocluecli

import (
	"path/filepath"
	"testing"
)

// A function of a function map takes types of its own package, which another
// package can declare under the same name, as go-translator and go-partial both
// declare Localizer.
func TestBuildIndexQualifiesFunctionMapParameterTypes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functionMaps": ["example.com/app/translator.FuncMap"]
}`)
	writeFile(t, root, "translator/translator.go", `package translator

import "html/template"

type Localizer interface {
	GetLocale() string
}

type Translator struct{}

func (t *Translator) FuncMap() template.FuncMap {
	return template.FuncMap{
		"tl": t.tl,
	}
}

func (t *Translator) tl(loc Localizer, key string, args ...any) string {
	return key
}
`)
	writeFile(t, root, "other/other.go", `package other

type Localizer interface {
	Locale() string
}
`)
	writeFile(t, root, "page.go", `package app

import (
	"example.com/app/other"
	"example.com/app/translator"
)

type Page struct {
	Loc   translator.Localizer
	Other other.Localizer
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/* @dot example.com/app.Page */}}{{ tl .Loc "Hello" }}`)

	idx, _, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}

	fn := idx.Funcs["example.com/app/translator.tl"]
	if len(fn.Params) != 3 || fn.Params[0] != "example.com/app/translator.Localizer" || fn.Params[2] != "...any" {
		t.Fatalf("tl params = %#v, want the Localizer qualified", fn.Params)
	}

	lsp := lspIndex{indexFile: idx, rootPath: root}
	contract := idx.Templates["templates/page.gohtml"]
	header := `{{/* @dot example.com/app.Page */}}`
	if items := diagnosticsForText(header+`{{ tl .Loc "Hello" }}`, lsp, contract); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want the translator's Localizer accepted", items)
	}
	assertDiagnostic(t, diagnosticsForText(header+`{{ tl .Other "Hello" }}`, lsp, contract),
		"Cannot pass example.com/app/other.Localizer to tl argument 1 because it expects example.com/app/translator.Localizer")
}

// Fields and methods of embedded structs are promoted, at any depth and from
// other packages, unless two embedded structs at the same depth share a name.
func TestBuildIndexPromotesFieldsOfEmbeddedStructs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "base/base.go", `package base

import "time"

type Review struct {
	SubmittedAt time.Time
	Note        string
}

func (r Review) Submitted() bool {
	return !r.SubmittedAt.IsZero()
}
`)
	writeFile(t, root, "panels.go", `package app

import "example.com/app/base"

type Heading struct {
	Title string
}

type PanelState struct {
	Heading Heading
	Failed  bool
	Shared  string
}

type Other struct {
	Shared string
}

type AttentionPanel struct {
	PanelState
	*Other
	Items []string
}

type RevisionPanel struct {
	base.Review
	State string
}
`)
	writeFile(t, root, "templates/panel.gohtml", `{{/* @dot example.com/app.AttentionPanel */}}{{ .Heading.Title }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}

	attention := idx.Types["example.com/app.AttentionPanel"]
	for _, name := range []string{"PanelState", "Heading", "Failed", "Items"} {
		if _, ok := attention.Fields[name]; !ok {
			t.Fatalf("AttentionPanel fields = %#v, want %s", attention.Fields, name)
		}
	}
	if _, ok := attention.Fields["Shared"]; ok {
		t.Fatal("a name two embedded structs share at the same depth is ambiguous and not promoted")
	}
	if attention.Fields["Heading"].Type != "Heading" || attention.Fields["Heading"].Line == 0 {
		t.Fatalf("promoted Heading = %#v, want its type and declaration", attention.Fields["Heading"])
	}

	revision := idx.Types["example.com/app.RevisionPanel"]
	if revision.Fields["SubmittedAt"].Type != "time.Time" || revision.Fields["Note"].Type != "string" {
		t.Fatalf("RevisionPanel fields = %#v, want the fields of base.Review", revision.Fields)
	}
	if _, ok := revision.Methods["Submitted"]; !ok {
		t.Fatalf("RevisionPanel methods = %#v, want the promoted Submitted", revision.Methods)
	}

	lsp := lspIndex{indexFile: idx, rootPath: root}
	text := `{{/* @dot example.com/app.AttentionPanel */}}{{ .Heading.Title }}{{ if .Failed }}{{ end }}{{ .Missing }}`
	items := diagnosticsForText(text, lsp, idx.Templates["templates/panel.gohtml"])
	if len(items) != 1 {
		t.Fatalf("diagnostics = %#v, want only the unknown field", items)
	}
	assertDiagnostic(t, items, "Unknown field 'Missing' on AttentionPanel")

	revisionText := `{{/* @dot example.com/app.RevisionPanel */}}{{ .SubmittedAt.Format "2006" }} {{ .Note }} {{ if .Submitted }}{{ end }} {{ .State }}`
	if items := diagnosticsForText(revisionText, lsp, templateIndex{Dot: "example.com/app.RevisionPanel"}); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want the fields and methods of the embedded struct", items)
	}
}

// An any value can hold whatever a template or function expects; only the
// running template knows.
func TestLSPAcceptsAnyValues(t *testing.T) {
	idx := lspIndex{indexFile: indexFile{
		Types: map[string]goTypeIndex{
			"example.com/app.Placeholder": {
				Name:    "Placeholder",
				Package: "example.com/app",
				Fields:  map[string]fieldIndex{"Data": {Type: "any"}, "Raw": {Type: "interface{}"}, "Count": {Type: "int"}},
			},
			"example.com/app.PanelHeading": {
				Name:    "PanelHeading",
				Package: "example.com/app",
				Fields:  map[string]fieldIndex{"Title": {Type: "string"}},
			},
		},
		Funcs: map[string]goFuncIndex{
			"example.com/app.Title": {
				Name:      "Title",
				Signature: "func(PanelHeading) string",
				Params:    []string{"example.com/app.PanelHeading"},
				Result:    "string",
				Results:   []string{"string"},
				ReturnOK:  true,
			},
		},
		Short: map[string][]string{
			"Placeholder":  {"example.com/app.Placeholder"},
			"PanelHeading": {"example.com/app.PanelHeading"},
		},
	}}
	contract := templateIndex{Dot: "example.com/app.Placeholder", Funcs: map[string]string{"title": "example.com/app.Title"}}
	text := `{{/* @dot example.com/app.Placeholder */}}<div></div>
{{define "panel-title"}}{{/* @dot example.com/app.PanelHeading */}}{{.Title}}{{end}}
{{template "panel-title" .Data}}{{template "panel-title" .Raw}}{{ title .Data }}`

	if items := diagnosticsForTextScoped(text, idx, contract, "page.gohtml"); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want any values accepted", items)
	}
	invalid := diagnosticsForTextScoped(text+`{{template "panel-title" .Count}}`, idx, contract, "page.gohtml")
	assertDiagnostic(t, invalid, "Template panel-title expects PanelHeading, got int")
}

// Named types other than structs and interfaces, such as an array type, are
// indexed with what they are defined as.
func TestBuildIndexIndexesNamedArrayTypes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "directory/directory.go", `package directory

type Day struct {
	Open  string
	Close string
}

type OpeningHours [7]Day

type Status string

func (s Status) Label() string {
	return string(s)
}

type Entry struct {
	Hours  OpeningHours
	Status Status
}
`)
	writeFile(t, root, "templates/hours.gohtml", `{{/* @dot example.com/app/directory.OpeningHours */}}{{range .}}{{.Open}}{{end}}`)
	writeFile(t, root, "templates/entry.gohtml", `{{/* @dot example.com/app/directory.Entry */}}{{template "hours.gohtml" .Hours}}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}

	hours := idx.Types["example.com/app/directory.OpeningHours"]
	if hours.Underlying != "[7]Day" || hours.Struct {
		t.Fatalf("OpeningHours = %#v, want an array of Day", hours)
	}
	if _, ok := idx.Types["example.com/app/directory.Status"].Methods["Label"]; !ok {
		t.Fatalf("Status = %#v, want its methods", idx.Types["example.com/app/directory.Status"])
	}

	lsp := lspIndex{indexFile: idx, rootPath: root}
	entry := `{{/* @dot example.com/app/directory.Entry */}}{{template "hours.gohtml" .Hours}}{{ len .Hours }}{{range .Hours}}{{.Close}}{{.Nope}}{{end}}{{ .Status.Label }}`
	items := diagnosticsForTextScoped(entry, lsp, idx.Templates["templates/entry.gohtml"], "templates/entry.gohtml")
	if len(items) != 1 {
		t.Fatalf("diagnostics = %#v, want only the unknown field of Day", items)
	}
	assertDiagnostic(t, items, "Unknown field 'Nope' on Day")
}

func TestBuildIndexReadsFunctionLiteralsOfFunctionMaps(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functionMaps": ["example.com/app/render.placeholderFuncMap"]
}`)
	writeFile(t, root, "render/render.go", `package render

import "html/template"

type Runtime struct {
	Path string
}

func placeholderFuncMap() template.FuncMap {
	return template.FuncMap{
		// content is the content of the page.
		"content": func() template.HTML { return "" },
		"runtime": func() *Runtime { return nil },
		"partial": func(*Runtime, string, ...any) template.HTML { return "" },
	}
}
`)
	writeFile(t, root, "templates/layout.gohtml", `{{ content }}`)

	idx, _, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("problems = %#v", idx.Problems)
	}

	target := idx.Functions["partial"]
	fn := idx.Funcs[target]
	if len(fn.Params) != 3 || fn.Params[0] != "*example.com/app/render.Runtime" || fn.Result != "template.HTML" && fn.Result != "html/template.HTML" {
		t.Fatalf("partial = %q %#v, want the literal's signature", target, fn)
	}
	if doc := idx.Funcs[idx.Functions["content"]].Doc; doc != "content is the content of the page." {
		t.Fatalf("content doc = %q", doc)
	}

	lsp := lspIndex{indexFile: idx, rootPath: root}
	contract := idx.Templates["templates/layout.gohtml"]
	if items := diagnosticsForText(`{{ partial runtime "x" 1 }}{{ runtime.Path }}`, lsp, contract); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want valid calls accepted", items)
	}
	assertDiagnostic(t, diagnosticsForText(`{{ partial "x" "y" }}`, lsp, contract), "Cannot pass string to partial argument 1 because it expects *example.com/app/render.Runtime")
}

// A template outside the indexed folders, such as a generated one, still gets
// the project's functions.
func TestDocumentDiagnosticsGiveFunctionsToTemplatesOutsideTheIndex(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{"functionMaps": ["example.com/app.Funcs"]}`)
	writeFile(t, root, "funcs.go", `package app

import "html/template"

func Funcs() template.FuncMap {
	return template.FuncMap{"asset": Asset}
}

func Asset(path string) string {
	return path
}
`)

	idx, _, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}

	lsp := lspIndex{indexFile: idx, rootPath: root}
	outside := filepath.Join(t.TempDir(), "generated", "page.gohtml")
	items, ok := documentDiagnostics(outside, `{{ asset 1 }}`, lsp)
	if !ok {
		t.Fatal("a template outside the index should be checked")
	}
	assertDiagnostic(t, items, "Cannot pass int to asset argument 1 because it expects string")

	contract, _ := contractForPathAt(outside, `{{ asset "x" }}`, lsp, 0)
	if contract.Funcs["asset"] != "example.com/app.Asset" {
		t.Fatalf("funcs = %#v, want the project's functions", contract.Funcs)
	}
}
