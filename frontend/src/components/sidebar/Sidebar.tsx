import { Activity, Bot, Database, GitBranch, Plus, X } from 'lucide-react'
import { HistoryPanel } from './HistoryPanel'
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
    <div className="flex h-full flex-col border-r border-slate-200 bg-[#fbfcfe] dark:border-slate-800 dark:bg-slate-950">
      <div className="flex items-center justify-between border-b border-slate-100 px-3 py-4 dark:border-slate-900">
        <div className="flex items-center gap-2">
          <span className="flex h-7 w-7 items-center justify-center rounded-md bg-accent text-[10px] font-bold text-white shadow-sm shadow-accent/20">
            OC
          </span>
          <div>
            <h2 className="text-xs font-semibold text-zinc-900 dark:text-white">OpsCaption</h2>
            <p className="text-[10px] text-zinc-400 dark:text-zinc-500">智能运维工作台</p>
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-800 dark:hover:text-zinc-300 lg:hidden"
        >
          <X size={18} />
        </button>
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto px-2.5 py-4 scrollbar-thin">
        <div className="space-y-1">
          {[
            ['事故视图', Activity, () => onWorkbenchModeChange('aiops'), workbenchMode === 'aiops'],
            ['Plan 编排', GitBranch, () => { onAIOpsEngineChange('plan_execute_replan'); onWorkbenchModeChange('aiops') }, workbenchMode === 'aiops' && aiOpsEngine === 'plan_execute_replan'],
            ['智能问答', Bot, () => { onModeChange('quick'); onWorkbenchModeChange('chat') }, workbenchMode === 'chat'],
            ['证据中心', Database, () => onWorkbenchModeChange('settings'), workbenchMode === 'settings'],
          ].map(([label, Icon, action, active]) => {
            const NavIcon = Icon as typeof Activity
            return <button key={label as string} onClick={action as () => void} className={`flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-xs transition ${active ? 'bg-sky-50 font-semibold text-sky-700 dark:bg-sky-500/10 dark:text-sky-300' : 'text-zinc-500 hover:bg-zinc-100 hover:text-zinc-700 dark:text-zinc-400 dark:hover:bg-zinc-900 dark:hover:text-zinc-200'}`}><NavIcon size={14} />{label as string}</button>
          })}
        </div>
        <div className="rounded-lg border border-amber-200 bg-amber-50/55 p-2.5 dark:border-amber-500/20 dark:bg-amber-500/10">
          <div className="flex items-center justify-between"><span className="text-[11px] font-semibold text-amber-800 dark:text-amber-200">GoS 证据推理</span><span className="size-1.5 rounded-full bg-emerald-500" /></div>
          <p className="mt-1 text-[10px] leading-4 text-amber-700/80 dark:text-amber-200/70">候选根因 → 证据支持 / 反驳</p>
          <button onClick={() => { onAIOpsEngineChange('gos_engine'); onWorkbenchModeChange('aiops') }} className="mt-2 w-full rounded bg-white/80 py-1 text-[10px] font-medium text-amber-700 shadow-sm dark:bg-slate-900/70 dark:text-amber-200">切换到 GoS</button>
        </div>
        <HistoryPanel
          onSelect={onLoadSession}
          onSelectIncident={onLoadIncident}
          currentSessionId={currentSessionId}
          currentIncidentId={currentIncidentId}
          incidents={incidents}
          messageCount={messages.length}
        />
      </div>

      <div className="mt-auto border-t border-zinc-100 px-2.5 py-2 dark:border-zinc-900">
        <button
          onClick={() => onWorkbenchModeChange?.('settings')}
          className="flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-xs text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-700 dark:hover:bg-zinc-900 dark:hover:text-zinc-200"
        >
          工具与 Skill 管理
        </button>
      </div>

      <div className="border-t border-zinc-200/80 p-2.5 dark:border-zinc-800/60">
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="flex w-full items-center justify-center gap-1.5 rounded-lg bg-accent py-2.5 text-xs font-medium text-white shadow-sm shadow-accent/20 transition-all duration-200 hover:brightness-110 active:scale-[0.98] disabled:cursor-not-allowed disabled:opacity-40 disabled:shadow-none"
          title={isLoading ? '请等待当前请求完成' : newLabel}
        >
          <Plus size={16} />
          {isLoading ? '请求中...' : newLabel}
        </button>
      </div>
    </div>
  )
}
