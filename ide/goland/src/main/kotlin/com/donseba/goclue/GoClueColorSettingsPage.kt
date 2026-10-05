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
        {{/* @dot example.com/app.<type>View</type> */}}
        {{/* @gen <namespace>money</namespace> example.com/app/moneyfuncs */}}
        <title>{{ <function>upper</function> <field>.Title</field> }} · {{ <struct>.Site</struct>.<field>Name</field> }}</title>
        {{ range <variable>${'$'}item</variable> := <field>.Items</field> }}
            <p>{{ <variable>${'$'}item</variable>.<field>Name</field> }}: {{ <namespace>money</namespace>.<method>EUR</method> <variable>${'$'}item</variable>.<field>Cents</field> }}</p>
        {{ end }}
        {{ with <struct>.Event</struct> }}<time>{{ <struct>.Start</struct>.<method>Format</method> "2 Jan" }}</time>{{ end }}
        {{ if <builtin>eq</builtin> (<builtin>len</builtin> <field>.Items</field>) 0 }}{{ <method>.Summary</method> }}{{ end }}
        {{ range <field>.Tags</field> }}<span>{{ <root>.</root> }}</span>{{ end }}

        {{/* @model <root>Page</root> example.com/app.<type>Page</type> */}}
        <h1>{{ <root>Page</root>.<field>Title</field> }}</h1>
    """.trimIndent()

    override fun getAdditionalHighlightingTagToDescriptorMap(): Map<String, TextAttributesKey> = mapOf(
        "function" to GoClueHighlighting.FUNCTION,
        "builtin" to GoClueHighlighting.BUILTIN_FUNCTION,
        "method" to GoClueHighlighting.METHOD,
        "field" to GoClueHighlighting.FIELD,
        "struct" to GoClueHighlighting.STRUCT_FIELD,
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
        AttributesDescriptor("Field holding a struct (.Site in .Site.Name)", GoClueHighlighting.STRUCT_FIELD),
        AttributesDescriptor("Variable (\$item)", GoClueHighlighting.VARIABLE),
        AttributesDescriptor("Template data (the dot, \$, @model roots)", GoClueHighlighting.ROOT),
        AttributesDescriptor("Type", GoClueHighlighting.TYPE),
        AttributesDescriptor("Generated namespace (@gen)", GoClueHighlighting.NAMESPACE),
    )

    override fun getColorDescriptors(): Array<ColorDescriptor> = ColorDescriptor.EMPTY_ARRAY
}
