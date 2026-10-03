package com.donseba.goclue

import com.intellij.codeInsight.daemon.DaemonCodeAnalyzer
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project

object GoClueEditorRefresh {
    fun refresh(project: Project) {
        ApplicationManager.getApplication().invokeLater {
            if (!project.isDisposed) {
                DaemonCodeAnalyzer.getInstance(project).restart("go-clue index refreshed")
            }
        }
    }
}
