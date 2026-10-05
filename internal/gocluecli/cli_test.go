package gocluecli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildIndexScansTemplateContractsAndFieldMetadata(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "todo.go", `package app

// Todo is rendered by todo templates.
type Todo struct {
	// Title is the visible task title.
	Title string
	Done bool
	private string
}

// Label returns the template label.
func (t Todo) Label() string {
	return t.Title
}

type Page struct {
	Items []Todo
}

type User struct {
	Name string
}

// CurrentUser returns the active user.
func CurrentUser() User {
	return User{Name: "Ada"}
}

type privateState struct {
	Token string
}
`)
	writeFile(t, root, "templates/todos.gohtml", `{{/*
@model page Page
*/}}
{{ range $todo := page.Items }}{{ $todo.Title }}{{ end }}
`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("expected no problems, got %#v", idx.Problems)
	}

	todoType := idx.Types["example.com/app.Todo"]
	if todoType.Name != "Todo" {
		t.Fatalf("Todo type missing from index: %#v", idx.Types)
	}
	if todoType.Doc == "" {
		t.Fatal("expected type doc")
	}
	title := todoType.Fields["Title"]
	if title.Type != "string" {
		t.Fatalf("Title type = %q, want string", title.Type)
	}
	if title.Doc == "" {
		t.Fatal("expected field doc")
	}
	if title.Line == 0 || title.Column == 0 || title.File == "" {
		t.Fatalf("expected field source position, got %#v", title)
	}
	if _, ok := todoType.Fields["private"]; ok {
		t.Fatal("unexported field should not be indexed")
	}
	method := todoType.Methods["Label"]
	if method.Type != "string" {
		t.Fatalf("Label return type = %q, want string", method.Type)
	}
	if method.Signature == "" || method.Doc == "" || method.Line == 0 {
		t.Fatalf("expected method metadata, got %#v", method)
	}
	if _, ok := idx.Types["example.com/app.User"]; !ok {
		t.Fatal("unused exported structs should stay indexed for @model completion")
	}
	fn := idx.Funcs["example.com/app.CurrentUser"]
	if fn.Result != "User" || fn.Signature == "" || fn.Doc == "" {
		t.Fatalf("expected function result metadata, got %#v", fn)
	}
	if _, ok := idx.Types["example.com/app.privateState"]; ok {
		t.Fatal("unexported structs should not be indexed")
	}

	contract := idx.Templates["templates/todos.gohtml"]
	if contract.Roots["page"] != "example.com/app.Page" {
		t.Fatalf("@model page = %q", contract.Roots["page"])
	}
}

func TestBuildIndexScansDotContract(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "user.go", `package app

type User struct {
	Name string
}
`)
	writeFile(t, root, "templates/user_row.gohtml", `{{/*
@dot User
*/}}
<td>{{ .Name }}</td>
`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed for @dot annotations")
	}
	contract := idx.Templates["templates/user_row.gohtml"]
	if contract.Dot != "example.com/app.User" {
		t.Fatalf("@dot = %q, want User", contract.Dot)
	}
}

func TestBuildIndexScansOneLineTemplateCommentContracts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "user.go", `package app

type Page struct {
	Users []User
}

type User struct {
	Name string
}

func FirstUser() User {
	return User{}
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/* @model Page Page */}}
{{/* @func firstUser FirstUser */}}
{{ define "table_row" }}{{/* @dot User */}}<td>{{ .Name }}</td>{{ end }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed for one-line annotations")
	}
	page := idx.Templates["templates/page.gohtml"]
	if page.Roots["Page"] != "example.com/app.Page" {
		t.Fatalf("Page model = %q", page.Roots["Page"])
	}
	if page.Funcs["firstUser"] != "FirstUser" {
		t.Fatalf("firstUser func = %q", page.Funcs["firstUser"])
	}
	row := idx.Templates["templates/page.gohtml#table_row"]
	if row.Dot != "example.com/app.User" {
		t.Fatalf("row dot = %q", row.Dot)
	}
}

func TestBuildIndexReadsGoClueFunctionSignatures(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "helpers.go", `package app

// Async is registered as a template helper.
//go-clue:sig func(endpoint string, params ...any) html/template.HTML
//go-clue:sig func(interaction example.com/app.Interaction) html/template.HTML
func Async() {}

type Interaction struct {
	Endpoint string
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@func async example.com/app.Async
*/}}
{{ async "/stats" }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed for @func annotations")
	}
	fn := idx.Funcs["example.com/app.Async"]
	if len(fn.Signatures) != 2 {
		t.Fatalf("signatures = %#v, want 2", fn.Signatures)
	}
	if got := fn.Signatures[0].Params; len(got) != 2 || got[0] != "string" || got[1] != "...any" {
		t.Fatalf("first signature params = %#v", got)
	}
	if strings.Contains(fn.Doc, "go-clue:sig") {
		t.Fatalf("function doc still contains go-clue:sig comments: %q", fn.Doc)
	}
	if idx.Templates["templates/page.gohtml"].Funcs["async"] != "example.com/app.Async" {
		t.Fatalf("template funcs = %#v", idx.Templates["templates/page.gohtml"].Funcs)
	}
}

