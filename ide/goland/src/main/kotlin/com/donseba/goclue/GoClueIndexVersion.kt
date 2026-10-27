package com.donseba.goclue

import com.google.gson.JsonObject
import com.intellij.openapi.diagnostic.Logger

// GoClueIndexVersion knows the formats of the index.json files the plugin
// reads. go-clue writes the format's version into every index: version 2
// stores the results of functions relative to their package ("User"),
// version 3 fully qualified ("example.com/app.User"). The plugin reads both.
// An index of another version is not read at all, so it is not misread.
internal object GoClueIndexVersion {
    const val OLDEST = 2
    const val NEWEST = 3

    private val log = Logger.getInstance(GoClueIndexVersion::class.java)

    // version returns the version of the parsed index, or null when it has none.
    fun version(index: JsonObject?): Int? {
        val value = index?.get("version") ?: return null
        if (!value.isJsonPrimitive || !value.asJsonPrimitive.isNumber) return null

        return value.asInt
    }

    fun isSupported(version: Int?): Boolean = version != null && version in OLDEST..NEWEST

    // problem describes why the index at source, of the given version, is not
    // read, or returns null when the plugin reads it.
    fun problem(version: Int?, source: String): String? {
        if (isSupported(version)) return null

        val found = version?.let { "version $it" } ?: "no version"
        val advice = if (version != null && version > NEWEST) {
            "It was written by a newer go-clue; update the go-clue plugin."
        } else {
            "Write it again with a current go-clue CLI."
        }

        return "go-clue index $source has $found; this plugin reads versions $OLDEST to $NEWEST. $advice"
    }

    // check returns the problem of the parsed index at source, and writes it to
    // the IDE log, or returns null when the plugin reads the index.
    fun check(index: JsonObject?, source: String): String? {
        val problem = problem(version(index), source) ?: return null
        log.warn(problem)

        return problem
    }
}
