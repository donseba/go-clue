package gocluecli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLSPStartsInFolderWithoutGoMod(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "services", "web")
	templatePath := filepath.Join(module, "templates", "page.gohtml")
	text := `{{/*
@model page example.com/web.Page
*/}}
{{ page.Title }} {{ page.Missing }}`

	writeTestFile(t, filepath.Join(module, "go.mod"), "module example.com/web\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(module, "page.go"), "package web\n\ntype Page struct {\n\tTitle string\n}\n")
	writeTestFile(t, templatePath, text)

	var input bytes.Buffer

	writeTestRPC(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	writeTestRPC(t, &input, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        uriFromPath(templatePath),
				"languageId": "gohtml",
				"version":    1,
				"text":       text,
			},
		},
	})

	var output, logs bytes.Buffer

	if err := runLSP(&input, &output, &logs, root); err != nil {
		t.Fatalf("runLSP() error = %v", err)
	}

	if !strings.Contains(logs.String(), "no go.mod in or above "+root) {
		t.Fatalf("log = %q, want a line about the missing go.mod", logs.String())
	}

	var (
		initialized bool
		diagnostics []diagnostic
		published   bool
	)

	reader := bufio.NewReader(&output)
	for {
		msg, err := readRPCMessage(reader)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("readRPCMessage() error = %v", err)
		}

		if string(msg.ID) == "1" && msg.Result != nil {
			initialized = true
		}

		if msg.Method == lspMethodPublishDiagnostics {
			var params struct {
				Diagnostics []diagnostic `json:"diagnostics"`
			}
			if err := json.Unmarshal(msg.Params, &params); err != nil {
				t.Fatalf("diagnostics params: %v", err)
			}

			diagnostics = params.Diagnostics
			published = true
		}
	}

	if !initialized {
		t.Fatalf("no initialize result in output:\n%s", output.String())
	}

	if !published {
		t.Fatalf("no diagnostics published for the template:\n%s", output.String())
	}

	// The template is checked with the index of its own module: the type is
	// known, so only the missing field is reported.
	if len(diagnostics) != 1 || !strings.HasPrefix(diagnostics[0].Message, unknownFieldDiagnosticPrefix) {
		t.Fatalf("diagnostics = %#v, want one unknown field", diagnostics)
	}
}

func TestLSPRootIsNearestModuleAboveRoot(t *testing.T) {
	module := t.TempDir()
	templates := filepath.Join(module, "templates")

	writeTestFile(t, filepath.Join(module, "go.mod"), "module example.com/app\n")
	writeTestFile(t, filepath.Join(templates, "page.gohtml"), "")

	var logs bytes.Buffer

	root, err := lspRoot(templates, &logs)
	if err != nil {
		t.Fatalf("lspRoot() error = %v", err)
	}

	if root != module {
		t.Fatalf("lspRoot() = %q, want %q", root, module)
	}

	if !strings.Contains(logs.String(), "using the module at "+module) {
		t.Fatalf("log = %q, want a line about the module above", logs.String())
	}

	logs.Reset()

	root, err = lspRoot(module, &logs)
	if err != nil {
		t.Fatalf("lspRoot() error = %v", err)
	}

	if root != module || logs.Len() != 0 {
		t.Fatalf("lspRoot() = %q with log %q, want %q without log", root, logs.String(), module)
	}
}

func TestLSPRebuildsIndexFileOfAnotherVersion(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "templates", "page.gohtml")
	indexPath := filepath.Join(root, goClueDir, indexFileName)

	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	writeTestFile(t, filepath.Join(root, goClueDir, "config.json"), `{"writeIndex": true}`)
	writeTestFile(t, filepath.Join(root, "page.go"), "package app\n\ntype Page struct {\n\tTitle string\n}\n")
	writeTestFile(t, templatePath, "{{/*\n@model page example.com/app.Page\n*/}}\n{{ page.Title }}")

	old := emptyIndexFile()
	old.Version = indexVersion - 1
	old.Module = "example.com/app"
	old.Types["example.com/app.Old"] = goTypeIndex{Name: "Old", Package: "example.com/app"}

	if err := writeJSON(old, indexPath); err != nil {
		t.Fatalf("writeJSON() error = %v", err)
	}

	// The index file is newer than the source, so only its version makes the
	// server build the index again.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(indexPath, future, future); err != nil {
		t.Fatal(err)
	}

	assertBuilt := func(name string, idx indexFile) {
		t.Helper()

		if _, ok := idx.Types["example.com/app.Old"]; ok {
			t.Fatalf("%s: the index file of version %d was used", name, old.Version)
		}

		if _, ok := idx.Types["example.com/app.Page"]; !ok {
			t.Fatalf("%s: the index was not built from the source: %#v", name, idx.Types)
		}

		if idx.Version != indexVersion {
			t.Fatalf("%s: version = %d, want %d", name, idx.Version, indexVersion)
		}
	}

	idx, path, _, err := loadOrBuildIndex(root)
	if err != nil {
		t.Fatalf("loadOrBuildIndex() error = %v", err)
	}

	assertBuilt("loadOrBuildIndex", idx)

	if path != indexPath {
		t.Fatalf("loadOrBuildIndex() path = %q, want %q so the file is still watched", path, indexPath)
	}

	server := &lspServer{root: root, indexes: make(map[string]cachedLSPIndex)}
	assertBuilt("indexForURI", server.indexForURI(uriFromPath(templatePath)).indexFile)

	// A rewritten index file of another version does not replace the index
	// in memory.
	current := emptyIndexFile()
	current.Module = "example.com/app"
	server = &lspServer{root: root, indexPath: indexPath, idx: current}

	if server.refreshIndex() {
		t.Fatal("refreshIndex() = true for an index file of another version")
	}

	if server.idx.Module != current.Module || len(server.idx.Types) != 0 {
		t.Fatalf("refreshIndex() replaced the index: %#v", server.idx)
	}
}

func writeTestRPC(t *testing.T, w io.Writer, msg map[string]any) {
	t.Helper()

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fmt.Fprintf(w, rpcContentFormat, len(data), data); err != nil {
		t.Fatal(err)
	}
}
