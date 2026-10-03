package com.donseba.goclue

import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.roots.ProjectRootManager
import com.intellij.openapi.vfs.LocalFileSystem
import java.io.File
import java.security.MessageDigest
import java.util.concurrent.ConcurrentHashMap
import java.util.regex.Pattern
import java.util.concurrent.TimeUnit

object GoClueIndexer {
    private val runtime = GoClueRuntime()
    private val pendingShadowBuilds = ConcurrentHashMap.newKeySet<String>()

    @Volatile
    private var cachedGoRoot: String? = null

    @Volatile
    var lastLspExecutable: String? = null
        private set

    @Volatile
    var lastLspVersion: String? = null
        private set

    @Volatile
    var lastLspRoot: String? = null
        private set

    fun findModuleRoot(filePath: String?): File? {
        return runtime.findModuleRoot(filePath)
    }

    fun moduleRoots(project: Project): List<File> {
        val paths = goClueReadAction {
            FileEditorManager.getInstance(project).openFiles.map { it.path } +
                ProjectRootManager.getInstance(project).contentRoots.map { it.path } +
                listOfNotNull(project.basePath)
        }
        return paths.mapNotNull(::findModuleRoot).distinctBy { it.canonicalPath }
    }

    fun moduleRoot(project: Project, filePath: String? = null): File? {
        findModuleRoot(filePath)?.let { return it }
        val selected = goClueReadAction { FileEditorManager.getInstance(project).selectedFiles.map { it.path } }
        return selected.firstNotNullOfOrNull(::findModuleRoot) ?: moduleRoots(project).singleOrNull()
    }

    fun commandEnvironment(root: File): Map<String, String> = runtime.commandEnvironment(root)

    fun run(root: File, outFile: File): ProcessResult {
        val commands = executableCommands(root, "go-clue", "index", "-o", outFile.path, ".")
        return runCommands(root, commands, 60, "go-clue index timed out after 60 seconds")
    }

    fun runStdout(root: File): ProcessResult {
        val commands = executableCommands(root, "go-clue", "index")
        return runCommands(root, commands, 60, "go-clue index timed out after 60 seconds")
    }

    private fun runCommands(
        root: File,
        commands: List<List<String>>,
        timeoutSeconds: Long,
        timeoutMessage: String,
    ): ProcessResult {
        var lastError = ""
        var executableMissing = false
        for (command in commands) {
            try {
                val process = ProcessBuilder(command)
                    .directory(root)
                    .redirectErrorStream(false)
                    .apply { environment().putAll(commandEnvironment(root)) }
                    .start()
                val finished = process.waitFor(timeoutSeconds, TimeUnit.SECONDS)
                if (!finished) {
                    process.destroyForcibly()
                    return ProcessResult(1, "", timeoutMessage)
                }
                val stdout = process.inputStream.bufferedReader().readText()
                val stderr = process.errorStream.bufferedReader().readText()
                return ProcessResult(process.exitValue(), stdout, stderr)
            } catch (err: Exception) {
                executableMissing = true
                lastError = err.message ?: err.javaClass.simpleName
            }
        }

        return ProcessResult(
            1,
            "",
            "Could not find go-clue. Install it with: go install github.com/donseba/go-clue@latest\n$lastError",
            missingGoClue = executableMissing,
        )
    }

    fun enabled(project: Project, root: File): Boolean {
        if (!GoClueSettings.getInstance(project).state.enabled) return false
        return projectConfigEnabled(root)
    }

    fun autoIndexEnabled(project: Project, root: File): Boolean {
        if (!enabled(project, root)) return false
        projectConfigIndexValue(root)?.let { return it }
        return GoClueSettings.getInstance(project).state.autoIndex
    }

    fun indexTarget(project: Project, root: File): File {
        return if (autoIndexEnabled(project, root)) {
            File(root, ".go-clue/index.json")
        } else {
            shadowIndexFile(root)
        }
    }

    fun shadowIndexFile(root: File): File {
        val digest = MessageDigest.getInstance("SHA-1")
            .digest(root.canonicalPath.toByteArray(Charsets.UTF_8))
            .joinToString("") { "%02x".format(it) }
        val base = File(System.getProperty("java.io.tmpdir"), "go-clue-goland-index")
        return File(File(base, digest), "index.json")
    }

    fun requestShadowIndex(project: Project, root: File) {
        if (!enabled(project, root) || autoIndexEnabled(project, root)) return
        val outFile = shadowIndexFile(root)
        val key = outFile.canonicalPath
        if (!pendingShadowBuilds.add(key)) return

        ApplicationManager.getApplication().executeOnPooledThread {
            try {
                outFile.parentFile.mkdirs()
                val result = run(root, outFile)
                if (result.exitCode == 0) {
                    ApplicationManager.getApplication().invokeLater {
                        LocalFileSystem.getInstance().refreshAndFindFileByIoFile(outFile)
                        GoClueEditorRefresh.refresh(project)
                    }
                }
            } finally {
                pendingShadowBuilds.remove(key)
            }
        }
    }

