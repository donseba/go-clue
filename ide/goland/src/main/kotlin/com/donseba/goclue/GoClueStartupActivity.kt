package com.donseba.goclue

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.startup.ProjectActivity
import java.io.File

class GoClueStartupActivity : ProjectActivity {
    override suspend fun execute(project: Project) {
        GoClueIndexWatcher.install(project)

        GoClueIndexer.moduleRoots(project).forEach { root -> startIndex(project, root) }
    }

    private fun startIndex(project: Project, root: File) {
        if (!GoClueIndexer.enabled(project, root)) return

        object : Task.Backgroundable(project, "Building go-clue index", false) {
            override fun run(indicator: ProgressIndicator) {
                indicator.text = "Running go-clue index"
                val outFile = GoClueIndexer.indexTarget(project, root)
                if (GoClueIndexer.autoIndexEnabled(project, root) && outFile.isFile) return
                outFile.parentFile.mkdirs()
                val result = GoClueIndexer.run(root, outFile)
                if (result.exitCode != 0) {
                    if (result.missingGoClue) {
                        GoClueCliInstaller.offerInstallAndIndex(project, root, outFile, indexMessage(root, outFile))
                        return
                    }
                    notify(project, "go-clue index not built", result.stderr.ifBlank { result.stdout }, NotificationType.WARNING)
                    return
                }
                ApplicationManager.getApplication().invokeLater {
                    GoClueIndex.refreshVirtualIndex(project)
                    GoClueEditorRefresh.refresh(project)
                    if (outFile.isFile) {
                        notify(project, "go-clue index built", indexMessage(root, outFile), NotificationType.INFORMATION)
                    }
                }
            }
        }.queue()
    }

    private fun indexMessage(root: File, outFile: File): String {
        return if (outFile.path.startsWith(root.path)) ".go-clue/index.json created" else "go-clue shadow index created"
    }

    private fun notify(project: Project, title: String, content: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("go-clue")
            .createNotification(title, content.take(800), type)
            .notify(project)
    }
}