func TestBuildIndexReadsConfiguredTemplateFunctions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "templateFunctions": [
    {
      "name": "async",
      "path": "example.com/app.Async",
      "signatures": [
        "func(endpoint string, params ...any) html/template.HTML",
        "func(interaction example.com/app.Interaction) html/template.HTML"
      ]
    }
  ]
}`)
	writeFile(t, root, "helpers.go", `package app

func Async() {}

type Interaction struct {
	Endpoint string
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ async "/stats" }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed when config provides template functions")
	}
	if idx.Templates["templates/page.gohtml"].Funcs["async"] != "example.com/app.Async" {
		t.Fatalf("template funcs = %#v", idx.Templates["templates/page.gohtml"].Funcs)
	}
	if got := len(idx.Funcs["example.com/app.Async"].Signatures); got != 2 {
		t.Fatalf("configured signatures = %d, want 2", got)
	}
}

func TestBuildIndexScansNamedDefineContracts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "user.go", `package app

type User struct {
	Name string
}
`)
	writeFile(t, root, "templates/rows.gohtml", `{{/*
@dot User
*/}}
{{ define "table_row" }}
<tr><td>{{ .Name }}</td></tr>
{{ end }}
`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed for @dot annotations in named defines")
	}
	contract := idx.Templates["templates/rows.gohtml#table_row"]
	if contract.Name != "table_row" {
		t.Fatalf("table_row name = %q", contract.Name)
	}
	if contract.Dot != "example.com/app.User" {
		t.Fatalf("table_row @dot = %q, want User", contract.Dot)
	}
	if contract.Source != "templates/rows.gohtml" || contract.Line != 4 || contract.Column != 1 {
		t.Fatalf("table_row source = %#v, want define source position", contract)
	}
}

func TestBuildIndexPrefersInsideDefineContract(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "user.go", `package app

type User struct {
	Name string
}

type Admin struct {
	Name string
}
`)
	writeFile(t, root, "templates/rows.gohtml", `{{/*
@dot User
*/}}
{{ define "table_row" }}
{{/*
@dot Admin
*/}}
<tr><td>{{ .Name }}</td></tr>
{{ end }}
`)

	idx, _, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	contract := idx.Templates["templates/rows.gohtml#table_row"]
	if contract.Dot != "example.com/app.Admin" {
		t.Fatalf("table_row @dot = %q, want inside define contract to win", contract.Dot)
	}
}

func TestBuildIndexUsesGoTypesForImportsAliasesAndGenerics(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "todo.go", `package app

import "time"

type Timestamp = time.Time

type Box[T any] struct {
	Value T
}

type Page struct {
	GeneratedAt Timestamp
	Names Box[string]
}

func Lookup() (Page, error) {
	return Page{}, nil
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model Page Page
@func lookup Lookup
*/}}
{{ Page.GeneratedAt.Format "15:04:05" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	page := idx.Types["example.com/app.Page"]
	if page.Fields["GeneratedAt"].Type != "time.Time" {
		t.Fatalf("GeneratedAt type = %q, want time.Time", page.Fields["GeneratedAt"].Type)
	}
	timeType := idx.Types["time.Time"]
	format, ok := timeType.Methods["Format"]
	if !ok {
		t.Fatalf("time.Time methods = %#v, want Format method for imported named type", timeType.Methods)
	}
	if len(format.Params) != 1 || format.Params[0] != "string" {
		t.Fatalf("time.Time.Format params = %#v, want string", format.Params)
	}
	if page.Fields["Names"].Type != "Box[string]" {
		t.Fatalf("Names type = %q, want Box[string]", page.Fields["Names"].Type)
	}
	fn := idx.Funcs["example.com/app.Lookup"]
	if fn.Result != "Page" || len(fn.Results) != 2 || fn.Results[1] != "error" {
		t.Fatalf("Lookup metadata = %#v, want Page,error result metadata", fn)
	}
}

func TestBuildTemplateIndexRequiresContract(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "todo.go", `package app

type Todo struct {
	Title string
}
`)
	writeFile(t, root, "templates/no_contract.gohtml", `{{ .Title }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if needed {
		t.Fatal("index should not be needed without at least one template contract")
	}
	if len(idx.Types) != 0 {
		t.Fatalf("types should not be scanned when index is not needed, got %#v", idx.Types)
	}
}

func TestBuildIndexUsesConfigIncludeExclude(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "include": ["/"],
  "exclude": ["vendor", "internal/secret"]
}`)
	writeFile(t, root, "public.go", `package app

type Public struct {
	Title string
}
`)
	writeFile(t, root, "vendor/vendor.go", `package vendor

type VendorType struct {
	Title string
}
`)
	writeFile(t, root, "internal/secret/secret.go", `package secret

type Secret struct {
	Title string
}
`)
	writeFile(t, root, "templates/public.gohtml", `{{/*
@model public Public
*/}}
{{ public.Title }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if _, ok := idx.Types["example.com/app.Public"]; !ok {
		t.Fatal("public type should be indexed")
	}
	if _, ok := idx.Types["example.com/app/vendor.VendorType"]; ok {
		t.Fatal("vendor type should be excluded")
	}
	if _, ok := idx.Types["example.com/app/internal/secret.Secret"]; ok {
		t.Fatal("configured excluded type should be excluded")
	}
}

func TestBuildTemplateIndexHonorsDisabledConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "enabled": false,
  "functions": {
    "asset": "example.com/app.Asset"
  }
}`)
	writeFile(t, root, "page.go", `package app

