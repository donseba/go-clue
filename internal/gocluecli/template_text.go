package gocluecli

import (
	"strings"
)

// templateAction is an action of a template, from its {{ to its }}.
type templateAction struct {
	start, end int
	// comment marks a comment action, {{/* ... */}} or {{- /* ... */ -}}.
	comment bool
	// bodyStart and bodyEnd delimit the text between /* and */ of a comment.
	bodyStart, bodyEnd int
}

// templateActions returns the actions of text in order, as text/template reads
// them: a comment runs to its */}}, so the {{ and }} of examples inside it are
// part of the comment, and a }} inside a quoted string does not end an action.
// An action that is not closed ends the scan.
func templateActions(text string) []templateAction {
	var actions []templateAction

	cursor := 0
	for {
		open := strings.Index(text[cursor:], "{{")
		if open < 0 {
			return actions
		}

		start := cursor + open
		action, ok := scanTemplateAction(text, start)
		if !ok {
			return actions
		}

		actions = append(actions, action)
		cursor = action.end
	}
}

// templateActionIndexes returns the [start, end) offsets of the actions of
// text, comments included, like a regexp's FindAllStringIndex.
func templateActionIndexes(text string) [][]int {
	actions := templateActions(text)
	indexes := make([][]int, 0, len(actions))
	for _, action := range actions {
		indexes = append(indexes, []int{action.start, action.end})
	}

	return indexes
}

// scanTemplateAction reads the action that starts at the {{ at text[start:].
func scanTemplateAction(text string, start int) (templateAction, bool) {
	inner := start + len("{{")
	if trimMarkerAt(text, inner) {
		inner += 2
	}

	if strings.HasPrefix(text[inner:], "/*") {
		return scanTemplateComment(text, start, inner)
	}

	quote := byte(0)
	escaped := false
	for index := inner; index < len(text); index++ {
		ch := text[index]
		switch {
		case escaped:
			escaped = false
		case quote != 0 && quote != '`' && ch == '\\':
			escaped = true
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '`' || ch == '\'':
			quote = ch
		case ch == '}' && strings.HasPrefix(text[index:], "}}"):
			return templateAction{start: start, end: index + len("}}")}, true
		}
	}

	return templateAction{}, false
}

// scanTemplateComment reads the comment action that starts at the {{ at
// text[start:] and whose /* is at text[open:]. Like text/template, it ends at
// the first */ and needs }} right after it, or " -}}".
func scanTemplateComment(text string, start, open int) (templateAction, bool) {
	bodyStart := open + len("/*")
	closing := strings.Index(text[bodyStart:], "*/")
	if closing < 0 {
		return templateAction{}, false
	}

	bodyEnd := bodyStart + closing
	after := bodyEnd + len("*/")
	if trimMarkerBefore(text, after) {
		after += 2
	}

	if !strings.HasPrefix(text[after:], "}}") {
		return templateAction{}, false
	}

	return templateAction{
		start:     start,
		end:       after + len("}}"),
		comment:   true,
		bodyStart: bodyStart,
		bodyEnd:   bodyEnd,
	}, true
}

// trimMarkerAt reports whether text has a left trim marker, "- ", at offset.
func trimMarkerAt(text string, offset int) bool {
	return offset+1 < len(text) && text[offset] == '-' && isSpaceByte(text[offset+1])
}

// trimMarkerBefore reports whether text has a right trim marker, " -", at
// offset, before a closing }}.
func trimMarkerBefore(text string, offset int) bool {
	return offset+1 < len(text) && isSpaceByte(text[offset]) && text[offset+1] == '-'
}

// templateComments returns the comment actions of text.
func templateComments(text string) []templateAction {
	var comments []templateAction
	for _, action := range templateActions(text) {
		if action.comment {
			comments = append(comments, action)
		}
	}

	return comments
}

// annotationScanText is text with everything outside the bodies of its
// template comments blanked, so annotation patterns only find annotations
// where go-clue reads them, at the same offsets. Newlines stay, so a
// line-anchored pattern still sees the lines of a comment. Text without
// comments is returned as it is, like contractScanText does.
func annotationScanText(text string) string {
	comments := templateComments(text)
	if len(comments) == 0 {
		return text
	}

	masked := []byte(text)
	cursor := 0
	for _, comment := range comments {
		blankOutside(masked, cursor, comment.bodyStart)
		cursor = comment.bodyEnd
	}

	blankOutside(masked, cursor, len(masked))

	return string(masked)
}

// blankOutside replaces text[start:end] with spaces, but keeps newlines.
func blankOutside(text []byte, start, end int) {
	for index := start; index < end; index++ {
		if text[index] != '\n' {
			text[index] = ' '
		}
	}
}

// leadingComments returns the comments right before actions[index]: the
// comments that only whitespace separates from it and from each other, in
// order.
func leadingComments(text string, actions []templateAction, index int) []templateAction {
	first := index
	for first > 0 {
		previous := actions[first-1]
		if !previous.comment || strings.TrimSpace(text[previous.end:actions[first].start]) != "" {
			break
		}

		first--
	}

	return actions[first:index]
}

// leadingDefineContract is a comment with go-clue annotations right before a
// {{define}}, where they read as the contract of the whole file rather than
// of the define.
type leadingDefineContract struct {
	commentStart, commentEnd int
	// removeEnd is where the action after the comment starts, so removing
	// the comment up to there also removes the whitespace after it.
	removeEnd int
	// defineStart and defineEnd delimit the {{define}} action.
	defineStart, defineEnd int
	name                   string
}

// leadingDefineContracts returns the comments with go-clue annotations among
// the comments right before a {{define}}. Text that merely mentions an
// annotation, such as "@model" in a sentence, is no annotation.
func leadingDefineContracts(text string) []leadingDefineContract {
	actions := templateActions(text)

	var contracts []leadingDefineContract
	for index, action := range actions {
		if action.comment {
			continue
		}

		content, _, ok := actionContent(text[action.start:action.end])
		if !ok {
			continue
		}

		match := lspDefineActionRegexp.FindStringSubmatch(strings.TrimSpace(content))
		if len(match) != 2 {
			continue
		}

		comments := leadingComments(text, actions, index)
		for position, comment := range comments {
			if !hasGoClueAnnotation(text[comment.start:comment.end]) {
				continue
			}

			removeEnd := action.start
			if position+1 < len(comments) {
				removeEnd = comments[position+1].start
			}

			contracts = append(contracts, leadingDefineContract{
				commentStart: comment.start,
				commentEnd:   comment.end,
				removeEnd:    removeEnd,
				defineStart:  action.start,
				defineEnd:    action.end,
				name:         match[1],
			})
		}
	}

	return contracts
}
