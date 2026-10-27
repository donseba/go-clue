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

    @Test
    fun `examples in comments do not change the scope`() {
        val text = """{{/* Example: {{range .Menu}} lists the menu. */}}{{.Title}}"""
        assertEquals(view, ownerAt(text, text.indexOf(".Title") + 1))
    }

    @Test
    fun `fields inside comments are no references`() {
        val text = """{{/* {{.Home.Title}} */}}{{.Home.Title}}"""
        val refs = GoClueTemplateContext.fieldReferencesInRange(text, 0, text.length, index, contract)
        assertEquals(listOf(text.lastIndexOf("Home")), refs.filter { it.memberName == "Home" }.map { it.startOffset })
    }

    @Test
    fun `template actions read comments and strings like text-template`() {
        val text = """{{/* {{template "x" .Tabs}} */}}{{ printf "}}" .A }}{{- /* @dot a.B */ -}}{{ .C }}"""
        val actions = GoClueTemplateActions.actions(text).map { it.text(text) }
        assertEquals(
            listOf("""{{/* {{template "x" .Tabs}} */}}""", """{{ printf "}}" .A }}""", "{{- /* @dot a.B */ -}}", "{{ .C }}"),
            actions,
        )
        assertEquals("""{{template "x" .Tabs}}""" + "\n" + "@dot a.B\n", GoClueTemplateActions.commentBodies(text))

        val masked = GoClueTemplateActions.annotationText("<p>@dot x.Y</p>{{/* @dot a.B */}}")
        assertEquals(false, masked.contains("x.Y"))
        assertEquals(true, masked.contains("@dot a.B"))
    }

    @Test
    fun `named array types range over their elements`() {
        val directory = "example.com/app/directory"
        val entry = "$directory.Entry"
        val hours = "$directory.OpeningHours"
        val day = "$directory.Day"
        val named = GoClueIndex(
            types = mapOf(
                entry to type(entry, "Entry", "Hours" to "OpeningHours", pkg = directory),
                hours to type(hours, "OpeningHours", pkg = directory).copy(underlying = "[7]Day"),
                day to type(day, "Day", "Open" to "string", pkg = directory),
                "example.com/app/other.Day" to type("example.com/app/other.Day", "Day", pkg = "example.com/app/other"),
            ),
            funcs = emptyMap(),
            templates = emptyMap(),
            short = mapOf(
                "Entry" to listOf(entry),
                "OpeningHours" to listOf(hours),
                "Day" to listOf(day, "example.com/app/other.Day"),
            ),
        )
        val text = """{{range .Hours}}{{.Open}}{{end}}"""
        val owner = GoClueTemplateContext.fieldReferenceAt(text, text.indexOf(".Open") + 1, named, TemplateContract(roots = emptyMap(), dot = entry))
        assertEquals(day, owner?.ownerTypeName)
    }

    @Test
    fun `function results of the function's own package resolve`() {
        val users = "example.com/app/users"
        val user = "$users.User"
        val withFunc = GoClueIndex(
            types = mapOf(
                user to type(user, "User", "Name" to "string", pkg = users),
                "example.com/app/admin.User" to type("example.com/app/admin.User", "User", pkg = "example.com/app/admin"),
            ),
            funcs = mapOf(
                "$users.Current" to GoClueFunc("$users.Current", "Current", users, "users.go", 1, 1, "", "User", "func() User", emptyList()),
            ),
            templates = emptyMap(),
            short = mapOf("User" to listOf(user, "example.com/app/admin.User")),
        )
        val text = """{{ current.Name }}"""
        val ref = GoClueTemplateContext.fieldReferenceAt(text, text.indexOf("Name"), withFunc, TemplateContract(roots = emptyMap(), funcs = mapOf("current" to "$users.Current")))
        assertEquals(user, ref?.ownerTypeName)
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