type Page struct {
	Title string
}

func Asset(path string) string {
	return path
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model Page example.com/app.Page
*/}}
{{ Page.Title }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if needed {
		t.Fatal("disabled config should not require an index")
	}
	if len(idx.Templates) != 0 || len(idx.Types) != 0 || len(idx.Funcs) != 0 {
		t.Fatalf("disabled config should not scan project, got templates=%d types=%d funcs=%d", len(idx.Templates), len(idx.Types), len(idx.Funcs))
	}
}

func TestBuildIndexUsesConfigDefaultFunctions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functions": {
    "asset": "example.com/app.Asset"
  }
}`)
	writeFile(t, root, "page.go", `package app

type Page struct {
	Title string
}

func Asset(path string) string {
	return "/assets/" + path
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model page Page
*/}}
<link rel="stylesheet" href="{{ asset "app.css" }}">
{{ page.Title }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["asset"]; got != "example.com/app.Asset" {
		t.Fatalf("default asset func = %q", got)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexUsesConfigSymbolAnnotations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "symbolAnnotations": [
    {"name": "interaction", "type": "example.com/app.Interaction"},
    {"name": "component"}
  ]
}`)
	writeFile(t, root, "symbols.go", `package app

type Interaction struct {
	ID string
}

type Button struct {
	Label string
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@interaction LikesPoll
@component Button example.com/app.Button
*/}}
{{ LikesPoll }}
{{ Button.Label }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed for configured symbol annotations")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Roots["LikesPoll"]; got != "example.com/app.Interaction" {
		t.Fatalf("@interaction LikesPoll = %q", got)
	}
	if got := tmpl.Roots["Button"]; got != "example.com/app.Button" {
		t.Fatalf("@component Button = %q", got)
	}
	if idx.SymbolAliases["interaction"] != "example.com/app.Interaction" {
		t.Fatalf("symbol aliases = %#v", idx.SymbolAliases)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexAllowsExplicitCustomSymbolsByDefault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "symbols.go", `package app

type Button struct {
	Label string
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@jimmy Button example.com/app.Button
*/}}
{{ Button.Label }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed for explicit custom symbol annotations")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Roots["Button"]; got != "example.com/app.Button" {
		t.Fatalf("@jimmy Button = %q", got)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexStrictModeIgnoresUnknownCustomSymbols(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{"symbolStrictMode": true}`)
	writeFile(t, root, "symbols.go", `package app

type Button struct {
	Label string
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@jimmy Button example.com/app.Button
*/}}
{{ Button.Label }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if needed {
		t.Fatal("index should not be needed for an unknown strict-mode symbol")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if _, ok := tmpl.Roots["Button"]; ok {
		t.Fatalf("strict mode should not accept unknown @jimmy symbol: %#v", tmpl.Roots)
	}
	if !idx.SymbolStrict {
		t.Fatal("SymbolStrict = false, want true")
	}
}

func TestBuildIndexProjectsGenNamespace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "page.go", `package app

import "time"

type Page struct {
	GeneratedAt time.Time
}
`)
	writeFile(t, root, "viewfuncs/viewfuncs.go", `package viewfuncs

import "time"

// FormatTime formats t.
func FormatTime(t time.Time, layout string) string {
	return t.Format(layout)
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model Page Page
@gen view example.com/app/viewfuncs
*/}}
{{ view.FormatTime Page.GeneratedAt "15:04:05" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if tmpl.Gens["view"] != "example.com/app/viewfuncs" {
		t.Fatalf("@gen view = %q", tmpl.Gens["view"])
	}
	genType := tmpl.Roots["view"]
	if genType == "" {
		t.Fatalf("@gen namespace was not projected into Roots: %#v", tmpl)
	}
	viewType := idx.Types[genType]
	if viewType.File != "viewfuncs/viewfuncs.go" || viewType.Line != 1 || viewType.Column != 1 {
		t.Fatalf("generated namespace target = %#v, want source package file anchor", viewType)
	}
	if viewType.Methods["FormatTime"].Type != "string" {
		t.Fatalf("FormatTime method = %#v", viewType.Methods["FormatTime"])
	}
}

func TestBuildIndexLetsLocalFunctionOverrideConfigDefault(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functions": {
    "format": "example.com/app.GlobalFormat"
  }
}`)
	writeFile(t, root, "format.go", `package app

type Page struct {
	Title string
}

func GlobalFormat(value string) string {
	return value
}

