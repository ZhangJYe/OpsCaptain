import { useCallback, useEffect, useState } from 'react'
import { useTheme } from './hooks/useTheme'
import { useChat } from './hooks/useChat'
import { useIncidents } from './hooks/useIncidents'
import { MainLayout } from './components/layout/MainLayout'
import { AgentWorkbenchView } from './components/workbench/AgentWorkbenchView'
import { IncidentView } from './components/incident/IncidentView'
import { SettingsView } from './components/settings/SettingsView'
import { TopologyView } from './components/topology/TopologyView'
import { saveSession } from './lib/storage'
import type { AIOpsEngine, ChatSession, WorkbenchMode } from './types/chat'

const SKILL_STORAGE_KEY = 'opscaptain-selected-skills'
const AIOPS_ENGINE_STORAGE_KEY = 'opscaptain-aiops-engine'
const WORKBENCH_MODE_STORAGE_KEY = 'opscaptain-workbench-mode'
const PET_ENABLED_KEY = 'opscaptain-pet-enabled'

export default function App() {
  const { theme, toggle: toggleTheme } = useTheme()
  const chat = useChat()
  const incidents = useIncidents()
  const [sidebarOpen, setSidebarOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.innerWidth >= 1024
  })
  const [selectedSkillIds, setSelectedSkillIds] = useState<string[]>(() => {
    if (typeof window === 'undefined') return []
    try {
      const raw = localStorage.getItem(SKILL_STORAGE_KEY)
      const parsed = raw ? JSON.parse(raw) : []
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  })
  const [aiOpsEngine, setAIOpsEngine] = useState<AIOpsEngine>(() => {
    if (typeof window === 'undefined') return 'plan_execute_replan'
    const raw = localStorage.getItem(AIOPS_ENGINE_STORAGE_KEY)
    return raw === 'gos_engine' ? 'gos_engine' : 'plan_execute_replan'
  })
  const [workbenchMode, setWorkbenchMode] = useState<WorkbenchMode>(() => {
    if (typeof window === 'undefined') return 'aiops'
    const raw = localStorage.getItem(WORKBENCH_MODE_STORAGE_KEY)
    if (raw === 'chat' || raw === 'aiops' || raw === 'topology') return raw
    return 'aiops'
  })
  const [petEnabled, setPetEnabled] = useState<boolean>(() => {
    if (typeof window === 'undefined') return true
    const raw = localStorage.getItem(PET_ENABLED_KEY)
    return raw !== 'false'
  })
  const incidentEngine = incidents.incident?.engine_strategy === 'gos_engine' ? 'gos_engine' : 'plan_execute_replan'
  const displayedAIOpsEngine = incidents.incident && workbenchMode === 'aiops' ? incidentEngine : aiOpsEngine

  useEffect(() => {
    try {
      localStorage.setItem(SKILL_STORAGE_KEY, JSON.stringify(selectedSkillIds))
    } catch {
      return
    }
  }, [selectedSkillIds])

  useEffect(() => {
    try {
      localStorage.setItem(AIOPS_ENGINE_STORAGE_KEY, aiOpsEngine)
    } catch {
      return
    }
  }, [aiOpsEngine])

  useEffect(() => {
    try {
      localStorage.setItem(WORKBENCH_MODE_STORAGE_KEY, workbenchMode)
    } catch {
      return
    }
  }, [workbenchMode])

  useEffect(() => {
    try {
      localStorage.setItem(PET_ENABLED_KEY, String(petEnabled))
    } catch {
      return
    }
  }, [petEnabled])

  useEffect(() => {
    if (chat.messages.length === 0 || workbenchMode !== 'chat') {
      return
    }
    saveSession(chat.sessionId, chat.messages, {
      mode: chat.mode,
      workMode: 'react',
      selectedSkillIds,
    })
  }, [chat.sessionId, chat.messages, chat.mode, selectedSkillIds, workbenchMode])

  const handleSendChat = useCallback(
    (query: string) => {
      setWorkbenchMode('chat')
      chat.send(query, { selectedSkillIds })
    },
    [chat, selectedSkillIds],
  )

  const handleStartAIOps = useCallback(
    (query: string) => {
      setWorkbenchMode('aiops')
      void incidents.createIncident(query, aiOpsEngine).catch(() => undefined)
    },
    [aiOpsEngine, incidents],
  )

  const handleAIOpsEngineChange = useCallback(
    (engine: AIOpsEngine) => {
      if (incidents.incident && workbenchMode === 'aiops') {
        return
      }
      setAIOpsEngine(engine)
      setWorkbenchMode('aiops')
    },
    [incidents.incident, workbenchMode],
  )

  const handleLoadSession = useCallback(
    (session: ChatSession) => {
      const loaded = chat.loadSession(session)
      if (!loaded) {
        return
      }
      setSelectedSkillIds(Array.isArray(session.selectedSkillIds) ? session.selectedSkillIds : [])
      setWorkbenchMode('chat')
    },
    [chat],
  )

  const handleLoadIncident = useCallback(
    (incidentId: string) => {
      setWorkbenchMode('aiops')
      void incidents.loadIncident(incidentId).catch(() => undefined)
    },
    [incidents],
  )

  const handleNewChat = useCallback(() => {
    if (workbenchMode === 'aiops') {
      incidents.newIncident()
      return
    }
    chat.newSession()
  }, [chat, incidents, workbenchMode])

  return (
    <MainLayout
      theme={theme}
      sidebarOpen={sidebarOpen}
      onToggleSidebar={() => setSidebarOpen((value) => !value)}
      onCloseSidebar={() => setSidebarOpen(false)}
      onToggleTheme={toggleTheme}
      onNewChat={handleNewChat}
      onLoadSession={handleLoadSession}
      onLoadIncident={handleLoadIncident}
      chatMode={chat.mode}
      onModeChange={chat.setMode}
      workbenchMode={workbenchMode}
      onWorkbenchModeChange={setWorkbenchMode}
      aiOpsEngine={displayedAIOpsEngine}
      onAIOpsEngineChange={handleAIOpsEngineChange}
      sessionId={chat.sessionId}
      currentIncidentId={incidents.incident?.incident_id || ''}
      currentIncidentEngine={incidents.incident?.engine_strategy || ''}
      incidents={incidents.incidents}
      messages={chat.messages}
      selectedSkillIds={selectedSkillIds}
      onSelectedSkillIdsChange={setSelectedSkillIds}
      isLoading={chat.isLoading || incidents.isLoading}
    >
      {workbenchMode === 'settings' ? (
        <SettingsView onBack={() => setWorkbenchMode('chat')} />
      ) : workbenchMode === 'topology' ? (
        <TopologyView onBack={() => setWorkbenchMode('chat')} />
      ) : workbenchMode === 'aiops' ? (
        <IncidentView
          incident={incidents.incident}
          isLoading={incidents.isLoading}
          error={incidents.error}
          engine={displayedAIOpsEngine}
          onCreate={handleStartAIOps}
          onAppend={(query) => {
            void incidents.appendTurn(query).catch(() => undefined)
          }}
        />
      ) : (
        <AgentWorkbenchView
          messages={chat.messages}
          streamingContent={chat.streamingContent}
          streamingThoughts={chat.streamingThoughts}
          thinkingSteps={chat.thinkingSteps}
          suggestions={chat.suggestions}
          isLoading={chat.isLoading}
          loadingEngine={chat.loadingEngine}
          mode={chat.mode}
          workbenchMode="chat"
          selectedSkillIds={selectedSkillIds}
          petEnabled={petEnabled}
          aiOpsEngine={aiOpsEngine}
          onSend={handleSendChat}
          onStartAIOps={handleStartAIOps}
          onStop={chat.stop}
          onModeChange={chat.setMode}
          onTogglePet={() => setPetEnabled((value) => !value)}
          onClearSuggestions={chat.clearSuggestions}
        />
      )}
    </MainLayout>
  )
}
