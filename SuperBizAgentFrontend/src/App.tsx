import { useState, useCallback } from 'react'
import { useTheme } from './hooks/useTheme'
import { useChat } from './hooks/useChat'
import { MainLayout } from './components/layout/MainLayout'
import { ChatView } from './components/chat/ChatView'
import { WelcomeScreen } from './components/welcome/WelcomeScreen'
import { loadSession } from './lib/storage'
import type { ChatSession } from './types/chat'

export default function App() {
  const { theme, toggle: toggleTheme } = useTheme()
  const chat = useChat()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [showWelcome, setShowWelcome] = useState(true)

  const handleSend = useCallback(
    (query: string) => {
      setShowWelcome(false)
      chat.send(query)
    },
    [chat]
  )

  const handleLoadSession = useCallback(
    (session: ChatSession) => {
      chat.setMessages(session.messages)
      setShowWelcome(false)
    },
    [chat]
  )

  const handleNewChat = useCallback(() => {
    chat.clear()
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
    >
      {showWelcome && chat.messages.length === 0 ? (
        <WelcomeScreen onSend={handleSend} />
      ) : (
        <ChatView
          messages={chat.messages}
          streamingContent={chat.streamingContent}
          isLoading={chat.isLoading}
          mode={chat.mode}
          onSend={handleSend}
          onStop={chat.stop}
          onModeChange={chat.setMode}
        />
      )}
    </MainLayout>
  )
}
