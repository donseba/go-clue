package gocluecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// writeCheckProject writes a project with a function map, a template with
// an unknown field, one that calls a function nothing declares, and one that
// does not parse.
func writeCheckProject(t *testing.T, config string) string {
	t.Helper()

	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeFile(t, root, ".go-clue/config.json", config)
	writeFile(t, root, "app.go", `package app

import "html/template"

type Page struct {
	Title string
}

func Funcs() template.FuncMap {
	return template.FuncMap{"asset": Asset}
}

func Asset(path string) string {
	return path
}
`)
	writeFile(t, root, "templates/page.gohtml", "{{/* @dot example.com/app.Page */}}\n<h1>{{ .Title }}</h1>\n<p>{{ .Missing }}</p>\n")
	writeFile(t, root, "templates/assets.gohtml", "{{ asset \"app.css\" }}\n{{ shout \"hi\" }}\n{{ printf \"%s\" (whisper) }}\n")
	writeFile(t, root, "templates/broken.gohtml", "{{ if .Ready }}\n")
	writeFile(t, root, "vendor/skipped.gohtml", "{{ nope }}")

	return root
}

func runCheckForTest(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	err := runCheck(args, &stdout, &stderr)

	return stdout.String(), stderr.String(), err
}

func TestUnknownFunctionDiagnosticsAreOffUnlessConfigured(t *testing.T) {
	root := writeCheckProject(t, `{"functionMaps": ["example.com/app.Funcs"]}`)

	stdout, _, err := runCheckForTest(t, "-severity", "hint", root)
	if !errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want the parse error to fail the check", err)
	}
	if strings.Contains(stdout, "Unknown template function") {
		t.Fatalf("output = %q, want no unknown function diagnostics", stdout)
	}
}

func TestCheckReportsDiagnosticsOfAProject(t *testing.T) {
	root := writeCheckProject(t, `{
  "functionMaps": ["example.com/app.Funcs"],
  "unknownFunctions": "warning",
  "templateFunctions": [{"name": "whisper"}]
}`)

	stdout, stderr, err := runCheckForTest(t, root)
	if !errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want check to fail on the parse error", err)
	}

	for _, want := range []string{
		"templates/page.gohtml:3:8: error: Unknown field 'Missing' on Page",
		"templates/assets.gohtml:2:4: warning: Unknown template function 'shout'",
		"templates/broken.gohtml:2:1: error: Template does not parse: unexpected EOF",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output = %q, want %q", stdout, want)
		}
	}
	for _, unwanted := range []string{"whisper", "vendor", "'asset'"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("output = %q, want nothing about %s", stdout, unwanted)
		}
	}
	if !strings.Contains(stderr, "go-clue: 3 templates, 2 errors, 1 warnings") {
		t.Fatalf("summary = %q", stderr)
	}

	errorsOnly, _, _ := runCheckForTest(t, "-severity", "error", root)
	if strings.Contains(errorsOnly, "shout") {
		t.Fatalf("output = %q, want warnings left out", errorsOnly)
	}
}

func TestCheckFailsOnTheConfiguredSeverity(t *testing.T) {
	root := writeCheckProject(t, `{
  "functionMaps": ["example.com/app.Funcs"],
  "unknownFunctions": "warning"
}`)
	page := filepath.Join(root, "templates", "assets.gohtml")

	if _, _, err := runCheckForTest(t, page); err != nil {
		t.Fatalf("err = %v, want warnings to pass by default", err)
	}
	if _, _, err := runCheckForTest(t, "-fail-on", "warning", page); !errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want -fail-on warning to fail", err)
	}
	if _, _, err := runCheckForTest(t, "-severity", "loud", page); err == nil || errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want a usage error", err)
	}
}

func TestCheckWritesJSON(t *testing.T) {
	root := writeCheckProject(t, `{"functionMaps": ["example.com/app.Funcs"], "unknownFunctions": "error"}`)

	stdout, _, err := runCheckForTest(t, "-json", filepath.Join(root, "templates", "assets.gohtml"))
	if !errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want the unknown functions to fail the check", err)
	}

	var result checkResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("output = %q: %v", stdout, err)
	}
	if result.Templates != 1 || result.Errors != 2 || len(result.Diagnostics) != 2 {
		t.Fatalf("result = %#v, want the two unknown functions", result)
	}
	first := result.Diagnostics[0]
	if first.Line != 2 || first.Column != 4 || first.EndColumn != 9 || first.Severity != "error" || !strings.Contains(first.Message, "'shout'") {
		t.Fatalf("diagnostic = %#v", first)
	}
}

