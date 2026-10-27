package com.donseba.goclue

import org.junit.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class GoClueIndexVersionTest {
    private val source = "/project/.go-clue/index.json"

    @Test
    fun `indexes of version 2 and 3 are read`() {
        for (version in listOf(2, 3)) {
            val index = GoClueIndex.parse(indexJson("\"version\": $version,"), source, "/project", emptyList())

            assertNull(index.loadError, "version $version")
            assertEquals(setOf("example.com/app.Page"), index.types.keys, "version $version")
            assertEquals("example.com/app.Page", index.funcs["example.com/app.CurrentPage"]?.result)
        }
    }

    @Test
    fun `index of a newer version is not read and says to update the plugin`() {
        val index = GoClueIndex.parse(indexJson("\"version\": 4,"), source, "/project", emptyList())

        assertTrue(index.types.isEmpty())
        assertTrue(index.funcs.isEmpty())
        assertEquals(source, index.source)
        assertContains(index.loadError.orEmpty(), "has version 4")
        assertContains(index.loadError.orEmpty(), "reads versions 2 to 3")
        assertContains(index.loadError.orEmpty(), "update the go-clue plugin")
    }

    @Test
    fun `index without a version or of an older version is not read`() {
        val missing = GoClueIndex.parse(indexJson(""), source, "/project", emptyList())
        assertTrue(missing.types.isEmpty())
        assertContains(missing.loadError.orEmpty(), "has no version")

        val older = GoClueIndex.parse(indexJson("\"version\": 1,"), source, "/project", emptyList())
        assertTrue(older.types.isEmpty())
        assertContains(older.loadError.orEmpty(), "current go-clue CLI")
    }

    private fun indexJson(version: String): String = """
        {
          $version
          "module": "example.com/app",
          "types": {
            "example.com/app.Page": {"name": "Page", "package": "example.com/app", "fields": {"Title": {"type": "string"}}}
          },
          "funcs": {
            "example.com/app.CurrentPage": {"name": "CurrentPage", "package": "example.com/app", "result": "example.com/app.Page"}
          },
          "templates": {},
          "short": {"Page": ["example.com/app.Page"]}
        }
    """.trimIndent()
}
