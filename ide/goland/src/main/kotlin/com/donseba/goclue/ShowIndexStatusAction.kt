package com.donseba.goclue

import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.ui.Messages
import java.io.File

class ShowIndexStatusAction : AnAction() {
    override fun actionPerformed(event: AnActionEvent) {
        val project = event.project ?: return
        val filePath = event.getData(com.intellij.openapi.actionSystem.CommonDataKeys.VIRTUAL_FILE)?.path
        val index = GoClueIndex.load(project, filePath)
        val source = index.source ?: "no optional .go-clue/index.json file"
        val basePath = goClueReadAction { project.basePath }
        val relative = relativePath(basePath, filePath)
        val contract = index.contractForFile(project, filePath)
        val root = GoClueIndexer.moduleRoot(project, filePath)
        val installedVersion = root?.let { GoClueIndexer.commandVersion("go-clue", it) } ?: "-"
        val shadowIndex = root?.let { GoClueIndexer.shadowIndexFile(it) }
        val content = listOf(
            "Index source: $source",
            "Shadow index: ${shadowIndex?.path ?: "-"}",
            "Shadow exists: ${shadowIndex?.isFile ?: false}",
            "Enabled: ${root?.let { GoClueIndexer.enabled(project, it) } ?: "-"}",
            "Write project index: ${root?.let { GoClueIndexer.autoIndexEnabled(project, it) } ?: "-"}",
            "Index root: ${index.rootPath ?: "-"}",
            "Project: ${basePath ?: "-"}",
            "File: ${relative ?: "-"}",
            "Installed version: $installedVersion",
            "Installed executable: ${root?.let { GoClueIndexer.goClueExecutable(it) } ?: "-"}",
            "Module root: ${root?.path ?: "no Go module selected"}",
            "LSP executable: ${GoClueIndexer.lastLspExecutable ?: "-"}",
            "LSP root: ${GoClueIndexer.lastLspRoot ?: "-"}",
            "LSP version: ${GoClueIndexer.lastLspVersion ?: "-"}",
            "Contract: ${if (contract == null) "not matched" else "matched"}",
            "Templates: ${index.templates.size}",
            "Types: ${index.types.size}",
            "Error: ${index.loadError ?: "-"}",
            "Checked:",
            index.checkedPaths.take(12).joinToString("\n") { "  $it" },
        ).joinToString("\n")

        Messages.showInfoMessage(project, content, "go-clue Status")
    }

    private fun relativePath(basePath: String?, filePath: String?): String? {
        if (basePath == null || filePath == null) return filePath
        return try {
            File(basePath).toPath().relativize(File(filePath).toPath()).toString()
        } catch (_: Exception) {
            filePath
        }
    }
}
