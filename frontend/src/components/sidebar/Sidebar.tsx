import { Plus, X } from 'lucide-react'
import { ModeSelector } from './ModeSelector'
import { HistoryPanel } from './HistoryPanel'
import { ObservabilityPanel } from './ObservabilityPanel'
import { SkillPanel } from './SkillPanel'
import type { AIOpsEngine, ChatMessage, ChatMode, ChatSession, IncidentSession, WorkbenchMode } from '../../types/chat'

interface Props {
  onClose: () => void
  onNewChat: () => void
  onLoadSession: (session: ChatSession) => void
  onLoadIncident: (incidentId: string) => void
  currentSessionId: string
  currentIncidentId: string
  currentIncidentEngine: string
  incidents: IncidentSession[]
  messages: ChatMessage[]
  chatMode: ChatMode
  onModeChange: (mode: ChatMode) => void
  workbenchMode: WorkbenchMode
  onWorkbenchModeChange: (mode: WorkbenchMode) => void
  aiOpsEngine: AIOpsEngine
  onAIOpsEngineChange: (engine: AIOpsEngine) => void
  selectedSkillIds: string[]
  onSelectedSkillIdsChange: (ids: string[]) => void
  isLoading: boolean
}

export function Sidebar({
  onClose,
  onNewChat,
  onLoadSession,
  onLoadIncident,
  currentSessionId,
  currentIncidentId,
  currentIncidentEngine,
  incidents,
  messages,
  chatMode,
  onModeChange,
  workbenchMode,
  onWorkbenchModeChange,
  aiOpsEngine,
  onAIOpsEngineChange,
  selectedSkillIds,
  onSelectedSkillIdsChange,
  isLoading,
}: Props) {
  const newLabel = workbenchMode === 'aiops' ? '新建事故' : '新建会话'

  return (
    <div className="flex h-full flex-col border-r border-zinc-200/80 bg-white/92 backdrop-blur-xl dark:border-zinc-800/60 dark:bg-zinc-950/92">
      <div className="flex items-center justify-between border-b border-zinc-100 px-4 py-4 dark:border-zinc-900">
        <div className="flex items-center gap-2.5">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-accent text-xs font-bold text-white shadow-sm shadow-accent/20">
            OC
          </span>
          <div>
            <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">OpsCaption</h2>
            <p className="text-[11px] text-zinc-400 dark:text-zinc-500">运维诊断工作台</p>
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300 lg:hidden"
        >
          <X size={18} />
        </button>
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto px-3 py-4 scrollbar-thin">
        <ModeSelector
          value={chatMode}
          onChange={onModeChange}
          workbenchMode={workbenchMode}
          onWorkbenchModeChange={onWorkbenchModeChange}
          aiOpsEngine={aiOpsEngine}
          lockedAIOpsEngine={currentIncidentEngine}
          onAIOpsEngineChange={onAIOpsEngineChange}
        />
        {workbenchMode === 'chat' && <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />}
        <details className="group rounded-xl border border-zinc-200/80 bg-zinc-50/60 dark:border-zinc-800/70 dark:bg-zinc-900/30">
          <summary className="flex cursor-pointer list-none items-center justify-between px-3 py-2.5 text-xs font-medium text-zinc-500 outline-none transition hover:text-zinc-700 dark:text-zinc-400 dark:hover:text-zinc-200">
            服务连接
            <span className="text-zinc-400 transition-transform group-open:rotate-180">⌄</span>
          </summary>
          <div className="border-t border-zinc-200/70 px-3 py-3 dark:border-zinc-800/70"><ObservabilityPanel /></div>
        </details>
        <HistoryPanel
          onSelect={onLoadSession}
          onSelectIncident={onLoadIncident}
          currentSessionId={currentSessionId}
          currentIncidentId={currentIncidentId}
          incidents={incidents}
          messageCount={messages.length}
        />
      </div>

      <div className="mt-auto border-t border-zinc-100 px-3 py-2 dark:border-zinc-900">
        <button
          onClick={() => onWorkbenchModeChange?.('settings')}
          className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-900 dark:hover:text-zinc-200"
        >
          工具与 Skill 管理
        </button>
      </div>

      <div className="border-t border-zinc-200/80 p-3 dark:border-zinc-800/60">
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="flex w-full items-center justify-center gap-2 rounded-xl bg-accent py-2.5 text-sm font-medium text-white shadow-sm shadow-accent/20 transition-all duration-200 hover:brightness-110 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40 disabled:shadow-none"
          title={isLoading ? '请等待当前请求完成' : newLabel}
        >
          <Plus size={16} />
          {isLoading ? '请求中...' : newLabel}
        </button>
      </div>
    </div>
  )
}
