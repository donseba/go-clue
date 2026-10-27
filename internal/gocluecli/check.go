package gocluecli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// errCheckFailed reports that check found diagnostics at the severity that
// fails it. Main exits with 1 for it, without printing it again.
var errCheckFailed = errors.New("go-clue: check found problems")

// exitCodeError is an error that ends go-clue with its own exit code.
type exitCodeError struct {
	code int
	err  error
}

func (e exitCodeError) Error() string {
	return e.err.Error()
}

func (e exitCodeError) Unwrap() error {
	return e.err
}

type (
	// checkOptions are the flags of go-clue check.
	checkOptions struct {
		// json writes the result as JSON instead of text.
		json bool
		// minSeverity is the least severe diagnostic that is reported.
		minSeverity int
		// failOn is the least severe diagnostic that fails the check.
		failOn int
		// root is the project every path is checked in, instead of the
		// module each path is in.
		root string
		// config is a configuration file used instead of each project's
		// .go-clue/config.json.
		config string
		// problems also reports the problems of building the index, such as
		// a configured function map that does not exist.
		problems bool
	}

	// checkTarget is a project and the templates to check in it.
	checkTarget struct {
		root  string
		files []string
	}

	// checkDiagnostic is a diagnostic as check reports it, with 1-based
	// lines and columns.
	checkDiagnostic struct {
		File      string `json:"file"`
		Line      int    `json:"line,omitempty"`
		Column    int    `json:"column,omitempty"`
		EndLine   int    `json:"endLine,omitempty"`
		EndColumn int    `json:"endColumn,omitempty"`
		Severity  string `json:"severity"`
		Message   string `json:"message"`
		severity  int
	}

	// checkResult is what check reports.
	checkResult struct {
		Templates   int               `json:"templates"`
		Errors      int               `json:"errors"`
		Warnings    int               `json:"warnings"`
		Infos       int               `json:"infos"`
		Hints       int               `json:"hints"`
		Diagnostics []checkDiagnostic `json:"diagnostics"`
	}
)

// runCheck runs go-clue check: the diagnostics of the language server for
// every template of the projects or files in args, at once.
func runCheck(args []string, stdout, stderr io.Writer) error {
	opts, paths, err := parseCheckFlags(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}

	if err != nil {
		return exitCodeError{code: 2, err: err}
	}

	targets, err := checkTargets(paths, opts)
	if err != nil {
		return exitCodeError{code: 2, err: err}
	}

	result := checkResult{Diagnostics: []checkDiagnostic{}}
	failed := false
	for _, target := range targets {
		diagnostics, err := checkProject(target, opts)
		if err != nil {
			return exitCodeError{code: 2, err: err}
		}

		result.Templates += len(target.files)
		for _, item := range diagnostics {
			if item.severity <= opts.failOn {
				failed = true
			}

			if item.severity > opts.minSeverity {
				continue
			}

			result.add(item)
		}
	}

	if err := writeCheckResult(stdout, stderr, result, opts); err != nil {
		return exitCodeError{code: 2, err: err}
	}

	if failed {
		return errCheckFailed
	}

	return nil
}

// parseCheckFlags reads the flags of go-clue check and returns the paths
// after them, or the current directory.
func parseCheckFlags(args []string, stderr io.Writer) (checkOptions, []string, error) {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(stderr)

	jsonOutput := flags.Bool("json", false, "write the diagnostics as JSON")
	severity := flags.String("severity", "warning", "report diagnostics of this severity and worse: error, warning, info or hint")
	failOn := flags.String("fail-on", "error", "exit with 1 for diagnostics of this severity and worse: error, warning, info or hint")
	root := flags.String("root", "", "check every path in this project instead of the module each path is in")
	config := flags.String("config", "", "use this configuration file instead of each project's .go-clue/config.json")
	problems := flags.Bool("problems", false, "also report the problems of building the index, such as a missing function map")

	if err := flags.Parse(args); err != nil {
		return checkOptions{}, nil, err
	}

	opts := checkOptions{
		json:     *jsonOutput,
		root:     *root,
		config:   *config,
		problems: *problems,
	}

	var ok bool
	if opts.minSeverity, ok = parseSeverity(*severity); !ok {
		return checkOptions{}, nil, fmt.Errorf("go-clue check: unknown severity %q", *severity)
	}

	if opts.failOn, ok = parseSeverity(*failOn); !ok {
		return checkOptions{}, nil, fmt.Errorf("go-clue check: unknown severity %q", *failOn)
	}

	paths := flags.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	return opts, paths, nil
}

// checkTargets groups the templates of paths by the project they are checked
// in: the -root project, or else the module a path is in. A directory gets
// the templates under it that its project's configuration includes, a file is
// checked as it is, also outside its project.
func checkTargets(paths []string, opts checkOptions) ([]checkTarget, error) {
	byRoot := make(map[string]*checkTarget)
	var order []string

	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, err
		}

		root, err := checkRoot(absPath, opts)
		if err != nil {
			return nil, err
		}

		target, ok := byRoot[root]
		if !ok {
			target = &checkTarget{root: root}
			byRoot[root] = target
			order = append(order, root)
		}

		if !info.IsDir() {
			target.files = append(target.files, absPath)
			continue
		}

		cfg, err := checkConfig(root, opts)
		if err != nil {
			return nil, err
		}

		files, err := templateFilesUnder(root, absPath, cfg)
		if err != nil {
			return nil, err
		}

		target.files = append(target.files, files...)
	}

	targets := make([]checkTarget, 0, len(order))
	for _, root := range order {
		target := byRoot[root]
		sort.Strings(target.files)
		target.files = slices.Compact(target.files)
		targets = append(targets, *target)
	}

	return targets, nil
}

