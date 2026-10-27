package gocluecli

import (
	"fmt"
	"strings"
	"testing"
)

func TestTemplateActionsReadCommentsAndStringsLikeTextTemplate(t *testing.T) {
	text := `{{/* example: {{template "x" .Tabs}} */}}` +
		`{{ printf "}}" .A }}` +
		"{{- /* trimmed {{.B}} */ -}}" +
		"{{ `raw }}` }}" +
		`{{ .C }}{{ unterminated`

	var got []string
	for _, action := range templateActions(text) {
		got = append(got, text[action.start:action.end])
	}

	want := []string{
		`{{/* example: {{template "x" .Tabs}} */}}`,
		`{{ printf "}}" .A }}`,
		"{{- /* trimmed {{.B}} */ -}}",
		"{{ `raw }}` }}",
		`{{ .C }}`,
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("actions = %q, want %q", got, want)
	}

	comments := templateComments(text)
	if len(comments) != 2 {
		t.Fatalf("comments = %#v, want 2", comments)
	}
	if body := text[comments[1].bodyStart:comments[1].bodyEnd]; body != " trimmed {{.B}} " {
		t.Fatalf("trimmed comment body = %q", body)
	}
}

func TestAnnotationScanTextKeepsOnlyCommentBodies(t *testing.T) {
	text := "@dot outside.Type\n{{/* @dot inside.Type */}}\n<p>@model Page x.Page</p>"
	masked := annotationScanText(text)
	if len(masked) != len(text) {
		t.Fatalf("masked length = %d, want %d", len(masked), len(text))
	}
	if strings.Contains(masked, "outside") || strings.Contains(masked, "@model") {
		t.Fatalf("masked = %q, want text outside comments blanked", masked)
	}
	if !strings.Contains(masked, "@dot inside.Type") {
		t.Fatalf("masked = %q, want the comment's annotation", masked)
	}
	if plain := "@dot plain.Type"; annotationScanText(plain) != plain {
		t.Fatal("text without comments should stay as it is")
	}
}

// The examples and the mention of @model in a template's comment are text: they
// are neither checked as actions nor read as annotations, while the annotations
// of the comment forms go-clue documents still work.
func TestLSPIgnoresExamplesAndMentionsInComments(t *testing.T) {
	idx := lspIndex{indexFile: indexFile{
		Types: map[string]goTypeIndex{
			"example.com/app.SearchBox": {
				Name:    "SearchBox",
				Package: "example.com/app",
				Fields:  map[string]fieldIndex{"Text": {Type: "string"}},
			},
			"example.com/app.Tabs": {
				Name:    "Tabs",
				Package: "example.com/app",
				Fields:  map[string]fieldIndex{"Label": {Type: "string"}},
			},
		},
		Short: map[string][]string{
			"SearchBox": {"example.com/app.SearchBox"},
			"Tabs":      {"example.com/app.Tabs"},
		},
	}}

	text := `{{/* Shared parts of list screens: {{template "admin-search" Search}} in
     the header, with the search box the list template declares as @model Search
     (admin.SearchBox), {{template "admin-tabs" .Tabs}} for filter tabs and
     {{range .Pager}}{{end}} below. */}}
{{define "admin-search"}}
    {{/* @dot example.com/app.SearchBox */}}
    <input value="{{.Text}}">
{{end}}

{{define "admin-tabs"}}
    {{- /* @dot example.com/app.Tabs */ -}}
    <nav aria-label="{{.Label}}"></nav>
{{end}}

{{define "admin-multi"}}
    {{/*
    @dot example.com/app.SearchBox
    */}}
    {{.Missing}}
{{end}}
`

	items := diagnosticsForTextScoped(text, idx, templateIndex{}, "search.gohtml")
	if len(items) != 1 {
		t.Fatalf("diagnostics = %#v, want only the unknown field of the multi-line contract", items)
	}
	assertDiagnostic(t, items, "Unknown field 'Missing' on SearchBox")

	scope := scopeAt(text, strings.Index(text, "<input"), idx, contractForDefineAt(text, idx, "<input"))
	if scope.dotType != "example.com/app.SearchBox" {
		t.Fatalf("dot in admin-search = %q, want the define's @dot despite the {{range}} in the comment", scope.dotType)
	}
}

// contractForDefineAt is the contract of the define around needle in text.
func contractForDefineAt(text string, idx lspIndex, needle string) templateIndex {
	contract, _ := contractForPathAt("search.gohtml", text, idx, strings.Index(text, needle))
	return contract
}

func TestLeadingDefineContractNeedsARealAnnotation(t *testing.T) {
	mention := "{{/* the list template declares as @model Search */}}\n{{define \"a\"}}{{end}}"
	if items := leadingDefineContractDiagnostics(mention); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want none for a mention of @model", items)
	}

	annotated := "{{/* @dot example.com/app.Page */}}\n{{/* Sent to the admin. */}}\n{{define \"subject\"}}{{.Title}}{{end}}"
	items := leadingDefineContractDiagnostics(annotated)
	if len(items) != 1 || items[0].Message != fmt.Sprintf(moveDefineAnnotationsFormat, "subject") {
		t.Fatalf("diagnostics = %#v, want the hint for the annotated comment above the define", items)
	}

	spread := "{{/* @dot example.com/app.Page */}}{{define \"a\"}}{{end}}\n{{/* plain */}}\n{{define \"b\"}}{{end}}"
	contracts := leadingDefineContracts(spread)
	if len(contracts) != 1 || contracts[0].name != "a" {
		t.Fatalf("contracts = %#v, want only the comment right above define a", contracts)
	}
}

func TestDefineContractReadsTheCommentsRightAboveIt(t *testing.T) {
	src := "{{/* @dot example.com/app.Other */}}{{define \"first\"}}{{end}}\n" +
		"{{/* @dot example.com/app.Page */}}\n{{/* Sent to the admin. */}}\n{{define \"subject\"}}{{.Title}}{{end}}"
	start := strings.Index(src, `{{define "subject"}}`)
	bodyStart := start + len(`{{define "subject"}}`)
	bodyEnd := strings.LastIndex(src, "{{end}}")

	text := defineContractText(src, start, bodyStart, bodyEnd)
	if parseDot(text) != "example.com/app.Page" {
		t.Fatalf("contract text = %q, want the @dot of the comments above the define only", text)
	}
}
