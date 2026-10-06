package com.donseba.goclue

import com.intellij.openapi.editor.DefaultLanguageHighlighterColors
import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.editor.colors.TextAttributesKey.createTextAttributesKey

// The colors of template symbols, one per kind of semantic token. The light
// and dark schemes get their defaults from colorSchemes/; other schemes fall
// back to their colors for the same kind of Go symbol.
internal object GoClueHighlighting {
    val FUNCTION = createTextAttributesKey("GO_CLUE_FUNCTION", DefaultLanguageHighlighterColors.FUNCTION_CALL)
    val BUILTIN_FUNCTION = createTextAttributesKey("GO_CLUE_BUILTIN_FUNCTION", DefaultLanguageHighlighterColors.PREDEFINED_SYMBOL)
    val METHOD = createTextAttributesKey("GO_CLUE_METHOD", DefaultLanguageHighlighterColors.INSTANCE_METHOD)
    val FIELD = createTextAttributesKey("GO_CLUE_FIELD", DefaultLanguageHighlighterColors.INSTANCE_FIELD)
    val STRUCT_FIELD = createTextAttributesKey("GO_CLUE_STRUCT_FIELD", DefaultLanguageHighlighterColors.INSTANCE_FIELD)
    val VARIABLE = createTextAttributesKey("GO_CLUE_VARIABLE", DefaultLanguageHighlighterColors.LOCAL_VARIABLE)
    val ROOT = createTextAttributesKey("GO_CLUE_ROOT", DefaultLanguageHighlighterColors.PARAMETER)
    val TYPE = createTextAttributesKey("GO_CLUE_TYPE", DefaultLanguageHighlighterColors.CLASS_NAME)
    val NAMESPACE = createTextAttributesKey("GO_CLUE_NAMESPACE", DefaultLanguageHighlighterColors.CLASS_REFERENCE)

    // tokenTypes and tokenModifiers are the semantic token kinds the language
    // server reports.
    val tokenTypes = listOf("variable", "property", "type", "function", "method", "parameter", "namespace")
    val tokenModifiers = listOf("defaultLibrary", "struct")

    fun keyFor(tokenType: String, modifiers: List<String>): TextAttributesKey? {
        return when (tokenType) {
            "function" -> if ("defaultLibrary" in modifiers) BUILTIN_FUNCTION else FUNCTION
            "method" -> METHOD
            "property" -> if ("struct" in modifiers) STRUCT_FIELD else FIELD
            "variable" -> VARIABLE
            "parameter" -> ROOT
            "type" -> TYPE
            "namespace" -> NAMESPACE
            else -> null
        }
    }
}
