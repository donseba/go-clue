package com.donseba.goclue

import java.io.File
import java.util.concurrent.TimeUnit

internal class GoClueRuntime(
    private val environment: Map<String, String> = System.getenv(),
    private val userHome: String = System.getProperty("user.home"),
    osName: String = System.getProperty("os.name"),
) {
    val isWindows = osName.startsWith("Windows", ignoreCase = true)

    fun findModuleRoot(path: String?): File? {
        if (path == null) return null
        var dir = File(path).absoluteFile.let { if (it.isDirectory) it else it.parentFile }
        while (dir != null) {
            if (File(dir, "go.mod").isFile) return dir
            dir = dir.parentFile
        }
        return null
    }

    // isDependencySource reports whether a module root is read-only dependency
    // source: a module in the module cache (<GOMODCACHE>/<module>@<version>) or
    // the standard library in GOROOT. Its templates are not the user's to check.
    fun isDependencySource(root: File): Boolean {
        val path = root.absoluteFile
        env("GOMODCACHE")?.let { cache ->
            if (path.startsWith(File(cache).absoluteFile)) return true
        }
        var sawVersion = false
        var dir: File? = path
        while (dir != null) {
            if ('@' in dir.name) sawVersion = true
            if (sawVersion && dir.name == "mod" && dir.parentFile?.name == "pkg") return true
            dir = dir.parentFile
        }
        val module = runCatching {
            File(path, "go.mod").useLines { lines -> lines.firstOrNull { it.startsWith("module ") } }
        }.getOrNull()
        return module?.removePrefix("module ")?.trim() in setOf("std", "cmd")
    }

    fun findExecutable(root: File, executable: String): File? {
        val names = if (isWindows) listOf("$executable.exe", executable) else listOf(executable)
        val dirs = buildList {
            addAll(pathDirectories())
            if (executable == "go") {
                env("GOROOT")?.let { add(File(it, "bin").path) }
                if (isWindows) {
                    listOf("ProgramFiles", "ProgramFiles(x86)").forEach { key ->
                        env(key)?.let { add(File(it, "Go/bin").path) }
                    }
                } else {
                    addAll(listOf("/usr/local/go/bin", "/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"))
                }
            } else if (executable == "go-clue") {
                env("GOBIN")?.let { add(it) }
                env("GOPATH")?.let { addAll(goPathBins(it)) }
            }
        }
        findInDirectories(dirs, names)?.let { return it }
        if (executable != "go-clue") return null
        val fallback = buildList {
            goEnv(root, "GOBIN")?.let { add(it) }
            goEnv(root, "GOPATH")?.let { addAll(goPathBins(it)) }
            add(File(userHome, "go/bin").path)
            if (!isWindows) addAll(listOf("/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin"))
        }
        return findInDirectories(fallback, names)
    }

    private fun findInDirectories(dirs: List<String>, names: List<String>): File? =
        dirs.distinct().asSequence()
            .flatMap { dir -> names.asSequence().map { File(dir, it).absoluteFile } }
            .firstOrNull { it.isFile && it.canExecute() }

    fun commandEnvironment(root: File): Map<String, String> {
        val goBin = findExecutable(root, "go")?.parent
        val dirs = listOfNotNull(goBin) + pathDirectories()
        return mapOf("PATH" to dirs.distinct().joinToString(File.pathSeparator))
    }

    private fun pathDirectories(): List<String> = env("PATH")
        ?.split(File.pathSeparator)
        ?.filter { it.isNotBlank() && File(it).isAbsolute }
        .orEmpty()

    private fun goPathBins(value: String): List<String> = value.split(File.pathSeparator)
        .filter { it.isNotBlank() }
        .map { File(it, "bin").path }

    private fun env(name: String): String? = environment.entries
        .firstOrNull { it.key.equals(name, ignoreCase = isWindows) }
        ?.value?.takeIf { it.isNotBlank() }

    private fun goEnv(root: File, name: String): String? {
        val executable = findExecutable(root, "go") ?: return null
        return runCatching {
            val process = ProcessBuilder(executable.path, "env", name)
                .directory(root)
                .redirectErrorStream(true)
                .apply { environment().putAll(commandEnvironment(root)) }
                .start()
            if (!process.waitFor(5, TimeUnit.SECONDS)) {
                process.destroyForcibly()
                return@runCatching null
            }
            process.inputStream.bufferedReader().use { it.readText().trim() }
                .takeIf { process.exitValue() == 0 && it.isNotBlank() }
        }.getOrNull()
    }
}
