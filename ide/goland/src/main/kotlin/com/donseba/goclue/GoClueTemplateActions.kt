package com.donseba.goclue

// GoClueTemplateAction is an action of a template, from its {{ to its }}.
// For a comment, bodyStart and bodyEnd delimit the text between /* and */.
data class GoClueTemplateAction(
    val start: Int,
    val end: Int,
    val comment: Boolean = false,
    val bodyStart: Int = -1,
    val bodyEnd: Int = -1,
) {
    fun text(source: String): String = source.substring(start, end)
}

// GoClueTemplateActions reads the actions of a template like text/template
// does, as the language server's template_text.go does: a comment runs to its
// */}}, so the {{ and }} of examples inside it belong to the comment, and a }}
// inside a quoted string does not end an action.
object GoClueTemplateActions {
    fun actions(text: String): List<GoClueTemplateAction> {
        val actions = mutableListOf<GoClueTemplateAction>()
        var cursor = 0
        while (true) {
            val start = text.indexOf("{{", cursor)
            if (start < 0) return actions

            val action = scan(text, start) ?: return actions
            actions.add(action)
            cursor = action.end
        }
    }

    // codeActions are the actions of text that are not comments.
    fun codeActions(text: String): List<GoClueTemplateAction> = actions(text).filter { !it.comment }

    fun comments(text: String): List<GoClueTemplateAction> = actions(text).filter { it.comment }

    // inComment reports whether offset is inside a template comment.
    fun inComment(text: String, offset: Int): Boolean {
        return comments(text).any { offset >= it.start && offset < it.end }
    }

    // annotationText is text with everything outside the bodies of its
    // template comments blanked, so annotation patterns only find annotations
    // where go-clue reads them, at the same offsets. Newlines stay. Text
    // without comments is returned as it is.
    fun annotationText(text: String): String {
        val comments = comments(text)
        if (comments.isEmpty()) return text

        val chars = text.toCharArray()
        var cursor = 0
        for (comment in comments) {
            blank(chars, cursor, comment.bodyStart)
            cursor = comment.bodyEnd
        }
        blank(chars, cursor, chars.size)
        return String(chars)
    }

    // commentBodies returns the bodies of the comments of text, trimmed and
    // one after the other, where go-clue reads annotations. Text without
    // comments is returned as it is.
    fun commentBodies(text: String): String {
        val bodies = comments(text)
            .map { text.substring(it.bodyStart, it.bodyEnd).trim() }
            .filter { it.isNotBlank() }
        if (bodies.isEmpty()) return text
        return bodies.joinToString(separator = "\n", postfix = "\n")
    }

    // content is the text of an action between its delimiters and trim
    // markers.
    fun content(action: String): String? {
        val start = action.indexOf("{{")
        val end = action.lastIndexOf("}}")
        if (start < 0 || end < start) return null
        return action.substring(start + 2, end).trim().trim('-').trim()
    }

    private fun blank(chars: CharArray, start: Int, end: Int) {
        for (index in start until end) {
            if (chars[index] != '\n') chars[index] = ' '
        }
    }

    private fun scan(text: String, start: Int): GoClueTemplateAction? {
        var inner = start + 2
        if (trimMarkerAt(text, inner)) inner += 2
        if (text.startsWith("/*", inner)) return scanComment(text, start, inner)

        var quote: Char? = null
        var escaped = false
        var index = inner
        while (index < text.length) {
            val ch = text[index]
            when {
                escaped -> escaped = false
                quote != null && quote != '`' && ch == '\\' -> escaped = true
                quote != null -> if (ch == quote) quote = null
                ch == '"' || ch == '`' || ch == '\'' -> quote = ch
                ch == '}' && text.startsWith("}}", index) -> return GoClueTemplateAction(start, index + 2)
            }
            index++
        }
        return null
    }

    private fun scanComment(text: String, start: Int, open: Int): GoClueTemplateAction? {
        val bodyStart = open + 2
        val bodyEnd = text.indexOf("*/", bodyStart)
        if (bodyEnd < 0) return null

        var after = bodyEnd + 2
        if (trimMarkerBefore(text, after)) after += 2
        if (!text.startsWith("}}", after)) return null

        return GoClueTemplateAction(start, after + 2, comment = true, bodyStart = bodyStart, bodyEnd = bodyEnd)
    }

    private fun trimMarkerAt(text: String, offset: Int): Boolean {
        return offset + 1 < text.length && text[offset] == '-' && text[offset + 1].isWhitespace()
    }

    private fun trimMarkerBefore(text: String, offset: Int): Boolean {
        return offset + 1 < text.length && text[offset].isWhitespace() && text[offset + 1] == '-'
    }
}
