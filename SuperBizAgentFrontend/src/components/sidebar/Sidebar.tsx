import { X, Plus, Activity, Server, Radio } from 'lucide-react'
import { OperatorCard } from './OperatorCard'
import { ModeSelector } from './ModeSelector'
import { HistoryPanel } from './HistoryPanel'
import { ObservabilityPanel } from './ObservabilityPanel'
import type { ChatMessage, ChatMode, ChatSession } from '../../types/chat'

interface Props {
  onClose: () => void
  onNewChat: () => void
  onLoadSession: (s: ChatSession) => void
  currentSessionId: string
  messages: ChatMessage[]
  chatMode: ChatMode
  onModeChange: (m: ChatMode) => void
}

export function Sidebar({ onClose, onNewChat, onLoadSession, currentSessionId, messages, chatMode, onModeChange }: Props) {
  return (
    <div className="h-full glass flex flex-col">
      <div className="flex items-center justify-between p-4 border-b border-zinc-800/50">
        <div className="flex items-center gap-2">
          <span className="w-7 h-7 rounded-md bg-accent flex items-center justify-center text-xs font-bold text-white">
            OC
          </span>
          <div>
            <h2 className="text-sm font-semibold">OpsCaption</h2>
            <p className="text-xs text-zinc-500">运维诊断工作台</p>
          </div>
        </div>
        <button onClick={onClose} className="p-1.5 rounded-lg hover:bg-zinc-800 transition-colors lg:hidden">
          <X size={18} />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto scrollbar-thin p-3 space-y-4">
        <OperatorCard />

        <ModeSelector value={chatMode} onChange={onModeChange} />

        <ObservabilityPanel />

        <HistoryPanel
          onSelect={onLoadSession}
          currentSessionId={currentSessionId}
        />
      </div>

      <div className="p-3 border-t border-zinc-800/50">
        <button
          onClick={onNewChat}
          className="w-full flex items-center justify-center gap-2 py-2.5 rounded-xl bg-accent/10 text-accent hover:bg-accent/20 transition-colors text-sm font-medium"
        >
          <Plus size={16} />
          新建会话
        </button>
      </div>
    </div>
  )
}
