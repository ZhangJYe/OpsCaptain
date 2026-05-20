import { useState, useCallback, useEffect } from 'react'
import { useTheme } from './hooks/useTheme'
import { useChat } from './hooks/useChat'
import { MainLayout } from './components/layout/MainLayout'
import { AgentWorkbenchView } from './components/workbench/AgentWorkbenchView'
import { saveSession } from './lib/storage'
import type { AIOpsEngine, ChatSession } from './types/chat'

const SKILL_STORAGE_KEY = 'opscaptain-selected-skills'
const AIOPS_ENGINE_STORAGE_KEY = 'opscaptain-aiops-engine'
const PET_ENABLED_KEY = 'opscaptain-pet-enabled'

export default function App() {
  const { theme, toggle: toggleTheme } = useTheme()
  const chat = useChat()
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
  const [petEnabled, setPetEnabled] = useState<boolean>(() => {
    if (typeof window === 'undefined') return true
    const raw = localStorage.getItem(PET_ENABLED_KEY)
    return raw !== 'false'
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
      localStorage.setItem(AIOPS_ENGINE_STORAGE_KEY, aiOpsEngine)
    } catch {
      return
    }
  }, [aiOpsEngine])

  useEffect(() => {
    try {
      localStorage.setItem(PET_ENABLED_KEY, String(petEnabled))
    } catch {
      return
    }
  }, [petEnabled])

  useEffect(() => {
    if (chat.messages.length === 0) {
      return
    }
    saveSession(chat.sessionId, chat.messages, {
      mode: chat.mode,
      selectedSkillIds,
    })
  }, [chat.sessionId, chat.messages, chat.mode, selectedSkillIds])

  const handleSend = useCallback(
    (query: string) => {
      chat.send(query, { selectedSkillIds })
    },
    [chat, selectedSkillIds]
  )

  const handleStartAIOps = useCallback(
    (query: string) => {
      chat.sendAIOps(query, { aiOpsEngine })
    },
    [chat, aiOpsEngine]
  )

  const handleLoadSession = useCallback(
    (session: ChatSession) => {
      const loaded = chat.loadSession(session)
      if (!loaded) {
        return
      }
      setSelectedSkillIds(Array.isArray(session.selectedSkillIds) ? session.selectedSkillIds : [])
    },
    [chat]
  )

  const handleNewChat = useCallback(() => {
    chat.newSession()
  }, [chat])

  return (
    <MainLayout
      theme={theme}
      sidebarOpen={sidebarOpen}
      onToggleSidebar={() => setSidebarOpen((v) => !v)}
      onCloseSidebar={() => setSidebarOpen(false)}
      onToggleTheme={toggleTheme}
      onNewChat={handleNewChat}
      onLoadSession={handleLoadSession}
      chatMode={chat.mode}
      onModeChange={chat.setMode}
      aiOpsEngine={aiOpsEngine}
      onAIOpsEngineChange={setAIOpsEngine}
      sessionId={chat.sessionId}
      messages={chat.messages}
      selectedSkillIds={selectedSkillIds}
      onSelectedSkillIdsChange={setSelectedSkillIds}
      isLoading={chat.isLoading}
    >
      <AgentWorkbenchView
        messages={chat.messages}
        streamingContent={chat.streamingContent}
        streamingThoughts={chat.streamingThoughts}
        thinkingSteps={chat.thinkingSteps}
        suggestions={chat.suggestions}
        isLoading={chat.isLoading}
        loadingEngine={chat.loadingEngine}
        mode={chat.mode}
        selectedSkillIds={selectedSkillIds}
        petEnabled={petEnabled}
        aiOpsEngine={aiOpsEngine}
        onSend={handleSend}
        onStartAIOps={handleStartAIOps}
        onStop={chat.stop}
        onModeChange={chat.setMode}
        onTogglePet={() => setPetEnabled((v) => !v)}
        onClearSuggestions={chat.clearSuggestions}
      />
    </MainLayout>
  )
}
