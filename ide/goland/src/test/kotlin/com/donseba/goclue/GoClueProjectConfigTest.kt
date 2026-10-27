package com.donseba.goclue

import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class GoClueProjectConfigTest {
    @get:Rule
    val temporary = TemporaryFolder()

    @Test
    fun `module without configuration is enabled and does not set writeIndex`() {
        val root = temporary.newFolder("module")

        assertTrue(GoClueProjectConfig.enabled(root))
        assertNull(GoClueProjectConfig.writeIndex(root))
        assertEquals(File(root, ".go-clue/config.json"), GoClueProjectConfig.file(root))
    }

    @Test
    fun `legacy go-doc configuration is read when go-clue has none`() {
        val root = temporary.newFolder("legacy")
        config(root, ".go-doc", """{"enabled": false, "writeIndex": true}""")

        assertEquals(File(root, ".go-doc/config.json"), GoClueProjectConfig.file(root))
        assertFalse(GoClueProjectConfig.enabled(root))
        assertEquals(true, GoClueProjectConfig.writeIndex(root))
    }

    @Test
    fun `go-clue configuration wins over the legacy one and is not merged with it`() {
        val root = temporary.newFolder("both")
        config(root, ".go-doc", """{"enabled": false, "writeIndex": true}""")
        config(root, ".go-clue", """{"exclude": ["vendor"]}""")

        assertEquals(File(root, ".go-clue/config.json"), GoClueProjectConfig.file(root))
        assertTrue(GoClueProjectConfig.enabled(root))
        assertNull(GoClueProjectConfig.writeIndex(root))

        config(root, ".go-clue", """{"writeIndex": false}""")
        assertEquals(false, GoClueProjectConfig.writeIndex(root))
    }

    private fun config(root: File, dir: String, text: String) {
        File(root, "$dir/config.json").apply {
            parentFile.mkdirs()
            writeText(text)
        }
    }
}