    private fun projectConfigEnabled(root: File): Boolean {
        val config = File(root, ".go-clue/config.json")
        if (!config.isFile) return true
        val text = runCatching { config.readText() }.getOrNull() ?: return true
        return !Pattern.compile("\"enabled\"\\s*:\\s*false").matcher(text).find()
    }

    private fun projectConfigIndexValue(root: File): Boolean? {
        val config = File(root, ".go-clue/config.json")
        if (!config.isFile) return null
        val text = runCatching { config.readText() }.getOrNull() ?: return null
        if (Pattern.compile("\"writeIndex\"\\s*:\\s*true").matcher(text).find()) return true
        if (Pattern.compile("\"writeIndex\"\\s*:\\s*false").matcher(text).find()) return false
        return null
    }

    fun install(root: File): ProcessResult {
        val commands = executableCommands(root, "go", "install", "github.com/donseba/go-clue@latest")

        var lastError = ""
        var executableMissing = false
        for (command in commands) {
            try {
                val process = ProcessBuilder(command)
                    .directory(root)
                    .redirectErrorStream(false)
                    .apply { environment().putAll(commandEnvironment(root)) }
                    .start()
                val finished = process.waitFor(120, TimeUnit.SECONDS)
                if (!finished) {
                    process.destroyForcibly()
                    return ProcessResult(1, "", "go install timed out after 120 seconds")
                }
                val stdout = process.inputStream.bufferedReader().readText()
                val stderr = process.errorStream.bufferedReader().readText()
                return ProcessResult(process.exitValue(), stdout, stderr)
            } catch (err: Exception) {
                executableMissing = true
                lastError = err.message ?: err.javaClass.simpleName
            }
        }

        return ProcessResult(
            1,
            "",
            "Could not find Go. Install Go or configure GoLand so Go is available before installing go-clue.\n$lastError",
            missingGo = executableMissing,
        )
    }

    fun commandVersion(command: String, root: File): String {
        val resolvedCommand = if (File(command).isAbsolute) command else findExecutable(root, command)?.absolutePath ?: command
        return try {
            val process = ProcessBuilder(resolvedCommand, "version")
                .directory(root)
                .redirectErrorStream(true)
                .apply { environment().putAll(commandEnvironment(root)) }
                .start()
            if (!process.waitFor(5, TimeUnit.SECONDS)) {
                process.destroyForcibly()
                return "-"
            }
            process.inputStream.bufferedReader().readText().trim().ifBlank { "-" }
        } catch (_: Exception) {
            "-"
        }
    }

    fun goRoot(root: File): String? {
        cachedGoRoot?.let { return it }
        val fromEnv = System.getenv("GOROOT")?.takeIf { it.isNotBlank() }
        if (fromEnv != null) {
            cachedGoRoot = fromEnv
            return fromEnv
        }
        val commands = executableCommands(root, "go", "env", "GOROOT")
        for (command in commands) {
            try {
                val process = ProcessBuilder(command)
                    .directory(root)
                    .redirectErrorStream(true)
                    .apply { environment().putAll(commandEnvironment(root)) }
                    .start()
                if (!process.waitFor(5, TimeUnit.SECONDS)) {
                    process.destroyForcibly()
                    continue
                }
                val value = process.inputStream.bufferedReader().readText().trim()
                if (value.isNotBlank()) {
                    cachedGoRoot = value
                    return value
                }
            } catch (_: Exception) {
                continue
            }
        }
        return null
    }

    fun rememberLspExecutable(command: String, root: File) {
        lastLspExecutable = command
        lastLspRoot = root.path
        lastLspVersion = commandVersion(command, root)
    }

    fun goClueExecutable(root: File): String {
        return findExecutable(root, "go-clue")?.absolutePath ?: platformExecutableNames("go-clue").first()
    }

    private fun executableCommands(root: File, executable: String, vararg args: String): List<List<String>> {
        val resolved = runtime.findExecutable(root, executable)?.path ?: platformExecutableNames(executable).first()
        return listOf(listOf(resolved, *args))
    }

    private fun findExecutable(root: File, executable: String): File? = runtime.findExecutable(root, executable)

    private fun platformExecutableNames(executable: String): List<String> =
        if (runtime.isWindows) listOf("$executable.exe", executable) else listOf(executable)

    data class ProcessResult(
        val exitCode: Int,
        val stdout: String,
        val stderr: String,
        val missingGoClue: Boolean = false,
        val missingGo: Boolean = false,
    )
}