// A template outside the project, such as a generated one, is checked with
// the project's functions and configuration when -root names the project.
func TestCheckChecksTemplatesOutsideTheProjectWithRoot(t *testing.T) {
	root := writeCheckProject(t, `{"functionMaps": ["example.com/app.Funcs"], "unknownFunctions": "error"}`)
	generated := t.TempDir()
	writeFile(t, generated, "news/list.gohtml", "{{ asset 1 }}{{ shout }}")

	if _, _, err := runCheckForTest(t, generated); err == nil || errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want an error for a path in no module", err)
	}

	stdout, _, err := runCheckForTest(t, "-root", root, generated)
	if !errors.Is(err, errCheckFailed) {
		t.Fatalf("err = %v, want the unknown function to fail", err)
	}
	for _, want := range []string{
		"Cannot pass int to asset argument 1 because it expects string",
		"Unknown template function 'shout'",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("output = %q, want %q", stdout, want)
		}
	}
}

func TestCheckUsesAnotherConfiguration(t *testing.T) {
	root := writeCheckProject(t, `{"functionMaps": ["example.com/app.Funcs"]}`)
	config := filepath.Join(t.TempDir(), "config.json")
	writeFile(t, filepath.Dir(config), "config.json", `{"functionMaps": ["example.com/app.Funcs"], "unknownFunctions": "warning"}`)

	stdout, _, _ := runCheckForTest(t, "-config", config, filepath.Join(root, "templates", "assets.gohtml"))
	if !strings.Contains(stdout, "Unknown template function 'shout'") {
		t.Fatalf("output = %q, want the configuration's unknownFunctions", stdout)
	}
}

func TestUnknownFunctionDiagnosticsKnowContractsAndBuiltins(t *testing.T) {
	idx := lspIndex{indexFile: indexFile{
		UnknownFunctions: "warning",
		Functions:        map[string]string{"asset": "example.com/app.Asset"},
		Types: map[string]goTypeIndex{
			"example.com/app.Page": {Name: "Page", Package: "example.com/app", Fields: map[string]fieldIndex{"Title": {Type: "string"}}},
		},
		Short: map[string][]string{"Page": {"example.com/app.Page"}},
	}}
	text := `{{/*
@model Page example.com/app.Page
@func upper strings.ToUpper
*/}}
{{ asset "x" }}{{ printf "%s" Page.Title }}{{ upper "x" }}{{ if and true (not false) }}{{ end }}
{{define "row"}}{{/* @func lower strings.ToLower */}}{{ lower "x" }}{{ upper "y" }}{{end}}
{{ lower "z" }}{{ "x" | shout }}{{define "other"}}{{/* @dot example.com/app.Page */}}{{ upper .Title }}{{end}}`
	contract := templateIndex{
		Roots: map[string]string{"Page": "example.com/app.Page"},
		Funcs: map[string]string{"upper": "strings.ToUpper"},
	}

	var unknown []string
	for _, item := range unknownFunctionDiagnostics(text, idx, contract, "page.gohtml") {
		unknown = append(unknown, item.Message)
	}

	want := []string{
		"Unknown template function 'lower'; declare it in a function map or in templateFunctions of .go-clue/config.json",
		"Unknown template function 'shout'; declare it in a function map or in templateFunctions of .go-clue/config.json",
	}
	if strings.Join(unknown, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unknown = %q, want %q", unknown, want)
	}

	idx.UnknownFunctions = ""
	if items := unknownFunctionDiagnostics(text, idx, contract, "page.gohtml"); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want none when the diagnostic is off", items)
	}
}

// A function declared by name only, such as a templateFunctions entry
// without path or signatures, exists but is not checked.
func TestLSPDoesNotCheckCallsOfFunctionsWithoutSignature(t *testing.T) {
	idx := lspIndex{indexFile: indexFile{
		Funcs: map[string]goFuncIndex{"$go-clue/templatefunc.whisper": {Name: "whisper"}},
	}}
	contract := templateIndex{Funcs: map[string]string{"whisper": "$go-clue/templatefunc.whisper"}}

	if items := diagnosticsForText(`{{ whisper "a" 1 }}{{ "x" | whisper }}`, idx, contract); len(items) != 0 {
		t.Fatalf("diagnostics = %#v, want calls of a function without signature accepted", items)
	}
}