func LocalFormat(value string) string {
	return value
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model page Page
@func format example.com/app.LocalFormat
*/}}
{{ format page.Title }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["format"]; got != "example.com/app.LocalFormat" {
		t.Fatalf("local format func = %q", got)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildTemplateIndexUsesConfigFunctionWithoutModelContract(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functions": {
    "asset": "example.com/app.Asset"
  }
}`)
	writeFile(t, root, "assets.go", `package app

func Asset(path string) string {
	return "/assets/" + path
}
`)
	writeFile(t, root, "templates/layout.gohtml", `<script src="{{ asset "app.js" }}"></script>`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed when config declares default functions")
	}
	tmpl := idx.Templates["templates/layout.gohtml"]
	if got := tmpl.Funcs["asset"]; got != "example.com/app.Asset" {
		t.Fatalf("default asset func = %q", got)
	}
}

func TestPathMatchesNormalizesWindowsSeparators(t *testing.T) {
	tests := []struct {
		relative string
		pattern  string
		want     bool
	}{
		{relative: "internal/secret/model.go", pattern: `internal\secret`, want: true},
		{relative: `internal\secret\model.go`, pattern: "internal/secret", want: true},
		{relative: "templates/main.gohtml", pattern: "/", want: true},
		{relative: "templates/main.gohtml", pattern: "vendor", want: false},
	}
	for _, test := range tests {
		if got := pathMatches(test.relative, test.pattern); got != test.want {
			t.Fatalf("pathMatches(%q, %q) = %v, want %v", test.relative, test.pattern, got, test.want)
		}
	}
}

func TestBuildIndexSkipsNestedModules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "page.go", `package app

type Page struct {
	Title string
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model page Page
*/}}
{{ page.Title }}`)
	writeFile(t, root, "examples/todo/go.mod", "module example.com/app/examples/todo\n\ngo 1.26\n")
	writeFile(t, root, "examples/todo/todo.go", `package main

type Todo struct {
	Title string
}
`)
	writeFile(t, root, "examples/todo/templates/todo.gohtml", `{{/*
@model todo github.com/example/app/examples/todo.Todo
*/}}
{{ todo.Title }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if _, ok := idx.Templates["templates/page.gohtml"]; !ok {
		t.Fatal("root template should be indexed")
	}
	if _, ok := idx.Templates["examples/todo/templates/todo.gohtml"]; ok {
		t.Fatal("nested module template should not be indexed by parent module")
	}
	if _, ok := idx.Types["example.com/app/examples/todo.Todo"]; ok {
		t.Fatal("nested module type should not be indexed by parent module")
	}
}

func TestBuildIndexHandlesRecursiveLocalTypes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "node.go", `package app

type Page struct {
	Root Node
}

type Node struct {
	Label    string
	Parent   *Node
	Children []Node
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/*
@model Page Page
*/}}
{{ Page.Root.Label }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if _, ok := idx.Types["example.com/app.Page"]; !ok {
		t.Fatal("Page type should be indexed")
	}
	if _, ok := idx.Types["example.com/app.Node"]; !ok {
		t.Fatal("Node type should be indexed")
	}
}

func TestBuildIndexUsesConfigFunctionMap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functionMaps": [
    "example.com/app.TemplateFuncs"
  ]
}`)
	writeFile(t, root, "funcs.go", `package app

import "html/template"

func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"asset": Asset,
	}
}

func Asset(path string) string {
	return "/assets/" + path
}
`)
	writeFile(t, root, "templates/page.gohtml", `<link href="{{ asset "app.css" }}">`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed when config declares functionMaps")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["asset"]; got != "example.com/app.Asset" {
		t.Fatalf("funcmap asset = %q", got)
	}
	if idx.Funcs["example.com/app.Asset"].Signature == "" {
		t.Fatalf("asset function metadata missing: %#v", idx.Funcs["example.com/app.Asset"])
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexUsesAnnotatedFunctionMap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatDate": FormatDate,
	}
}

func FormatDate(v string) string {
	return v
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ formatDate "today" }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed when annotations declare function maps")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["formatDate"]; got != "example.com/app.FormatDate" {
		t.Fatalf("funcmap formatDate = %q", got)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexFunctionMapIndexesInterfaceReturnMethods(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

import "html/template"

type Token interface {
	Key() string
	Token(Context) string
}

type Context struct {
	ID string
}

//go-clue:funcmap
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"csrf": CSRF,
	}
}

