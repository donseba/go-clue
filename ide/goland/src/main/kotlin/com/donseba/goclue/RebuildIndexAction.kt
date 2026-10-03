package com.donseba.goclue

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.CommonDataKeys
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import java.io.File

class RebuildIndexAction : AnAction() {
    override fun actionPerformed(event: AnActionEvent) {
        val project = event.project ?: return
        val selectedPath = event.getData(CommonDataKeys.VIRTUAL_FILE)?.path
        object : Task.Backgroundable(project, "Rebuilding go-clue index", false) {
            override fun run(indicator: ProgressIndicator) {
                indicator.text = "Running go-clue index"
                rebuild(project, selectedPath)
            }
        }.queue()
    }

    private fun rebuild(project: Project, selectedPath: String?) {
        val root = GoClueIndexer.findModuleRoot(selectedPath) ?: goClueReadAction { project.basePath }?.let { File(it) } ?: return
        val outDir = File(root, ".go-clue")
        val outFile = File(outDir, "index.json")
        outDir.mkdirs()

        val result = GoClueIndexer.run(root, outFile)
        if (result.exitCode != 0) {
            if (result.missingGoClue) {
                GoClueCliInstaller.offerInstallAndIndex(project, root, outFile, ".go-clue/index.json updated")
                return
            }
            notify(project, "go-clue index failed", result.stderr.ifBlank { result.stdout }, NotificationType.ERROR)
            return
        }

        ApplicationManager.getApplication().invokeLater {
            GoClueIndex.refreshVirtualIndex(project)
            GoClueEditorRefresh.refresh(project)
            if (outFile.isFile) {
                notify(project, "go-clue index rebuilt", ".go-clue/index.json updated", NotificationType.INFORMATION)
            } else {
                notify(project, "go-clue index not needed", "No template contracts found; index not written.", NotificationType.INFORMATION)
            }
        }
    }

    private fun notify(project: Project, title: String, content: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("go-clue")
            .createNotification(title, content.take(800), type)
            .notify(project)
    }
}
