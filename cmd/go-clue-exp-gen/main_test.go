package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const helpersPackage = "github.com/donseba/go-clue/exp/gen/testdata/helpers"

func TestRunWithoutPackageFlagUsesDefaultPackageName(t *testing.T) {
	var out bytes.Buffer

	if err := run([]string{"-pkg", helpersPackage}, &out); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	source := out.String()
	if !strings.Contains(source, "\npackage "+defaultPackageName+"\n") {
		t.Fatalf("generated source does not declare package %s:\n%s", defaultPackageName, source)
	}

	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.AllErrors); err != nil {
		t.Fatalf("generated source is not parseable: %v\n%s", err, source)
	}
}

func TestRunWritesOutFileWithPackageFlag(t *testing.T) {
	file := filepath.Join(t.TempDir(), "gen", "gen.go")

	err := run([]string{"-package", "gen", "-namespace", "helpers", "-out", file, helpersPackage}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "\npackage gen\n") {
		t.Fatalf("generated file does not declare package gen:\n%s", data)
	}
}