//go-clue:sig func() example.com/app.Token
func CSRF() Token {
	return nil
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ csrf.Key }} {{ csrf.Token . }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	token := idx.Types["example.com/app.Token"]
	if token.Methods["Key"].Type != "string" {
		t.Fatalf("Token.Key method missing: %#v", token)
	}
	if token.Methods["Token"].Type != "string" || len(token.Methods["Token"].Params) != 1 || token.Methods["Token"].Params[0] != "Context" {
		t.Fatalf("Token.Token method = %#v", token.Methods["Token"])
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexReadsGoClueSignatureFromTemplateFuncAssignment(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

import (
	"html/template"
	"net/url"
)

type Runtime struct{}

func addRequestFuncs(funcs template.FuncMap) {
	// go-clue:sig func() *example.com/app.Runtime
	funcs["runtime"] = func() *Runtime {
		return nil
	}

	// go-clue:sig func() *net/url.URL
	funcs["url"] = func() *url.URL {
		return nil
	}

	// go-clue:sig func(runtime *example.com/app.Runtime, path string) html/template.HTML
	// go-clue:sig func(runtime *example.com/app.Runtime, path string, dot any) html/template.HTML
	funcs["partial"] = func(runtime *Runtime, path string, args ...any) template.HTML {
		return ""
	}
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ url.Path }} {{ partial runtime "/nav" . }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	urlTarget := tmpl.Funcs["url"]
	if urlTarget == "" {
		t.Fatalf("url function missing: %#v", tmpl.Funcs)
	}
	if idx.Funcs[urlTarget].Result != "*net/url.URL" {
		t.Fatalf("url function metadata = %#v", idx.Funcs[urlTarget])
	}
	partialTarget := tmpl.Funcs["partial"]
	if partialTarget == "" {
		t.Fatalf("partial function missing: %#v", tmpl.Funcs)
	}
	if got := len(idx.Funcs[partialTarget].Signatures); got != 2 {
		t.Fatalf("partial signatures = %#v, want 2", idx.Funcs[partialTarget].Signatures)
	}
	if got := idx.Funcs[partialTarget].Signatures[1].Params; len(got) != 3 || got[2] != "any" {
		t.Fatalf("partial second signature params = %#v", idx.Funcs[partialTarget].Signatures[1])
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexReadsGoClueSignatureFromFuncMapEntry(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	source := `package app

import (
	"context"
	"html/template"
)

//go-clue:funcmap
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		//go-clue:sig func(file string) string
		"asset": asset,
		"plain": Plain, //go-clue:sig func() int
		"upper": Upper,
		// greet says hello.
		//go-clue:sig func(name string) string
		"greet": func(ctx context.Context, name string) string {
			return "hello " + name
		},
	}
}

// asset is the URL of a theme file.
func asset(ctx context.Context, file string) string {
	return "/theme/" + file
}

func Plain() string {
	return ""
}

func Upper(v string) string {
	return v
}
`
	writeFile(t, root, "funcs.go", source)
	text := `{{ asset "site.css" }} {{ greet "you" }} {{ upper "x" }} {{ plain }}`
	writeFile(t, root, "templates/page.gohtml", text)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	asset := idx.Funcs[tmpl.Funcs["asset"]]
	if asset.File != "funcs.go" || asset.Line != lineOf(source, "func asset(") {
		t.Fatalf("asset declaration = %s:%d, want the asset function", asset.File, asset.Line)
	}
	if len(asset.Signatures) != 1 || asset.Signature != "func(file string) string" || len(asset.Params) != 1 || asset.Result != "string" {
		t.Fatalf("asset signature = %#v, want the entry's signature", asset)
	}
	if asset.Doc != "asset is the URL of a theme file." {
		t.Fatalf("asset doc = %q", asset.Doc)
	}
	greet := idx.Funcs[tmpl.Funcs["greet"]]
	if greet.File != "funcs.go" || greet.Line != lineOf(source, `"greet":`) || greet.Doc != "greet says hello." {
		t.Fatalf("greet = %#v, want the closure entry with its comment", greet)
	}
	for name, want := range map[string]string{"plain": "example.com/app.Plain", "upper": "example.com/app.Upper"} {
		if got := tmpl.Funcs[name]; got != want || len(idx.Funcs[got].Signatures) != 0 {
			t.Fatalf("%s = %q %#v, want %s without a trailing comment's signature", name, got, idx.Funcs[got].Signatures, want)
		}
	}

	lsp := lspIndex{indexFile: idx}
	if diagnostics := diagnosticsForText(text, lsp, tmpl); len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want calls matching the declared signatures", diagnostics)
	}
	invalid := diagnosticsForText(`{{ asset 1 }}`, lsp, tmpl)
	assertDiagnostic(t, invalid, "Cannot pass int to asset argument 1 because it expects string")

	uri := uriFromPath(filepath.Join(root, "templates", "page.gohtml"))
	server := &lspServer{root: root, idx: idx, docs: map[string]string{uri: text}}
	definitionResult := server.definition(textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     positionAt(text, strings.Index(text, "asset")+1),
	})
	gotDefinition, ok := definitionResult.(location)
	if !ok || !strings.HasSuffix(filepath.ToSlash(gotDefinition.URI), "/funcs.go") || gotDefinition.Range.Start.Line != lineOf(source, "func asset(")-1 {
		t.Fatalf("definition(asset) = %#v, want the asset function", definitionResult)
	}
}

func TestBuildIndexIgnoresFuncMapEntrySignatureWhenSignatureDiscoveryIsDisabled(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "discover": {
    "signatures": false
  }
}`)
	writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		//go-clue:sig func() int
		"asset": Asset,
	}
}

func Asset(file string) string {
	return file
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "site.css" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["asset"]; got != "example.com/app.Asset" || len(idx.Funcs[got].Signatures) != 0 {
		t.Fatalf("asset = %q %#v, want the Go function without the entry's signature", got, idx.Funcs[got])
	}
}

func TestBuildIndexReadsFuncMapEntrySignatureFromConfiguredProviderFunctionMap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "exclude": ["framework"],
  "providers": [
    "example.com/app/framework"
  ],
  "functionMaps": [
    "example.com/app/framework.funcs"
  ]
}`)
	source := `package framework

import (
	"context"
	"html/template"
)

func funcs() template.FuncMap {
	return template.FuncMap{
		//go-clue:sig func(key string) []string
		"banners": active,
	}
}

// active returns the active banners of an instance.
func active(ctx context.Context, key string) []string {
	return nil
}
`
	writeFile(t, root, "framework/funcs.go", source)
	writeFile(t, root, "templates/page.gohtml", `{{ range banners "hero" }}{{ . }}{{ end }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
	banners := idx.Funcs[idx.Templates["templates/page.gohtml"].Funcs["banners"]]
	if banners.File != "framework/funcs.go" || banners.Line != lineOf(source, "func active(") || banners.Package != "example.com/app/framework" {
		t.Fatalf("banners declaration = %#v, want the provider's active function", banners)
	}
	if banners.Signature != "func(key string) []string" || banners.Doc != "active returns the active banners of an instance." {
		t.Fatalf("banners = %#v, want the entry's signature and the function's doc", banners)
	}
}

func TestLSPAcceptsArgumentsImplementingInterfaceParameters(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "a/a.go", `package a

type Localizer interface {
	GetLocale() string
}
`)
	writeFile(t, root, "b/b.go", `package b

type Localizer interface {
	GetLocale() string
}

type Stranger interface {
	Name() string
}
`)
	writeFile(t, root, "funcs.go", `package app

import (
	"html/template"

	"example.com/app/a"
	"example.com/app/b"
)

type Page struct {
	Loc     a.Localizer
	Visitor Visitor
	Who     b.Stranger
	Robot   Robot
}

type Visitor struct{}

func (Visitor) GetLocale() string {
	return "en"
}

type Robot struct{}

func (Robot) GetLocale(fallback string) string {
	return fallback
}

//go-clue:funcmap
func Funcs() template.FuncMap {
	return template.FuncMap{
		"greet": Greet,
	}
}

func Greet(loc b.Localizer) string {
	return loc.GetLocale()
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{/* @dot example.com/app.Page */}}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if !idx.Types["example.com/app/b.Localizer"].Interface || idx.Types["example.com/app.Visitor"].Interface {
		t.Fatalf("interface flags: b.Localizer %v, Visitor %v", idx.Types["example.com/app/b.Localizer"].Interface, idx.Types["example.com/app.Visitor"].Interface)
	}
	lsp := lspIndex{indexFile: idx}
	contract := idx.Templates["templates/page.gohtml"]
	header := `{{/* @dot example.com/app.Page */}}`

	valid := diagnosticsForText(header+`{{ greet .Loc }} {{ greet .Visitor }}`, lsp, contract)
	if len(valid) != 0 {
		t.Fatalf("diagnostics = %#v, want types with the interface's methods accepted", valid)
	}
	invalid := diagnosticsForText(header+`{{ greet .Who }}`, lsp, contract)
	assertDiagnostic(t, invalid, "Cannot pass example.com/app/b.Stranger to greet argument 1 because it expects example.com/app/b.Localizer")
	wrongSignature := diagnosticsForText(header+`{{ greet .Robot }}`, lsp, contract)
	assertDiagnostic(t, wrongSignature, "Cannot pass example.com/app.Robot to greet argument 1 because it expects example.com/app/b.Localizer")
}

func TestBuildIndexUsesConfiguredProviderPackage(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "exclude": ["framework"],
  "providers": [
    "example.com/app/framework"
  ]
}`)
	writeFile(t, root, "framework/funcs.go", `package framework

import (
	"html/template"
	"net/url"
)

//go-clue:funcmap
var TemplateFuncs = template.FuncMap{
	"url": URL,
}

func URL() *url.URL {
	return nil
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ url.Path }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed when config declares providers")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["url"]; got != "example.com/app/framework.URL" {
		t.Fatalf("provider url = %q", got)
	}
	if idx.Funcs["example.com/app/framework.URL"].Result != "*net/url.URL" {
		t.Fatalf("provider function metadata = %#v", idx.Funcs["example.com/app/framework.URL"])
	}
}

