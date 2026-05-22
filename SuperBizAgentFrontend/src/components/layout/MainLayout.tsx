import { AnimatePresence, motion } from 'framer-motion'
import { TopBar } from './TopBar'
import { Sidebar } from '../sidebar/Sidebar'
import type { ReactNode } from 'react'
import type { AIOpsEngine, ChatMessage, ChatMode, ChatSession, IncidentSession, WorkMode } from '../../types/chat'
import { getSiteRecord } from '../../lib/utils'

interface Props {
  theme: string
  sidebarOpen: boolean
  onToggleSidebar: () => void
  onCloseSidebar: () => void
  onToggleTheme: () => void
  onNewChat: () => void
  onLoadSession: (s: ChatSession) => void
  onLoadIncident: (incidentId: string) => void
  chatMode: ChatMode
  onModeChange: (m: ChatMode) => void
  workMode: WorkMode
  onWorkModeChange: (mode: WorkMode) => void
  aiOpsEngine: AIOpsEngine
  onAIOpsEngineChange: (engine: AIOpsEngine) => void
  sessionId: string
  currentIncidentId: string
  incidents: IncidentSession[]
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
  onLoadIncident,
  chatMode,
  onModeChange,
  workMode,
  onWorkModeChange,
  aiOpsEngine,
  onAIOpsEngineChange,
  sessionId,
  currentIncidentId,
  incidents,
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
          <motion.div
            key="sidebar-overlay"
            className="fixed inset-0 z-40 bg-black/50 lg:hidden"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onCloseSidebar}
          />
        )}
      </AnimatePresence>

      <div
        className={`flex-shrink-0 overflow-hidden transition-[width] duration-200 ease-in-out ${
          sidebarOpen ? 'w-72' : 'w-0'
        }`}
      >
        <motion.aside
          className="fixed bottom-0 left-0 top-0 z-50 w-72 lg:static lg:z-0 lg:h-full"
          initial={false}
          animate={{ x: sidebarOpen ? 0 : -288 }}
          transition={{ type: 'spring', damping: 25, stiffness: 200 }}
        >
          <Sidebar
            onClose={onCloseSidebar}
            onNewChat={onNewChat}
            onLoadSession={onLoadSession}
            onLoadIncident={onLoadIncident}
            currentSessionId={sessionId}
            currentIncidentId={currentIncidentId}
            incidents={incidents}
            messages={messages}
            chatMode={chatMode}
            onModeChange={onModeChange}
            workMode={workMode}
            onWorkModeChange={onWorkModeChange}
            aiOpsEngine={aiOpsEngine}
            onAIOpsEngineChange={onAIOpsEngineChange}
            selectedSkillIds={selectedSkillIds}
            onSelectedSkillIdsChange={onSelectedSkillIdsChange}
            isLoading={isLoading}
          />
        </motion.aside>
      </div>

      <div className="relative flex flex-1 flex-col min-w-0 overflow-hidden">
        <TopBar
          theme={theme}
          onToggleSidebar={onToggleSidebar}
          onToggleTheme={onToggleTheme}
          chatMode={chatMode}
          workMode={workMode}
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
