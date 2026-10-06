package com.donseba.goclue

import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.progress.ProgressIndicator
import com.intellij.openapi.progress.Task
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import java.io.File

object GoClueCliInstaller {
    fun offerInstallAndIndex(project: Project, root: File, outFile: File, successMessage: String) {
        ApplicationManager.getApplication().invokeLater {
            val answer = Messages.showYesNoDialog(
                project,
                "go-clue was not found by GoLand.\n\nInstall it now with:\n\ngo install github.com/donseba/go-clue@latest",
                "Install go-clue CLI",
                "Install",
                "Cancel",
                Messages.getQuestionIcon(),
            )
            if (answer != Messages.YES) return@invokeLater

            object : Task.Backgroundable(project, "Installing go-clue CLI", false) {
                override fun run(indicator: ProgressIndicator) {
                    indicator.text = "Running go install github.com/donseba/go-clue@latest"
                    val install = GoClueIndexer.install(root)
                    if (install.exitCode != 0) {
                        notify(project, "go-clue install failed", install.stderr.ifBlank { install.stdout }, NotificationType.ERROR)
                        return
                    }

                    indicator.text = "Running go-clue index"
                    val result = GoClueIndexer.buildIndex(root, outFile)
                    if (result.exitCode != 0) {
                        notify(project, "go-clue index failed", result.stderr.ifBlank { result.stdout }, NotificationType.ERROR)
                        return
                    }

                    ApplicationManager.getApplication().invokeLater {
                        GoClueIndex.refreshVirtualIndex(project)
                        GoClueEditorRefresh.refresh(project)
                        if (outFile.isFile) {
                            notify(project, "go-clue index rebuilt", successMessage, NotificationType.INFORMATION)
                        } else {
                            notify(project, "go-clue index not needed", "No template contracts found; index not written.", NotificationType.INFORMATION)
                        }
                    }
                }
            }.queue()
        }
    }

    private fun notify(project: Project, title: String, content: String, type: NotificationType) {
        NotificationGroupManager.getInstance()
            .getNotificationGroup("go-clue")
            .createNotification(title, content.take(800), type)
            .notify(project)
    }
}