func TestBuildIndexUsesProviderAnnotation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "exclude": ["framework"]
}`)
	writeFile(t, root, "main.go", `package app

// go-clue:provider "example.com/app/framework/..."
func main() {}
`)
	writeFile(t, root, "framework/runtime/funcs.go", `package runtime

import "html/template"

func Add(funcs template.FuncMap) {
	// go-clue:sig func() string
	funcs["basePath"] = func() string {
		return "/"
	}
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ basePath }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("index should be needed when provider annotation is present")
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	target := tmpl.Funcs["basePath"]
	if target == "" {
		t.Fatalf("basePath missing from provider funcs: %#v", tmpl.Funcs)
	}
	if idx.Funcs[target].Result != "string" {
		t.Fatalf("basePath metadata = %#v", idx.Funcs[target])
	}
}

func TestBuildTemplateIndexAcceptsLegacyConfigurationAndAnnotations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-doc/config.json", `{
  "exclude": ["framework"],
  "functions": {"configured": "example.com/app.Configured"}
}`)
	writeFile(t, root, "main.go", `package app

// go-doc:provider "example.com/app/framework"
func Configured() string { return "configured" }
`)
	writeFile(t, root, "framework/funcs.go", `package framework

import "html/template"

//go-doc:funcmap
var TemplateFuncs = template.FuncMap{"asset": Asset}

