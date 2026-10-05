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

        // Only the project's own modules are indexed up front. Other modules,
        // such as the module of a dependency opened in an editor tab, are
        // indexed on demand when one of their templates is used.
        val roots = GoClueIndexer.projectModuleRoots(project).filter { GoClueIndexer.enabled(project, it) }
        if (roots.isNotEmpty()) startIndex(project, roots)
    }

    private fun startIndex(project: Project, roots: List<File>) {
        object : Task.Backgroundable(project, "Building go-clue index", false) {
            override fun run(indicator: ProgressIndicator) {
                val built = mutableListOf<String>()
                for (root in roots) {
                    indicator.text = "Running go-clue index in ${root.name}"
                    val outFile = GoClueIndexer.indexTarget(project, root)
                    if (GoClueIndexer.autoIndexEnabled(project, root) && outFile.isFile) continue
                    outFile.parentFile.mkdirs()
                    val result = GoClueIndexer.run(root, outFile)
                    if (result.exitCode != 0) {
                        if (result.missingGoClue) {
                            GoClueCliInstaller.offerInstallAndIndex(project, root, outFile, indexMessage(root, outFile))
                            return
                        }
                        notify(project, "go-clue index not built", result.stderr.ifBlank { result.stdout }, NotificationType.WARNING)
                        continue
                    }
                    if (outFile.isFile) {
                        val message = indexMessage(root, outFile)
                        built.add(if (roots.size == 1) message else "${root.name}: $message")
                    }
                }
                if (built.isEmpty()) return

                // One notification for the whole startup build.
                ApplicationManager.getApplication().invokeLater {
                    GoClueIndex.refreshVirtualIndex(project)
                    GoClueEditorRefresh.refresh(project)
                    notify(project, "go-clue index built", built.joinToString("\n"), NotificationType.INFORMATION)
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
