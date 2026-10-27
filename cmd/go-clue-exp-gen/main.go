package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/donseba/go-clue/exp/gen"
)

// defaultPackageName is the generated Go package name when -package is not
// given. It is the same default gen.GenerateDirectives uses.
const defaultPackageName = "gocluegen"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run parses the command line in args and writes the generated source to
// stdout, or to the -out file.
func run(args []string, stdout io.Writer) error {
	var opts gen.Options

	flags := flag.NewFlagSet("go-clue-exp-gen", flag.ExitOnError)
	out := flags.String("out", "", "write generated Go source to this file")
	templates := flags.String("templates", "", "comma-separated templates to scan for experimental @gen declarations; when omitted, go-clue scans the nearest Go module")
	flags.StringVar(&opts.PackagePath, "pkg", "", "Go package path to wrap")
	flags.StringVar(&opts.Namespace, "namespace", "", "template namespace name")
	flags.StringVar(&opts.PackageName, "package", defaultPackageName, "generated Go package name")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if opts.PackagePath == "" && flags.NArg() > 0 {
		opts.PackagePath = flags.Arg(0)
	}

	source, err := generate(context.Background(), opts, splitList(*templates))
	if err != nil {
		return err
	}

	if *out == "" {
		_, err := stdout.Write(source)

		return err
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}

	return os.WriteFile(*out, source, 0o644)
}

func generate(ctx context.Context, opts gen.Options, templates []string) ([]byte, error) {
	if len(templates) == 0 && opts.PackagePath != "" {
		return gen.Generate(ctx, opts)
	}
	if len(templates) == 0 {
		root, err := findModuleRoot(".")
		if err != nil {
			return nil, err
		}
		directives, err := gen.DirectivesFromDir(root)
		if err != nil {
			return nil, err
		}
		return gen.GenerateDirectives(ctx, opts.PackageName, directives)
	}

	directives, err := gen.DirectivesFromFiles(templates...)
	if err != nil {
		return nil, err
	}
	if opts.PackagePath != "" {
		directives = append(directives, gen.Directive{
			Namespace:   opts.Namespace,
			PackagePath: opts.PackagePath,
		})
	}
	return gen.GenerateDirectives(ctx, opts.PackageName, directives)
}

func findModuleRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("gen: no go.mod found above %s", start)
		}
		dir = parent
	}
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
