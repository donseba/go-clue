package com.donseba.goclue

import org.junit.Test
import kotlin.test.assertEquals

class GoClueTemplateContextTest {
    private val view = "example.com/app.View"
    private val item = "example.com/app.MenuItem"

    private val index = GoClueIndex(
        types = mapOf(
            view to type(view, "View", "Menu" to "[]$item", "Home" to item, "Title" to "string"),
            item to type(item, "MenuItem", "Title" to "string", "Active" to "bool", "Children" to "[]$item"),
        ),
        funcs = emptyMap(),
        templates = emptyMap(),
        short = mapOf("View" to listOf(view), "MenuItem" to listOf(item)),
    )
    private val contract = TemplateContract(roots = emptyMap(), dot = view)

    @Test
    fun `end of an if inside a range does not close the range`() {
        val text = """{{range .Menu}}{{if .Active}}x{{end}}{{range .Children}}{{.Title}}{{end}}{{end}}"""
        assertEquals(item, ownerAt(text, text.indexOf(".Children") + 1))
    }

    @Test
    fun `else of a range sees the outer dot`() {
        val text = """{{range .Menu}}{{.Active}}{{else}}{{.Title}}{{end}}"""
        assertEquals(view, ownerAt(text, text.lastIndexOf(".Title") + 1))
    }

    @Test
    fun `else with moves the dot to its value`() {
        val text = """{{if .Title}}{{.Title}}{{else with .Home}}{{.Active}}{{end}}{{.Title}}"""
        assertEquals(item, ownerAt(text, text.indexOf(".Active") + 1))
        assertEquals(view, ownerAt(text, text.lastIndexOf(".Title") + 1))
    }

    private fun ownerAt(text: String, offset: Int): String? {
        return GoClueTemplateContext.fieldReferenceAt(text, offset, index, contract)?.ownerTypeName
    }

    private fun type(fqName: String, name: String, vararg fields: Pair<String, String>): GoClueType {
        return GoClueType(
            fqName = fqName,
            name = name,
            pkg = "example.com/app",
            file = "app.go",
            line = 1,
            column = 1,
            doc = "",
            fields = fields.associate { (field, typ) -> field to GoClueField(field, typ, "", "app.go", 1, 1) },
            methods = emptyMap(),
        )
    }
}