// checkRoot returns the project path is checked in.
func checkRoot(path string, opts checkOptions) (string, error) {
	if opts.root != "" {
		return filepath.Abs(opts.root)
	}

	root := nearestModuleRoot(path)
	if root == "" {
		return "", fmt.Errorf("go-clue check: %s is in no Go module; pass -root", path)
	}

	return root, nil
}

// checkConfig returns the configuration of the project at root.
func checkConfig(root string, opts checkOptions) (indexConfig, error) {
	if opts.config == "" {
		return loadIndexConfig(root), nil
	}

	return loadIndexConfigFile(opts.config)
}

// templateFilesUnder returns the template files under dir. Inside the
// project at root, the folders go-clue skips and those the configuration
// excludes are left out, as are nested modules.
func templateFilesUnder(root, dir string, cfg indexConfig) ([]string, error) {
	inside := isWithin(root, dir)

	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if path == dir {
				return nil
			}

			if defaultSkippedDirs[entry.Name()] || hasGoMod(path) {
				return filepath.SkipDir
			}

			if inside && !shouldIncludePath(root, path, cfg) {
				return filepath.SkipDir
			}

			return nil
		}

		if !isTemplateFile(path) || inside && !shouldIncludePath(root, path, cfg) {
			return nil
		}

		files = append(files, path)
		return nil
	})

	return files, err
}

// isWithin reports whether path is root or under it.
func isWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// checkProject returns the diagnostics of the templates of target.
func checkProject(target checkTarget, opts checkOptions) ([]checkDiagnostic, error) {
	cfg, err := checkConfig(target.root, opts)
	if err != nil {
		return nil, err
	}

	built, _, err := buildIndexFromConfig(target.root, cfg, true)
	if err != nil {
		return nil, fmt.Errorf("go-clue check: index %s: %w", target.root, err)
	}

	idx := lspIndex{indexFile: built, rootPath: target.root}

	var diagnostics []checkDiagnostic
	if opts.problems {
		for _, item := range built.Problems {
			diagnostics = append(diagnostics, checkProblem(target.root, item))
		}
	}

	for _, path := range target.files {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		text := string(data)
		items, _ := documentDiagnostics(path, text, idx)
		if item, ok := templateParseDiagnostic(text); ok {
			items = append(items, item)
		}

		for _, item := range items {
			diagnostics = append(diagnostics, newCheckDiagnostic(path, item))
		}
	}

	return diagnostics, nil
}

// newCheckDiagnostic converts a diagnostic of the template at path.
func newCheckDiagnostic(path string, item diagnostic) checkDiagnostic {
	return checkDiagnostic{
		File:      displayPath(path),
		Line:      item.Range.Start.Line + 1,
		Column:    item.Range.Start.Character + 1,
		EndLine:   item.Range.End.Line + 1,
		EndColumn: item.Range.End.Character + 1,
		Severity:  severityName(item.Severity),
		Message:   item.Message,
		severity:  item.Severity,
	}
}

// checkProblem converts a problem of building the index of the project at
// root into a warning.
func checkProblem(root string, item problem) checkDiagnostic {
	file := item.File
	if file != "" && !filepath.IsAbs(file) && !strings.Contains(file, ":") {
		file = displayPath(filepath.Join(root, file))
	}

	if file == "" {
		file = displayPath(root)
	}

	return checkDiagnostic{
		File:     file,
		Severity: severityName(severityWarning),
		Message:  item.Message,
		severity: severityWarning,
	}
}

// displayPath is path relative to the working directory when it is under
// it, else path itself.
func displayPath(path string) string {
	wd, err := os.Getwd()
	if err != nil || !isWithin(wd, path) {
		return filepath.ToSlash(path)
	}

	relative, err := filepath.Rel(wd, path)
	if err != nil {
		return filepath.ToSlash(path)
	}

	return filepath.ToSlash(relative)
}

// add adds item to the result and counts it.
func (r *checkResult) add(item checkDiagnostic) {
	r.Diagnostics = append(r.Diagnostics, item)
	switch item.severity {
	case severityError:
		r.Errors++
	case severityWarning:
		r.Warnings++
	case severityInfo:
		r.Infos++
	default:
		r.Hints++
	}
}

// writeCheckResult writes the result: as JSON, or as one line per diagnostic
// with a summary on stderr.
func writeCheckResult(stdout, stderr io.Writer, result checkResult, opts checkOptions) error {
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		left, right := result.Diagnostics[i], result.Diagnostics[j]
		if left.File != right.File {
			return left.File < right.File
		}

		if left.Line != right.Line {
			return left.Line < right.Line
		}

		return left.Column < right.Column
	})

	if opts.json {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}

		_, err = fmt.Fprintf(stdout, "%s\n", data)
		return err
	}

	for _, item := range result.Diagnostics {
		location := item.File
		if item.Line > 0 {
			location = fmt.Sprintf("%s:%d:%d", item.File, item.Line, item.Column)
		}

		if _, err := fmt.Fprintf(stdout, "%s: %s: %s\n", location, item.Severity, item.Message); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(stderr, "go-clue: %d templates, %d errors, %d warnings, %d infos, %d hints\n",
		result.Templates, result.Errors, result.Warnings, result.Infos, result.Hints)
	return err
}
