import { useState, useCallback, useEffect } from 'react'
import { useTheme } from './hooks/useTheme'
import { useChat } from './hooks/useChat'
import { useIncidents } from './hooks/useIncidents'
import { MainLayout } from './components/layout/MainLayout'
import { ChatView } from './components/chat/ChatView'
import { IncidentView } from './components/incident/IncidentView'
import { WelcomeScreen } from './components/welcome/WelcomeScreen'
import { saveSession } from './lib/storage'
import type { AIOpsEngine, ChatSession, WorkMode } from './types/chat'

const SKILL_STORAGE_KEY = 'opscaptain-selected-skills'
const AIOPS_ENGINE_STORAGE_KEY = 'opscaptain-aiops-engine'
const WORK_MODE_STORAGE_KEY = 'opscaptain-work-mode'

export default function App() {
  const { theme, toggle: toggleTheme } = useTheme()
  const chat = useChat()
  const incidents = useIncidents()
  const [sidebarOpen, setSidebarOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.innerWidth >= 1024
  })
  const [showWelcome, setShowWelcome] = useState(true)
  const [workMode, setWorkMode] = useState<WorkMode>(() => {
    if (typeof window === 'undefined') return 'react'
    return localStorage.getItem(WORK_MODE_STORAGE_KEY) === 'aiops' ? 'aiops' : 'react'
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

  useEffect(() => {
    try {
      localStorage.setItem(SKILL_STORAGE_KEY, JSON.stringify(selectedSkillIds))
    } catch {
      return
    }
  }, [selectedSkillIds])

  useEffect(() => {
    try {
      localStorage.setItem(WORK_MODE_STORAGE_KEY, workMode)
    } catch {
      return
    }
  }, [workMode])

  useEffect(() => {
    try {
      localStorage.setItem(AIOPS_ENGINE_STORAGE_KEY, aiOpsEngine)
    } catch {
      return
    }
  }, [aiOpsEngine])

  useEffect(() => {
    if (chat.messages.length === 0 || workMode !== 'react') {
      return
    }
    saveSession(chat.sessionId, chat.messages, {
      mode: chat.mode,
      workMode,
      selectedSkillIds,
    })
  }, [chat.sessionId, chat.messages, chat.mode, workMode, selectedSkillIds])

  const handleSendChat = useCallback(
    (query: string) => {
      setShowWelcome(false)
      setWorkMode('react')
      chat.send(query, { selectedSkillIds })
    },
    [chat, selectedSkillIds]
  )

  const handleStartAIOps = useCallback(
    (query: string) => {
      setShowWelcome(false)
      setWorkMode('aiops')
      void incidents.createIncident(query, aiOpsEngine).catch(() => undefined)
    },
    [aiOpsEngine, incidents]
  )

  const handleSend = useCallback(
    (query: string) => {
      if (workMode === 'aiops') {
        handleStartAIOps(query)
        return
      }
      handleSendChat(query)
    },
    [handleSendChat, handleStartAIOps, workMode]
  )

  const handleLoadSession = useCallback(
    (session: ChatSession) => {
      const loaded = chat.loadSession(session)
      if (!loaded) {
        return
      }
      setSelectedSkillIds(Array.isArray(session.selectedSkillIds) ? session.selectedSkillIds : [])
      setWorkMode(session.workMode === 'aiops' ? 'aiops' : 'react')
      setShowWelcome(false)
    },
    [chat]
  )

  const handleNewChat = useCallback(() => {
    if (workMode === 'aiops') {
      incidents.newIncident()
      setShowWelcome(false)
      return
    }
    const created = chat.newSession()
    if (!created) {
      return
    }
    setShowWelcome(true)
  }, [chat, incidents, workMode])

  const handleLoadIncident = useCallback(
    (incidentId: string) => {
      setWorkMode('aiops')
      setShowWelcome(false)
      void incidents.loadIncident(incidentId).catch(() => undefined)
    },
    [incidents]
  )

  const handleWorkModeChange = useCallback(
    (next: WorkMode) => {
      setWorkMode(next)
      setShowWelcome(next === 'react' && chat.messages.length === 0)
    },
    [chat.messages.length]
  )

  return (
    <MainLayout
      theme={theme}
      sidebarOpen={sidebarOpen}
      onToggleSidebar={() => setSidebarOpen((v) => !v)}
      onCloseSidebar={() => setSidebarOpen(false)}
      onToggleTheme={toggleTheme}
      onNewChat={handleNewChat}
      onLoadSession={handleLoadSession}
      onLoadIncident={handleLoadIncident}
      chatMode={chat.mode}
      onModeChange={chat.setMode}
      workMode={workMode}
      onWorkModeChange={handleWorkModeChange}
      aiOpsEngine={aiOpsEngine}
      onAIOpsEngineChange={setAIOpsEngine}
      sessionId={chat.sessionId}
      currentIncidentId={incidents.incident?.incident_id || ''}
      incidents={incidents.incidents}
      messages={chat.messages}
      selectedSkillIds={selectedSkillIds}
      onSelectedSkillIdsChange={setSelectedSkillIds}
      isLoading={chat.isLoading || incidents.isLoading}
    >
      {workMode === 'aiops' ? (
        <IncidentView
          incident={incidents.incident}
          isLoading={incidents.isLoading}
          error={incidents.error}
          engine={aiOpsEngine}
          onCreate={handleStartAIOps}
          onAppend={(query) => {
            void incidents.appendTurn(query).catch(() => undefined)
          }}
        />
      ) : showWelcome && chat.messages.length === 0 ? (
        <WelcomeScreen
          onSend={handleSend}
          onStartAIOps={handleStartAIOps}
          aiOpsEngine={aiOpsEngine}
        />
      ) : (
        <ChatView
          messages={chat.messages}
          streamingContent={chat.streamingContent}
          streamingThoughts={chat.streamingThoughts}
          thinkingSteps={chat.thinkingSteps}
          suggestions={chat.suggestions}
          isLoading={chat.isLoading}
          mode={chat.mode}
          workMode={workMode}
          aiOpsEngine={aiOpsEngine}
          selectedSkillIds={selectedSkillIds}
          onSend={handleSend}
          onStop={chat.stop}
          onModeChange={chat.setMode}
          onClearSuggestions={chat.clearSuggestions}
        />
      )}
    </MainLayout>
  )
}