func Asset(path string) string { return path }

func Add(funcs template.FuncMap) {
	// go-doc:sig func() string
	funcs["basePath"] = func() string { return "/" }
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }} {{ basePath }} {{ configured }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if !needed {
		t.Fatal("legacy metadata should enable template indexing")
	}
	for _, name := range []string{"asset", "basePath", "configured"} {
		target := idx.Templates["templates/page.gohtml"].Funcs[name]
		if target == "" || idx.Funcs[target].Result != "string" {
			t.Errorf("legacy function %q missing or incorrect: target = %q, metadata = %#v", name, target, idx.Funcs[target])
		}
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}

	writeFile(t, root, ".go-clue/config.json", `{"enabled": false}`)
	_, needed, err = buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() with new configuration error = %v", err)
	}
	if needed {
		t.Fatal("new configuration should override legacy configuration and disable indexing")
	}
}

func TestBuildIndexCanDisableProviderAndSignatureDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "discover": {
    "providers": false,
    "signatures": false
  }
}`)
	writeFile(t, root, "main.go", `package app

// go-clue:provider "example.com/app/framework/..."
func main() {}
`)
	writeFile(t, root, "framework/runtime/funcs.go", `package runtime

import "html/template"

func Add(funcs template.FuncMap) {
	// go-clue:sig func() string
	funcs["basePath"] = func() string {
		return "/"
	}
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ basePath }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if needed {
		t.Fatal("index should not be needed when provider and signature discovery are disabled")
	}
	if len(idx.Funcs) != 0 {
		t.Fatalf("disabled discovery should not index provider funcs: %#v", idx.Funcs)
	}
}

func TestBuildIndexUsesAnnotatedVarFunctionMap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
var TemplateFuncs = template.FuncMap{
	"asset": Asset,
}

