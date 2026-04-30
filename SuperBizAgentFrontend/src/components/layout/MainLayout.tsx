import { AnimatePresence, motion } from 'framer-motion'
import { TopBar } from './TopBar'
import { Sidebar } from '../sidebar/Sidebar'
import type { ReactNode } from 'react'
import type { ChatMessage, ChatMode, ChatSession } from '../../types/chat'
import { getSiteRecord } from '../../lib/utils'

interface Props {
  theme: string
  sidebarOpen: boolean
  onToggleSidebar: () => void
  onCloseSidebar: () => void
  onToggleTheme: () => void
  onNewChat: () => void
  onLoadSession: (s: ChatSession) => void
  chatMode: ChatMode
  onModeChange: (m: ChatMode) => void
  sessionId: string
  messages: ChatMessage[]
  children: ReactNode
}

export function MainLayout({
  theme,
  sidebarOpen,
  onToggleSidebar,
  onCloseSidebar,
  onToggleTheme,
  onNewChat,
  onLoadSession,
  chatMode,
  onModeChange,
  sessionId,
  messages,
  children,
}: Props) {
  const siteRecord = getSiteRecord()

  return (
    <div className="flex h-screen overflow-hidden">
      <AnimatePresence>
        {sidebarOpen && (
          <>
            <motion.div
              className="fixed inset-0 bg-black/50 z-40 lg:hidden"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onClick={onCloseSidebar}
            />
            <motion.aside
              className="fixed left-0 top-0 bottom-0 z-50 w-72 lg:relative lg:z-0"
              initial={{ x: -288 }}
              animate={{ x: 0 }}
              exit={{ x: -288 }}
              transition={{ type: 'spring', damping: 25, stiffness: 200 }}
            >
              <Sidebar
                onClose={onCloseSidebar}
                onNewChat={onNewChat}
                onLoadSession={onLoadSession}
                currentSessionId={sessionId}
                messages={messages}
                chatMode={chatMode}
                onModeChange={onModeChange}
              />
            </motion.aside>
          </>
        )}
      </AnimatePresence>
      <div className="flex-1 flex flex-col min-w-0">
        <TopBar
          theme={theme}
          onToggleSidebar={onToggleSidebar}
          onToggleTheme={onToggleTheme}
          chatMode={chatMode}
        />
        <main className="flex-1 overflow-hidden">{children}</main>
        {siteRecord ? (
          <footer className="border-t border-zinc-800/50 px-4 py-3 text-center text-xs text-zinc-500">
            <span className="mr-1">ICP备案号：</span>
            <a
              className="font-medium text-zinc-400 transition-colors hover:text-zinc-200 hover:underline"
              href={siteRecord.icpLink}
              target="_blank"
              rel="noopener noreferrer"
            >
              {siteRecord.icpNumber}
            </a>
          </footer>
        ) : null}
      </div>
    </div>
  )
}
