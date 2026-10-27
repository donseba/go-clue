package gocluecli

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template/parse"
)

// Diagnostic severities, as the language server protocol numbers them.
const (
	severityError   = 1
	severityWarning = 2
	severityInfo    = 3
	severityHint    = 4
)

const (
	unknownFunctionMessageFormat = "Unknown template function '%s'; declare it in a function map or in templateFunctions of .go-clue/config.json"
	templateParseMessagePrefix   = "Template does not parse: "
)

// templateParseErrorPattern reads the line and message of a text/template
// parse error, such as "template: check:12: unexpected {{end}}".
var templateParseErrorPattern = regexp.MustCompile(`^template: [^:]*:(\d+):(?:(\d+):)?\s*(.*)$`)

// severityNames are the names of the severities, as configuration and the
// check command take them.
var severityNames = map[string]int{
	"error":       severityError,
	"warning":     severityWarning,
	"info":        severityInfo,
	"information": severityInfo,
	"hint":        severityHint,
}

// parseSeverity returns the severity named name.
func parseSeverity(name string) (int, bool) {
	severity, ok := severityNames[strings.ToLower(strings.TrimSpace(name))]
	return severity, ok
}

// severityName returns the name of severity.
func severityName(severity int) string {
	switch severity {
	case severityError:
		return "error"
	case severityWarning:
		return "warning"
	case severityInfo:
		return "info"
	default:
		return "hint"
	}
}

// diagnosticSeverityName returns the canonical name of a configured severity,
// or "" when the diagnostic is off: for "", "off", "none" or an unknown name.
func diagnosticSeverityName(name string) string {
	severity, ok := parseSeverity(name)
	if !ok {
		return ""
	}

	return severityName(severity)
}

// unknownFunctionDiagnostics reports the calls in text to functions that are
// neither text/template's own, nor in the contract of the file or of the
// define they are in (function maps, configured functions, @func and typed
// roots), nor among the project's functions. It is off unless the configuration sets unknownFunctions, and it
// reports nothing for text that does not parse.
func unknownFunctionDiagnostics(text string, idx lspIndex, contract templateIndex, relative string) []diagnostic {
	severity, ok := parseSeverity(idx.UnknownFunctions)
	if !ok {
		return nil
	}

	identifiers, err := templateIdentifiers(text)
	if err != nil {
		return nil
	}

	var items []diagnostic
	for _, identifier := range identifiers {
		name := identifier.Ident
		if _, ok := builtInTemplateFuncs[name]; ok || idx.Functions[name] != "" {
			continue
		}

		// Functions are those of the whole template set, so the file's
		// declarations count inside its defines too.
		if contract.Funcs[name] != "" || contract.Roots[name] != "" {
			continue
		}

		start := int(identifier.Pos)
		actionContract := activeContractAt(text, idx, relative, contract, start)
		if actionContract.Funcs[name] != "" || actionContract.Roots[name] != "" {
			continue
		}

		items = append(items, diagnostic{
			Range:    rangeFromOffsets(text, start, start+len(name)),
			Severity: severity,
			Source:   lspSource,
			Message:  fmt.Sprintf(unknownFunctionMessageFormat, name),
		})
	}

	return items
}

// templateIdentifiers returns the identifiers text calls, in order. In a
// parsed template every identifier is a function or a typed root.
func templateIdentifiers(text string) ([]*parse.IdentifierNode, error) {
	trees, err := parseTemplateText(text)
	if err != nil {
		return nil, err
	}

	var identifiers []*parse.IdentifierNode
	for _, tree := range trees {
		identifiers = append(identifiers, nodeIdentifiers(tree.Root)...)
	}

	sort.Slice(identifiers, func(i, j int) bool { return identifiers[i].Pos < identifiers[j].Pos })

	return identifiers, nil
}

// parseTemplateText parses text like text/template does, without knowing its
// functions, and returns its trees: the file's and those of its defines.
func parseTemplateText(text string) (map[string]*parse.Tree, error) {
	tree := parse.New("check")
	tree.Mode = parse.SkipFuncCheck

	trees := make(map[string]*parse.Tree)
	if _, err := tree.Parse(text, "", "", trees); err != nil {
		return nil, err
	}

	return trees, nil
}

// nodeIdentifiers returns the identifiers under node.
func nodeIdentifiers(node parse.Node) []*parse.IdentifierNode {
	var identifiers []*parse.IdentifierNode

	switch node := node.(type) {
	case *parse.ListNode:
		if node == nil {
			return nil
		}

		for _, child := range node.Nodes {
			identifiers = append(identifiers, nodeIdentifiers(child)...)
		}
	case *parse.ActionNode:
		identifiers = nodeIdentifiers(node.Pipe)
	case *parse.IfNode:
		identifiers = branchIdentifiers(&node.BranchNode)
	case *parse.RangeNode:
		identifiers = branchIdentifiers(&node.BranchNode)
	case *parse.WithNode:
		identifiers = branchIdentifiers(&node.BranchNode)
	case *parse.TemplateNode:
		identifiers = nodeIdentifiers(node.Pipe)
	case *parse.PipeNode:
		if node == nil {
			return nil
		}

		for _, command := range node.Cmds {
			identifiers = append(identifiers, nodeIdentifiers(command)...)
		}
	case *parse.CommandNode:
		for _, arg := range node.Args {
			identifiers = append(identifiers, nodeIdentifiers(arg)...)
		}
	case *parse.ChainNode:
		identifiers = nodeIdentifiers(node.Node)
	case *parse.IdentifierNode:
		identifiers = []*parse.IdentifierNode{node}
	}

	return identifiers
}

// branchIdentifiers returns the identifiers of an if, range or with.
func branchIdentifiers(node *parse.BranchNode) []*parse.IdentifierNode {
	identifiers := nodeIdentifiers(node.Pipe)
	identifiers = append(identifiers, nodeIdentifiers(node.List)...)
	return append(identifiers, nodeIdentifiers(node.ElseList)...)
}

// templateParseDiagnostic reports that text does not parse as a template.
func templateParseDiagnostic(text string) (diagnostic, bool) {
	_, err := parseTemplateText(text)
	if err == nil {
		return diagnostic{}, false
	}

	message := err.Error()
	line := 0
	if match := templateParseErrorPattern.FindStringSubmatch(message); len(match) == 4 {
		line, _ = strconv.Atoi(match[1])
		line--
		message = match[3]
	}

	start := offsetAt(text, position{Line: max(0, line)})
	end := strings.IndexByte(text[start:], '\n')
	if end < 0 {
		end = len(text) - start
	}

	return diagnostic{
		Range:    rangeFromOffsets(text, start, start+end),
		Severity: severityError,
		Source:   lspSource,
		Message:  templateParseMessagePrefix + message,
	}, true
}
