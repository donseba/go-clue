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

    @Test
    fun `same-package field types resolve when another package shares their name`() {
        val documents = "example.com/app/documents"
        val listView = "$documents.ListView"
        val category = "$documents.Category"
        val shared = GoClueIndex(
            types = mapOf(
                listView to type(listView, "ListView", "AllURL" to "string", "Category" to "*Category", "Categories" to "[]Category", pkg = documents),
                category to type(
                    category,
                    "Category",
                    "Name" to "string",
                    "URL" to "string",
                    "Count" to "int",
                    "Active" to "bool",
                    pkg = documents,
                ),
                "example.com/app/directory.Category" to type("example.com/app/directory.Category", "Category", pkg = "example.com/app/directory"),
                "example.com/app/publishing.Category" to type("example.com/app/publishing.Category", "Category", pkg = "example.com/app/publishing"),
            ),
            funcs = emptyMap(),
            templates = emptyMap(),
            short = mapOf(
                "ListView" to listOf(listView),
                "Category" to listOf(category, "example.com/app/directory.Category", "example.com/app/publishing.Category"),
            ),
        )
        val list = TemplateContract(roots = emptyMap(), dot = listView)
        val text = """{{if .Categories}}
            <a href="{{.AllURL}}" {{if not .Category}}aria-current="true"{{end}}>All</a>
            {{range .Categories}}
                <a href="{{.URL}}" {{if .Active}}aria-current="true"{{end}}>{{.Name}} ({{.Count}})</a>
            {{end}}
        {{end}}"""

        for (field in listOf(".URL", ".Active", ".Name", ".Count")) {
            val owner = GoClueTemplateContext.fieldReferenceAt(text, text.indexOf(field) + 1, shared, list)?.ownerTypeName
            assertEquals(category, owner, field)
        }

        val pointer = GoClueTemplateContext.fieldReferenceAt("{{.Category.Name}}", 12, shared, list)?.ownerTypeName
        assertEquals(category, pointer)
    }

    private fun ownerAt(text: String, offset: Int): String? {
        return GoClueTemplateContext.fieldReferenceAt(text, offset, index, contract)?.ownerTypeName
    }

    private fun type(
        fqName: String,
        name: String,
        vararg fields: Pair<String, String>,
        pkg: String = "example.com/app",
    ): GoClueType {
        return GoClueType(
            fqName = fqName,
            name = name,
            pkg = pkg,
            file = "app.go",
            line = 1,
            column = 1,
            doc = "",
            fields = fields.associate { (field, typ) -> field to GoClueField(field, typ, "", "app.go", 1, 1) },
            methods = emptyMap(),
        )
    }
}
