import { AnimatePresence, motion } from 'framer-motion'
import { TopBar } from './TopBar'
import { Sidebar } from '../sidebar/Sidebar'
import { ChangeEventSentinel } from '../change-events/ChangeEventSentinel'
import type { ReactNode } from 'react'
import type { AIOpsEngine, ChatMessage, ChatMode, ChatSession, IncidentSession, WorkbenchMode } from '../../types/chat'
import { getSiteRecord } from '../../lib/utils'

interface Props {
  theme: string
  sidebarOpen: boolean
  onToggleSidebar: () => void
  onCloseSidebar: () => void
  onToggleTheme: () => void
  onNewChat: () => void
  onLoadSession: (session: ChatSession) => void
  onLoadIncident: (incidentId: string) => void
  chatMode: ChatMode
  onModeChange: (mode: ChatMode) => void
  workbenchMode: WorkbenchMode
  onWorkbenchModeChange: (mode: WorkbenchMode) => void
  aiOpsEngine: AIOpsEngine
  onAIOpsEngineChange: (engine: AIOpsEngine) => void
  sessionId: string
  currentIncidentId: string
  currentIncidentEngine: string
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
  workbenchMode,
  onWorkbenchModeChange,
  aiOpsEngine,
  onAIOpsEngineChange,
  sessionId,
  currentIncidentId,
  currentIncidentEngine,
  incidents,
  messages,
  selectedSkillIds,
  onSelectedSkillIdsChange,
  isLoading,
  children,
}: Props) {
  const siteRecord = getSiteRecord()

  return (
    <div className="relative h-[100dvh] min-h-[100dvh] w-full overflow-hidden bg-[#f1f5f9] dark:bg-[#0B1120]">
      <div className="pointer-events-none absolute -top-1/4 -left-1/4 h-[60%] w-[60%] rounded-full bg-sky-200/30 blur-3xl dark:bg-sky-400/15" />
      <div className="pointer-events-none absolute -bottom-1/4 -right-1/4 h-[50%] w-[50%] rounded-full bg-amber-200/25 blur-3xl dark:bg-amber-400/10" />
      <div className="pointer-events-none absolute top-1/3 -right-1/6 h-[55%] w-[55%] rounded-full bg-sky-300/15 blur-3xl dark:bg-sky-500/8" />

      <div className="relative z-10 h-[100dvh] overflow-hidden border border-white/60 bg-white/70 backdrop-blur-2xl sm:m-2 sm:h-[calc(100dvh-16px)] sm:rounded-[22px] sm:shadow-[0_8px_40px_rgba(0,0,0,0.06)] dark:border-white/10 dark:bg-slate-800/60 dark:shadow-none">
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_12%_18%,rgba(255,255,255,0.3),transparent_38%),radial-gradient(circle_at_88%_82%,rgba(0,0,0,0.04),transparent_30%)] dark:bg-[radial-gradient(circle_at_12%_18%,rgba(255,255,255,0.04),transparent_38%),radial-gradient(circle_at_88%_82%,rgba(255,255,255,0.02),transparent_30%)]" />

        <div className="relative flex h-full flex-col lg:flex-row">
          <AnimatePresence>
            {sidebarOpen && (
              <motion.div
                key="sidebar-overlay"
                className="fixed inset-0 z-40 bg-black/50 lg:hidden"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                exit={{ opacity: 0 }}
                onClick={onCloseSidebar}
                aria-label="关闭侧栏"
              />
            )}
          </AnimatePresence>

          <div className={`flex-shrink-0 overflow-hidden transition-[width] duration-200 ease-in-out ${sidebarOpen ? 'w-72' : 'w-0'}`}>
            <motion.aside
              className={`fixed bottom-0 left-0 top-0 z-50 w-72 lg:static lg:z-0 lg:h-full ${sidebarOpen ? '' : 'pointer-events-none'}`}
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
                currentIncidentEngine={currentIncidentEngine}
                incidents={incidents}
                messages={messages}
                chatMode={chatMode}
                onModeChange={onModeChange}
                workbenchMode={workbenchMode}
                onWorkbenchModeChange={onWorkbenchModeChange}
                aiOpsEngine={aiOpsEngine}
                onAIOpsEngineChange={onAIOpsEngineChange}
                selectedSkillIds={selectedSkillIds}
                onSelectedSkillIdsChange={onSelectedSkillIdsChange}
                isLoading={isLoading}
              />
            </motion.aside>
          </div>

          <div className="relative flex min-w-0 flex-1 flex-col overflow-hidden">
            <TopBar
              theme={theme}
              onToggleSidebar={onToggleSidebar}
              onToggleTheme={onToggleTheme}
              chatMode={chatMode}
              workbenchMode={workbenchMode}
              aiOpsEngine={aiOpsEngine}
              onNewChat={onNewChat}
              isLoading={isLoading}
            />
            <main className="relative flex-1 overflow-hidden">{children}</main>
            {siteRecord && (
              <footer className="border-t border-white/40 bg-white/30 px-4 py-2.5 text-center text-xs text-zinc-400 backdrop-blur-sm dark:border-white/5 dark:bg-slate-900/30 dark:text-zinc-600">
                <span className="mr-1">ICP备案号:</span>
                <a
                  className="font-medium text-zinc-500 transition-colors hover:text-sky-500 dark:text-zinc-500 dark:hover:text-sky-400"
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
        <ChangeEventSentinel />
      </div>
    </div>
  )
}
