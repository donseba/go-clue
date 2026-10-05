@file:Suppress("DEPRECATION")

package com.donseba.goclue

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.platform.lsp.api.LspServerSupportProvider
import com.intellij.platform.lsp.api.LspServerDescriptor
import com.intellij.platform.lsp.api.customization.LspCustomization
import com.intellij.platform.lsp.api.customization.LspSemanticTokensSupport
import com.intellij.psi.PsiFile
import java.io.File
import java.nio.file.Files
import java.nio.file.StandardCopyOption

internal class GoClueLspServerSupportProvider : LspServerSupportProvider {
    override fun fileOpened(
        project: Project,
        file: VirtualFile,
        serverStarter: LspServerSupportProvider.LspServerStarter,
    ) {
        if (!isSupportedTemplate(file)) return
        val root = GoClueIndexer.findModuleRoot(file.path) ?: return
        if (!GoClueIndexer.enabled(project, root)) return
        val virtualRoot = goClueReadAction { LocalFileSystem.getInstance().findFileByPath(root.path) } ?: return
        serverStarter.ensureServerStarted(GoClueLspServerDescriptor(project, virtualRoot))
        GoClueIndexer.requestShadowIndex(project, root)
    }
}

private class GoClueLspServerDescriptor(project: Project, moduleRoot: VirtualFile) :
    LspServerDescriptor(project, "go-clue", moduleRoot) {
    private val root = File(moduleRoot.path)
    override val lspCustomization = object : LspCustomization() {
        override val semanticTokensCustomizer = object : LspSemanticTokensSupport() {
            override val tokenTypes: List<String> = GoClueHighlighting.tokenTypes
            override val tokenModifiers: List<String> = GoClueHighlighting.tokenModifiers

            override fun shouldAskServerForSemanticTokens(psiFile: PsiFile): Boolean {
                return goClueReadAction { isSupportedTemplate(psiFile.virtualFile) }
            }

            override fun getTextAttributesKey(
                tokenType: String,
                modifiers: List<String>,
            ): TextAttributesKey? {
                return GoClueHighlighting.keyFor(tokenType, modifiers)
            }
        }
    }

    override fun isSupportedFile(file: VirtualFile): Boolean {
        return isSupportedTemplate(file) &&
            GoClueIndexer.findModuleRoot(file.path)?.canonicalFile == root.canonicalFile &&
            GoClueIndexer.enabled(project, root)
    }

    override fun createCommandLine(): GeneralCommandLine {
        val executable = goClueLspExecutable(root.path)
        GoClueIndexer.rememberLspExecutable(executable, root)
        return GeneralCommandLine(executable, "lsp", root.path)
            .withWorkDirectory(root)
            .withEnvironment(GoClueIndexer.commandEnvironment(root))
    }
}

internal fun isSupportedTemplate(file: VirtualFile): Boolean {
    return file.extension in setOf("gohtml", "tmpl", "html")
}

private fun goClueLspExecutable(root: String): String {
    val executable = GoClueIndexer.goClueExecutable(File(root))
    if (!isWindows()) return executable

    val installed = File(executable).takeIf { it.isAbsolute && it.isFile } ?: return executable

    return try {
        val cacheDir = File(System.getProperty("java.io.tmpdir"), "go-clue-goland-lsp")
        cacheDir.mkdirs()
        cleanupOldLspCopies(cacheDir)
        val copy = File(cacheDir, "go-clue-lsp-${ProcessHandle.current().pid()}-${System.currentTimeMillis()}.exe")
        Files.copy(installed.toPath(), copy.toPath(), StandardCopyOption.REPLACE_EXISTING)
        copy.absolutePath
    } catch (_: Exception) {
        installed.absolutePath
    }
}

private fun cleanupOldLspCopies(cacheDir: File) {
    cacheDir.listFiles { file -> file.name.matches(Regex("""go-clue-lsp-\d+-\d+\.exe""")) }
        ?.forEach { file ->
            runCatching { file.delete() }
        }
}

private fun isWindows(): Boolean {
    return System.getProperty("os.name").startsWith("Windows", ignoreCase = true)
}