func Asset(path string) string {
	return path
}
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["asset"]; got != "example.com/app.Asset" {
		t.Fatalf("funcmap asset = %q", got)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildIndexUsesFunctionMapReturnedThroughVariable(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	source := `package app

import "html/template"

type Form struct{}

//go-clue:funcmap
func (f *Form) FuncMap() template.FuncMap {
	funcMap := template.FuncMap{
		"form_render": f.render,
	}

	return funcMap
}

func (f *Form) render(model any) string {
	return ""
}
`
	writeFile(t, root, "funcs.go", source)
	writeFile(t, root, "templates/page.gohtml", `{{ form_render . }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
	target := idx.Templates["templates/page.gohtml"].Funcs["form_render"]
	if target != "example.com/app.render" {
		t.Fatalf("funcmap form_render = %q", target)
	}
	if fn := idx.Funcs[target]; fn.File != "funcs.go" || fn.Line != lineOf(source, "func (f *Form) render(") {
		t.Fatalf("form_render declaration = %s:%d, want the render method", fn.File, fn.Line)
	}
}

func TestBuildIndexReportsFunctionMapVariableChangedAfterItsLiteral(t *testing.T) {
	bodies := map[string]string{
		"reassigned": `funcs := template.FuncMap{
		"asset": Asset,
	}
	if admin {
		funcs = template.FuncMap{}
	}
	return funcs`,
		"entry added": `funcs := template.FuncMap{}
	funcs["asset"] = Asset
	return funcs`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
			writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
func TemplateFuncs(admin bool) template.FuncMap {
	`+body+`
}

func Asset(path string) string {
	return path
}
`)
			writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }}`)

			idx, err := buildIndex(root)
			if err != nil {
				t.Fatalf("buildIndex() error = %v", err)
			}
			if got := idx.Templates["templates/page.gohtml"].Funcs["asset"]; got != "" {
				t.Fatalf("funcmap asset = %q, want none from a changed variable", got)
			}
			if len(idx.Problems) != 1 || idx.Problems[0].Message != funcMapDynamicMessage {
				t.Fatalf("problems = %#v, want the dynamic funcmap problem", idx.Problems)
			}
		})
	}
}

func TestBuildIndexFunctionMapDuplicateDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
func A() template.FuncMap {
	return template.FuncMap{
		"asset": AssetA,
	}
}

//go-clue:funcmap
func B() template.FuncMap {
	return template.FuncMap{
		"asset": AssetB,
	}
}

func AssetA(path string) string { return path }
func AssetB(path string) string { return path }
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if !hasProblem(idx.Problems, `function "asset" is declared by multiple funcmaps`) {
		t.Fatalf("duplicate funcmap problem missing: %#v", idx.Problems)
	}
}

func TestBuildIndexExplicitFunctionOverridesFunctionMap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functions": {
    "asset": "example.com/app.AssetOverride"
  },
  "functionMaps": [
    "example.com/app.TemplateFuncs"
  ]
}`)
	writeFile(t, root, "funcs.go", `package app

import "html/template"

func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"asset": Asset,
	}
}

func Asset(path string) string { return path }
func AssetOverride(path string) string { return "/override/" + path }
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["asset"]; got != "example.com/app.AssetOverride" {
		t.Fatalf("asset should resolve to explicit config function, got %q", got)
	}
}

func TestBuildIndexFunctionMapInvalidAnnotationAndDynamicConstruction(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
func Nope() string {
	return "nope"
}

//go-clue:funcmap
func TemplateFuncs() template.FuncMap {
	fm := template.FuncMap{}
	fm["asset"] = Asset
	return fm
}

func Asset(path string) string { return path }
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	if !hasProblem(idx.Problems, "does not return template.FuncMap or map[string]any") {
		t.Fatalf("invalid annotation problem missing: %#v", idx.Problems)
	}
	if !hasProblem(idx.Problems, "dynamic funcmap construction is not supported yet") {
		t.Fatalf("dynamic construction problem missing: %#v", idx.Problems)
	}
}

func TestBuildIndexFunctionMapSupportsMapStringAnyAndDynamicKeyDiagnostic(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "funcs.go", `package app

const assetName = "asset"

//go-clue:funcmap
var TemplateFuncs = map[string]any{
	"format": Format,
	assetName: Asset,
}

func Format(v string) string { return v }
func Asset(path string) string { return path }
`)
	writeFile(t, root, "templates/page.gohtml", `{{ format "x" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["format"]; got != "example.com/app.Format" {
		t.Fatalf("map[string]any format = %q", got)
	}
	if _, ok := tmpl.Funcs["asset"]; ok {
		t.Fatalf("dynamic-key asset should be skipped: %#v", tmpl.Funcs)
	}
	if !hasProblem(idx.Problems, "funcmap entry uses a dynamic key and was skipped") {
		t.Fatalf("dynamic key problem missing: %#v", idx.Problems)
	}
}

func TestBuildIndexFunctionMapSupportsTextTemplateAndMapStringInterface(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "functionMaps": [
    "example.com/app.TextFuncs"
  ]
}`)
	writeFile(t, root, "funcs.go", `package app

import "text/template"

func TextFuncs() template.FuncMap {
	return template.FuncMap{
		"title": Title,
	}
}

//go-clue:funcmap
var InterfaceFuncs = map[string]interface{}{
	"slug": Slug,
}

func Title(v string) string { return v }
func Slug(v string) string { return v }
`)
	writeFile(t, root, "templates/page.gohtml", `{{ title "x" }} {{ slug "x" }}`)

	idx, err := buildIndex(root)
	if err != nil {
		t.Fatalf("buildIndex() error = %v", err)
	}
	tmpl := idx.Templates["templates/page.gohtml"]
	if got := tmpl.Funcs["title"]; got != "example.com/app.Title" {
		t.Fatalf("text/template funcmap title = %q", got)
	}
	if got := tmpl.Funcs["slug"]; got != "example.com/app.Slug" {
		t.Fatalf("map[string]interface{} slug = %q", got)
	}
	if len(idx.Problems) != 0 {
		t.Fatalf("unexpected problems: %#v", idx.Problems)
	}
}

func TestBuildTemplateIndexCanDisableFunctionMapDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", `{
  "discover": {
    "functionMaps": false
  }
}`)
	writeFile(t, root, "funcs.go", `package app

import "html/template"

//go-clue:funcmap
func TemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"asset": Asset,
	}
}

func Asset(path string) string { return path }
`)
	writeFile(t, root, "templates/page.gohtml", `{{ asset "app.css" }}`)

	idx, needed, err := buildTemplateIndex(root)
	if err != nil {
		t.Fatalf("buildTemplateIndex() error = %v", err)
	}
	if needed {
		t.Fatal("index should not be needed when functionMap discovery is disabled and no other contracts exist")
	}
	if len(idx.Types) != 0 || len(idx.Funcs) != 0 {
		t.Fatalf("disabled discovery should not force Go scan, got types=%d funcs=%d", len(idx.Types), len(idx.Funcs))
	}
}

func TestIndexCommandRemovesStaleOutputWhenNoParamContractExists(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, "templates/no_contract.gohtml", `{{ .Title }}`)
	out := filepath.Join(root, ".go-clue", "index.json")
	writeFile(t, root, ".go-clue/index.json", `{"stale": true}`)

	if err := Run([]string{"index", "-o", out, root}); err != nil {
		t.Fatalf("Run(index) error = %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("expected stale index to be removed, stat err = %v", err)
	}
}

func TestWriteJSONWritesOutputFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), ".go-clue", "index.json")
	if err := writeJSON(indexFile{Version: 2, Module: "example.com/app"}, out); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 || data[0] != '{' {
		t.Fatalf("expected utf-8 json object, got %q", data)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func hasProblem(problems []problem, message string) bool {
	for _, problem := range problems {
		if strings.Contains(problem.Message, message) {
			return true
		}
	}
	return false
}

func lineOf(text, needle string) int {
	return strings.Count(text[:strings.Index(text, needle)], "\n") + 1
}
