import { X, Plus } from 'lucide-react'
import { OperatorCard } from './OperatorCard'
import { ModeSelector } from './ModeSelector'
import { HistoryPanel } from './HistoryPanel'
import { ObservabilityPanel } from './ObservabilityPanel'
import { SkillPanel } from './SkillPanel'
import type { ChatMessage, ChatMode, ChatSession } from '../../types/chat'

interface Props {
  onClose: () => void
  onNewChat: () => void
  onLoadSession: (s: ChatSession) => void
  currentSessionId: string
  messages: ChatMessage[]
  chatMode: ChatMode
  onModeChange: (m: ChatMode) => void
  selectedSkillIds: string[]
  onSelectedSkillIdsChange: (ids: string[]) => void
  isLoading: boolean
}

export function Sidebar({
  onClose,
  onNewChat,
  onLoadSession,
  currentSessionId,
  messages,
  chatMode,
  onModeChange,
  selectedSkillIds,
  onSelectedSkillIdsChange,
  isLoading,
}: Props) {
  return (
    <div className="h-full glass flex flex-col">
      <div className="flex items-center justify-between border-b border-zinc-200/80 p-4 dark:border-zinc-800/50">
        <div className="flex items-center gap-2">
          <span className="w-7 h-7 rounded-md bg-accent flex items-center justify-center text-xs font-bold text-white">
            OC
          </span>
          <div>
            <h2 className="text-sm font-semibold">OpsCaption</h2>
            <p className="text-xs text-zinc-500 dark:text-zinc-500">运维诊断工作台</p>
          </div>
        </div>
        <button
          onClick={onClose}
          className="rounded-lg p-1.5 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-800 lg:hidden"
        >
          <X size={18} />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto scrollbar-thin p-3 space-y-4">
        <OperatorCard />

        <ModeSelector value={chatMode} onChange={onModeChange} />

        <SkillPanel selectedSkillIds={selectedSkillIds} onChange={onSelectedSkillIdsChange} />

        <ObservabilityPanel />

        <HistoryPanel
          onSelect={onLoadSession}
          currentSessionId={currentSessionId}
          messageCount={messages.length}
        />
      </div>

      <div className="border-t border-zinc-200/80 p-3 dark:border-zinc-800/50">
        <button
          onClick={onNewChat}
          disabled={isLoading}
          className="w-full flex items-center justify-center gap-2 rounded-xl py-2.5 text-sm font-medium transition-colors bg-accent/10 text-accent hover:bg-accent/20 disabled:opacity-40 disabled:cursor-not-allowed"
          title={isLoading ? '请等待当前请求完成' : '新建会话'}
        >
          <Plus size={16} />
          {isLoading ? '请求中...' : '新建会话'}
        </button>
      </div>
    </div>
  )
}
