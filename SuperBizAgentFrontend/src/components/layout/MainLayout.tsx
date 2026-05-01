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
  selectedSkillIds: string[]
  onSelectedSkillIdsChange: (ids: string[]) => void
  isLoading: boolean
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
  selectedSkillIds,
  onSelectedSkillIdsChange,
  isLoading,
  children,
}: Props) {
  const siteRecord = getSiteRecord()

  return (
    <div className="relative flex h-screen overflow-hidden bg-[#fafafa] text-zinc-900 dark:bg-[#09090b] dark:text-zinc-100">
      <AnimatePresence>
        {sidebarOpen && (
          <>
            <motion.div
              className="fixed inset-0 z-40 bg-black/50 lg:hidden"
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              onClick={onCloseSidebar}
            />
            <motion.aside
              className="fixed bottom-0 left-0 top-0 z-50 w-72 lg:relative lg:z-0"
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
                selectedSkillIds={selectedSkillIds}
                onSelectedSkillIdsChange={onSelectedSkillIdsChange}
                isLoading={isLoading}
              />
            </motion.aside>
          </>
        )}
      </AnimatePresence>

      <div className="relative flex flex-1 flex-col min-w-0 overflow-hidden">
        <TopBar
          theme={theme}
          onToggleSidebar={onToggleSidebar}
          onToggleTheme={onToggleTheme}
          chatMode={chatMode}
          onNewChat={onNewChat}
          isLoading={isLoading}
        />
        <main className="relative flex-1 overflow-hidden">{children}</main>
        {siteRecord && (
          <footer className="border-t border-zinc-200/80 bg-white/88 px-4 py-2.5 text-center text-xs text-zinc-400 backdrop-blur-xl dark:border-zinc-800/60 dark:bg-zinc-950/90 dark:text-zinc-600">
            <span className="mr-1">ICP备案号：</span>
            <a
              className="font-medium text-zinc-500 transition-colors hover:text-zinc-700 dark:text-zinc-500 dark:hover:text-zinc-300"
              href={siteRecord.icpLink}
              target="_blank"
              rel="noopener noreferrer"
            >
              {siteRecord.icpNumber}
            </a>
          </footer>
        )}
      </div>
    </div>
  )
}
