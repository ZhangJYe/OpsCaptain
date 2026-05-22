import { X, Plus } from 'lucide-react'
import { OperatorCard } from './OperatorCard'
import { ModeSelector } from './ModeSelector'
import { HistoryPanel } from './HistoryPanel'
import { ObservabilityPanel } from './ObservabilityPanel'
import { SkillPanel } from './SkillPanel'
import type { AIOpsEngine, ChatMessage, ChatMode, ChatSession, IncidentSession, WorkMode } from '../../types/chat'

interface Props {
  onClose: () => void
  onNewChat: () => void
  onLoadSession: (s: ChatSession) => void
  onLoadIncident: (incidentId: string) => void
  currentSessionId: string
  currentIncidentId: string
  incidents: IncidentSession[]
  messages: ChatMessage[]
  chatMode: ChatMode
  onModeChange: (m: ChatMode) => void
  workMode: WorkMode
  onWorkModeChange: (mode: WorkMode) => void
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
  incidents,
  messages,
  chatMode,
  onModeChange,
  workMode,
  onWorkModeChange,
  aiOpsEngine,
  onAIOpsEngineChange,
  selectedSkillIds,
  onSelectedSkillIdsChange,
  isLoading,
}: Props) {
  return (
    <div className="flex h-full flex-col border-r border-zinc-200/80 bg-white/92 backdrop-blur-xl dark:border-zinc-800/60 dark:bg-zinc-950/92">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-4">
        <div className="flex items-center gap-2.5">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-accent text-xs font-bold text-white shadow-sm shadow-accent/20">
            OC
          </span>
          <div>
            <h2 className="text-sm font-semibold text-zinc-900 dark:text-white">OpsCaption</h2>
            <p className="text-[11px] text-zinc-500 dark:text-zinc-500">运维诊断工作台</p>
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300 lg:hidden"
        >
          <X size={18} />
        </button>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto scrollbar-thin px-3 space-y-3">
        <OperatorCard />
        <ModeSelector
          value={chatMode}
          onChange={onModeChange}
          workMode={workMode}
          onWorkModeChange={onWorkModeChange}
          aiOpsEngine={aiOpsEngine}
          onAIOpsEngineChange={onAIOpsEngineChange}
        />
        {workMode === 'react' && <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />}
        <ObservabilityPanel />
        <HistoryPanel
          onSelect={onLoadSession}
          onSelectIncident={onLoadIncident}
          currentSessionId={currentSessionId}
          currentIncidentId={currentIncidentId}
          incidents={incidents}
          messageCount={messages.length}
        />
      </div>

      {/* New chat button */}
      <div className="border-t border-zinc-200/80 p-3 dark:border-zinc-800/60">
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="flex w-full items-center justify-center gap-2 rounded-xl bg-accent py-2.5 text-sm font-medium text-white shadow-sm shadow-accent/20 transition-all duration-200 hover:brightness-110 active:scale-[0.98] disabled:opacity-40 disabled:cursor-not-allowed disabled:shadow-none"
          title={isLoading ? '请等待当前请求完成' : workMode === 'aiops' ? '新建事故' : '新建会话'}
        >
          <Plus size={16} />
          {isLoading ? '请求中...' : workMode === 'aiops' ? '新建事故' : '新建会话'}
        </button>
      </div>
    </div>
  )
}
