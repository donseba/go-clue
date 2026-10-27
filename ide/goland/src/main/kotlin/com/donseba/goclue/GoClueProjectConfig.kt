package com.donseba.goclue

import java.io.File

// GoClueProjectConfig reads the settings of a module's go-clue configuration
// that the plugin acts on itself. Like the go-clue CLI and language server, it
// reads .go-clue/config.json, or the .go-doc/config.json of go-doc when that
// file does not exist. The two files are never merged.
internal object GoClueProjectConfig {
    private val enabledFalse = Regex("\"enabled\"\\s*:\\s*false")
    private val writeIndexTrue = Regex("\"writeIndex\"\\s*:\\s*true")
    private val writeIndexFalse = Regex("\"writeIndex\"\\s*:\\s*false")

    // file returns the configuration file of the module at root, which need
    // not exist.
    fun file(root: File): File {
        val current = File(root, ".go-clue/config.json")
        if (current.exists()) return current

        val legacy = File(root, ".go-doc/config.json")
        if (legacy.exists()) return legacy

        return current
    }

    // enabled reports whether the configuration leaves go-clue on.
    fun enabled(root: File): Boolean {
        val text = text(root) ?: return true

        return !enabledFalse.containsMatchIn(text)
    }

    // writeIndex returns the configured writeIndex, or null when it is not set.
    fun writeIndex(root: File): Boolean? {
        val text = text(root) ?: return null
        if (writeIndexTrue.containsMatchIn(text)) return true
        if (writeIndexFalse.containsMatchIn(text)) return false

        return null
    }

    private fun text(root: File): String? {
        val config = file(root)
        if (!config.isFile) return null

        return runCatching { config.readText() }.getOrNull()
    }
}
