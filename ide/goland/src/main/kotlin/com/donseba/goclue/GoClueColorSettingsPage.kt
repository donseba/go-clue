package com.donseba.goclue

import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.fileTypes.PlainSyntaxHighlighter
import com.intellij.openapi.fileTypes.SyntaxHighlighter
import com.intellij.openapi.options.colors.AttributesDescriptor
import com.intellij.openapi.options.colors.ColorDescriptor
import com.intellij.openapi.options.colors.ColorSettingsPage
import javax.swing.Icon

// GoClueColorSettingsPage is Settings | Editor | Color Scheme | go-clue, where
// the colors of template symbols can be changed per scheme.
internal class GoClueColorSettingsPage : ColorSettingsPage {
    override fun getDisplayName(): String = "go-clue"

    override fun getIcon(): Icon? = null

    override fun getHighlighter(): SyntaxHighlighter = PlainSyntaxHighlighter()

    override fun getDemoText(): String = """
        {{/* @model <root>Page</root> example.com/app.<type>Page</type> */}}
        {{/* @gen <namespace>money</namespace> example.com/app/moneyfuncs */}}
        <h1>{{ <function>upper</function> <root>Page</root>.<field>Title</field> }}</h1>
        {{ range <variable>${'$'}item</variable> := <root>Page</root>.<field>Items</field> }}
            <p>{{ <variable>${'$'}item</variable>.<field>Name</field> }}: {{ <namespace>money</namespace>.<method>EUR</method> <variable>${'$'}item</variable>.<field>Cents</field> }}</p>
        {{ end }}
        {{ if <builtin>eq</builtin> (<builtin>len</builtin> <root>Page</root>.<field>Items</field>) 0 }}{{ <root>Page</root>.<method>Summary</method> }}{{ end }}
    """.trimIndent()

    override fun getAdditionalHighlightingTagToDescriptorMap(): Map<String, TextAttributesKey> = mapOf(
        "function" to GoClueHighlighting.FUNCTION,
        "builtin" to GoClueHighlighting.BUILTIN_FUNCTION,
        "method" to GoClueHighlighting.METHOD,
        "field" to GoClueHighlighting.FIELD,
        "variable" to GoClueHighlighting.VARIABLE,
        "root" to GoClueHighlighting.ROOT,
        "type" to GoClueHighlighting.TYPE,
        "namespace" to GoClueHighlighting.NAMESPACE,
    )

    override fun getAttributeDescriptors(): Array<AttributesDescriptor> = arrayOf(
        AttributesDescriptor("Template function", GoClueHighlighting.FUNCTION),
        AttributesDescriptor("Built-in function (eq, len, printf, …)", GoClueHighlighting.BUILTIN_FUNCTION),
        AttributesDescriptor("Method", GoClueHighlighting.METHOD),
        AttributesDescriptor("Field", GoClueHighlighting.FIELD),
        AttributesDescriptor("Variable (\$item)", GoClueHighlighting.VARIABLE),
        AttributesDescriptor("Typed root (@model, @symbol, …)", GoClueHighlighting.ROOT),
        AttributesDescriptor("Type", GoClueHighlighting.TYPE),
        AttributesDescriptor("Generated namespace (@gen)", GoClueHighlighting.NAMESPACE),
    )

    override fun getColorDescriptors(): Array<ColorDescriptor> = ColorDescriptor.EMPTY_ARRAY
}
