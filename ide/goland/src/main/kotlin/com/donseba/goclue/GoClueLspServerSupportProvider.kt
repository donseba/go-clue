@file:Suppress("DEPRECATION")

package com.donseba.goclue

import com.intellij.execution.configurations.GeneralCommandLine
import com.intellij.openapi.editor.colors.TextAttributesKey
import com.intellij.openapi.editor.markup.TextAttributes
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.platform.lsp.api.LspServerSupportProvider
import com.intellij.platform.lsp.api.ProjectWideLspServerDescriptor
import com.intellij.platform.lsp.api.customization.LspCustomization
import com.intellij.platform.lsp.api.customization.LspSemanticTokensSupport
import com.intellij.psi.PsiFile
import com.intellij.ui.JBColor
import java.awt.Font
import java.io.File
import java.nio.file.Files
import java.nio.file.StandardCopyOption

internal class GoClueLspServerSupportProvider : LspServerSupportProvider {
    override fun fileOpened(
        project: Project,
        file: VirtualFile,
        serverStarter: LspServerSupportProvider.LspServerStarter,
    ) {
        val root = GoClueIndexer.findModuleRoot(file.path) ?: goClueReadAction { project.basePath }?.let { java.io.File(it) }
        if (isSupportedTemplate(file) && (root == null || GoClueIndexer.enabled(project, root))) {
            serverStarter.ensureServerStarted(GoClueLspServerDescriptor(project))
        }
    }
}

private class GoClueLspServerDescriptor(project: Project) : ProjectWideLspServerDescriptor(project, "go-clue") {
    override val lspCustomization = object : LspCustomization() {
        override val semanticTokensCustomizer = object : LspSemanticTokensSupport() {
            override val tokenTypes: List<String> = listOf("variable", "property", "type", "function")
            override val tokenModifiers: List<String> = emptyList()

            override fun shouldAskServerForSemanticTokens(psiFile: PsiFile): Boolean {
                return goClueReadAction { isSupportedTemplate(psiFile.virtualFile) }
            }

            override fun getTextAttributesKey(
                tokenType: String,
                modifiers: List<String>,
            ): TextAttributesKey? {
                return when (tokenType) {
                    "variable" -> GO_CLUE_ACCESSOR
                    "property" -> GO_CLUE_FIELD
                    "type" -> GO_CLUE_TYPE
                    "function" -> GO_CLUE_FUNCTION
                    else -> null
                }
            }
        }
    }

    override fun isSupportedFile(file: VirtualFile): Boolean {
        val root = GoClueIndexer.findModuleRoot(file.path) ?: goClueReadAction { project.basePath }?.let { java.io.File(it) }
        return isSupportedTemplate(file) && (root == null || GoClueIndexer.enabled(project, root))
    }

    override fun createCommandLine(): GeneralCommandLine {
        val root = goClueReadAction { project.basePath } ?: "."
        val executable = goClueLspExecutable(root)
        GoClueIndexer.rememberLspExecutable(executable, File(root))
        return GeneralCommandLine(executable, "lsp", root).withWorkDirectory(root)
    }
}

internal fun isSupportedTemplate(file: VirtualFile): Boolean {
    return file.extension in setOf("gohtml", "tmpl", "html")
}

private val GO_CLUE_ACCESSOR = clickableKey("GO_CLUE_ACCESSOR", JBColor(0xC586C0, 0xC586C0))
private val GO_CLUE_FIELD = clickableKey("GO_CLUE_FIELD", JBColor(0x9CDCFE, 0x9CDCFE))
private val GO_CLUE_TYPE = clickableKey("GO_CLUE_TYPE", JBColor(0x4EC9B0, 0x4EC9B0))
private val GO_CLUE_FUNCTION = clickableKey("GO_CLUE_FUNCTION", JBColor(0xDCDCAA, 0xDCDCAA))

@Suppress("DEPRECATION")
private fun clickableKey(name: String, color: JBColor): TextAttributesKey {
    return TextAttributesKey.createTextAttributesKey(
        name,
        TextAttributes(color, null, null, null, Font.PLAIN),
    )
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
    return System.getProperty("os.name").lowercase().contains("win")
}
