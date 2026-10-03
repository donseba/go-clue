package com.donseba.goclue

import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.Storage
import com.intellij.openapi.components.service
import com.intellij.openapi.project.Project
import com.intellij.openapi.components.State as ComponentState

@Service(Service.Level.PROJECT)
@ComponentState(name = "GoClueSettings", storages = [Storage("go-clue.xml")])
class GoClueSettings : PersistentStateComponent<GoClueSettings.SettingsState> {
    private var currentState = SettingsState()

    override fun getState(): SettingsState = currentState

    override fun loadState(state: SettingsState) {
        currentState = state
    }

    data class SettingsState(
        var enabled: Boolean = true,
        var autoIndex: Boolean = false,
    )

    companion object {
        fun getInstance(project: Project): GoClueSettings = project.service()
    }
}
