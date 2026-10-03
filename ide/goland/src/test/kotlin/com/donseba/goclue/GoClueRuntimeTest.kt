package com.donseba.goclue

import java.io.File
import org.junit.Assume.assumeFalse
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class GoClueRuntimeTest {
    @get:Rule
    val temporary = TemporaryFolder()

    @Test
    fun `template uses its nearest module instead of the outer project`() {
        val project = temporary.newFolder("project with spaces")
        File(project, "go.mod").writeText("module example.com/outer\n")
        val module = File(project, "services/web").apply { mkdirs() }
        File(module, "go.mod").writeText("module example.com/web\n")
        val template = File(module, "templates/page.gohtml").apply {
            parentFile.mkdirs()
            writeText("{{ .Title }}")
        }
        val runtime = GoClueRuntime()
        assertEquals(module, runtime.findModuleRoot(template.path))
        assertEquals(module, runtime.findModuleRoot(template.parent))
        assertEquals(project, runtime.findModuleRoot(project.path))
        assertNull(runtime.findModuleRoot(temporary.newFolder("no-module").path))
    }

    @Test
    fun `global executable on PATH takes precedence over GOBIN`() {
        val global = temporary.newFolder("global tools")
        val gobin = temporary.newFolder("custom gobin")
        val expected = executable(global, "go-clue")
        executable(gobin, "go-clue")
        val runtime = GoClueRuntime(mapOf("PATH" to global.path, "GOBIN" to gobin.path))
        assertEquals(expected, runtime.findExecutable(temporary.root, "go-clue"))
    }

    @Test
    fun `GUI launch resolves configured GOBIN and default home Go bin`() {
        val gobin = temporary.newFolder("gobin with spaces")
        val expected = executable(gobin, "go-clue")
        val runtime = GoClueRuntime(mapOf("PATH" to "/usr/bin:/bin", "GOBIN" to gobin.path))
        assertEquals(expected, runtime.findExecutable(temporary.root, "go-clue"))

        val home = temporary.newFolder("home")
        val homeBin = File(home, "go/bin").apply { mkdirs() }
        val homeExecutable = executable(homeBin, "go-clue")
        val sdk = temporary.newFolder("empty env SDK")
        val sdkBin = File(sdk, "bin").apply { mkdirs() }
        executable(sdkBin, "go")
        assertEquals(homeExecutable, GoClueRuntime(mapOf("GOROOT" to sdk.path), home.path).findExecutable(temporary.root, "go-clue"))
    }

    @Test
    fun `GUI child process receives the resolved Go toolchain on PATH`() {
        val sdk = temporary.newFolder("Go SDK")
        val sdkBin = File(sdk, "bin").apply { mkdirs() }
        val gobin = temporary.newFolder("go env gobin")
        executable(sdkBin, "go", "printf '%s\\n' '${gobin.path}'")
        val cli = executable(gobin, "go-clue", "command -v go")
        val guiPath = temporary.newFolder("GUI launch PATH")
        val runtime = GoClueRuntime(mapOf("PATH" to guiPath.path, "GOROOT" to sdk.path))
        assertEquals(cli, runtime.findExecutable(temporary.root, "go-clue"))
        val process = ProcessBuilder(cli.path)
            .apply { environment().putAll(runtime.commandEnvironment(temporary.root)) }
            .start()
        val output = process.inputStream.bufferedReader().use { it.readText().trim() }
        assertEquals(0, process.waitFor())
        assertEquals(File(sdkBin, "go").path, output)
    }

    @Test
    fun `Darwin and Mac OS X are never treated as Windows`() {
        assertFalse(GoClueRuntime(osName = "Darwin").isWindows)
        assertFalse(GoClueRuntime(osName = "Mac OS X").isWindows)
        assertTrue(GoClueRuntime(osName = "Windows 11").isWindows)
    }

    private fun executable(directory: File, name: String, body: String = "exit 0"): File {
        assumeFalse("Shell executable fixtures require macOS or Linux", GoClueRuntime().isWindows)
        return File(directory, name).apply {
            writeText("#!/bin/sh\n$body\n")
            check(setExecutable(true))
        }
    }
}
