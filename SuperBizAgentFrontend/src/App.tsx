import { useState, useCallback, useEffect } from 'react'
import { useTheme } from './hooks/useTheme'
import { useChat } from './hooks/useChat'
import { MainLayout } from './components/layout/MainLayout'
import { ChatView } from './components/chat/ChatView'
import { WelcomeScreen } from './components/welcome/WelcomeScreen'
import { saveSession } from './lib/storage'
import type { ChatSession } from './types/chat'

const SKILL_STORAGE_KEY = 'opscaptain-selected-skills'

export default function App() {
  const { theme, toggle: toggleTheme } = useTheme()
  const chat = useChat()
  const [sidebarOpen, setSidebarOpen] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.innerWidth >= 1024
  })
  const [showWelcome, setShowWelcome] = useState(true)
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

  useEffect(() => {
    try {
      localStorage.setItem(SKILL_STORAGE_KEY, JSON.stringify(selectedSkillIds))
    } catch {
      return
    }
  }, [selectedSkillIds])

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
      setShowWelcome(false)
      chat.send(query, { selectedSkillIds })
    },
    [chat, selectedSkillIds]
  )

  const handleLoadSession = useCallback(
    (session: ChatSession) => {
      const loaded = chat.loadSession(session)
      if (!loaded) {
        return
      }
      setSelectedSkillIds(Array.isArray(session.selectedSkillIds) ? session.selectedSkillIds : [])
      setShowWelcome(false)
    },
    [chat]
  )

  const handleNewChat = useCallback(() => {
    const created = chat.newSession()
    if (!created) {
      return
    }
    setShowWelcome(true)
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
      sessionId={chat.sessionId}
      messages={chat.messages}
      selectedSkillIds={selectedSkillIds}
      onSelectedSkillIdsChange={setSelectedSkillIds}
      isLoading={chat.isLoading}
    >
      {showWelcome && chat.messages.length === 0 ? (
        <WelcomeScreen onSend={handleSend} />
      ) : (
        <ChatView
          messages={chat.messages}
          streamingContent={chat.streamingContent}
          streamingThoughts={chat.streamingThoughts}
          thinkingSteps={chat.thinkingSteps}
          suggestions={chat.suggestions}
          isLoading={chat.isLoading}
          mode={chat.mode}
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
